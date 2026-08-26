package federation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func newTestClient(handler http.HandlerFunc) (*jfclient.Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := jfclient.New(srv.URL, "test-token", "", "", false)
	return c, srv
}

func TestListLocalRepos(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "local" {
			t.Fatalf("expected type=local, got %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"key": "repo1"}, {"key": "repo2"}})
	})
	defer srv.Close()

	repos, err := ListLocalRepos(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 || repos[0] != "repo1" || repos[1] != "repo2" {
		t.Fatalf("unexpected repos: %v", repos)
	}
}

func TestListFederatedRepos(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "federated" {
			t.Fatalf("expected type=federated, got %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"key": "fed1"}})
	})
	defer srv.Close()

	repos, err := ListFederatedRepos(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 || repos[0] != "fed1" {
		t.Fatalf("unexpected repos: %v", repos)
	}
}

func TestListReposPropagatesError(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	if _, err := ListLocalRepos(c); err == nil {
		t.Fatalf("expected error")
	}
}
