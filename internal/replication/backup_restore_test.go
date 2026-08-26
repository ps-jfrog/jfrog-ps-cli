package replication

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestBackupWriteAndLoadRoundtrip(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/replications/repo1":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"url": "https://t/artifactory/repo1"}})
		}
	})
	defer srv.Close()

	data, err := Backup(c, []string{"repo1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := data["repo1"]; !ok {
		t.Fatalf("expected repo1 in backup data, got %+v", data)
	}

	path := filepath.Join(t.TempDir(), "backup.json")
	if err := WriteBackupFile(path, data); err != nil {
		t.Fatalf("unexpected error writing backup: %v", err)
	}

	loaded, err := LoadBackupFile(path)
	if err != nil {
		t.Fatalf("unexpected error loading backup: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}
}

func TestBackupReportsGetFailure(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	if _, err := Backup(c, []string{"repo1"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRestoreDryRunMakesNoHTTPCalls(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run should not issue any HTTP calls, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()

	data := map[string]json.RawMessage{"repo1": json.RawMessage(`{"url":"https://t/artifactory/repo1"}`)}
	results := Restore(c, data, true)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected dry-run success marker, got %+v", results)
	}
}

func TestRestoreAppliesPUT(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	data := map[string]json.RawMessage{"repo1": json.RawMessage(`{"url":"https://t/artifactory/repo1"}`)}
	results := Restore(c, data, false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected success, got %+v", results)
	}
	if gotMethod != http.MethodPut || gotPath != "/artifactory/api/replications/repo1" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["url"] != "https://t/artifactory/repo1" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
}

func TestRestoreResultsSortedByRepo(t *testing.T) {
	data := map[string]json.RawMessage{
		"repoB": json.RawMessage(`{}`),
		"repoA": json.RawMessage(`{}`),
	}
	results := Restore(nil, data, true)
	if len(results) != 2 || results[0].Repo != "repoA" || results[1].Repo != "repoB" {
		t.Fatalf("expected sorted results, got %+v", results)
	}
}
