package analyze

import (
	"fmt"
	"reflect"
	"sort"
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
		{"platform", "Platform Config", renderPlatformConfigPanel},
		{"xray", "Xray", renderXrayPanel},
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
		{"platform-tokens", "Scoped tokens", p.ScopedTokens, []tableColumn{
			{Key: "token_id", Label: "Token ID"}, {Key: "subject", Label: "Subject"}, {Key: "scope", Label: "Scope"},
			{Key: "expiry", Label: "Expiry (unix)", Numeric: true}, {Key: "refreshable", Label: "Refreshable"},
		}},
		{"platform-certificates", "Certificates", p.Certificates, []tableColumn{
			{Key: "certificateAlias", Label: "Alias"}, {Key: "issuedTo", Label: "Issued To"}, {Key: "issuedBy", Label: "Issued By"}, {Key: "validUntil", Label: "Valid Until"},
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
		{"xray-curation", "Curation policies", x.CurationPolicies, []tableColumn{
			{Key: "name", Label: "Name"}, {Key: "enabled", Label: "Enabled"}, {Key: "scope", Label: "Scope"}, {Key: "policy_action", Label: "Action"},
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
