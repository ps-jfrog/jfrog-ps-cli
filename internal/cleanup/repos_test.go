package cleanup

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestReadRepoKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "repos.txt")
	content := "repo-a\n# a comment\n\nrepo-b\n   \nrepo-c\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	keys, err := ReadRepoKeys(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"repo-a", "repo-b", "repo-c"}
	if len(keys) != len(want) {
		t.Fatalf("expected %v, got %v", want, keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, keys)
		}
	}
}

func TestReadRepoKeysMissingFile(t *testing.T) {
	if _, err := ReadRepoKeys("/nonexistent/path/repos.txt"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestDeleteReposDryRunNeverCallsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := DeleteRepos(client, []string{"repo-a", "repo-b"}, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.DryRun || res.Success {
			t.Fatalf("expected dry-run non-success result, got %+v", res)
		}
	}
}

func TestDeleteReposRealDeleteStatuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/artifactory/api/repositories/") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "repo-ok"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "repo-missing"):
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := DeleteRepos(client, []string{"repo-ok", "repo-missing", "repo-other"}, false)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Success || results[0].StatusCode != http.StatusOK {
		t.Fatalf("expected repo-ok to succeed: %+v", results[0])
	}
	if results[1].Success || results[1].StatusCode != http.StatusNotFound {
		t.Fatalf("expected repo-missing to fail with 404: %+v", results[1])
	}
	if results[2].Success || results[2].StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected repo-other to fail with 500: %+v", results[2])
	}
}
