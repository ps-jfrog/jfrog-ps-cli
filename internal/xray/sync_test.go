package xray

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestSyncPoliciesDryRunMakesNoTargetCalls(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"global-pol"},{"name":"proj-pol","project_key":"proj1"}]`))
	}))
	defer source.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Fatalf("dry-run must not issue POST calls to target")
		}
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	results, err := SyncPolicies(srcClient, tgtClient, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.DryRun || r.Success {
			t.Errorf("expected dry-run no-op result, got %+v", r)
		}
	}
}

func TestSyncPoliciesAppliesGlobalAndProjectScoped(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"global-pol"},{"name":"proj-pol","project_key":"proj1"}]`))
	}))
	defer source.Close()

	var calls []*http.Request
	var bodies []map[string]interface{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, r)
		bodies = append(bodies, body)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	results, err := SyncPolicies(srcClient, tgtClient, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected success, got %+v", r)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 target calls, got %d", len(calls))
	}

	// Find the project-scoped call and verify projectKey query param + stripped body.
	var foundProjectScoped, foundGlobal bool
	for i, req := range calls {
		if req.URL.Query().Get("projectKey") == "proj1" {
			foundProjectScoped = true
			if _, ok := bodies[i]["project_key"]; ok {
				t.Errorf("expected project_key stripped from body, got %v", bodies[i])
			}
			if bodies[i]["name"] != "proj-pol" {
				t.Errorf("unexpected body: %v", bodies[i])
			}
		} else if req.URL.Query().Get("projectKey") == "" {
			foundGlobal = true
			if bodies[i]["name"] != "global-pol" {
				t.Errorf("unexpected body: %v", bodies[i])
			}
		}
	}
	if !foundProjectScoped || !foundGlobal {
		t.Fatalf("expected one global and one project-scoped call, calls=%v", calls)
	}
}

func TestSyncWatchesStripsNestedProjectKey(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"watch1","general_data":{"project_key":"proj1","other":"x"}}]`))
	}))
	defer source.Close()

	var gotQuery string
	var gotBody map[string]interface{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	results, err := SyncWatches(srcClient, tgtClient, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected results: %+v", results)
	}
	if gotQuery != "projectKey=proj1" {
		t.Fatalf("expected projectKey=proj1 query, got %q", gotQuery)
	}
	gd, ok := gotBody["general_data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected general_data in body, got %v", gotBody)
	}
	if _, ok := gd["project_key"]; ok {
		t.Errorf("expected project_key stripped from general_data, got %v", gd)
	}
	if gd["other"] != "x" {
		t.Errorf("expected other general_data fields preserved, got %v", gd)
	}
}

func TestSyncWatchesDryRunMakesNoTargetCalls(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"watch1"}]`))
	}))
	defer source.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Fatalf("dry-run must not issue POST calls to target")
		}
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	if _, err := SyncWatches(srcClient, tgtClient, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncIgnoreRulesAppliesProjectScoped(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"rule1","project_key":"proj1"}]`))
	}))
	defer source.Close()

	var gotQuery string
	var gotBody map[string]interface{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/xray/api/v1/ignore_rules" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	results, err := SyncIgnoreRules(srcClient, tgtClient, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("unexpected results: %+v", results)
	}
	if gotQuery != "projectKey=proj1" {
		t.Fatalf("expected projectKey=proj1 query, got %q", gotQuery)
	}
	if _, ok := gotBody["project_key"]; ok {
		t.Errorf("expected project_key stripped from body, got %v", gotBody)
	}
}

func TestSyncIgnoreRulesDryRunMakesNoTargetCalls(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"rule1"}]`))
	}))
	defer source.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Fatalf("dry-run must not issue POST calls to target")
		}
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	if _, err := SyncIgnoreRules(srcClient, tgtClient, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestSyncNeverUsesHardcodedServer guards against the original bash script's
// bug of hardcoding a server ID/alias for project-scoped entities: every
// mutating call in this file must go through the *target client instance*
// passed in by the caller, never a separately-constructed client. This test
// simply verifies that both global and project-scoped policies land on the
// same target test server.
func TestSyncNeverUsesHardcodedServer(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"global-pol"},{"name":"proj-pol","project_key":"proj1"}]`))
	}))
	defer source.Close()

	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	srcClient := jfclient.New(source.URL, "tok", "", "", false)
	tgtClient := jfclient.New(target.URL, "tok", "", "", false)
	if _, err := SyncPolicies(srcClient, tgtClient, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 2 {
		t.Fatalf("expected both policies posted to the single target server, got %d hits", hits)
	}
}
