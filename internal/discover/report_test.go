package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReport_SkipsAQLForEmptyRepos(t *testing.T) {
	aqlCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artifactory/api/storageinfo":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repositoriesSummaryList": []map[string]interface{}{
					{"repoKey": "empty-repo", "repoType": "LOCAL", "packageType": "Generic", "filesCount": 0, "usedSpaceInBytes": 0},
					{"repoKey": "full-repo", "repoType": "LOCAL", "packageType": "Maven", "filesCount": 5, "usedSpaceInBytes": 1024},
				},
			})
		case "/artifactory/api/search/aql":
			aqlCalls++
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	rows, warnings, err := Report(testClient(t, srv), ReportOptions{Concurrency: 2})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// full-repo runs 3 AQL queries (created/modified/downloaded); empty-repo runs none.
	if aqlCalls != 3 {
		t.Fatalf("expected 3 AQL calls (only for the non-empty repo), got %d", aqlCalls)
	}
	// Sorted by repo key: "empty-repo" < "full-repo".
	if rows[0].RepoKey != "empty-repo" || rows[1].RepoKey != "full-repo" {
		t.Fatalf("unexpected row order: %+v", rows)
	}
}

func TestReport_WithPoliciesMatchesEachRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artifactory/api/storageinfo":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"repositoriesSummaryList": []map[string]interface{}{
					{"repoKey": "docker-local", "repoType": "LOCAL", "packageType": "Docker", "filesCount": 0, "usedSpaceInBytes": 0},
				},
			})
		case "/artifactory/api/cleanup/packages/policies":
			_ = json.NewEncoder(w).Encode([]CleanupPolicy{
				{Key: "docker-policy", Enabled: true, SearchCriteria: SearchCriteria{Repos: []string{"docker-*"}}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	rows, _, err := Report(testClient(t, srv), ReportOptions{Concurrency: 1, WithPolicies: true})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].Policy.Keys != "docker-policy" {
		t.Fatalf("expected matched policy on the row, got %+v", rows)
	}
}

func TestReport_IncludeExcludeFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"repositoriesSummaryList": []map[string]interface{}{
				{"repoKey": "team-a-npm", "repoType": "LOCAL", "packageType": "Npm", "filesCount": 0},
				{"repoKey": "team-a-backup", "repoType": "LOCAL", "packageType": "Generic", "filesCount": 0},
				{"repoKey": "team-b-npm", "repoType": "LOCAL", "packageType": "Npm", "filesCount": 0},
			},
		})
	}))
	defer srv.Close()

	rows, _, err := Report(testClient(t, srv), ReportOptions{
		Concurrency: 1,
		Include:     []string{"team-a-*"},
		Exclude:     []string{"*-backup"},
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if len(rows) != 1 || rows[0].RepoKey != "team-a-npm" {
		t.Fatalf("expected only team-a-npm to survive include+exclude, got %+v", rows)
	}
}
