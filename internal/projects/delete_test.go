package projects

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReadKeysFromFile_SkipsBlanksAndComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	content := "proj-a\n\n# a comment\n  \nproj-b\n#proj-c\nproj-d\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	keys, err := ReadKeysFromFile(path)
	if err != nil {
		t.Fatalf("ReadKeysFromFile returned error: %v", err)
	}
	want := []string{"proj-a", "proj-b", "proj-d"}
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

func TestDeleteProjects_AppliesAndReportsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/access/api/v1/projects/ok-proj":
			w.WriteHeader(http.StatusNoContent)
		case "/access/api/v1/projects/bad-proj":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := DeleteProjects(c, []string{"ok-proj", "bad-proj"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	if results[0].Err != nil || results[0].StatusCode != http.StatusNoContent {
		t.Fatalf("expected ok-proj to succeed, got %+v", results[0])
	}
	if results[1].Err == nil {
		t.Fatalf("expected bad-proj to report an error, got %+v", results[1])
	}
}
