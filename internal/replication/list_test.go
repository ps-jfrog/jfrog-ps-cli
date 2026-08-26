package replication

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

func TestCheckNotConfigured404(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	st, err := Check(c, "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Configured {
		t.Fatalf("expected not configured, got configured")
	}
}

func TestCheckConfiguredArray(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]string{
			{"url": "https://target1.example.com/artifactory/repo1"},
			{"url": "https://target2.example.com/artifactory/repo1"},
		})
	})
	defer srv.Close()

	st, err := Check(c, "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.Configured {
		t.Fatalf("expected configured")
	}
	if len(st.TargetURLs) != 2 {
		t.Fatalf("expected 2 target urls, got %v", st.TargetURLs)
	}
}

func TestCheckConfiguredSingleObject(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://target.example.com/artifactory/repo1"})
	})
	defer srv.Close()

	st, err := Check(c, "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.Configured || len(st.TargetURLs) != 1 {
		t.Fatalf("expected configured with 1 target url, got %+v", st)
	}
}

func TestCheckEmptyArrayNotConfigured(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	defer srv.Close()

	st, err := Check(c, "repo1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Configured {
		t.Fatalf("expected empty array to mean not configured")
	}
}

func TestListAllLocalRepos(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/artifactory/api/repositories":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"key": "repo1"}, {"key": "repo2"}})
		case r.URL.Path == "/artifactory/api/replications/repo1":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"url": "https://t/artifactory/repo1"}})
		case r.URL.Path == "/artifactory/api/replications/repo2":
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	statuses, err := List(c, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if !statuses[0].Configured || statuses[1].Configured {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}
}

func TestReposWithReplicationFilters(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/replications/repo1":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"url": "https://t/artifactory/repo1"}})
		case "/artifactory/api/replications/repo2":
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	found, err := ReposWithReplication(c, []string{"repo1", "repo2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 1 || found[0] != "repo1" {
		t.Fatalf("expected [repo1], got %v", found)
	}
}
