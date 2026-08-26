package projects

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

func TestList_ReturnsProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/access/api/v1/projects" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Project{
			{ProjectKey: "proj1", DisplayName: "Project One"},
			{ProjectKey: "proj2", DisplayName: "Project Two"},
		})
	}))
	defer srv.Close()

	list, err := List(testClient(t, srv))
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(list) != 2 || list[0].ProjectKey != "proj1" || list[1].DisplayName != "Project Two" {
		t.Fatalf("unexpected projects: %+v", list)
	}
}

func TestList_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	if _, err := List(testClient(t, srv)); err == nil {
		t.Fatal("expected error on 401 response, got nil")
	} else if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected error to mention status code, got: %v", err)
	}
}
