package analyze

import (
	"testing"

	"jfrogpscli/internal/instance"
)

func TestRepoResourceFindings_LooksUpExplainText(t *testing.T) {
	rows := []Row{
		{Fields: map[string]string{"repo_name": "docker-local"}, Findings: []string{"empty", "huge"}},
		{Fields: map[string]string{"repo_name": "npm-local"}, Findings: nil},
	}
	defs := []FindingDef{
		{ID: "empty", Explain: "zero files"},
		{ID: "huge", Explain: "over threshold"},
	}

	got := repoResourceFindings(rows, defs)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(got), got)
	}
	for _, f := range got {
		if f.ResourceType != "repo" || f.ResourceName != "docker-local" {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
	if got[0].Detail != "zero files" || got[1].Detail != "over threshold" {
		t.Errorf("expected explain text looked up from defs, got %+v", got)
	}
}

func TestComputeResourceFindings_NilSnapshotReturnsNil(t *testing.T) {
	if got := computeResourceFindings(nil); got != nil {
		t.Fatalf("expected nil for a nil snapshot, got %+v", got)
	}
}

func TestComputeResourceFindings_PermissionTargets(t *testing.T) {
	snap := &instance.Snapshot{
		Permissions: []instance.PermissionTarget{
			{Name: "orphaned", Repos: []string{"docker-local"}}, // no principals
			{Name: "wide-open", Repos: []string{"**"}, UserActions: map[string][]string{"alice": {"READ"}}},
			{Name: "healthy", Repos: []string{"docker-local"}, UserActions: map[string][]string{"alice": {"READ"}}},
		},
	}
	got := computeResourceFindings(snap)

	byName := map[string][]string{}
	for _, f := range got {
		byName[f.ResourceName] = append(byName[f.ResourceName], f.Finding)
	}
	if !contains(byName["orphaned"], "no-principals") {
		t.Errorf("expected 'orphaned' to be flagged no-principals, got %+v", byName)
	}
	if !contains(byName["wide-open"], "wildcard-scope") {
		t.Errorf("expected 'wide-open' to be flagged wildcard-scope, got %+v", byName)
	}
	if len(byName["healthy"]) != 0 {
		t.Errorf("expected 'healthy' to have no findings, got %+v", byName["healthy"])
	}
}

func TestComputeResourceFindings_ProjectsFederationReplicationTokensWatches(t *testing.T) {
	snap := &instance.Snapshot{
		Projects: []instance.Project{
			{Key: "empty-proj"},
			{Key: "active-proj", RepoCount: 3},
		},
		FederatedRepos: []instance.FederatedRepo{
			{RepoKey: "single-member-repo", Members: []instance.FederationMember{{URL: "https://a", Enabled: true}}},
			{RepoKey: "healthy-fed-repo", Members: []instance.FederationMember{
				{URL: "https://a", Enabled: true}, {URL: "https://b", Enabled: true},
			}},
		},
		Replications: []instance.ReplicationConfig{
			{RepoKey: "disabled-repl", Enabled: false},
			{RepoKey: "active-repl", Enabled: true},
		},
		Platform: instance.PlatformConfig{
			ScopedTokens: []instance.ScopedToken{
				{TokenID: "t1", Subject: "u1", Expiry: 0},
				{TokenID: "t2", Subject: "u2", Expiry: 1999999999},
			},
		},
		Xray: instance.XrayConfig{
			Watches: []instance.XrayWatch{
				{Name: "inactive-watch", Active: false},
				{Name: "active-watch", Active: true},
			},
			Policies: []instance.XrayPolicy{
				{Name: "empty-policy"},
				{Name: "real-policy", Rules: []map[string]interface{}{{"name": "r1"}}},
			},
		},
	}

	got := computeResourceFindings(snap)
	want := map[string]string{
		"empty-proj":         "empty-project",
		"single-member-repo": "single-member",
		"disabled-repl":      "disabled",
		"u1":                 "non-expiring",
		"inactive-watch":     "inactive",
		"empty-policy":       "no-rules",
	}
	byName := map[string]string{}
	for _, f := range got {
		byName[f.ResourceName] = f.Finding
	}
	for name, finding := range want {
		if byName[name] != finding {
			t.Errorf("expected %q to be flagged %q, got %q (all: %+v)", name, finding, byName[name], got)
		}
	}
	for _, healthy := range []string{"active-proj", "healthy-fed-repo", "active-repl", "u2", "active-watch", "real-policy"} {
		if _, flagged := byName[healthy]; flagged {
			t.Errorf("expected %q to have no finding, got %q", healthy, byName[healthy])
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
