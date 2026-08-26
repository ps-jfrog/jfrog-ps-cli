package repos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func testClient(t *testing.T, srv *httptest.Server) *jfclient.Client {
	t.Helper()
	return jfclient.New(srv.URL, "test-token", "", "", false)
}

func TestList_TypeQueryParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/repositories" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Repo{{Key: "libs-release", Type: "local", URL: "https://x/libs-release"}})
	}))
	defer srv.Close()

	c := testClient(t, srv)
	repos, err := List(c, "local")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if gotQuery != "type=local" {
		t.Fatalf("expected query type=local, got %q", gotQuery)
	}
	if len(repos) != 1 || repos[0].Key != "libs-release" {
		t.Fatalf("unexpected repos: %+v", repos)
	}
}

func TestList_AllOmitsTypeParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Repo{})
	}))
	defer srv.Close()

	c := testClient(t, srv)
	if _, err := List(c, "all"); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("expected no query params for type=all, got %q", gotQuery)
	}

	if _, err := List(c, ""); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("expected no query params for type=\"\", got %q", gotQuery)
	}
}

func TestList_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	if _, err := List(c, "local"); err == nil {
		t.Fatal("expected error on 403 response, got nil")
	} else if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected error to mention status code, got: %v", err)
	}
}
