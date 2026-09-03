package cleanup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"jfrogpscli/internal/jfclient"
)

// storageFixture builds a fake GET /artifactory/api/storage/{repo}[/{path}]
// handler from a map of path -> children. The empty string "" is the repo
// root.
func storageFixture(t *testing.T, repo string, tree map[string]storageInfo) *httptest.Server {
	t.Helper()
	prefix := "/artifactory/api/storage/" + repo
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Path
		if path != prefix && len(path) > len(prefix) {
			path = path[len(prefix)+1:]
		} else {
			path = ""
		}
		info, ok := tree[path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	}))
}

func TestFindEmptyFoldersMinimalCoveringSet(t *testing.T) {
	tree := map[string]storageInfo{
		"": {Children: []storageChild{
			{URI: "/empty1", Folder: true},
			{URI: "/full", Folder: true},
		}},
		"empty1": {Children: []storageChild{
			{URI: "/nested-empty", Folder: true},
		}},
		"empty1/nested-empty": {Children: nil},
		"full": {Children: []storageChild{
			{URI: "/file.txt", Folder: false},
			{URI: "/inner-empty", Folder: true},
		}},
		"full/inner-empty": {Children: nil},
	}
	srv := storageFixture(t, "test-repo", tree)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	got, err := FindEmptyFolders(client, "test-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sort.Strings(got)
	want := []string{"empty1", "full/inner-empty"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestFindEmptyFoldersNeverReturnsRepoRoot(t *testing.T) {
	tree := map[string]storageInfo{
		"": {Children: nil},
	}
	srv := storageFixture(t, "empty-repo", tree)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	got, err := FindEmptyFolders(client, "empty-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no candidates for an entirely empty repo (root is never a candidate), got %v", got)
	}
}

func TestFindEmptyFoldersNoEmptyFolders(t *testing.T) {
	tree := map[string]storageInfo{
		"": {Children: []storageChild{
			{URI: "/file.txt", Folder: false},
			{URI: "/dir", Folder: true},
		}},
		"dir": {Children: []storageChild{
			{URI: "/other.txt", Folder: false},
		}},
	}
	srv := storageFixture(t, "repo", tree)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	got, err := FindEmptyFolders(client, "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no candidates, got %v", got)
	}
}

func TestFindEmptyFoldersReturnsErrorForUnreadableRepoRoot(t *testing.T) {
	// Regression: a failure fetching the repo root itself (typo'd or
	// inaccessible repo) used to be swallowed the same way an unreadable
	// subfolder is, silently reporting "0 empty folders" instead of erroring.
	srv := storageFixture(t, "repo", map[string]storageInfo{
		// "repo" tree deliberately left empty, so the root path "" 404s.
	})
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	got, err := FindEmptyFolders(client, "repo")
	if err == nil {
		t.Fatalf("expected an error for an unreadable repo root, got candidates %v", got)
	}
}

func TestDeleteEmptyFoldersDryRunNeverCallsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := DeleteEmptyFolders(client, "repo", []string{"a", "b/c"}, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.DryRun || res.Success {
			t.Fatalf("expected dry-run non-success result, got %+v", res)
		}
	}
}

func TestDeleteEmptyFoldersRealDeleteStatuses(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/artifactory/repo/empty1":
			w.WriteHeader(http.StatusNoContent)
		case "/artifactory/repo/full/inner-empty":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := DeleteEmptyFolders(client, "repo", []string{"empty1", "full/inner-empty"}, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success || results[0].StatusCode != http.StatusNoContent {
		t.Fatalf("expected empty1 to succeed: %+v", results[0])
	}
	if results[1].Success || results[1].StatusCode != http.StatusNotFound {
		t.Fatalf("expected full/inner-empty to fail with 404: %+v", results[1])
	}
}
