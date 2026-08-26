package federationcmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestConvertToFederatedRequiresRepoOrAll(t *testing.T) {
	if code := Run([]string{"convert-to-federated", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestConvertToFederatedRejectsRepoAndAllTogether(t *testing.T) {
	if code := Run([]string{"convert-to-federated", "--repo", "r1", "--all", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestConvertToFederatedDryRunEndToEndMakesNoMutatingCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			t.Fatalf("dry-run should not issue mutating calls, got %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	code := Run([]string{"convert-to-federated", "--repo", "repo1", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestResyncRequiresRepoOrAll(t *testing.T) {
	if code := Run([]string{"resync", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestResyncDryRunEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Fatalf("dry-run should not issue mutating calls, got %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	code := Run([]string{"resync", "--repo", "fed1", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestCheckMembersEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/repositories/fed1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"members": []map[string]interface{}{
					{"url": "https://a", "enabled": true},
					{"url": "https://b", "enabled": true},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	code := Run([]string{"check-members", "--repo", "fed1", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestConvertToLocalRequiresRepo(t *testing.T) {
	if code := Run([]string{"convert-to-local", "--url", "https://x.example.com", "--token", "tok"}); code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestConvertToLocalDryRunEndToEndMakesNoMutatingCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut || r.Method == http.MethodPost && r.URL.Path != "/artifactory/api/search/aql" || r.Method == http.MethodDelete {
			t.Fatalf("dry-run should not issue mutating calls, got %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/artifactory/api/search/aql" {
			_, _ = w.Write([]byte(`{"results":[]}`))
		}
	}))
	defer srv.Close()

	code := Run([]string{"convert-to-local", "--repo", "fed1", "--url", srv.URL, "--token", "tok"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}
