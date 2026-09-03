package federation

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func aqlItemsResponse(items ...struct{ Path, Name string }) string {
	type result struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	results := make([]result, 0, len(items))
	for _, it := range items {
		results = append(results, result{Path: it.Path, Name: it.Name})
	}
	b, _ := json.Marshal(map[string]interface{}{"results": results})
	return string(b)
}

func TestPlanConvertToLocalMakesOnlyReadCalls(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			t.Fatalf("plan should only issue read calls, got %s %s", r.Method, r.URL.Path)
		}
		if r.Method == http.MethodPost && r.URL.Path != "/artifactory/api/search/aql" {
			t.Fatalf("plan should only POST to AQL search, got %s", r.URL.Path)
		}
		if r.URL.Path == "/artifactory/api/search/aql" {
			_, _ = w.Write([]byte(aqlItemsResponse(
				struct{ Path, Name string }{Path: "dir1", Name: "a.txt"},
				struct{ Path, Name string }{Path: ".", Name: "b.txt"},
			)))
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()

	plan, err := PlanConvertToLocal(c, "fed1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.TempRepo != "fed1-temp" {
		t.Fatalf("expected default temp repo name fed1-temp, got %s", plan.TempRepo)
	}
	if len(plan.Items) != 2 || plan.Items[0] != "dir1/a.txt" || plan.Items[1] != "b.txt" {
		t.Fatalf("unexpected items: %v", plan.Items)
	}
}

func TestConvertToLocalFullSuccess(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var calls []string
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/repositories/fed1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"key":     "fed1",
				"rclass":  "federated",
				"members": []map[string]interface{}{{"url": "https://a", "enabled": true}},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/artifactory/api/repositories/fed1-tmp":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			_, _ = w.Write([]byte(aqlItemsResponse(
				struct{ Path, Name string }{Path: ".", Name: "a.txt"},
				struct{ Path, Name string }{Path: "sub", Name: "b.txt"},
			)))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/artifactory/api/copy/fed1/"):
			if !strings.Contains(r.URL.RawQuery, "to=fed1-tmp") {
				t.Fatalf("expected copy-to-temp target, got query %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/artifactory/api/repositories/fed1":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/artifactory/api/repositories/fed1":
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["rclass"] != "local" || body["key"] != "fed1" {
				t.Fatalf("expected local rclass and key fed1, got %+v", body)
			}
			if _, ok := body["members"]; ok {
				t.Fatalf("expected members to be stripped, got %+v", body)
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/artifactory/api/copy/fed1-tmp/"):
			if !strings.Contains(r.URL.RawQuery, "to=fed1") {
				t.Fatalf("expected copy-back target, got query %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	result, err := ConvertToLocal(c, "fed1", "fed1-tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completed result, got %+v", result)
	}
	if result.ItemCount != 2 {
		t.Fatalf("expected 2 items, got %d", result.ItemCount)
	}
	if len(result.CopyFailures) != 0 {
		t.Fatalf("expected no copy failures, got %v", result.CopyFailures)
	}

	// Sanity: delete must come after both copy-to-temp calls (order matters).
	deleteIdx, copyToTempCount := -1, 0
	for i, call := range calls {
		if call == "DELETE /artifactory/api/repositories/fed1" {
			deleteIdx = i
		}
		if strings.HasPrefix(call, "POST /artifactory/api/copy/fed1/") {
			copyToTempCount++
			if deleteIdx != -1 {
				t.Fatalf("copy to temp happened after delete")
			}
		}
	}
	if deleteIdx == -1 || copyToTempCount != 2 {
		t.Fatalf("unexpected call sequence: %v", calls)
	}
}

func TestConvertToLocalAbortsBeforeDeleteIfCopyToTempFails(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/repositories/fed1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"key": "fed1", "rclass": "federated"})
		case r.Method == http.MethodPut && r.URL.Path == "/artifactory/api/repositories/fed1-temp":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			_, _ = w.Write([]byte(aqlItemsResponse(
				struct{ Path, Name string }{Path: ".", Name: "a.txt"},
			)))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/artifactory/api/copy/fed1/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("copy failed"))
		case r.Method == http.MethodDelete:
			t.Fatalf("must not delete the source repo when a copy failed")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	result, err := ConvertToLocal(c, "fed1", "")
	if err == nil {
		t.Fatalf("expected error due to copy failure")
	}
	if len(result.CopyFailures) != 1 {
		t.Fatalf("expected 1 copy failure recorded, got %v", result.CopyFailures)
	}
	if result.Completed {
		t.Fatalf("expected incomplete result")
	}
}

func TestConvertToLocalReportsFailureOnCopyBack(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleted := false
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/repositories/fed1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"key": "fed1", "rclass": "federated"})
		case r.Method == http.MethodPut && r.URL.Path == "/artifactory/api/repositories/fed1-temp":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			_, _ = w.Write([]byte(aqlItemsResponse(
				struct{ Path, Name string }{Path: ".", Name: "a.txt"},
			)))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/artifactory/api/copy/fed1/"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete && r.URL.Path == "/artifactory/api/repositories/fed1":
			deleted = true
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/artifactory/api/repositories/fed1":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/artifactory/api/copy/fed1-temp/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("copy back failed"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	result, err := ConvertToLocal(c, "fed1", "")
	if err == nil {
		t.Fatalf("expected error due to copy-back failure")
	}
	if !deleted {
		t.Fatalf("expected source repo to have been deleted/recreated before copy-back")
	}
	if len(result.CopyFailures) != 1 {
		t.Fatalf("expected 1 copy failure recorded, got %v", result.CopyFailures)
	}
	if result.Completed {
		t.Fatalf("expected incomplete result")
	}
}

func TestTempRepoNameDefault(t *testing.T) {
	if got := TempRepoName("repo1", ""); got != "repo1-temp" {
		t.Fatalf("expected repo1-temp, got %s", got)
	}
	if got := TempRepoName("repo1", "custom"); got != "custom" {
		t.Fatalf("expected custom, got %s", got)
	}
}

func TestBuildLocalConfigStripsMembersAndSetsRclassAndKey(t *testing.T) {
	cfg := map[string]interface{}{
		"key":     "fed1",
		"rclass":  "federated",
		"members": []interface{}{"a"},
		"other":   "value",
	}
	out := buildLocalConfig(cfg, "fed1-temp")
	if out["rclass"] != "local" || out["key"] != "fed1-temp" || out["other"] != "value" {
		t.Fatalf("unexpected config: %+v", out)
	}
	if _, ok := out["members"]; ok {
		t.Fatalf("expected members to be stripped: %+v", out)
	}
	// Original cfg must be untouched.
	if _, ok := cfg["members"]; !ok {
		t.Fatalf("original config must not be mutated")
	}
}
