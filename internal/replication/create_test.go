package replication

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBuildPayloadDefaultsTargetRepoToSource(t *testing.T) {
	p := BuildPayload("repo1", CreateOptions{TargetURL: "https://target.example.com/", Cron: "0 0 * * * ?", Enabled: true})
	want := "https://target.example.com/artifactory/repo1"
	if p.URL != want {
		t.Fatalf("expected url %q, got %q", want, p.URL)
	}
	if p.RepoKey != "repo1" {
		t.Fatalf("expected repoKey repo1, got %s", p.RepoKey)
	}
}

func TestBuildPayloadHonorsTargetRepo(t *testing.T) {
	p := BuildPayload("repo1", CreateOptions{TargetURL: "https://target.example.com", TargetRepo: "repo1-mirror"})
	want := "https://target.example.com/artifactory/repo1-mirror"
	if p.URL != want {
		t.Fatalf("expected url %q, got %q", want, p.URL)
	}
}

func TestCreateDryRunMakesNoPUT(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("dry-run should not issue mutating calls, got %s %s", r.Method, r.URL.Path)
		}
	})
	defer srv.Close()

	results := Create(c, []string{"repo1", "repo2"}, CreateOptions{TargetURL: "https://target.example.com"}, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected dry-run result to be marked success, got %+v", r)
		}
	}
}

func TestCreateAppliesPUTWithExpectedPayload(t *testing.T) {
	var gotBody Payload
	var gotMethod, gotPath string
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	results := Create(c, []string{"repo1"}, CreateOptions{
		TargetURL:   "https://target.example.com",
		Cron:        "0 0/6 * * * ?",
		Username:    "svc",
		Password:    "secret",
		SyncDeletes: true,
		Enabled:     true,
	}, false)

	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected success, got %+v", results)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/artifactory/api/replications/repo1" {
		t.Fatalf("unexpected path %s", gotPath)
	}
	if gotBody.RepoKey != "repo1" || gotBody.Username != "svc" || !gotBody.SyncDeletes {
		t.Fatalf("unexpected payload: %+v", gotBody)
	}
}

func TestCreateReportsFailure(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	defer srv.Close()

	results := Create(c, []string{"repo1"}, CreateOptions{TargetURL: "https://target.example.com"}, false)
	if len(results) != 1 || results[0].Success || results[0].Err == nil {
		t.Fatalf("expected failure result, got %+v", results)
	}
}
