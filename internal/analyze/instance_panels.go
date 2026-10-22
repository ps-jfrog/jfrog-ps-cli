package analyze

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"jfrogpscli/internal/instance"
)

// tableColumn describes one column of a generic client-side searchable
// table (see mountTable in template.go's JS).
type tableColumn struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Numeric bool   `json:"numeric,omitempty"`
}

// renderInstanceTabs builds the "Projects"/"Access"/"Federation &
// Replication"/"Platform Config"/"Xray"/"Curation" nav buttons and panels
// from an instance snapshot, in that order. Overview, Repos, All Findings,
// and Relationships are positioned around these by Render itself (Overview
// needs the repo count, which isn't part of an instance.Snapshot). Returns
// ("", "", nil) when snap is nil, so Render's single-panel behavior is
// unchanged without one.
func renderInstanceTabs(snap *instance.Snapshot) (navHTML, panelsHTML string, err error) {
	if snap == nil {
		return "", "", nil
	}

	sections := []struct {
		id    string
		label string
		build func(*instance.Snapshot) (string, error)
	}{
		{"projects", "Projects", renderProjectsPanel},
		{"access", "Access", renderAccessPanel},
		{"fedrep", "Federation & Replication", renderFederationReplicationPanel},
		{"platform", "Platform Config", renderPlatformConfigPanel},
		{"apptrust", "AppTrust", renderAppTrustPanel},
		{"xray", "Xray", renderXrayPanel},
		{"curation", "Curation", renderCurationPanel},
	}

	var nav, panels strings.Builder
	for _, s := range sections {
		content, err := s.build(snap)
		if err != nil {
			return "", "", fmt.Errorf("render %s panel: %w", s.id, err)
		}
		fmt.Fprintf(&nav, `<button class="report-tab" data-report-tab="%s">%s</button>`, s.id, htmlEscape(s.label))
		fmt.Fprintf(&panels, "\n  <section class=\"report-panel\" data-report-panel=\"%s\">\n%s\n  </section>\n", s.id, content)
	}
	return nav.String(), panels.String(), nil
}

func renderOverviewPanel(snap *instance.Snapshot, repoCount int) (string, error) {
	sys := snap.System
	var b strings.Builder

	if snap.Name != "" || snap.URL != "" {
		b.WriteString(`<h2>Instance</h2>`)
		b.WriteString(`<p class="instance-line">`)
		if snap.Name != "" {
			fmt.Fprintf(&b, `<strong>%s</strong>`, htmlEscape(snap.Name))
		}
		if snap.URL != "" {
			if snap.Name != "" {
				b.WriteString(" &mdash; ")
			}
			fmt.Fprintf(&b, `<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`,
				htmlAttr(snap.URL), htmlEscape(snap.URL))
		}
		b.WriteString(`</p>`)
	}

	b.WriteString(`<h2>Component versions</h2><section class="stats">`)
	fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>Artifactory (rev %s)</span></div>`,
		htmlEscape(orDash(sys.ArtifactoryVersion)), htmlEscape(orDash(sys.ArtifactoryRevision)))
	b.WriteString(componentStatCard("Xray", sys.XrayAvailable, sys.XrayVersion))
	b.WriteString(componentStatCard("Access", sys.AccessAvailable, sys.AccessVersion))
	b.WriteString(componentStatCard("Distribution", sys.DistributionAvailable, sys.DistributionVersion))
	b.WriteString("</section>")

	b.WriteString(`<h2>Storage</h2><section class="stats">`)
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--purple)"><strong>%s</strong><span>artifacts size</span></div>`, htmlEscape(orDash(sys.ArtifactsSize)))
	fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>artifacts count</span></div>`, htmlEscape(orDash(sys.ArtifactsCount)))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--teal)"><strong>%s</strong><span>binaries size (deduped)</span></div>`, htmlEscape(orDash(sys.BinariesSize)))
	fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>optimization</span></div>`, htmlEscape(orDash(sys.Optimization)))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--amber)"><strong>%s</strong><span>storage type</span></div>`, htmlEscape(orDash(sys.StorageType)))
	b.WriteString("</section>")

	b.WriteString(`<h2>Environment counts</h2><section class="stats">`)
	fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>projects</span></div>`, len(snap.Projects))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--purple)"><strong>%d</strong><span>repositories</span></div>`, repoCount)
	fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>users</span></div>`, len(snap.Users))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--teal)"><strong>%d</strong><span>groups</span></div>`, len(snap.Groups))
	fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>permission targets</span></div>`, len(snap.Permissions))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--amber)"><strong>%d</strong><span>federated repos</span></div>`, len(snap.FederatedRepos))
	fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>replication configs</span></div>`, len(snap.Replications))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--slate)"><strong>%d</strong><span>global roles</span></div>`, len(snap.Platform.GlobalRoles))
	totalAppTrust := len(snap.AppTrustPolicies) + len(snap.AppTrustApplications) +
		len(snap.AppTrustRules) + len(snap.AppTrustTemplates) +
		len(snap.AppTrustStages) + len(snap.AppTrustEvidence) + len(snap.AppTrustReleaseBundles)
	if totalAppTrust > 0 {
		fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>apptrust applications</span></div>`, len(snap.AppTrustApplications))
		fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--purple)"><strong>%d</strong><span>apptrust policies</span></div>`, len(snap.AppTrustPolicies))
		fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>apptrust rules</span></div>`, len(snap.AppTrustRules))
		fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--teal)"><strong>%d</strong><span>release bundles</span></div>`, len(snap.AppTrustReleaseBundles))
	}
	b.WriteString("</section>")

	if len(sys.Addons) > 0 {
		fmt.Fprintf(&b, `<p><strong>Licensed add-ons (%d):</strong> %s</p>`, len(sys.Addons), htmlEscape(strings.Join(sys.Addons, ", ")))
	}

	b.WriteString("<h2>Authentication</h2>")
	var authLines []string
	if snap.Security.SAMLConfigured {
		authLines = append(authLines, "SAML SSO is configured.")
	}
	if snap.Security.LDAPConfigured {
		authLines = append(authLines, "LDAP is configured.")
	}
	for _, p := range snap.Security.OIDCProviders {
		authLines = append(authLines, fmt.Sprintf("OIDC provider %q (%s) - issuer %s", p.Name, p.ProviderType, p.IssuerURL))
	}
	if len(authLines) == 0 {
		b.WriteString(`<div class="callout info"><strong>No SAML/OIDC/LDAP configuration detected.</strong> Authentication is likely internal-realm (username/password) only.</div>`)
	} else {
		fmt.Fprintf(&b, `<div class="callout info"><strong>Authentication methods configured:</strong><ul>%s</ul></div>`, liList(authLines))
	}

	b.WriteString("<h2>Repository signing keys</h2>")
	if len(snap.Security.SigningKeys) > 0 {
		var keyLines []string
		for _, k := range snap.Security.SigningKeys {
			keyLines = append(keyLines, fmt.Sprintf("%s (%s) - alias %s", k.PairName, k.PairType, k.Alias))
		}
		fmt.Fprintf(&b, `<div class="callout warn"><strong>&#9888; %d signing key(s) configured.</strong> Used to sign repository metadata/packages (e.g. Debian, RPM, Alpine, Opkg). Key material itself is never captured in this report - only pair name, type, and alias.<ul>%s</ul></div>`,
			len(snap.Security.SigningKeys), liList(keyLines))
	} else {
		b.WriteString(`<div class="callout ok"><strong>No repository signing keys configured.</strong></div>`)
	}

	b.WriteString("<h2>Anonymous access</h2>")
	if repos := anonymousAccessRepos(snap.Permissions); len(repos) > 0 {
		fmt.Fprintf(&b, `<div class="callout warn"><strong>&#9888; The built-in "anonymous" user has a permission-target grant scoped to %d repo pattern(s).</strong> There's no dedicated "is anonymous access enabled" API - Artifactory implements it as grants to this built-in user, so this is derived from the same permission-target data as the Access tab's Repo Access table. Confirm this is intentional.<ul>%s</ul></div>`,
			len(repos), liList(repos))
	} else {
		b.WriteString(`<div class="callout ok"><strong>No anonymous access grants detected</strong> - no permission target grants the "anonymous" user any action.</div>`)
	}

	if len(snap.Warnings) > 0 {
		b.WriteString("<h2>Collection warnings</h2>")
		fmt.Fprintf(&b, `<div class="callout warn"><ul>%s</ul></div>`, liList(snap.Warnings))
	}

	return b.String(), nil
}

