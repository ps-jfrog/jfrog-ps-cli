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
		URL:  "https://psazuse.jfrog.io",
		Name: "psazuse",
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
		Projects:     []instance.Project{{Key: "team-a", DisplayName: "Team A", RepoCount: 2, Repos: []string{"r1", "r2"}}},
		Replications: []instance.ReplicationConfig{{RepoKey: "libs-release-local", Enabled: true}},
		FederatedRepos: []instance.FederatedRepo{
			{RepoKey: "fed-repo", Members: []instance.FederationMember{{URL: "https://x/artifactory/fed-repo", Enabled: true}}},
		},
		Platform: instance.PlatformConfig{
			GlobalRoles:    []instance.GlobalRole{{Name: "Global Developers", Type: "CUSTOM_GLOBAL"}},
			ScopedTokens:   []instance.ScopedToken{{TokenID: "t1", Subject: "u1"}},
			PasswordPolicy: &instance.PasswordExpirationPolicy{Enabled: true, PasswordMaxAge: 180},
			Webhooks:       []instance.Webhook{{Key: "wh1", Domain: "artifact"}},
		},
		Xray: instance.XrayConfig{
			Policies: []instance.XrayPolicy{{Name: "sec1", Type: "security"}},
			Watches:  []instance.XrayWatch{{Name: "w1", Active: true}},
			Settings: &instance.XraySettings{MaxDiskDataUsage: 80, DBSyncUpdatesTime: "12:01"},
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
		`data-report-tab="repos"`, `data-report-tab="all-findings"`, `data-report-tab="relationships"`, `data-report-tab="overview"`, `data-report-tab="access"`,
		`data-report-tab="projects"`, `data-report-tab="fedrep"`, `data-report-tab="platform"`, `data-report-tab="xray"`,
		`data-report-panel="all-findings"`, `data-report-panel="relationships"`, `data-report-panel="overview"`, `data-report-panel="access"`,
		`data-report-panel="projects"`, `data-report-panel="fedrep"`, `data-report-panel="platform"`, `data-report-panel="xray"`,
		`mountTable("access-users"`, `mountTable("access-groups"`, `mountTable("access-permissions"`,
		`mountTable("projects-table"`, `mountTable("federation-table"`, `mountTable("replication-table"`,
		`mountTable("platform-roles"`, `mountTable("platform-tokens"`, `mountTable("platform-webhooks"`,
		`mountTable("platform-signing-keys"`, `mountTable("xray-policies"`, `mountTable("xray-watches"`, `mountTable("all-findings-table"`,
		`id="rel-project-search"`, `id="rel-select"`, `id="rel-summary"`, `id="rel-diagram"`, `mountTable("rel-table"`, "REL_DATA",
		"<h2>Environment counts</h2>", `"repos":["r1","r2"]`,
		"7.165.0", "3.152.6", "github", "alice", "team-a", "libs-release-local", "fed-repo",
		`<h2>Instance</h2>`, `<strong>psazuse</strong>`,
		`class="header-instance" href="https://psazuse.jfrog.io"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected output to contain %q, it did not", want)
		}
	}

	if strings.Count(html, "</script>") < 10 {
		t.Fatalf("expected at least 10 </script> tags (shared helpers + main + mounted tables + relationships), got %d", strings.Count(html, "</script>"))
	}

	if !strings.Contains(html, `<button class="report-tab active" data-report-tab="overview">`) {
		t.Error("expected Overview to be the default active tab when instance data is present")
	}
	if strings.Contains(html, `<button class="report-tab active" data-report-tab="repos">`) {
		t.Error("expected Repos to NOT be the default active tab when instance data is present")
	}
	if !strings.Contains(html, `<section class="report-panel active" data-report-panel="overview">`) {
		t.Error("expected the Overview panel to have the active class")
	}
	if strings.Contains(html, `<section class="report-panel active" data-report-panel="repos">`) {
		t.Error("expected the Repos panel to NOT have the active class when instance data is present")
	}

	// Overview, Repos, Projects, Access, Federation & Replication, Platform
	// Config, Xray, All Findings, Relationships, Wave Planning, Migration
	// Workflow, in that order.
	order := []string{
		`data-report-tab="overview"`, `data-report-tab="repos"`,
		`data-report-tab="projects"`, `data-report-tab="access"`, `data-report-tab="fedrep"`,
		`data-report-tab="platform"`, `data-report-tab="xray"`, `data-report-tab="all-findings"`,
		`data-report-tab="relationships"`, `data-report-tab="waves"`, `data-report-tab="migration"`,
	}
	last := -1
	for _, tab := range order {
		idx := strings.Index(html, tab)
		if idx == -1 {
			t.Fatalf("tab %q not found", tab)
		}
		if idx < last {
			t.Fatalf("expected tab order Overview, Repos, Projects, Access, Federation & Replication, Platform Config, Xray, All Findings, Relationships, Wave Planning, Migration Workflow - %q appeared out of order", tab)
		}
		last = idx
	}
}

func TestRender_AllFindingsTab_CombinesRepoAndResourceFindings(t *testing.T) {
	header := []string{"repo_name", "files_count"}
	rows := []Row{mkRow(map[string]string{"repo_name": "docker-local", "files_count": "0"})}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	snap := testSnapshot()
	snap.Permissions = []instance.PermissionTarget{{Name: "orphaned-pt"}} // no principals -> a finding

	html, err := Render(header, rows, RenderOptions{
		SourceFile: "report.csv", Opts: DefaultOptions(), GeneratedAt: time.Now(), Instance: snap,
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(html, `"resource_type":"repo"`) {
		t.Error("expected a repo entry in the all-findings data")
	}
	if !strings.Contains(html, "orphaned-pt") || !strings.Contains(html, "no-principals") {
		t.Error("expected the orphaned permission target finding in the all-findings data")
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
	if strings.Count(html, "</script>") != 4 {
		t.Fatalf("expected exactly 4 </script> tags (shared helpers + wave planning + migration workflow + main) without instance data, got %d", strings.Count(html, "</script>"))
	}
	if !strings.Contains(html, `data-report-tab="waves"`) {
		t.Fatal("expected the Wave Planning tab even without instance data")
	}
	if !strings.Contains(html, `data-report-tab="migration"`) {
		t.Fatal("expected the Migration Workflow tab even without instance data")
	}
	if !strings.Contains(html, `<section class="report-panel active" data-report-panel="repos">`) {
		t.Error("expected Repos to be the default active panel without instance data")
	}
	if strings.Contains(html, `class="header-instance"`) {
		t.Error("expected no header instance link without instance data")
	}
}

func TestRenderRelationshipsPanel_EmbedsProjectsAndRepos(t *testing.T) {
	rows := []Row{
		mkRow(map[string]string{"repo_name": "docker-local", "project_key": "team-a", "package_type": "Docker", "repo_type": "LOCAL"}),
		mkRow(map[string]string{"repo_name": "generic-local", "project_key": "", "package_type": "Generic", "repo_type": "LOCAL"}),
	}
	snap := testSnapshot() // has Project{Key: "team-a", DisplayName: "Team A", RepoCount: 2}

	panel, err := renderRelationshipsPanel(rows, snap)
	if err != nil {
		t.Fatalf("renderRelationshipsPanel returned error: %v", err)
	}

	for _, want := range []string{
		`id="rel-project-search"`,
		`id="rel-select"`,
		`<option value="global">Global (repos with no project)</option>`,
		`<option value="project:team-a">Team A (team-a)</option>`,
		`id="rel-summary"`, `id="rel-diagram"`, `id="rel-table"`,
		"const REL_DATA", `"key":"team-a"`, `"name":"docker-local"`, `"project_key":"team-a"`,
		"function relConnections", "function updateRelationships", "function filterProjectOptions",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("expected relationships panel to contain %q", want)
		}
	}

	if strings.Contains(panel, `value="repo:`) {
		t.Error("expected no per-repo <option> - relationships is Global/Project only, not repo-selectable")
	}
	if !strings.Contains(panel, `optgroup label="Projects"`) {
		t.Error("expected a Projects optgroup for filterProjectOptions to target")
	}
}

