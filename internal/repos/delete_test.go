package repos

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestReadKeysFromFile_SkipsBlanksAndComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	content := "repo-a\n\n# a comment\n  \nrepo-b\n#repo-c\nrepo-d\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	keys, err := ReadKeysFromFile(path)
	if err != nil {
		t.Fatalf("ReadKeysFromFile returned error: %v", err)
	}
	want := []string{"repo-a", "repo-b", "repo-d"}
	if len(keys) != len(want) {
		t.Fatalf("expected %v, got %v", want, keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, keys)
		}
	}
}

func TestReadKeysFromFile_MissingFile(t *testing.T) {
	if _, err := ReadKeysFromFile("/nonexistent/path/keys.txt"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDeleteRepos_DryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not issue any HTTP call, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := DeleteRepos(c, []string{"a", "b"}, true)
	if results != nil {
		t.Fatalf("expected nil results for dry-run, got %+v", results)
	}
}

func TestDeleteRepos_AppliesAndReportsStatus(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/artifactory/api/repositories/ok-repo":
			w.WriteHeader(http.StatusNoContent)
		case "/artifactory/api/repositories/bad-repo":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := DeleteRepos(c, []string{"ok-repo", "bad-repo"}, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	if results[0].Err != nil || results[0].StatusCode != http.StatusNoContent {
		t.Fatalf("expected ok-repo to succeed, got %+v", results[0])
	}
	if results[1].Err == nil {
		t.Fatalf("expected bad-repo to report an error, got %+v", results[1])
	}
}
