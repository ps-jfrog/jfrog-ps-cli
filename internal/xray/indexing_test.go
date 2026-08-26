package xray

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestIndexStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/binMgr/default/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"indexed_repos":["repo-a","repo-c"]}`))
	})
	mux.HandleFunc("/artifactory/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("type"); got != "local" {
			t.Fatalf("expected type=local, got %q", got)
		}
		_, _ = w.Write([]byte(`[{"key":"repo-a","type":"local"},{"key":"repo-b","type":"local"}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	statuses, err := IndexStatus(client, "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	want := map[string]bool{"repo-a": true, "repo-b": false}
	for _, s := range statuses {
		if s.Indexed != want[s.Repo] {
			t.Errorf("repo %q: expected indexed=%v, got %v", s.Repo, want[s.Repo], s.Indexed)
		}
	}
}

func TestListRepositories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/repositories" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "remote" {
			t.Fatalf("expected type=remote, got %q", got)
		}
		_, _ = w.Write([]byte(`[{"key":"r1","type":"remote"},{"key":"r2","type":"remote"}]`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	repos, err := ListRepositories(client, "remote")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 || repos[0] != "r1" || repos[1] != "r2" {
		t.Fatalf("unexpected repos: %v", repos)
	}
}

func TestToggleIndexingDryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Fatalf("dry-run must not issue POST calls")
		}
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := ToggleIndexing(client, []string{"repo-a", "repo-b"}, true, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.DryRun {
			t.Errorf("expected DryRun=true for %q", res.Repo)
		}
		if res.Success {
			t.Errorf("dry-run result should not report success for %q", res.Repo)
		}
	}
}

func TestToggleIndexingAppliesChanges(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var body map[string]bool
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["xrayIndex"] != false {
			t.Fatalf("expected xrayIndex=false, got %v", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := ToggleIndexing(client, []string{"repo-a"}, false, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success, got %+v", results[0])
	}
	if len(calls) != 1 || calls[0] != "/artifactory/api/repositories/repo-a" {
		t.Fatalf("unexpected calls: %v", calls)
	}
}

func TestReindexDryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not issue any HTTP calls, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	for _, mode := range []string{ReindexModeIndex, ReindexModeForce, ReindexModeScanNow} {
		res, err := Reindex(client, "repo-a", mode, true)
		if err != nil {
			t.Fatalf("unexpected error for mode %q: %v", mode, err)
		}
		if !res.DryRun || res.Success {
			t.Errorf("mode %q: expected dry-run no-op result, got %+v", mode, res)
		}
	}
}

func TestReindexModeIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xray/api/v1/index/repository/repo-a" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	res, err := Reindex(client, "repo-a", ReindexModeIndex, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
}

func TestReindexModeForce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xray/api/v1/forceReindex" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["is_non_indexed_repo_allowed"] != true {
			t.Fatalf("expected is_non_indexed_repo_allowed=true, got %v", body)
		}
		repos, ok := body["repos"].([]interface{})
		if !ok || len(repos) != 1 || repos[0] != "repo-a" {
			t.Fatalf("unexpected repos field: %v", body["repos"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	res, err := Reindex(client, "repo-a", ReindexModeForce, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
}

func TestReindexModeScanNow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xray/api/v2/index" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	res, err := Reindex(client, "repo-a", ReindexModeScanNow, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
}

func TestReindexUnknownMode(t *testing.T) {
	client := jfclient.New("http://example.invalid", "tok", "", "", false)
	if _, err := Reindex(client, "repo-a", "bogus", true); err == nil {
		t.Fatalf("expected error for unknown mode")
	}
}
