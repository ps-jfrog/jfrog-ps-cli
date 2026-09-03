package replicationcmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestRunNoArgsReturnsError(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRunHelpReturnsZero(t *testing.T) {
	if code := Run([]string{"-h"}); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if code := Run([]string{"help"}); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestRunUnknownSubcommandReturnsError(t *testing.T) {
	if code := Run([]string{"bogus"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestListSubcommandHelp(t *testing.T) {
	if code := Run([]string{"list", "-h"}); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestCreateRequiresRepoOrRepoFile(t *testing.T) {
	if code := Run([]string{"create", "--target-url", "https://x.example.com", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestCreateRequiresTargetURL(t *testing.T) {
	if code := Run([]string{"create", "--repo", "repo1", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestDeleteRequiresRepoOrAll(t *testing.T) {
	if code := Run([]string{"delete", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestDeleteRejectsRepoAndAllTogether(t *testing.T) {
	if code := Run([]string{"delete", "--repo", "r1", "--all", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestListEndToEndAgainstMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/replications/repo1":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"url": "https://target.example.com/artifactory/repo1"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	code := Run([]string{"list", "--repo", "repo1", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestCreateDryRunEndToEndMakesNoMutatingCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut || r.Method == http.MethodPost || r.Method == http.MethodDelete {
			t.Fatalf("dry-run should not issue mutating calls, got %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	code := Run([]string{"create", "--repo", "repo1", "--target-url", "https://target.example.com", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestCreateFromRepoFile(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var mu sync.Mutex
	var puts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			mu.Lock()
			puts = append(puts, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	repoFile := filepath.Join(t.TempDir(), "repos.txt")
	if err := os.WriteFile(repoFile, []byte("# comment\nrepo1\n\nrepo2\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}

	code := Run([]string{"create", "--repo-file", repoFile, "--target-url", "https://target.example.com", "--dry-run=false", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if len(puts) != 2 {
		t.Fatalf("expected 2 PUT calls, got %v", puts)
	}
}

func TestBackupRequiresOutput(t *testing.T) {
	if code := Run([]string{"backup", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRestoreRequiresInput(t *testing.T) {
	if code := Run([]string{"restore", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRestoreDryRunEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run should not issue any HTTP calls, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	inputFile := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(inputFile, []byte(`{"repo1":{"url":"https://t/artifactory/repo1"}}`), 0o644); err != nil {
		t.Fatalf("write backup file: %v", err)
	}

	code := Run([]string{"restore", "--input", inputFile, "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}