func TestRenderOverviewPanel_HighlightsSigningKeysWhenPresent(t *testing.T) {
	snap := testSnapshot()
	panel, err := renderOverviewPanel(snap, 42)
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
	panel, err := renderOverviewPanel(snap, 42)
	if err != nil {
		t.Fatalf("renderOverviewPanel returned error: %v", err)
	}
	if !strings.Contains(panel, `class="callout ok"`) {
		t.Fatal("expected an ok callout when no signing keys are configured")
	}
}

func TestRenderOverviewPanel_ShowsEnvironmentCounts(t *testing.T) {
	snap := testSnapshot() // 1 project, 1 user, 1 group, 1 permission target, 1 replication, 1 federated repo, 1 global role
	panel, err := renderOverviewPanel(snap, 42)
	if err != nil {
		t.Fatalf("renderOverviewPanel returned error: %v", err)
	}
	if !strings.Contains(panel, "<h2>Environment counts</h2>") {
		t.Fatal("expected an Environment counts section")
	}
	for _, want := range []string{
		`<strong>42</strong><span>repositories</span>`,
		`<strong>1</strong><span>projects</span>`,
		`<strong>1</strong><span>users</span>`,
		`<strong>1</strong><span>groups</span>`,
		`<strong>1</strong><span>permission targets</span>`,
		`<strong>1</strong><span>federated repos</span>`,
		`<strong>1</strong><span>replication configs</span>`,
		`<strong>1</strong><span>global roles</span>`,
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("expected overview panel to contain %q", want)
		}
	}
}

