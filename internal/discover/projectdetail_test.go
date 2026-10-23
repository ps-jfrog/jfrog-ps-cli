package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"jfrogpscli/internal/instance"
)

func TestEnrichProjectDetail_PopulatesRolesAndEnvironments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v1/projects/team-a/roles":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"name": "Developer", "type": "PREDEFINED"}})
		case "/access/api/v1/projects/team-a/environments":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "DEV"}})
		default:
			// members/users, members/groups - simulate the common 404.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	projects := []instance.Project{{Key: "team-a"}}
	EnrichProjectDetail(testClient(t, srv), projects, 2, nil)

	if len(projects[0].Roles) != 1 || projects[0].Roles[0].Name != "Developer" {
		t.Errorf("unexpected Roles: %+v", projects[0].Roles)
	}
	if len(projects[0].Environments) != 1 || projects[0].Environments[0] != "DEV" {
		t.Errorf("unexpected Environments: %+v", projects[0].Environments)
	}
	if len(projects[0].Users) != 0 || len(projects[0].Groups) != 0 {
		t.Errorf("expected empty Users/Groups on 404, got users=%+v groups=%+v", projects[0].Users, projects[0].Groups)
	}
}

func TestEnrichProjectDetail_SkipsProjectsWithoutKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no HTTP call should be made for a project with an empty key, got %s", r.URL.Path)
	}))
	defer srv.Close()

	projects := []instance.Project{{Key: ""}}
	EnrichProjectDetail(testClient(t, srv), projects, 2, nil)
}

func TestEnrichProjectDetail_MembersWhenPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v1/projects/team-b/users":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"members": []map[string]interface{}{{"name": "alice", "roles": []string{"Developer"}}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	projects := []instance.Project{{Key: "team-b"}}
	EnrichProjectDetail(testClient(t, srv), projects, 2, nil)

	if len(projects[0].Users) != 1 || projects[0].Users[0].Name != "alice" {
		t.Errorf("unexpected Users: %+v", projects[0].Users)
	}
}

func TestEnrichProjectDetail_ReportsProgressIncludingEmptyKeyProjects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// An empty-key project makes no HTTP calls but must still count toward
	// the progress total, so a caller's bar reaches 100%.
	projects := []instance.Project{{Key: "team-a"}, {Key: ""}, {Key: "team-b"}}
	var mu sync.Mutex
	var calls, lastDone, lastTotal int
	EnrichProjectDetail(testClient(t, srv), projects, 2, func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		lastDone, lastTotal = done, total
	})
	if calls != 3 {
		t.Fatalf("expected 3 progress calls, got %d", calls)
	}
	if lastDone != 3 || lastTotal != 3 {
		t.Fatalf("expected final progress 3/3, got %d/%d", lastDone, lastTotal)
	}
}
