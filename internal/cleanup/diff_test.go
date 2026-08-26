package cleanup

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestDiffKey(t *testing.T) {
	cases := []struct {
		m    ArtifactMatch
		want string
	}{
		{ArtifactMatch{Path: "a/b", Name: "n.txt"}, "a/b/n.txt"},
		{ArtifactMatch{Path: ".", Name: "n.txt"}, "n.txt"},
		{ArtifactMatch{Path: "", Name: "n.txt"}, "n.txt"},
	}
	for _, tc := range cases {
		if got := DiffKey(tc.m); got != tc.want {
			t.Fatalf("DiffKey(%+v) = %q, want %q", tc.m, got, tc.want)
		}
	}
}

func TestDiffAQLQuery(t *testing.T) {
	q := DiffAQLQuery("my-repo")
	if q != `items.find({"repo":"my-repo"}).include("path","name","sha256")` {
		t.Fatalf("unexpected query: %q", q)
	}
}

func newAQLServer(t *testing.T, results string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[` + results + `]}`))
	}))
}

func TestTargetOnlyArtifacts(t *testing.T) {
	sourceSrv := newAQLServer(t, `
		{"repo":"my-repo","path":"a","name":"shared.txt","sha256":"s1"},
		{"repo":"my-repo","path":".","name":"root-shared.txt","sha256":"s2"}
	`)
	defer sourceSrv.Close()

	targetSrv := newAQLServer(t, `
		{"repo":"my-repo","path":"a","name":"shared.txt","sha256":"s1"},
		{"repo":"my-repo","path":".","name":"root-shared.txt","sha256":"s2"},
		{"repo":"my-repo","path":"b","name":"extra.txt","sha256":"s3"}
	`)
	defer targetSrv.Close()

	source := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	target := jfclient.New(targetSrv.URL, "tok", "", "", false)

	onlyInTarget, err := TargetOnlyArtifacts(source, target, "my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(onlyInTarget) != 1 {
		t.Fatalf("expected 1 target-only artifact, got %d: %+v", len(onlyInTarget), onlyInTarget)
	}
	if onlyInTarget[0].Name != "extra.txt" {
		t.Fatalf("expected extra.txt, got %+v", onlyInTarget[0])
	}
}

func TestTargetOnlyArtifactsSourceError(t *testing.T) {
	sourceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sourceSrv.Close()
	targetSrv := newAQLServer(t, "")
	defer targetSrv.Close()

	source := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	target := jfclient.New(targetSrv.URL, "tok", "", "", false)

	if _, err := TargetOnlyArtifacts(source, target, "my-repo"); err == nil {
		t.Fatalf("expected error when source query fails")
	}
}

// TestDiffDeleteNeverCallsDeleteInDryRun locks in that the deletion step used
// by diff-delete-artifacts (DeleteArtifacts against the target client) never
// issues a DELETE when dryRun=true.
func TestDiffDeleteNeverCallsDeleteInDryRun(t *testing.T) {
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called against target when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer targetSrv.Close()

	target := jfclient.New(targetSrv.URL, "tok", "", "", false)
	candidates := []ArtifactMatch{{Repo: "my-repo", Path: "b", Name: "extra.txt"}}
	results := DeleteArtifacts(target, candidates, true)
	if len(results) != 1 || !results[0].DryRun || results[0].Success {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
}
