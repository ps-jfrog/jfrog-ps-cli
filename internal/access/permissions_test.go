package access

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *jfclient.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return jfclient.New(srv.URL, "test-token", "", "", false)
}

func TestListPermissions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/artifactory/api/security/permissions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"perm1","uri":"/perm1"},{"name":"perm2","uri":"/perm2"}]`))
	})

	got, err := ListPermissions(c)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(got) != 2 || got[0].Name != "perm1" || got[1].URI != "/perm2" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestListPermissions_Error(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	})
	if _, err := ListPermissions(c); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestShowPermission(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/security/permissions/my-perm" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name":"my-perm",
			"repositories":["repo1","repo2"],
			"principals":{
				"users":{"user1":["m","w","n","r"]},
				"groups":{"group1":["r"]}
			}
		}`))
	})

	got, err := ShowPermission(c, "my-perm")
	if err != nil {
		t.Fatalf("ShowPermission: %v", err)
	}
	if got.Name != "my-perm" || len(got.Repositories) != 2 {
		t.Fatalf("unexpected result: %+v", got)
	}
	if _, ok := got.Principals.Users["user1"]; !ok {
		t.Fatalf("expected user1 in principals: %+v", got.Principals)
	}
}

func TestFindEmptyPermissions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artifactory/api/security/permissions":
			w.Write([]byte(`[{"name":"empty-perm"},{"name":"full-perm"},{"name":"INTERNAL"}]`))
		case "/artifactory/api/security/permissions/empty-perm":
			w.Write([]byte(`{"name":"empty-perm","repositories":[]}`))
		case "/artifactory/api/security/permissions/full-perm":
			w.Write([]byte(`{"name":"full-perm","repositories":["repo1"]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	got, err := FindEmptyPermissions(c)
	if err != nil {
		t.Fatalf("FindEmptyPermissions: %v", err)
	}
	if len(got) != 1 || got[0] != "empty-perm" {
		t.Fatalf("unexpected result: %+v (INTERNAL should be excluded)", got)
	}
}

func TestListUsersInPermission(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name":"my-perm",
			"repositories":["repo1"],
			"principals":{"users":{"bob":["r"],"alice":["w"]},"groups":{}}
		}`))
	})

	got, err := ListUsersInPermission(c, "my-perm")
	if err != nil {
		t.Fatalf("ListUsersInPermission: %v", err)
	}
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("expected sorted [alice bob], got %+v", got)
	}
}