func TestRenderProjectsPanel_IncludesRepoListForDrawer(t *testing.T) {
	panel, err := renderProjectsPanel(testSnapshot()) // Project{Key: "team-a", Repos: []string{"r1", "r2"}}
	if err != nil {
		t.Fatalf("renderProjectsPanel returned error: %v", err)
	}
	// The "repos" column isn't in the visible table's columns list, but it's
	// still part of each row's embedded JSON object, so mountTable's
	// click-to-expand drawer (which dumps every field via Object.entries)
	// shows a project's repos without a dedicated column for it.
	if !strings.Contains(panel, `"repos":["r1","r2"]`) {
		t.Error(`expected projects-table data to embed "repos":["r1","r2"] for the row-click drawer`)
	}
}

func TestRenderPlatformConfigPanel(t *testing.T) {
	panel, err := renderPlatformConfigPanel(testSnapshot())
	if err != nil {
		t.Fatalf("renderPlatformConfigPanel returned error: %v", err)
	}
	for _, want := range []string{
		"Global Developers", "180", "wh1", "artifact",
		"<h2>Signing keys</h2>", `mountTable("platform-signing-keys"`, "k1", "GPG",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("expected platform config panel to contain %q", want)
		}
	}
}

func TestRenderXrayPanel_ShowsDataWhenAvailable(t *testing.T) {
	panel, err := renderXrayPanel(testSnapshot())
	if err != nil {
		t.Fatalf("renderXrayPanel returned error: %v", err)
	}
	for _, want := range []string{"sec1", "w1", "12:01"} {
		if !strings.Contains(panel, want) {
			t.Errorf("expected xray panel to contain %q", want)
		}
	}
}

func TestRenderXrayPanel_UnavailableShowsNotice(t *testing.T) {
	snap := testSnapshot()
	snap.System.XrayAvailable = false
	panel, err := renderXrayPanel(snap)
	if err != nil {
		t.Fatalf("renderXrayPanel returned error: %v", err)
	}
	if !strings.Contains(panel, "not available on this instance") {
		t.Fatalf("expected an Xray-unavailable notice, got: %s", panel)
	}
	if strings.Contains(panel, "mountTable(") {
		t.Fatal("expected no tables mounted when Xray is unavailable")
	}
}

func TestRenderOverviewPanel_NoAuthConfigured(t *testing.T) {
	snap := testSnapshot()
	snap.Security.OIDCProviders = nil
	panel, err := renderOverviewPanel(snap, 42)
	if err != nil {
		t.Fatalf("renderOverviewPanel returned error: %v", err)
	}
	if !strings.Contains(panel, "No SAML/OIDC/LDAP configuration detected") {
		t.Fatal("expected a no-auth-configured notice")
	}
}
