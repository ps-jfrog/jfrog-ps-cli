package repos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestMissingOnTarget(t *testing.T) {
	source := []Repo{{Key: "a"}, {Key: "b"}, {Key: "c"}}
	target := []Repo{{Key: "b"}}

	missing := MissingOnTarget(source, target)
	if len(missing) != 2 || missing[0].Key != "a" || missing[1].Key != "c" {
		t.Fatalf("unexpected missing set: %+v", missing)
	}
}

func TestMissingOnTarget_Empty(t *testing.T) {
	source := []Repo{{Key: "a"}}
	target := []Repo{{Key: "a"}}
	if missing := MissingOnTarget(source, target); len(missing) != 0 {
		t.Fatalf("expected no missing repos, got %+v", missing)
	}
}

func reposHandler(repos []Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos)
	}
}

func TestDiff_EndToEnd(t *testing.T) {
	source := httptest.NewServer(reposHandler([]Repo{{Key: "a", Type: "local"}, {Key: "b", Type: "local"}}))
	defer source.Close()
	target := httptest.NewServer(reposHandler([]Repo{{Key: "a", Type: "local"}}))
	defer target.Close()

	result, err := Diff(testClient(t, source), testClient(t, target), "local")
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if len(result.MissingOnTarget) != 1 || result.MissingOnTarget[0].Key != "b" {
		t.Fatalf("unexpected diff result: %+v", result)
	}
}

func TestCopyMissing_DryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("dry-run must not issue mutating call, got %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := CopyMissing(c, c, []Repo{{Key: "a"}, {Key: "b"}}, true)
	if results != nil {
		t.Fatalf("expected nil results for dry-run, got %+v", results)
	}
}

func TestCopyMissing_AppliesAndStripsProjectKey(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	sourceHits := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sourceHits++
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method on source: %s", r.Method)
		}
		if r.URL.Path != "/artifactory/api/repositories/reponly" {
			t.Fatalf("unexpected path on source: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"key":        "reponly",
			"rclass":     "local",
			"projectKey": "myproj",
		})
	}))
	defer source.Close()

	var putBody map[string]interface{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT to target, got %s", r.Method)
		}
		if r.URL.Path != "/artifactory/api/repositories/reponly" {
			t.Fatalf("unexpected path on target: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&putBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	results := CopyMissing(testClient(t, source), testClient(t, target), []Repo{{Key: "reponly"}}, false)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
	if sourceHits != 1 {
		t.Fatalf("expected exactly 1 GET to source, got %d", sourceHits)
	}
	if _, present := putBody["projectKey"]; present {
		t.Fatalf("expected projectKey to be stripped before PUT, got body: %+v", putBody)
	}
	if putBody["key"] != "reponly" {
		t.Fatalf("expected other fields to survive, got body: %+v", putBody)
	}
}

func TestCopyMissing_ReportsPerRepoErrors(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer source.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	results := CopyMissing(testClient(t, source), testClient(t, target), []Repo{{Key: "missing"}}, false)
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("expected an error result, got %+v", results)
	}
}
