package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/instance"
)

func TestGetPermissionTargets_ListsAndFetchesDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v2/permissions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"permissions": []map[string]string{{"name": "p1"}, {"name": "p2"}},
			})
		case "/access/api/v2/permissions/p1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"resources": map[string]interface{}{
					"artifact": map[string]interface{}{
						"actions": map[string]interface{}{
							"users":  map[string][]string{"alice": {"READ"}},
							"groups": map[string][]string{"team-a": {"READ", "WRITE"}},
						},
						"targets": map[string]interface{}{"docker-local": map[string]interface{}{}},
					},
				},
			})
		case "/access/api/v2/permissions/p2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"resources": map[string]interface{}{
					"artifact": map[string]interface{}{
						"actions": map[string]interface{}{},
						"targets": map[string]interface{}{"npm-local": map[string]interface{}{}},
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	targets, truncated, warnings, err := GetPermissionTargets(testClient(t, srv), 0, 4)
	if err != nil {
		t.Fatalf("GetPermissionTargets returned error: %v", err)
	}
	if truncated || len(warnings) != 0 {
		t.Fatalf("expected no truncation/warnings, got truncated=%v warnings=%v", truncated, warnings)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(targets), targets)
	}

	var p1 *instance.PermissionTarget
	for i := range targets {
		if targets[i].Name == "p1" {
			p1 = &targets[i]
		}
	}
	if p1 == nil {
		t.Fatal("expected p1 in results")
	}
	if len(p1.Repos) != 1 || p1.Repos[0] != "docker-local" {
		t.Fatalf("unexpected p1 repos: %v", p1.Repos)
	}
	if got := p1.UserActions["alice"]; len(got) != 1 || got[0] != "READ" {
		t.Fatalf("unexpected p1 user actions: %v", p1.UserActions)
	}
	if got := p1.GroupActions["team-a"]; len(got) != 2 {
		t.Fatalf("unexpected p1 group actions: %v", p1.GroupActions)
	}
}

func TestGetPermissionTargets_CapTruncates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/access/api/v2/permissions" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"permissions": []map[string]string{{"name": "p1"}, {"name": "p2"}, {"name": "p3"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"resources": map[string]interface{}{}})
	}))
	defer srv.Close()

	targets, truncated, _, err := GetPermissionTargets(testClient(t, srv), 2, 4)
	if err != nil {
		t.Fatalf("GetPermissionTargets returned error: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets after cap, got %d", len(targets))
	}
}

func TestGetPermissionTargets_OneFailureDoesNotAbortOthers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/access/api/v2/permissions" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"permissions": []map[string]string{{"name": "ok"}, {"name": "broken"}},
			})
			return
		}
		if r.URL.Path == "/access/api/v2/permissions/broken" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"resources": map[string]interface{}{}})
	}))
	defer srv.Close()

	targets, _, warnings, err := GetPermissionTargets(testClient(t, srv), 0, 4)
	if err != nil {
		t.Fatalf("GetPermissionTargets returned error: %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "ok" {
		t.Fatalf("expected only the successful target, got %+v", targets)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for the broken target, got %v", warnings)
	}
}
