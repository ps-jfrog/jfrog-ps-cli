package xray

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

const sampleRepoConfig = `{
  "repo_name": "repo-a",
  "repo_config": {
    "vuln_contextual_analysis": false,
    "retention_in_days": 90,
    "exposures": {
      "scanners_category": {
        "secrets_scan": false,
        "services_scan": false,
        "applications_scan": true,
        "iac_scan": false
      }
    }
  },
  "repo_paths_config": {"patterns": []}
}`

func TestToggleJASDryRunMakesNoPUT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Fatalf("dry-run must not issue PUT calls")
		}
		if r.URL.Path != "/xray/api/v1/repos_config/repo-a" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(sampleRepoConfig))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	v := true
	results := ToggleJAS(client, []string{"repo-a"}, JASToggleOptions{EnableExposures: &v}, true)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if !res.DryRun || res.Changed {
		t.Fatalf("expected dry-run result with no changes recorded, got %+v", res)
	}
	if res.Before.ApplicationsScan == nil || !*res.Before.ApplicationsScan {
		t.Fatalf("expected Before.ApplicationsScan=true, got %+v", res.Before)
	}
	if res.Before.SecretsScan == nil || *res.Before.SecretsScan {
		t.Fatalf("expected Before.SecretsScan=false, got %+v", res.Before)
	}
}

func TestToggleJASAppliesChangesAndPUTsWholeConfig(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var putBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(sampleRepoConfig))
		case http.MethodPut:
			if r.URL.Path != "/xray/api/v1/repos_config" {
				t.Fatalf("unexpected PUT path %q", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	enableExposures := true
	enableCA := true
	results := ToggleJAS(client, []string{"repo-a"}, JASToggleOptions{
		EnableExposures:          &enableExposures,
		EnableContextualAnalysis: &enableCA,
	}, false)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if !res.Success || !res.Changed {
		t.Fatalf("expected a successful, changed result, got %+v", res)
	}
	if res.After.SecretsScan == nil || !*res.After.SecretsScan {
		t.Fatalf("expected After.SecretsScan=true, got %+v", res.After)
	}
	if res.After.VulnContextualAnalysis == nil || !*res.After.VulnContextualAnalysis {
		t.Fatalf("expected After.VulnContextualAnalysis=true, got %+v", res.After)
	}

	if putBody["repo_name"] != "repo-a" {
		t.Fatalf("expected repo_name=repo-a in PUT body, got %v", putBody["repo_name"])
	}
	repoConfig, ok := putBody["repo_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected repo_config object in PUT body, got %v", putBody["repo_config"])
	}
	if repoConfig["vuln_contextual_analysis"] != true {
		t.Fatalf("expected vuln_contextual_analysis=true in PUT body, got %v", repoConfig["vuln_contextual_analysis"])
	}
	if _, hasPathsConfig := putBody["repo_paths_config"]; !hasPathsConfig {
		t.Fatalf("expected repo_paths_config to be passed through in PUT body")
	}
}

func TestToggleJASRealRunBlockedByDefaultWriteGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Fatalf("write guard should have blocked the PUT before it reached the server")
		}
		_, _ = w.Write([]byte(sampleRepoConfig))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	v := true
	results := ToggleJAS(client, []string{"repo-a"}, JASToggleOptions{EnableExposures: &v}, false)
	if len(results) != 1 || results[0].Success || results[0].Error == "" {
		t.Fatalf("expected the PUT to be rejected by the default write guard, got %+v", results)
	}
}

func TestToggleJASNoChangeSkipsPUT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Fatalf("expected no PUT when nothing changed")
		}
		_, _ = w.Write([]byte(sampleRepoConfig))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	v := true
	// applications_scan is already true in sampleRepoConfig, but
	// EnableExposures forces *every* present scanner to true, so this
	// should still flip secrets/services/iac -- use a retention-only
	// request instead to exercise the true no-op path.
	same := 90
	results := ToggleJAS(client, []string{"repo-a"}, JASToggleOptions{RetentionInDays: &same}, false)
	_ = v
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success || results[0].Changed {
		t.Fatalf("expected success with no change, got %+v", results[0])
	}
}

func TestToggleJASRepoConfigFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	v := true
	results := ToggleJAS(client, []string{"missing-repo"}, JASToggleOptions{EnableExposures: &v}, false)
	if len(results) != 1 || results[0].Error == "" {
		t.Fatalf("expected a recorded error for a failed GET, got %+v", results)
	}
}

func TestListRepositoriesForProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/repositories" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("project"); got != "my-proj" {
			t.Fatalf("expected project=my-proj, got %q", got)
		}
		_, _ = w.Write([]byte(`[{"key":"repo-a"},{"key":"repo-b"}]`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	repos, err := ListRepositoriesForProject(client, "my-proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 || repos[0] != "repo-a" || repos[1] != "repo-b" {
		t.Fatalf("unexpected repos: %v", repos)
	}
}
