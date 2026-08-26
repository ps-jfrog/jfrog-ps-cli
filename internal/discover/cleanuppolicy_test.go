package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestListCleanupPolicies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/cleanup/packages/policies" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]CleanupPolicy{
			{Key: "p1", Enabled: true, SearchCriteria: SearchCriteria{Repos: []string{"docker-*"}}},
		})
	}))
	defer srv.Close()

	policies, err := ListCleanupPolicies(testClient(t, srv))
	if err != nil {
		t.Fatalf("ListCleanupPolicies returned error: %v", err)
	}
	if len(policies) != 1 || policies[0].Key != "p1" {
		t.Fatalf("unexpected policies: %+v", policies)
	}
}

func TestCleanupPolicy_Applies(t *testing.T) {
	cases := []struct {
		name   string
		policy CleanupPolicy
		repo   RepoSummary
		want   bool
	}{
		{
			name:   "matches repo wildcard",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{Repos: []string{"docker-*"}}},
			repo:   RepoSummary{RepoKey: "docker-local", PackageType: "Docker"},
			want:   true,
		},
		{
			name:   "no repo pattern match",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{Repos: []string{"npm-*"}}},
			repo:   RepoSummary{RepoKey: "docker-local", PackageType: "Docker"},
			want:   false,
		},
		{
			name:   "excluded repo wins",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{Repos: []string{"**"}, ExcludedRepos: []string{"docker-local"}}},
			repo:   RepoSummary{RepoKey: "docker-local", PackageType: "Docker"},
			want:   false,
		},
		{
			name:   "package type mismatch",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{Repos: []string{"**"}, PackageTypes: []string{"maven"}}},
			repo:   RepoSummary{RepoKey: "docker-local", PackageType: "Docker"},
			want:   false,
		},
		{
			name:   "package type case-insensitive match",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{Repos: []string{"**"}, PackageTypes: []string{"docker"}}},
			repo:   RepoSummary{RepoKey: "docker-local", PackageType: "Docker"},
			want:   true,
		},
		{
			name: "project scope excludes non-member repo",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{
				Repos: []string{"**"}, IncludeAllProjects: false, IncludedProjects: []string{"team-a"},
			}},
			repo: RepoSummary{RepoKey: "docker-local", PackageType: "Docker", ProjectKey: "team-b"},
			want: false,
		},
		{
			name: "include all projects bypasses project scope",
			policy: CleanupPolicy{SearchCriteria: SearchCriteria{
				Repos: []string{"**"}, IncludeAllProjects: true, IncludedProjects: []string{"team-a"},
			}},
			repo: RepoSummary{RepoKey: "docker-local", PackageType: "Docker", ProjectKey: "team-b"},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.Applies(tc.repo); got != tc.want {
				t.Errorf("Applies() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchPolicies_JoinsMultipleMatches(t *testing.T) {
	repo := RepoSummary{RepoKey: "docker-local", PackageType: "Docker"}
	policies := []CleanupPolicy{
		{Key: "p1", Enabled: true, CronExp: "0 0 2 * * ?", SearchCriteria: SearchCriteria{
			Repos: []string{"docker-*"}, CreatedBeforeInDays: intPtr(90),
		}},
		{Key: "p2", Enabled: false, CronExp: "0 0 3 * * ?", SearchCriteria: SearchCriteria{
			Repos: []string{"**"}, LastDownloadedBeforeInMonths: intPtr(6),
		}},
		{Key: "p3-no-match", SearchCriteria: SearchCriteria{Repos: []string{"npm-*"}}},
	}

	match := MatchPolicies(repo, policies)
	if match.Keys != "p1|p2" {
		t.Fatalf("expected Keys 'p1|p2', got %q", match.Keys)
	}
	if match.Enabled != "true|false" {
		t.Fatalf("expected Enabled 'true|false', got %q", match.Enabled)
	}
	if match.CreatedBeforeDays != "90|" {
		t.Fatalf("expected CreatedBeforeDays '90|', got %q", match.CreatedBeforeDays)
	}
	if match.LastDownloadedBeforeMonths != "|6" {
		t.Fatalf("expected LastDownloadedBeforeMonths '|6', got %q", match.LastDownloadedBeforeMonths)
	}
}

func TestMatchPolicies_NoMatchesYieldsEmptyStrings(t *testing.T) {
	repo := RepoSummary{RepoKey: "docker-local", PackageType: "Docker"}
	match := MatchPolicies(repo, []CleanupPolicy{{Key: "p1", SearchCriteria: SearchCriteria{Repos: []string{"npm-*"}}}})
	if match.Keys != "" || match.CronExps != "" {
		t.Fatalf("expected empty match, got %+v", match)
	}
}

func TestFormatIntPtr(t *testing.T) {
	if got := FormatIntPtr(nil); got != "" {
		t.Errorf("FormatIntPtr(nil) = %q, want empty", got)
	}
	if got := FormatIntPtr(intPtr(42)); got != "42" {
		t.Errorf("FormatIntPtr(42) = %q, want 42", got)
	}
}
