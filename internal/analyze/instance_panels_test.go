package analyze

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"jfrogpscli/internal/instance"
)

func testSnapshot() *instance.Snapshot {
	return &instance.Snapshot{
		System: instance.SystemInfo{
			ArtifactoryVersion: "7.165.0", ArtifactoryRevision: "865",
			XrayAvailable: true, XrayVersion: "3.152.6",
			Addons: []string{"docker"},
		},
		Security: instance.SecurityInfo{
			OIDCProviders: []instance.OIDCProvider{{Name: "github", ProviderType: "GitHub"}},
			SigningKeys:   []instance.SigningKey{{PairName: "k1", PairType: "GPG", Alias: "k1"}},
		},
		Users:        []instance.User{{Username: "alice", Realm: "internal"}},
		Groups:       []instance.Group{{Name: "team-a"}},
		Permissions:  []instance.PermissionTarget{{Name: "p1", Repos: []string{"docker-local"}}},
		Projects:     []instance.Project{{Key: "team-a", DisplayName: "Team A", RepoCount: 2}},
		Replications: []instance.ReplicationConfig{{RepoKey: "libs-release-local", Enabled: true}},
		FederatedRepos: []instance.FederatedRepo{
			{RepoKey: "fed-repo", Members: []instance.FederationMember{{URL: "https://x/artifactory/fed-repo", Enabled: true}}},
		},
		Warnings: []string{"one soft-failed call"},
	}
}

func TestRender_WithInstance_AddsTabsAndPanels(t *testing.T) {
	header := []string{"repo_name", "package_type", "repo_type", "files_count", "usage_gb"}
	rows := []Row{mkRow(map[string]string{"repo_name": "docker-local", "package_type": "Docker", "repo_type": "LOCAL", "files_count": "0", "usage_gb": "0"})}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	html, err := Render(header, rows, RenderOptions{
		SourceFile: "report.csv", Opts: DefaultOptions(), GeneratedAt: time.Now(), Instance: testSnapshot(),
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if m := regexp.MustCompile(`__[A-Z_]+__`).FindString(html); m != "" {
		t.Fatalf("found unreplaced template placeholder: %q", m)
	}

	for _, want := range []string{
		`data-report-tab="repos"`, `data-report-tab="overview"`, `data-report-tab="access"`,
		`data-report-tab="projects"`, `data-report-tab="fedrep"`,
		`data-report-panel="overview"`, `data-report-panel="access"`,
		`data-report-panel="projects"`, `data-report-panel="fedrep"`,
		`mountTable("access-users"`, `mountTable("access-groups"`, `mountTable("access-permissions"`,
		`mountTable("projects-table"`, `mountTable("federation-table"`, `mountTable("replication-table"`,
		"7.165.0", "3.152.6", "github", "alice", "team-a", "libs-release-local", "fed-repo",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected output to contain %q, it did not", want)
		}
	}

	if strings.Count(html, "</script>") < 8 {
		t.Fatalf("expected at least 8 </script> tags (shared helpers + main + 6 mounted tables), got %d", strings.Count(html, "</script>"))
	}
}

func TestRender_WithoutInstance_NoTabsPanelsOrLeaks(t *testing.T) {
	header := []string{"repo_name", "files_count"}
	rows := []Row{mkRow(map[string]string{"repo_name": "a", "files_count": "0"})}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	html, err := Render(header, rows, RenderOptions{SourceFile: "x.csv", Opts: DefaultOptions(), GeneratedAt: time.Now()})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if strings.Contains(html, "data-report-tab=\"overview\"") {
		t.Fatal("expected no Overview tab without instance data")
	}
	if strings.Count(html, "</script>") != 2 {
		t.Fatalf("expected exactly 2 </script> tags (shared helpers + main) without instance data, got %d", strings.Count(html, "</script>"))
	}
}

func TestRenderOverviewPanel_HighlightsSigningKeysWhenPresent(t *testing.T) {
	snap := testSnapshot()
	panel, err := renderOverviewPanel(snap)
	if err != nil {
		t.Fatalf("renderOverviewPanel returned error: %v", err)
	}
	if !strings.Contains(panel, `class="callout warn"`) {
		t.Fatal("expected a warn callout when signing keys are configured")
	}
	if !strings.Contains(panel, "k1") {
		t.Fatal("expected signing key pair name in output")
	}
}

func TestRenderOverviewPanel_NoSigningKeysIsOK(t *testing.T) {
	snap := testSnapshot()
	snap.Security.SigningKeys = nil
	panel, err := renderOverviewPanel(snap)
	if err != nil {
		t.Fatalf("renderOverviewPanel returned error: %v", err)
	}
	if !strings.Contains(panel, `class="callout ok"`) {
		t.Fatal("expected an ok callout when no signing keys are configured")
	}
}

func TestRenderOverviewPanel_NoAuthConfigured(t *testing.T) {
	snap := testSnapshot()
	snap.Security.OIDCProviders = nil
	panel, err := renderOverviewPanel(snap)
	if err != nil {
		t.Fatalf("renderOverviewPanel returned error: %v", err)
	}
	if !strings.Contains(panel, "No SAML/OIDC/LDAP configuration detected") {
		t.Fatal("expected a no-auth-configured notice")
	}
}
