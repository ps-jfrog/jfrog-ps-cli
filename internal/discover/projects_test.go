package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProjects_FillsRepoCountFromRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"display_name": "Team A", "project_key": "team-a", "storage_quota_bytes": -1},
			{"display_name": "Team B", "project_key": "team-b", "storage_quota_bytes": 1024},
		})
	}))
	defer srv.Close()

	repos := []RepoSummary{
		{RepoKey: "r1", ProjectKey: "team-a"},
		{RepoKey: "r2", ProjectKey: "team-a"},
		{RepoKey: "r3", ProjectKey: "default"},
	}
	projects, err := GetProjects(testClient(t, srv), repos)
	if err != nil {
		t.Fatalf("GetProjects returned error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	byKey := map[string]int{}
	for _, p := range projects {
		byKey[p.Key] = p.RepoCount
	}
	if byKey["team-a"] != 2 {
		t.Errorf("expected team-a RepoCount 2, got %d", byKey["team-a"])
	}
	if byKey["team-b"] != 0 {
		t.Errorf("expected team-b RepoCount 0, got %d", byKey["team-b"])
	}
}
