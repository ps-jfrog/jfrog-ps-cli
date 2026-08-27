package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
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

	targets, truncated, warnings, err := GetPermissionTargets(testClient(t, srv), 0, 4, "7.72.0", nil)
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

	targets, truncated, _, err := GetPermissionTargets(testClient(t, srv), 2, 4, "7.72.0", nil)
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

	targets, _, warnings, err := GetPermissionTargets(testClient(t, srv), 0, 4, "7.72.0", nil)
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

func TestGetPermissionTargets_FallsBackToLegacyOnUnexpected404(t *testing.T) {
	// Safety net: even when the version says v2 (7.72.0+) should exist, an
	// actual 404 (custom build, unusual deployment) still falls back to the
	// deprecated Artifactory Security API for both listing and detail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/api/v2/permissions":
			w.WriteHeader(http.StatusNotFound)
		case "/artifactory/api/security/permissions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "p1"}})
		case "/artifactory/api/security/permissions/p1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repositories": []string{"docker-local"},
				"principals": map[string]interface{}{
					"users":  map[string][]string{"alice": {"READ"}},
					"groups": map[string][]string{"team-a": {"READ", "WRITE"}},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	targets, truncated, warnings, err := GetPermissionTargets(testClient(t, srv), 0, 4, "7.72.0", nil)
	if err != nil {
		t.Fatalf("GetPermissionTargets returned error: %v", err)
	}
	if truncated || len(warnings) != 0 {
		t.Fatalf("expected no truncation/warnings, got truncated=%v warnings=%v", truncated, warnings)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d: %+v", len(targets), targets)
	}
	p1 := targets[0]
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

func TestGetPermissionTargets_LowVersionGoesStraightToLegacy(t *testing.T) {
	// Below 7.72.0 the Access API v2 permissions endpoint doesn't exist -
	// GetPermissionTargets must not even try it (unlike users/groups at
	// 7.49.3, permissions kept the legacy-only window open longer).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/api/v2/permissions":
			t.Fatalf("should not probe %s on an instance below 7.72.0", r.URL.Path)
		case "/artifactory/api/security/permissions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "p1"}})
		case "/artifactory/api/security/permissions/p1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repositories": []string{"docker-local"},
				"principals": map[string]interface{}{
					"users":  map[string][]string{"alice": {"READ"}},
					"groups": map[string][]string{"team-a": {"READ", "WRITE"}},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	targets, truncated, warnings, err := GetPermissionTargets(testClient(t, srv), 0, 4, "7.31.13", nil)
	if err != nil {
		t.Fatalf("GetPermissionTargets returned error: %v", err)
	}
	if truncated || len(warnings) != 0 {
		t.Fatalf("expected no truncation/warnings, got truncated=%v warnings=%v", truncated, warnings)
	}
	if len(targets) != 1 || targets[0].Name != "p1" {
		t.Fatalf("expected 1 target p1, got %+v", targets)
	}
}

func TestGetPermissionTargets_ReportsProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/access/api/v2/permissions" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"permissions": []map[string]string{{"name": "p1"}, {"name": "p2"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"resources": map[string]interface{}{}})
	}))
	defer srv.Close()

	var mu sync.Mutex
	var calls, lastDone, lastTotal int
	_, _, _, err := GetPermissionTargets(testClient(t, srv), 0, 4, "7.72.0", func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("GetPermissionTargets returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 progress calls, got %d", calls)
	}
	if lastDone != 2 || lastTotal != 2 {
		t.Fatalf("expected final progress 2/2, got %d/%d", lastDone, lastTotal)
	}
}