// anonymousPrincipal is Artifactory's built-in username for unauthenticated
// requests. There's no dedicated "is anonymous access enabled" REST
// endpoint - Artifactory implements the "Allow Anonymous Access" toggle as
// permission-target grants to this built-in user, so that's what gets
// scanned for here instead of a global setting.
const anonymousPrincipal = "anonymous"

// anonymousGrant returns the actions (if any) a permission target grants to
// the anonymous user.
func anonymousGrant(p instance.PermissionTarget) []string {
	for user, actions := range p.UserActions {
		if strings.EqualFold(user, anonymousPrincipal) {
			return actions
		}
	}
	return nil
}

// anonymousAccessRepos returns the deduped, sorted set of repo patterns
// (as scoped on their permission targets - a wildcard like "**" is
// returned as-is, not expanded against a repo list, since the repo list
// itself isn't part of an instance.Snapshot) that grant the anonymous user
// any action.
func anonymousAccessRepos(perms []instance.PermissionTarget) []string {
	seen := map[string]bool{}
	for _, p := range perms {
		if len(anonymousGrant(p)) == 0 {
			continue
		}
		for _, r := range p.Repos {
			seen[r] = true
		}
	}
	repos := make([]string, 0, len(seen))
	for r := range seen {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	return repos
}

func renderAccessPanel(snap *instance.Snapshot) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "<h2>Users (%d)</h2>", len(snap.Users))
	if snap.UsersTruncated {
		b.WriteString(`<div class="callout warn">User list truncated - increase --max-users on 'discover instance' for the full list.</div>`)
	}
	usersHTML, err := renderGenericTable("access-users", snap.Users, []tableColumn{
		{Key: "username", Label: "Username"}, {Key: "realm", Label: "Realm"}, {Key: "status", Label: "Status"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(usersHTML)

	fmt.Fprintf(&b, "<h2>Groups (%d)</h2>", len(snap.Groups))
	if snap.GroupsTruncated {
		b.WriteString(`<div class="callout warn">Group list truncated - increase --max-groups on 'discover instance' for the full list.</div>`)
	}
	groupsHTML, err := renderGenericTable("access-groups", snap.Groups, []tableColumn{
		{Key: "name", Label: "Group"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(groupsHTML)

	fmt.Fprintf(&b, "<h2>Permission targets (%d)</h2>", len(snap.Permissions))
	b.WriteString("<p>Repo scope and user/group grants per permission target - click a row for full detail.</p>")
	if snap.PermissionsTruncated {
		b.WriteString(`<div class="callout warn">Permission target list truncated - increase --max-permissions on 'discover instance' for the full list.</div>`)
	}
	permsHTML, err := renderGenericTable("access-permissions", snap.Permissions, []tableColumn{
		{Key: "name", Label: "Permission Target"}, {Key: "repos", Label: "Repos"},
		{Key: "user_actions", Label: "User Grants"}, {Key: "group_actions", Label: "Group Grants"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(permsHTML)

	grants := repoAccessRows(snap.Permissions)
	fmt.Fprintf(&b, "<h2>Repo access by user/group (%d)</h2>", len(grants))
	b.WriteString(`<p>One row per principal &times; repo &times; permission target, joined from the table above - search for a username (or "anonymous") to see every repo they can reach and what they can do there. Group rows show the group's own grant, not its resolved members - group membership isn't collected.</p>`)
	grantsHTML, err := renderGenericTable("access-repo-grants", grants, []tableColumn{
		{Key: "principal", Label: "Principal"}, {Key: "principal_type", Label: "Type"}, {Key: "repo", Label: "Repo"},
		{Key: "actions", Label: "Actions"}, {Key: "via_permission_target", Label: "Via Permission Target"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(grantsHTML)

	fmt.Fprintf(&b, "<h2>Scoped tokens (grouped by account, %d)</h2>", len(snap.Platform.ScopedTokens))
	tokensHTML, err := renderGenericTable("access-tokens", scopedTokenRows(snap.Platform.ScopedTokens), []tableColumn{
		{Key: "subject", Label: "Account"}, {Key: "token_id", Label: "Token ID"}, {Key: "scope", Label: "Scope"},
		{Key: "issued", Label: "Issued"}, {Key: "expires", Label: "Expires"}, {Key: "refreshable", Label: "Refreshable"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(tokensHTML)

	return b.String(), nil
}

func renderProjectsPanel(snap *instance.Snapshot) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, `<h2>Projects (%d)</h2><p>Repos not assigned to any project show project_key %q on the Repos tab. Click a project row for detailed attributes, roles, stages, and repository assignments.</p>`, len(snap.Projects), "default")

	// Build summary view for each project
	type roleDetail struct {
		Name         string   `json:"name"`
		Type         string   `json:"type"`
		Environments []string `json:"environments,omitempty"`
		Actions      []string `json:"actions,omitempty"`
	}

	type memberDetail struct {
		Name  string   `json:"name"`
		Roles []string `json:"roles,omitempty"`
	}

	type projectSummary struct {
		Key              string          `json:"key"`
		DisplayName      string          `json:"display_name"`
		Description      string          `json:"description,omitempty"`
		RepoCount        int             `json:"repo_count"`
		StorageQuotaGB   float64         `json:"storage_quota_gb"`
		RoleCount        int             `json:"role_count"`
		EnvironmentCount int             `json:"environment_count"`
		UserCount        int             `json:"user_count"`
		GroupCount       int             `json:"group_count"`
		Repos            []string        `json:"repos,omitempty"`
		Environments     []string        `json:"environments,omitempty"`
		Roles            []roleDetail    `json:"roles,omitempty"`
		Users            []memberDetail  `json:"users,omitempty"`
		Groups           []memberDetail  `json:"groups,omitempty"`
	}

	summaries := make([]projectSummary, len(snap.Projects))
	for i, p := range snap.Projects {
		summaries[i] = projectSummary{
			Key:              p.Key,
			DisplayName:      p.DisplayName,
			Description:      p.Description,
			RepoCount:        p.RepoCount,
			StorageQuotaGB:   float64(p.StorageQuotaBytes) / (1024 * 1024 * 1024),
			RoleCount:        len(p.Roles),
			EnvironmentCount: len(p.Environments),
			UserCount:        len(p.Users),
			GroupCount:       len(p.Groups),
			Repos:            p.Repos,
			Environments:     p.Environments,
		}
		for _, r := range p.Roles {
			summaries[i].Roles = append(summaries[i].Roles, roleDetail{
				Name:         r.Name,
				Type:         r.Type,
				Environments: r.Environments,
				Actions:      r.Actions,
			})
		}
		for _, u := range p.Users {
			summaries[i].Users = append(summaries[i].Users, memberDetail{
				Name:  u.Name,
				Roles: u.Roles,
			})
		}
		for _, g := range p.Groups {
			summaries[i].Groups = append(summaries[i].Groups, memberDetail{
				Name:  g.Name,
				Roles: g.Roles,
			})
		}
	}

	jsonData, err := jsonForHTML(summaries)
	if err != nil {
		return "", fmt.Errorf("encoding projects: %w", err)
	}

	fmt.Fprintf(&b, `
<div id="projects-container"></div>
<script>
const PROJECTS_DATA = %s;

function renderProjectsPanel() {
  const container = document.getElementById("projects-container");
  if (!container) return;

  let html = '<div class="gt-toolbar"><input type="search" placeholder="Search projects..." class="gt-search"><span class="gt-count"></span><button class="tab" id="select-all-btn">Select All</button></div>';
  html += '<div class="generic-table"><div class="table-wrap"><table><thead></thead><tbody></tbody></table></div>';
  html += '<section class="drawer" id="projects-drawer"></section></div>';

  container.innerHTML = html;

  const search = container.querySelector(".gt-search");
  const countEl = container.querySelector(".gt-count");
  const selectAllBtn = container.querySelector("#select-all-btn");
  const thead = container.querySelector("thead");
  const tbody = container.querySelector("tbody");
  const drawer = container.querySelector("#projects-drawer");

  // Table columns
  const columns = [
    {key: "display_name", label: "Name"},
    {key: "key", label: "Key"},
    {key: "description", label: "Description"},
    {key: "repo_count", label: "Repos", numeric: true},
    {key: "storage_quota_gb", label: "Quota (GB)", numeric: true},
    {key: "role_count", label: "Roles", numeric: true},
    {key: "environment_count", label: "Environments", numeric: true},
  ];

  thead.innerHTML = "<tr>" + columns.map(c =>
    "<th" + (c.numeric ? ' class="number"' : "") + ">" + escapeHtml(c.label) + "</th>"
  ).join("") + "</tr>";

  function formatStorageQuota(bytes) {
    if (!bytes) return "Unlimited";
    if (bytes >= 1024 * 1024 * 1024 * 1024) return (bytes / (1024 * 1024 * 1024 * 1024)).toFixed(2) + " TB";
    if (bytes >= 1024 * 1024 * 1024) return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB";
    if (bytes >= 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(2) + " MB";
    return (bytes / 1024).toFixed(2) + " KB";
  }

  function renderProjectDetail(project) {
    let html = '<div style="display: grid; gap: 20px;">';

    // Basic info section
    html += '<section><h3>' + escapeHtml(project.display_name || project.key) + '</h3>';
    html += '<dl class="drawer-grid">';
    html += '<div><dt>Project Key</dt><dd>' + escapeHtml(project.key) + '</dd></div>';
    if (project.description) {
      html += '<div><dt>Description</dt><dd>' + escapeHtml(project.description) + '</dd></div>';
    }
    html += '<div><dt>Repositories</dt><dd>' + project.repo_count + '</dd></div>';
    html += '<div><dt>Storage Quota</dt><dd>' + (project.storage_quota_gb > 0 ? project.storage_quota_gb.toFixed(2) + " GB" : "Unlimited") + '</dd></div>';
    html += '<div><dt>Environments</dt><dd>' + (project.environment_count || 0) + '</dd></div>';
    html += '</dl></section>';

    // Roles section
    if (project.roles && project.roles.length > 0) {
      html += '<section><h4>Roles (' + project.roles.length + ')</h4>';
      html += '<table style="width: 100%; font-size: 13px;"><thead><tr><th>Name</th><th>Type</th><th>Environments</th><th>Actions</th></tr></thead><tbody>';
      for (const role of project.roles) {
        html += '<tr><td>' + escapeHtml(role.name) + '</td>';
        html += '<td>' + escapeHtml(role.type) + '</td>';
        html += '<td>' + (role.environments ? role.environments.join(", ") : "-") + '</td>';
        html += '<td>' + (role.actions ? role.actions.join(", ") : "-") + '</td></tr>';
      }
      html += '</tbody></table></section>';
    }

    // Environments section
    if (project.environments && project.environments.length > 0) {
      html += '<section><h4>Environments (' + project.environments.length + ')</h4>';
      html += '<p>' + project.environments.map(e => '<span class="badge">' + escapeHtml(e) + '</span>').join("") + '</p></section>';
    }

    // Repositories section
    if (project.repos && project.repos.length > 0) {
      html += '<section><h4>Repositories (' + project.repos.length + ')</h4>';
      html += '<div style="max-height: 200px; overflow-y: auto;">';
      html += '<ul style="padding-left: 20px; margin: 8px 0;">';
      for (const repo of project.repos) {
        html += '<li>' + escapeHtml(repo) + '</li>';
      }
      html += '</ul></div></section>';
    }

    // Users section
    if (project.users && project.users.length > 0) {
      html += '<section><h4>Users (' + project.users.length + ')</h4>';
      html += '<table style="width: 100%; font-size: 13px;"><thead><tr><th>User</th><th>Roles</th></tr></thead><tbody>';
      for (const user of project.users) {
        html += '<tr><td>' + escapeHtml(user.name) + '</td>';
        html += '<td>' + (user.roles ? user.roles.join(", ") : "-") + '</td></tr>';
      }
      html += '</tbody></table></section>';
    }

    // Groups section
    if (project.groups && project.groups.length > 0) {
      html += '<section><h4>Groups (' + project.groups.length + ')</h4>';
      html += '<table style="width: 100%; font-size: 13px;"><thead><tr><th>Group</th><th>Roles</th></tr></thead><tbody>';
      for (const group of project.groups) {
        html += '<tr><td>' + escapeHtml(group.name) + '</td>';
        html += '<td>' + (group.roles ? group.roles.join(", ") : "-") + '</td></tr>';
      }
      html += '</tbody></table></section>';
    }

    html += '</div>';
    return html;
  }

  function draw() {
    const q = search.value.trim().toLowerCase();
    const rows = PROJECTS_DATA.filter(p => !q || JSON.stringify(p).toLowerCase().includes(q));
    countEl.textContent = rows.length.toLocaleString() + " / " + PROJECTS_DATA.length.toLocaleString();

    tbody.innerHTML = rows.map((r, i) => '<tr data-index="' + i + '" style="cursor: pointer;">' +
      columns.map(c => {
        let val = r[c.key];
        if (c.key === "storage_quota_gb") val = (val > 0 ? val.toFixed(2) : "Unlimited");
        return "<td" + (c.numeric ? ' class="number"' : "") + " title=\"" + escapeHtml(String(val || "")) + "\">" +
          escapeHtml(String(val || "")) + "</td>";
      }).join("") +
      "</tr>").join("");

    tbody.querySelectorAll("tr").forEach(tr => tr.addEventListener("click", () => {
      const row = rows[Number(tr.dataset.index)];
      drawer.innerHTML = renderProjectDetail(row);
      drawer.classList.add("show");
      drawer.scrollIntoView({behavior: "smooth", block: "nearest"});
    }));
  }

  search.addEventListener("input", draw);
  selectAllBtn.addEventListener("click", () => {
    const allRows = tbody.querySelectorAll("tr");
    allRows.forEach(tr => tr.style.backgroundColor = "#eaf2ff");
  });

  draw();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", renderProjectsPanel);
} else {
  renderProjectsPanel();
}
</script>
`, jsonData)

	return b.String(), nil
}

// federationRow is the Federation & Replication tab's per-repo display row:
// a FederatedRepo's raw member list, plus a computed Status so a degraded
// or single-member federation config is visible at a glance instead of
// requiring a click into the raw members blob - the same "needs 2+ enabled
// members" signal already used for the single-member cross-resource
// finding in resource_findings.go.
type federationRow struct {
	RepoKey      string                      `json:"repo_key"`
	Status       string                      `json:"status"`
	EnabledCount int                         `json:"enabled_members"`
	TotalCount   int                         `json:"total_members"`
	Members      []instance.FederationMember `json:"members"`
}

func federationRows(repos []instance.FederatedRepo) []federationRow {
	out := make([]federationRow, len(repos))
	for i, f := range repos {
		enabled := 0
		for _, m := range f.Members {
			if m.Enabled {
				enabled++
			}
		}
		total := len(f.Members)
		var status string
		switch {
		case total == 0:
			status = "No members configured"
		case total == 1:
			status = "Single member (not fully federated)"
		case enabled < total:
			status = fmt.Sprintf("Degraded (%d/%d enabled)", enabled, total)
		default:
			status = fmt.Sprintf("Healthy (%d/%d enabled)", enabled, total)
		}
		out[i] = federationRow{RepoKey: f.RepoKey, Status: status, EnabledCount: enabled, TotalCount: total, Members: f.Members}
	}
	return out
}

func renderFederationReplicationPanel(snap *instance.Snapshot) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "<h2>Federated repos (%d)</h2>", len(snap.FederatedRepos))
	fedHTML, err := renderGenericTable("federation-table", federationRows(snap.FederatedRepos), []tableColumn{
		{Key: "repo_key", Label: "Repo"}, {Key: "status", Label: "Status"}, {Key: "members", Label: "Members"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(fedHTML)

	fmt.Fprintf(&b, "<h2>Replication configs (%d)</h2>", len(snap.Replications))
	repHTML, err := renderGenericTable("replication-table", snap.Replications, []tableColumn{
		{Key: "repo_key", Label: "Repo"}, {Key: "replication_type", Label: "Type"}, {Key: "enabled", Label: "Enabled"},
		{Key: "url", Label: "Target URL"}, {Key: "cron_exp", Label: "Cron"}, {Key: "sync_deletes", Label: "Sync Deletes"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(repHTML)

	return b.String(), nil
}

// tokenRow is the Platform Config tab's per-token display row: a
// ScopedToken with IssuedAt/Expiry rendered as human-readable dates instead
// of raw unix seconds.
type tokenRow struct {
	TokenID     string `json:"token_id"`
	Subject     string `json:"subject"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Issued      string `json:"issued,omitempty"`
	Expires     string `json:"expires,omitempty"`
	Refreshable bool   `json:"refreshable"`
}

// scopedTokenRows sorts a snapshot's scoped tokens by account (Subject),
// then token ID, so every token belonging to the same account sits
// together in the rendered table - the CSV/JSON already capture Subject
// (the account) and IssuedAt (creation date) per token; this just makes
// both visible and groups by account for an access review.
// repoAccessRow is one direct grant: a user or group principal, one repo
// from a permission target's scope, and the actions granted there - the
// per-principal x per-repo view that PermissionTarget's own
// Repos/UserActions/GroupActions (grouped the other way, by target) don't
// give directly. A group row shows the group's own grant, not its member
// users - group membership isn't collected by "discover instance", so
// resolving a group grant down to individual users isn't possible from
// this data alone.
type repoAccessRow struct {
	Principal     string `json:"principal"`
	PrincipalType string `json:"principal_type"`
	Repo          string `json:"repo"`
	Actions       string `json:"actions"`
	ViaTarget     string `json:"via_permission_target"`
}

// repoAccessRows inverts every permission target's Repos x
// UserActions/GroupActions into one row per principal x repo, sorted by
// principal (then repo) so a given user's or group's rows sit together -
// answers "what can this user/group do, and on which repos" directly,
// without clicking into each permission target individually.
func repoAccessRows(perms []instance.PermissionTarget) []repoAccessRow {
	var out []repoAccessRow
	for _, p := range perms {
		repos := p.Repos
		if len(repos) == 0 {
			repos = []string{"(none scoped)"}
		}
		for _, repo := range repos {
			for user, actions := range p.UserActions {
				out = append(out, repoAccessRow{Principal: user, PrincipalType: "user", Repo: repo, Actions: strings.Join(actions, ", "), ViaTarget: p.Name})
			}
			for group, actions := range p.GroupActions {
				out = append(out, repoAccessRow{Principal: group, PrincipalType: "group", Repo: repo, Actions: strings.Join(actions, ", "), ViaTarget: p.Name})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Principal != out[j].Principal {
			return out[i].Principal < out[j].Principal
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].ViaTarget < out[j].ViaTarget
	})
	return out
}

func scopedTokenRows(tokens []instance.ScopedToken) []tokenRow {
	out := make([]tokenRow, len(tokens))
	for i, t := range tokens {
		out[i] = tokenRow{
			TokenID:     t.TokenID,
			Subject:     t.Subject,
			Description: t.Description,
			Scope:       t.Scope,
			Issued:      formatUnixSeconds(t.IssuedAt, "-"),
			Expires:     formatUnixSeconds(t.Expiry, "Never"),
			Refreshable: t.Refreshable,
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].TokenID < out[j].TokenID
	})
	return out
}

// formatUnixSeconds renders a unix-seconds timestamp as a human-readable
// UTC date, or zeroLabel for a zero/missing value (e.g. "-" for an unset
// IssuedAt, "Never" for a non-expiring token's zero Expiry).
func formatUnixSeconds(sec int64, zeroLabel string) string {
	if sec == 0 {
		return zeroLabel
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02 15:04 UTC")
}

func renderPlatformConfigPanel(snap *instance.Snapshot) (string, error) {
	p := snap.Platform
	var b strings.Builder

	b.WriteString(`<h2>Singleton config</h2><section class="stats">`)
	fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>license type</span></div>`, htmlEscape(licenseType(p.License)))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--purple)"><strong>%s</strong><span>password max age (days)</span></div>`, htmlEscape(passwordMaxAge(p.PasswordPolicy)))
	fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>failed-login lockout attempts</span></div>`, htmlEscape(loginAttempts(p.UserLockPolicy)))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--teal)"><strong>%s</strong><span>mail server configured</span></div>`, yesNo(p.MailServer != nil))
	fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>reverse proxy configured</span></div>`, yesNo(p.ReverseProxy != nil))
	fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--amber)"><strong>%s</strong><span>trash retention (days)</span></div>`, htmlEscape(trashRetention(p.Trash)))
	b.WriteString("</section>")

	tables := []struct {
		id, label string
		data      interface{}
		cols      []tableColumn
	}{
		{"platform-roles", "Global roles", p.GlobalRoles, []tableColumn{
			{Key: "name", Label: "Name"}, {Key: "type", Label: "Type"}, {Key: "environments", Label: "Environments"}, {Key: "actions", Label: "Actions"},
		}},
		{"platform-environments", "Global environments", p.GlobalEnvironments, []tableColumn{
			{Key: "name", Label: "Name"},
		}},
		{"platform-certificates", "Certificates", p.Certificates, []tableColumn{
			{Key: "certificateAlias", Label: "Alias"}, {Key: "issuedTo", Label: "Issued To"}, {Key: "issuedBy", Label: "Issued By"}, {Key: "validUntil", Label: "Valid Until"},
		}},
		// Key material itself is never captured - only pair name, type, and
		// alias, same as the highlighted callout on the Overview tab.
		{"platform-signing-keys", "Signing keys", snap.Security.SigningKeys, []tableColumn{
			{Key: "pair_name", Label: "Pair Name"}, {Key: "pair_type", Label: "Pair Type"}, {Key: "alias", Label: "Alias"},
		}},
		{"platform-propertysets", "Property sets", p.PropertySets, []tableColumn{
			{Key: "name", Label: "Name"}, {Key: "propertiesCount", Label: "Properties", Numeric: true},
		}},
		{"platform-proxies", "Proxies", p.Proxies, []tableColumn{
			{Key: "key", Label: "Key"}, {Key: "host", Label: "Host"}, {Key: "port", Label: "Port", Numeric: true}, {Key: "defaultProxy", Label: "Default"},
		}},
		{"platform-backups", "Backups", p.Backups, []tableColumn{
			{Key: "key", Label: "Key"}, {Key: "enabled", Label: "Enabled"}, {Key: "cronExp", Label: "Cron"}, {Key: "retentionPeriodHours", Label: "Retention (hrs)", Numeric: true},
		}},
		{"platform-archive-policies", "Archive cleanup policies", p.ArchiveCleanupPolicies, []tableColumn{
			{Key: "key", Label: "Key"}, {Key: "enabled", Label: "Enabled"}, {Key: "cron_exp", Label: "Cron"},
		}},
		{"platform-workers", "Workers", p.Workers, []tableColumn{
			{Key: "key", Label: "Key"}, {Key: "enabled", Label: "Enabled"}, {Key: "action", Label: "Action"},
		}},
		{"platform-webhooks", "Webhooks", p.Webhooks, []tableColumn{
			{Key: "key", Label: "Key"}, {Key: "enabled", Label: "Enabled"}, {Key: "domain", Label: "Domain"},
			{Key: "event_types", Label: "Event Types"}, {Key: "handler_urls", Label: "Handler URLs"},
		}},
	}
	for _, t := range tables {
		fmt.Fprintf(&b, "<h2>%s</h2>", htmlEscape(t.label))
		tableHTML, err := renderGenericTable(t.id, t.data, t.cols)
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	return b.String(), nil
}

func renderXrayPanel(snap *instance.Snapshot) (string, error) {
	x := snap.Xray
	var b strings.Builder

	if !snap.System.XrayAvailable {
		b.WriteString(`<div class="callout info"><strong>Xray is not available on this instance.</strong></div>`)
		return b.String(), nil
	}

	if x.Settings != nil {
		b.WriteString(`<h2>Settings</h2><section class="stats">`)
		fmt.Fprintf(&b, `<div class="stat"><strong>%d</strong><span>max disk data usage %%</span></div>`, x.Settings.MaxDiskDataUsage)
		fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--purple)"><strong>%d</strong><span>job interval (sec)</span></div>`, x.Settings.JobInterval)
		fmt.Fprintf(&b, `<div class="stat"><strong>%s</strong><span>db sync time</span></div>`, htmlEscape(orDash(x.Settings.DBSyncUpdatesTime)))
		fmt.Fprintf(&b, `<div class="stat" style="border-top-color:var(--teal)"><strong>%s</strong><span>sends analytics</span></div>`, yesNo(x.Settings.AllowSendingAnalytics))
		b.WriteString("</section>")
	}

	tables := []struct {
		id, label string
		data      interface{}
		cols      []tableColumn
	}{
		{"xray-policies", "Policies", x.Policies, []tableColumn{
			{Key: "name", Label: "Name"}, {Key: "type", Label: "Type"}, {Key: "author", Label: "Author"}, {Key: "rules", Label: "Rules"},
		}},
		{"xray-watches", "Watches", x.Watches, []tableColumn{
			{Key: "name", Label: "Name"}, {Key: "active", Label: "Active"}, {Key: "project_key", Label: "Project"},
			{Key: "assigned_policies", Label: "Assigned Policies"}, {Key: "resources", Label: "Resources"},
		}},
		{"xray-ignore-rules", "Ignore rules", x.IgnoreRules, []tableColumn{
			{Key: "id", Label: "ID"}, {Key: "author", Label: "Author"}, {Key: "notes", Label: "Notes"}, {Key: "ignore_filters", Label: "Filters"},
		}},
	}
	for _, t := range tables {
		fmt.Fprintf(&b, "<h2>%s (%d)</h2>", htmlEscape(t.label), reflectLen(t.data))
		tableHTML, err := renderGenericTable(t.id, t.data, t.cols)
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	return b.String(), nil
}

// renderCurationPanel is its own tab, separate from the core Xray tab -
// curation is a distinct, independently-licensed feature (see
// XrayConfig.CurationAvailable), so it gets its own notice when unavailable
// rather than being buried as a subsection of Xray.
func renderCurationPanel(snap *instance.Snapshot) (string, error) {
	x := snap.Xray
	var b strings.Builder

	if !x.CurationAvailable {
		b.WriteString(`<div class="callout info"><strong>Curation is not enabled/available on this instance.</strong></div>`)
		return b.String(), nil
	}

	tables := []struct {
		id, label string
		data      interface{}
		cols      []tableColumn
	}{
		{"curation-policies", "Curation policies", x.CurationPolicies, []tableColumn{
			{Key: "name", Label: "Name"}, {Key: "enabled", Label: "Enabled"}, {Key: "scope", Label: "Scope"}, {Key: "policy_action", Label: "Action"},
		}},
		{"curation-waivers", "Curation waivers", curationWaiverRows(x.CurationWaivers), []tableColumn{
			{Key: "id", Label: "ID", Numeric: true}, {Key: "status", Label: "Status"}, {Key: "requested_by", Label: "Requested By"},
			{Key: "justification", Label: "Justification"}, {Key: "created_at", Label: "Created"},
		}},
	}
	for _, t := range tables {
		fmt.Fprintf(&b, "<h2>%s (%d)</h2>", htmlEscape(t.label), reflectLen(t.data))
		tableHTML, err := renderGenericTable(t.id, t.data, t.cols)
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	return b.String(), nil
}

// curationWaiverRow is the Curation tab's per-waiver display row: a
// XrayCurationWaiver with its requester(s) flattened into two display
// fields, since a waiver can (rarely) carry more than one requester.
type curationWaiverRow struct {
	ID            int64  `json:"id"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at,omitempty"`
	RequestedBy   string `json:"requested_by,omitempty"`
	Justification string `json:"justification,omitempty"`
}

func curationWaiverRows(waivers []instance.XrayCurationWaiver) []curationWaiverRow {
	out := make([]curationWaiverRow, len(waivers))
	for i, w := range waivers {
		var requesters, justifications []string
		for _, r := range w.Requesters {
			requesters = append(requesters, r.User)
			if r.Justification != "" {
				justifications = append(justifications, r.Justification)
			}
		}
		out[i] = curationWaiverRow{
			ID:            w.ID,
			Status:        w.Status,
			CreatedAt:     w.CreatedAt,
			RequestedBy:   strings.Join(requesters, ", "),
			Justification: strings.Join(justifications, "; "),
		}
	}
	return out
}

// relProject and relRepo are the Relationships tab's embedded dataset -
// deliberately flat and small (just enough to filter by project_key
// client-side), independent of the richer instance.Project/Row shapes.
type relProject struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	RepoCount   int    `json:"repo_count"`
}

type relRepo struct {
	Name        string `json:"name"`
	ProjectKey  string `json:"project_key"`
	PackageType string `json:"package_type"`
	RepoType    string `json:"repo_type"`
}

// renderRelationshipsPanel builds the Project -> Repo correlation tab: a
// picker (Global, or one project) plus a hub-and-spoke diagram and full
// searchable table of that project's repos (or, for Global, every repo with
// no project). A text search box narrows the project list, since an
// instance can have hundreds of projects. Needs both the repos CSV rows
// (for each repo's project_key, not carried on instance.Project) and the
// instance snapshot (for project display names/repo lists).
func renderRelationshipsPanel(rows []Row, snap *instance.Snapshot) (string, error) {
	projects := make([]relProject, 0, len(snap.Projects))
	for _, p := range snap.Projects {
		projects = append(projects, relProject{Key: p.Key, DisplayName: p.DisplayName, RepoCount: p.RepoCount})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Key < projects[j].Key })

	repos := make([]relRepo, 0, len(rows))
	for _, r := range rows {
		repos = append(repos, relRepo{
			Name: r.Get("repo_name"), ProjectKey: r.Get("project_key"),
			PackageType: r.Get("package_type"), RepoType: r.Get("repo_type"),
		})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })

	data := struct {
		Projects []relProject `json:"projects"`
		Repos    []relRepo    `json:"repos"`
	}{projects, repos}
	jsonData, err := jsonForHTML(data)
	if err != nil {
		return "", fmt.Errorf("encoding relationships data: %w", err)
	}

	var b strings.Builder
	b.WriteString("<h2>Project &harr; Repo correlation</h2>")
	b.WriteString("<p>Select \"Global\" or a project to see its repos - repos with no project show up under Global. " +
		"The diagram shows up to 24 - the table below always has the full list.</p>")

	b.WriteString(`<div class="rel-toolbar">`)
	b.WriteString(`<input type="search" id="rel-project-search" placeholder="Search projects...">`)
	b.WriteString(`<select id="rel-select">`)
	b.WriteString(`<option value="global">Global (repos with no project)</option>`)
	if len(projects) > 0 {
		b.WriteString(`<optgroup label="Projects">`)
		for _, p := range projects {
			label := p.Key
			if p.DisplayName != "" && p.DisplayName != p.Key {
				label = fmt.Sprintf("%s (%s)", p.DisplayName, p.Key)
			}
			fmt.Fprintf(&b, `<option value="project:%s">%s</option>`, htmlAttr(p.Key), htmlEscape(label))
		}
		b.WriteString(`</optgroup>`)
	}
	b.WriteString(`</select></div>`)

	b.WriteString(`<div id="rel-summary" class="rel-summary"></div>`)
	b.WriteString(`<div id="rel-diagram" class="rel-diagram-wrap"></div>`)
	b.WriteString(`<div id="rel-table"></div>`)

	fmt.Fprintf(&b, "<script>\nconst REL_DATA = %s;\n%s\n</script>\n", jsonData, relationshipsScript)

	return b.String(), nil
}

// renderAllFindingsPanel builds the combined cross-resource findings tab:
// every repo finding (with its explanation looked up from defs) plus every
// non-repo resource finding computed from snap, as one searchable table.
func renderAllFindingsPanel(rows []Row, defs []FindingDef, snap *instance.Snapshot) (string, error) {
	findings := repoResourceFindings(rows, defs)
	findings = append(findings, computeResourceFindings(snap)...)

	var b strings.Builder
	fmt.Fprintf(&b, "<h2>All findings (%d)</h2>", len(findings))
	b.WriteString("<p>Every flagged resource across Repos, Access, Projects, Federation &amp; Replication, Platform Config, and Xray - one searchable list. See each resource's own tab for full context.</p>")
	tableHTML, err := renderGenericTable("all-findings-table", findings, []tableColumn{
		{Key: "resource_type", Label: "Resource Type"}, {Key: "resource_name", Label: "Resource Name"},
		{Key: "finding", Label: "Finding"}, {Key: "detail", Label: "Detail"},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(tableHTML)
	return b.String(), nil
}

// renderGenericTable emits a container div plus an inline <script> call to
// mountTable() with data/columns embedded as JSON literals.
func renderGenericTable(containerID string, data interface{}, columns []tableColumn) (string, error) {
	jsonData, err := jsonForHTML(data)
	if err != nil {
		return "", fmt.Errorf("encoding %s data: %w", containerID, err)
	}
	jsonCols, err := jsonForHTML(columns)
	if err != nil {
		return "", fmt.Errorf("encoding %s columns: %w", containerID, err)
	}
	return fmt.Sprintf("<div id=%q></div>\n<script>mountTable(%q, %s, %s, PAGE_SIZE);</script>\n",
		containerID, containerID, jsonData, jsonCols), nil
}

func componentStatCard(name string, available bool, version string) string {
	if !available {
		return fmt.Sprintf(`<div class="stat" style="border-top-color:var(--slate)"><strong>-</strong><span>%s (not available)</span></div>`, htmlEscape(name))
	}
	return fmt.Sprintf(`<div class="stat"><strong>%s</strong><span>%s</span></div>`, htmlEscape(orDash(version)), htmlEscape(name))
}

// renderAppTrustPanel displays AppTrust data
func renderAppTrustPanel(snap *instance.Snapshot) (string, error) {
	var b strings.Builder

	hasData := len(snap.AppTrustPolicies) > 0 || len(snap.AppTrustApplications) > 0 ||
		len(snap.AppTrustRules) > 0 || len(snap.AppTrustTemplates) > 0 ||
		len(snap.AppTrustStages) > 0 || len(snap.AppTrustEvidence) > 0 ||
		len(snap.AppTrustReleaseBundles) > 0

	if !hasData {
		b.WriteString(`<div class="callout info"><strong>No AppTrust data found.</strong> AppTrust may not be licensed or configured on this instance.</div>`)
		return b.String(), nil
	}

	// Applications
	if len(snap.AppTrustApplications) > 0 {
		apps := make([]interface{}, len(snap.AppTrustApplications))
		for i, a := range snap.AppTrustApplications {
			apps[i] = struct {
				ApplicationKey  string `json:"application_key"`
				ApplicationName string `json:"application_name"`
				ProjectKey      string `json:"project_key"`
				Criticality     string `json:"criticality"`
				MaturityLevel   string `json:"maturity_level"`
				UserOwners      string `json:"user_owners"`
				GroupOwners     string `json:"group_owners"`
				Description     string `json:"description"`
			}{
				ApplicationKey:  a.ApplicationKey,
				ApplicationName: a.ApplicationName,
				ProjectKey:      a.ProjectKey,
				Criticality:     orDash(a.Criticality),
				MaturityLevel:   orDash(a.MaturityLevel),
				UserOwners:      strings.Join(a.UserOwners, ", "),
				GroupOwners:     strings.Join(a.GroupOwners, ", "),
				Description:     a.Description,
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Applications (%d)</h2>", len(apps)))
		tableHTML, err := renderGenericTable("apptrust-applications", apps, []tableColumn{
			{Key: "application_name", Label: "Application Name"},
			{Key: "application_key", Label: "Key"},
			{Key: "project_key", Label: "Project"},
			{Key: "criticality", Label: "Criticality"},
			{Key: "maturity_level", Label: "Maturity"},
			{Key: "user_owners", Label: "User Owners"},
			{Key: "group_owners", Label: "Group Owners"},
			{Key: "description", Label: "Description"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Policies (JFrog Unified Policy Service)
	if len(snap.AppTrustPolicies) > 0 {
		policies := make([]interface{}, len(snap.AppTrustPolicies))
		for i, p := range snap.AppTrustPolicies {
			policies[i] = struct {
				Name                string `json:"name"`
				Enabled             string `json:"enabled"`
				Mode                string `json:"mode"`
				ScopeType           string `json:"scope_type"`
				StageKey            string `json:"stage_key"`
				StageGate           string `json:"stage_gate"`
				WaiverRequestConfig string `json:"waiver_request_config"`
				Description         string `json:"description"`
			}{
				Name:                p.Name,
				Enabled:             yesNo(p.Enabled),
				Mode:                orDash(p.Mode),
				ScopeType:           orDash(p.Scope.ScopeType),
				StageKey:            orDash(p.StageKey),
				StageGate:           orDash(p.StageGate),
				WaiverRequestConfig: orDash(p.WaiverRequestConfig),
				Description:         p.Description,
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Policies (%d)</h2>", len(policies)))
		tableHTML, err := renderGenericTable("apptrust-policies", policies, []tableColumn{
			{Key: "name", Label: "Policy Name"},
			{Key: "enabled", Label: "Enabled"},
			{Key: "mode", Label: "Mode"},
			{Key: "scope_type", Label: "Scope"},
			{Key: "stage_key", Label: "Stage"},
			{Key: "stage_gate", Label: "Gate"},
			{Key: "waiver_request_config", Label: "Waiver Config"},
			{Key: "description", Label: "Description"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Rules (JFrog Unified Policy Service)
	if len(snap.AppTrustRules) > 0 {
		rules := make([]interface{}, len(snap.AppTrustRules))
		for i, r := range snap.AppTrustRules {
			rules[i] = struct {
				Name        string `json:"name"`
				TemplateID  string `json:"template_id"`
				IsCustom    string `json:"is_custom"`
				Description string `json:"description"`
				CreatedBy   string `json:"created_by"`
			}{
				Name:        r.Name,
				TemplateID:  r.TemplateID,
				IsCustom:    yesNo(r.IsCustom),
				Description: r.Description,
				CreatedBy:   orDash(r.CreatedBy),
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Rules (%d)</h2>", len(rules)))
		tableHTML, err := renderGenericTable("apptrust-rules", rules, []tableColumn{
			{Key: "name", Label: "Rule Name"},
			{Key: "template_id", Label: "Template ID"},
			{Key: "is_custom", Label: "Custom"},
			{Key: "description", Label: "Description"},
			{Key: "created_by", Label: "Created By"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Templates (JFrog Unified Policy Service)
	if len(snap.AppTrustTemplates) > 0 {
		templates := make([]interface{}, len(snap.AppTrustTemplates))
		for i, t := range snap.AppTrustTemplates {
			templates[i] = struct {
				Name           string `json:"name"`
				Category       string `json:"category"`
				Scanners       string `json:"scanners"`
				DataSourceType string `json:"data_source_type"`
				IsCustom       string `json:"is_custom"`
				Description    string `json:"description"`
			}{
				Name:           t.Name,
				Category:       orDash(t.Category),
				Scanners:       strings.Join(t.Scanners, ", "),
				DataSourceType: orDash(t.DataSourceType),
				IsCustom:       yesNo(t.IsCustom),
				Description:    t.Description,
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Templates (%d)</h2>", len(templates)))
		tableHTML, err := renderGenericTable("apptrust-templates", templates, []tableColumn{
			{Key: "name", Label: "Template Name"},
			{Key: "category", Label: "Category"},
			{Key: "scanners", Label: "Scanners"},
			{Key: "data_source_type", Label: "Data Source"},
			{Key: "is_custom", Label: "Custom"},
			{Key: "description", Label: "Description"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Lifecycle Stages (per-project)
	if len(snap.AppTrustStages) > 0 {
		stages := make([]interface{}, len(snap.AppTrustStages))
		for i, s := range snap.AppTrustStages {
			stages[i] = struct {
				ProjectKey string `json:"project_key"`
				Category   string `json:"category"`
				StageName  string `json:"stage_name"`
				Scope      string `json:"scope"`
				Gates      string `json:"gates"`
			}{
				ProjectKey: orDash(s.ProjectKey),
				Category:   orDash(s.Category),
				StageName:  s.StageName,
				Scope:      orDash(s.Scope),
				Gates:      strings.Join(s.Gates, ", "),
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Lifecycle Stages (%d)</h2>", len(stages)))
		tableHTML, err := renderGenericTable("apptrust-stages", stages, []tableColumn{
			{Key: "stage_name", Label: "Stage Name"},
			{Key: "category", Label: "Category"},
			{Key: "scope", Label: "Scope"},
			{Key: "project_key", Label: "Project (if scoped)"},
			{Key: "gates", Label: "Gates"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Evidence
	if len(snap.AppTrustEvidence) > 0 {
		evidence := make([]interface{}, len(snap.AppTrustEvidence))
		for i, e := range snap.AppTrustEvidence {
			evidence[i] = struct {
				Name              string `json:"name"`
				Repository        string `json:"repository"`
				PredicateCategory string `json:"predicate_category"`
				PredicateType     string `json:"predicate_type"`
				Stage             string `json:"stage"`
				Verified          string `json:"verified"`
				CreatedBy         string `json:"created_by"`
			}{
				Name:              orDash(e.Name),
				Repository:        orDash(e.Repository),
				PredicateCategory: orDash(e.PredicateCategory),
				PredicateType:     orDash(e.PredicateType),
				Stage:             orDash(e.Stage),
				Verified:          yesNo(e.Verified),
				CreatedBy:         orDash(e.CreatedBy),
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Evidence (%d)</h2>", len(evidence)))
		tableHTML, err := renderGenericTable("apptrust-evidence", evidence, []tableColumn{
			{Key: "name", Label: "Evidence Name"},
			{Key: "repository", Label: "Repository"},
			{Key: "predicate_category", Label: "Predicate Category"},
			{Key: "predicate_type", Label: "Predicate Type"},
			{Key: "stage", Label: "Stage"},
			{Key: "verified", Label: "Verified"},
			{Key: "created_by", Label: "Created By"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Release Bundles (v2 names)
	if len(snap.AppTrustReleaseBundles) > 0 {
		bundles := make([]interface{}, len(snap.AppTrustReleaseBundles))
		for i, rb := range snap.AppTrustReleaseBundles {
			bundles[i] = struct {
				ReleaseBundleName string `json:"release_bundle_name"`
				RepositoryKey     string `json:"repository_key"`
				ProjectKey        string `json:"project_key"`
				ProjectName       string `json:"project_name"`
			}{
				ReleaseBundleName: rb.ReleaseBundleName,
				RepositoryKey:     rb.RepositoryKey,
				ProjectKey:        orDash(rb.ProjectKey),
				ProjectName:       orDash(rb.ProjectName),
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Release Bundles (%d)</h2>", len(bundles)))
		tableHTML, err := renderGenericTable("apptrust-bundles", bundles, []tableColumn{
			{Key: "release_bundle_name", Label: "Bundle Name"},
			{Key: "repository_key", Label: "Repository"},
			{Key: "project_key", Label: "Project Key"},
			{Key: "project_name", Label: "Project Name"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Application → Policy relationships (with scope mechanism)
	if len(snap.AppTrustApplicationPolicies) > 0 {
		appPolicies := make([]interface{}, len(snap.AppTrustApplicationPolicies))
		for i, ap := range snap.AppTrustApplicationPolicies {
			appPolicies[i] = struct {
				ApplicationName string `json:"application_name"`
				ApplicationKey  string `json:"application_key"`
				ProjectKey      string `json:"project_key"`
				PolicyName      string `json:"policy_name"`
				ScopeType       string `json:"scope_type"`
				ScopeMechanism  string `json:"scope_mechanism"`
				PolicyMode      string `json:"policy_mode"`
				PolicyEnabled   string `json:"policy_enabled"`
			}{
				ApplicationName: ap.ApplicationName,
				ApplicationKey:  ap.ApplicationKey,
				ProjectKey:      orDash(ap.ProjectKey),
				PolicyName:      ap.PolicyName,
				ScopeType:       ap.ScopeType,
				ScopeMechanism:  orDash(ap.ScopeMechanism),
				PolicyMode:      orDash(ap.PolicyMode),
				PolicyEnabled:   yesNo(ap.PolicyEnabled),
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Application → Policy Links (%d)</h2>", len(appPolicies)))
		tableHTML, err := renderGenericTable("apptrust-app-policies", appPolicies, []tableColumn{
			{Key: "application_name", Label: "Application"},
			{Key: "application_key", Label: "App Key"},
			{Key: "project_key", Label: "Project"},
			{Key: "policy_name", Label: "Policy"},
			{Key: "scope_type", Label: "Scope Type"},
			{Key: "scope_mechanism", Label: "Scope Mechanism"},
			{Key: "policy_mode", Label: "Mode"},
			{Key: "policy_enabled", Label: "Enabled"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	// Policy → Rule relationships (with template info)
	if len(snap.AppTrustPolicyRules) > 0 {
		policyRules := make([]interface{}, len(snap.AppTrustPolicyRules))
		for i, pr := range snap.AppTrustPolicyRules {
			policyRules[i] = struct {
				PolicyName   string `json:"policy_name"`
				PolicyID     string `json:"policy_id"`
				RuleName     string `json:"rule_name"`
				RuleID       string `json:"rule_id"`
				RuleOrder    string `json:"rule_order"`
				TemplateName string `json:"template_name"`
				TemplateID   string `json:"template_id"`
			}{
				PolicyName:   pr.PolicyName,
				PolicyID:     pr.PolicyID,
				RuleName:     pr.RuleName,
				RuleID:       pr.RuleID,
				RuleOrder:    strconv.Itoa(pr.RuleOrder),
				TemplateName: orDash(pr.TemplateName),
				TemplateID:   orDash(pr.TemplateID),
			}
		}
		b.WriteString(fmt.Sprintf("<h2>AppTrust Policy → Rule Links (%d)</h2>", len(policyRules)))
		tableHTML, err := renderGenericTable("apptrust-policy-rules", policyRules, []tableColumn{
			{Key: "policy_name", Label: "Policy"},
			{Key: "policy_id", Label: "Policy ID"},
			{Key: "rule_name", Label: "Rule"},
			{Key: "rule_id", Label: "Rule ID"},
			{Key: "rule_order", Label: "Order"},
			{Key: "template_name", Label: "Template"},
			{Key: "template_id", Label: "Template ID"},
		})
		if err != nil {
			return "", err
		}
		b.WriteString(tableHTML)
	}

	return b.String(), nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func licenseType(l *instance.LicenseInfo) string {
	if l == nil {
		return "-"
	}
	return orDash(l.Type)
}

func passwordMaxAge(p *instance.PasswordExpirationPolicy) string {
	if p == nil || !p.Enabled {
		return "-"
	}
	return fmt.Sprintf("%d", p.PasswordMaxAge)
}

func loginAttempts(p *instance.UserLockPolicy) string {
	if p == nil || !p.Enabled {
		return "-"
	}
	return fmt.Sprintf("%d", p.LoginAttempts)
}

func trashRetention(t *instance.TrashConfig) string {
	if t == nil || !t.Enabled {
		return "-"
	}
	return fmt.Sprintf("%d", t.RetentionPeriodDays)
}

// reflectLen returns len(v) for any slice value, or 0 for anything else -
// used to put a count in an <h2> heading next to a table whose data is
// typed as interface{}.
func reflectLen(v interface{}) int {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return 0
	}
	return rv.Len()
}

func liList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "<li>%s</li>", htmlEscape(item))
	}
	return b.String()
}
