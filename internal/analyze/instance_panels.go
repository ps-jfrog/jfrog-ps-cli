package analyze

import (
	"fmt"
	"strings"

	"jfrogpscli/internal/instance"
)

// tableColumn describes one column of a generic client-side searchable
// table (see mountTable in template.go's JS).
type tableColumn struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Numeric bool   `json:"numeric,omitempty"`
}

// renderInstanceTabs builds the "Overview"/"Access"/"Projects"/"Federation &
// Replication" nav buttons and panels from an instance snapshot. Returns
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
		{"overview", "Overview", renderOverviewPanel},
		{"access", "Access", renderAccessPanel},
		{"projects", "Projects", renderProjectsPanel},
		{"fedrep", "Federation & Replication", renderFederationReplicationPanel},
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

func renderOverviewPanel(snap *instance.Snapshot) (string, error) {
	sys := snap.System
	var b strings.Builder

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

	if len(snap.Warnings) > 0 {
		b.WriteString("<h2>Collection warnings</h2>")
		fmt.Fprintf(&b, `<div class="callout warn"><ul>%s</ul></div>`, liList(snap.Warnings))
	}

	return b.String(), nil
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

	return b.String(), nil
}

func renderProjectsPanel(snap *instance.Snapshot) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, `<h2>Projects (%d)</h2><p>Repos not assigned to any project show project_key %q on the Repos tab.</p>`, len(snap.Projects), "default")
	projectsHTML, err := renderGenericTable("projects-table", snap.Projects, []tableColumn{
		{Key: "key", Label: "Project Key"}, {Key: "display_name", Label: "Name"}, {Key: "description", Label: "Description"},
		{Key: "repo_count", Label: "Repos", Numeric: true}, {Key: "storage_quota_bytes", Label: "Storage Quota (bytes)", Numeric: true},
	})
	if err != nil {
		return "", err
	}
	b.WriteString(projectsHTML)
	return b.String(), nil
}

func renderFederationReplicationPanel(snap *instance.Snapshot) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "<h2>Federated repos (%d)</h2>", len(snap.FederatedRepos))
	fedHTML, err := renderGenericTable("federation-table", snap.FederatedRepos, []tableColumn{
		{Key: "repo_key", Label: "Repo"}, {Key: "members", Label: "Members"},
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
	return fmt.Sprintf("<div id=%q></div>\n<script>mountTable(%q, %s, %s);</script>\n",
		containerID, containerID, jsonData, jsonCols), nil
}

func componentStatCard(name string, available bool, version string) string {
	if !available {
		return fmt.Sprintf(`<div class="stat" style="border-top-color:var(--slate)"><strong>-</strong><span>%s (not available)</span></div>`, htmlEscape(name))
	}
	return fmt.Sprintf(`<div class="stat"><strong>%s</strong><span>%s</span></div>`, htmlEscape(orDash(version)), htmlEscape(name))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func liList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "<li>%s</li>", htmlEscape(item))
	}
	return b.String()
}
