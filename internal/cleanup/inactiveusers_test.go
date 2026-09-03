package cleanup

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jfrogpscli/internal/jfclient"
)

func TestFetchUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/security/users" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"name":"alice","realm":"internal","lastLoggedIn":1600000000000},
			{"name":"bob","realm":"ldap","lastLoggedIn":0}
		]`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	users, err := FetchUsers(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Name != "alice" || users[0].Realm != "internal" || users[0].LastLoggedIn != 1600000000000 {
		t.Fatalf("unexpected user[0]: %+v", users[0])
	}
	if users[1].Name != "bob" || users[1].LastLoggedIn != 0 {
		t.Fatalf("unexpected user[1]: %+v", users[1])
	}
}

func TestFetchUsersErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	if _, err := FetchUsers(client); err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
}

func TestInactiveCandidates(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	sixMonthsAgo := now.AddDate(0, -6, 0).UnixMilli()
	oneMonthAgo := now.AddDate(0, -1, 0).UnixMilli()

	users := []UserActivity{
		{Name: "never-logged-in", Realm: "ldap", LastLoggedIn: 0},
		{Name: "long-inactive", Realm: "ldap", LastLoggedIn: sixMonthsAgo},
		{Name: "recently-active", Realm: "ldap", LastLoggedIn: oneMonthAgo},
		{Name: "internal-excluded", Realm: "internal", LastLoggedIn: 0},
	}

	candidates := InactiveCandidates(users, 3, "internal", now)

	names := map[string]bool{}
	for _, c := range candidates {
		names[c.Name] = true
	}
	if !names["never-logged-in"] {
		t.Fatalf("expected never-logged-in to be a candidate: %+v", candidates)
	}
	if !names["long-inactive"] {
		t.Fatalf("expected long-inactive to be a candidate: %+v", candidates)
	}
	if names["recently-active"] {
		t.Fatalf("expected recently-active to NOT be a candidate: %+v", candidates)
	}
	if names["internal-excluded"] {
		t.Fatalf("expected internal-excluded to NOT be a candidate (excluded realm): %+v", candidates)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected exactly 2 candidates, got %d: %+v", len(candidates), candidates)
	}
}

func TestDeleteUsersDryRunNeverCallsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	users := []UserActivity{{Name: "alice"}, {Name: "bob"}}
	results := DeleteUsers(client, users, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.DryRun || res.Success {
			t.Fatalf("expected dry-run non-success result, got %+v", res)
		}
	}
}

func TestDeleteUsersRealDelete(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/access/api/v2/users/") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if strings.HasSuffix(r.URL.Path, "bob") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	users := []UserActivity{{Name: "alice"}, {Name: "bob"}}
	results := DeleteUsers(client, users, false)
	if !results[0].Success {
		t.Fatalf("expected alice delete to succeed: %+v", results[0])
	}
	if results[1].Success || results[1].Error == "" {
		t.Fatalf("expected bob delete to fail: %+v", results[1])
	}
}
