package jfclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret-token", "", "", false)
	resp, err := c.Get("/artifactory/api/repositories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.OK() {
		t.Fatalf("expected 2xx, got %d", resp.StatusCode)
	}
	var out map[string]bool
	if err := resp.Unmarshal(&out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out["ok"] {
		t.Fatalf("expected ok=true, got %v", out)
	}
}

func TestBasicAuthFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			t.Fatalf("expected basic auth admin/secret, got ok=%v user=%q pass=%q", ok, user, pass)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "", "admin", "secret", false)
	resp, err := c.Get("/artifactory/api/repositories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.OK() {
		t.Fatalf("expected 2xx, got %d", resp.StatusCode)
	}
}

func TestPostJSONBody(t *testing.T) {
	WritesDisabled = false
	t.Cleanup(func() { WritesDisabled = true })

	type payload struct {
		Name string `json:"name"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected application/json content-type, got %q", ct)
		}
		var p payload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if p.Name != "my-repo" {
			t.Fatalf("expected name=my-repo, got %q", p.Name)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", "", false)
	resp, err := c.Post("/access/api/v2/permissions", payload{Name: "my-repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
}

func TestAQLPlainTextBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "text/plain" {
			t.Fatalf("expected text/plain content-type, got %q", ct)
		}
		if r.URL.Path != "/artifactory/api/search/aql" {
			t.Fatalf("expected AQL endpoint, got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", "", false)
	resp, err := c.AQL(`items.find({"repo":"my-repo"})`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.OK() {
		t.Fatalf("expected 2xx, got %d", resp.StatusCode)
	}
}

func TestErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"message":"not found"}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "", "", false)
	resp, err := c.Get("/artifactory/api/repositories/nope")
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if resp.OK() {
		t.Fatalf("expected non-2xx")
	}
	if err := resp.Error("get repo"); err == nil {
		t.Fatalf("expected Error() to return non-nil for 404")
	}
}
