package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jfrogpscli/internal/instance"
)

// RenderOptions carries the rendering-time context Render needs beyond the
// parsed rows themselves.
type RenderOptions struct {
	SourceFile     string
	Opts           Options
	GeneratedAt    time.Time
	OutputDir      string // Directory for output files
	SplitByProject bool   // Generate separate reports per project
	PageSize       int    // Rows per page (0 = no pagination)
	// Instance, if non-nil, adds Overview/Access/Projects/Federation &
	// Replication tabs built from a "discover instance" snapshot. Nil
	// renders exactly the single-panel report this package always has.
	Instance *instance.Snapshot
}

// Render builds a self-contained interactive HTML page from header/rows
// (as returned by ParseCSV, after ComputeFindings has run).
// If SplitByProject is true, generates separate reports per project and returns the index HTML.
func Render(header []string, rows []Row, ro RenderOptions) (string, error) {
	defs := ro.Opts.Defs(header)

	// If split by project, generate separate reports and return index
	if ro.SplitByProject && ro.OutputDir != "" {
		return renderSplitByProject(header, rows, ro, defs)
	}
	counts := findingCounts(rows)

	totalUsageGB := 0.0
	packageTypes := map[string]int{}
	findingRepoCount := 0
	for _, r := range rows {
		totalUsageGB += r.Float("usage_gb")
		if pt := r.Get("package_type"); pt != "" {
			packageTypes[pt]++
		}
		if len(r.Findings) > 0 {
			findingRepoCount++
		}
	}

	jsonRows, err := jsonForHTML(buildJSONRows(rows))
	if err != nil {
		return "", fmt.Errorf("encoding rows: %w", err)
	}
	jsonFindingDefs, err := jsonForHTML(defs)
	if err != nil {
		return "", fmt.Errorf("encoding finding defs: %w", err)
	}

	statCards := []statCard{
		{fmt.Sprintf("%d", len(rows)), "repositories", ""},
		{fmt.Sprintf("%.2f TB", totalUsageGB/1024), "logical used space", "purple"},
		{fmt.Sprintf("%d", len(packageTypes)), "package types", "teal"},
		{fmt.Sprintf("%d", findingRepoCount), "repos with 1+ finding", "amber"},
	}

	var findingCardsHTML strings.Builder
	for _, d := range defs {
		fmt.Fprintf(&findingCardsHTML, `
    <button class="finding %s" data-finding="%s" data-explain="%s">
      <strong>%d</strong><h3>%s</h3>
    </button>`, toneClass(d.Tone), htmlAttr(d.ID), htmlAttr(d.Explain), counts[d.ID], htmlEscape(d.Label))
	}

	var statCardsHTML strings.Builder
	for _, s := range statCards {
		border := ""
		if s.Tone != "" {
			border = fmt.Sprintf(` style="border-top-color:var(--%s)"`, s.Tone)
		}
		fmt.Fprintf(&statCardsHTML, `
    <div class="stat"%s><strong>%s</strong><span>%s</span></div>`, border, htmlEscape(s.Value), htmlEscape(s.Label))
	}

	// Overview is the default tab once there's an instance snapshot to show
	// one for; without it, Repos stays the default, exactly as before this
	// report had an Overview tab.
	reposActiveClass := ""
	if ro.Instance == nil {
		reposActiveClass = " active"
	}

	// Repos, Wave Planning, and Migration Workflow are always present, with
	// or without an instance snapshot - neither needs anything beyond the
	// repo rows/findings already embedded for the Repos tab. Both are
	// ordered last (Wave Planning feeds Migration Workflow's wave-based
	// scope, so Migration Workflow follows it).
	reposNav := fmt.Sprintf(`<button class="report-tab%s" data-report-tab="repos">Repos</button>`, reposActiveClass)
	wavesNav := `<button class="report-tab" data-report-tab="waves">Wave Planning</button>`
	wavesPanel := "\n  <section class=\"report-panel\" data-report-panel=\"waves\">\n" + wavePlanningPanel + "\n  </section>\n"
	migrationNav := `<button class="report-tab" data-report-tab="migration">Migration Workflow</button>`
	migrationPanel := "\n  <section class=\"report-panel\" data-report-panel=\"migration\">\n" + migrationWorkflowPanel + "\n  </section>\n"

	navTabsHTML, extraPanelsHTML, err := renderInstanceTabs(ro.Instance)
	if err != nil {
		return "", err
	}
	if navTabsHTML != "" {
		// Order: Overview, Repos, [Projects/Access/Fedrep/Platform/Xray
		// from renderInstanceTabs], All Findings, Relationships, Wave
		// Planning, Migration Workflow. Overview is built here rather than
		// by renderInstanceTabs because it needs the repo count, which
		// isn't part of an instance.Snapshot.
		overviewContent, err := renderOverviewPanel(ro.Instance, len(rows))
		if err != nil {
			return "", fmt.Errorf("render overview panel: %w", err)
		}
		overviewNav := `<button class="report-tab active" data-report-tab="overview">Overview</button>`
		overviewPanel := "\n  <section class=\"report-panel active\" data-report-panel=\"overview\">\n" + overviewContent + "\n  </section>\n"

		// Only worth its own tab once there's cross-resource data to
		// combine with the repo findings - without an instance snapshot,
		// the Repos tab's own findings cards/explorer already cover this.
		allFindingsContent, err := renderAllFindingsPanel(rows, defs, ro.Instance)
		if err != nil {
			return "", fmt.Errorf("render all-findings panel: %w", err)
		}
		allFindingsNav := `<button class="report-tab" data-report-tab="all-findings">All Findings</button>`
		allFindingsPanel := "\n  <section class=\"report-panel\" data-report-panel=\"all-findings\">\n" + allFindingsContent + "\n  </section>\n"

		relationshipsContent, err := renderRelationshipsPanel(rows, ro.Instance)
		if err != nil {
			return "", fmt.Errorf("render relationships panel: %w", err)
		}
		relationshipsNav := `<button class="report-tab" data-report-tab="relationships">Relationships</button>`
		relationshipsPanel := "\n  <section class=\"report-panel\" data-report-panel=\"relationships\">\n" + relationshipsContent + "\n  </section>\n"

		navTabsHTML = overviewNav + reposNav + navTabsHTML + allFindingsNav + relationshipsNav + wavesNav + migrationNav
		extraPanelsHTML = overviewPanel + extraPanelsHTML + allFindingsPanel + relationshipsPanel + wavesPanel + migrationPanel
	} else {
		navTabsHTML = reposNav + wavesNav + migrationNav
		extraPanelsHTML = wavesPanel + migrationPanel
	}

	var headerInstanceHTML string
	if ro.Instance != nil && ro.Instance.URL != "" {
		headerInstanceHTML = fmt.Sprintf(
			`<a class="header-instance" href="%s" target="_blank" rel="noopener noreferrer">%s</a>`,
			htmlAttr(ro.Instance.URL), htmlEscape(ro.Instance.URL))
	}

	replacer := strings.NewReplacer(
		"__CSS__", css,
		"__SOURCE__", htmlEscape(ro.SourceFile),
		"__STAT_CARDS__", statCardsHTML.String(),
		"__FINDING_CARDS__", findingCardsHTML.String(),
		"__ROW_COUNT__", fmt.Sprintf("%d", len(rows)),
		"__JSON_ROWS__", jsonRows,
		"__JSON_FINDINGS__", jsonFindingDefs,
		"__PAGE_SIZE__", fmt.Sprintf("%d", ro.PageSize),
		"__GENERATED_AT__", htmlEscape(ro.GeneratedAt.Format("2006-01-02 15:04:05 MST")),
		"__NAV_TABS__", navTabsHTML,
		"__EXTRA_PANELS__", extraPanelsHTML,
		"__REPOS_ACTIVE_CLASS__", reposActiveClass,
		"__HEADER_INSTANCE__", headerInstanceHTML,
	)
	return replacer.Replace(pageTemplate), nil
}

type statCard struct {
	Value string
	Label string
	Tone  string
}

type projectInfo struct {
	Key       string
	Name      string
	RepoCount int
	SizeGB    float64
	Findings  int
}

func findingCounts(rows []Row) map[string]int {
	counts := map[string]int{}
	for _, r := range rows {
		for _, f := range r.Findings {
			counts[f]++
		}
	}
	return counts
}

// buildJSONRows converts each Row into a JSON-friendly map: numeric columns
// become numbers (so the table sorts/formats correctly), everything else
// stays a string, and "findings" is added.
func buildJSONRows(rows []Row) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		obj := make(map[string]interface{}, len(r.Fields)+1)
		for k, v := range r.Fields {
			switch k {
			case "files_count":
				obj[k] = r.Int(k)
			case "usage_gb":
				obj[k] = r.Float(k)
			default:
				obj[k] = v
			}
		}
		findings := r.Findings
		if findings == nil {
			findings = []string{}
		}
		obj["findings"] = findings
		out = append(out, obj)
	}
	// Stable order for deterministic output.
	sort.Slice(out, func(i, j int) bool {
		return fmt.Sprint(out[i]["repo_name"]) < fmt.Sprint(out[j]["repo_name"])
	})
	return out
}

func jsonForHTML(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(b), "</", "<\\/"), nil
}

func toneClass(tone string) string {
	if tone == "" {
		return "info"
	}
	return tone
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(s)
}

func htmlAttr(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(s)
}

// renderSplitByProject generates separate HTML reports for each project
// and returns an index HTML file listing all projects.
func renderSplitByProject(header []string, rows []Row, ro RenderOptions, defs []FindingDef) (string, error) {
	// Group rows by project
	projectRows := make(map[string][]Row)
	projectNames := make(map[string]string)
	var projects []string

	for _, r := range rows {
		proj := r.Get("project_key")
		if proj == "" {
			proj = "default"
		}
		if len(projectRows[proj]) == 0 {
			projects = append(projects, proj)
		}
		projectRows[proj] = append(projectRows[proj], r)
	}
	sort.Strings(projects)

	// Get project display names from instance if available
	if ro.Instance != nil {
		for _, p := range ro.Instance.Projects {
			projectNames[p.Key] = p.DisplayName
		}
	}

	// Generate individual project reports
	var projectList []projectInfo

	for _, proj := range projects {
		projRows := projectRows[proj]
		displayName := projectNames[proj]
		if displayName == "" {
			displayName = proj
		}

		// Count findings and size for this project
		sizeGB := 0.0
		findingCount := 0
		for _, r := range projRows {
			sizeGB += r.Float("usage_gb")
			findingCount += len(r.Findings)
		}

		projectList = append(projectList, projectInfo{
			Key:       proj,
			Name:      displayName,
			RepoCount: len(projRows),
			SizeGB:    sizeGB,
			Findings:  findingCount,
		})

		// Render individual project report
		roProject := ro
		projectHTML, err := renderSingleReport(header, projRows, roProject, defs)
		if err != nil {
			return "", fmt.Errorf("render project %s: %w", proj, err)
		}

		// Write project report to file
		filename := fmt.Sprintf("%s/project-%s.html", ro.OutputDir, sanitizeFilename(proj))
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return "", fmt.Errorf("create directory for %s: %w", filename, err)
		}
		if err := writeFile(filename, projectHTML); err != nil {
			return "", fmt.Errorf("write project report %s: %w", filename, err)
		}
	}

	// Generate index HTML
	return generateIndexHTML(projectList, ro), nil
}

// renderSingleReport is the core rendering logic used for individual project reports
func renderSingleReport(header []string, rows []Row, ro RenderOptions, defs []FindingDef) (string, error) {
	counts := findingCounts(rows)

	totalUsageGB := 0.0
	packageTypes := map[string]int{}
	findingRepoCount := 0
	for _, r := range rows {
		totalUsageGB += r.Float("usage_gb")
		if pt := r.Get("package_type"); pt != "" {
			packageTypes[pt]++
		}
		if len(r.Findings) > 0 {
			findingRepoCount++
		}
	}

	jsonRows, err := jsonForHTML(buildJSONRows(rows))
	if err != nil {
		return "", fmt.Errorf("encoding rows: %w", err)
	}
	jsonFindingDefs, err := jsonForHTML(defs)
	if err != nil {
		return "", fmt.Errorf("encoding finding defs: %w", err)
	}

	statCards := []statCard{
		{fmt.Sprintf("%d", len(rows)), "repositories", ""},
		{fmt.Sprintf("%.2f TB", totalUsageGB/1024), "logical used space", "purple"},
		{fmt.Sprintf("%d", len(packageTypes)), "package types", "teal"},
		{fmt.Sprintf("%d", findingRepoCount), "repos with 1+ finding", "amber"},
	}

	var findingCardsHTML strings.Builder
	for _, d := range defs {
		fmt.Fprintf(&findingCardsHTML, `
    <button class="finding %s" data-finding="%s" data-explain="%s">
      <strong>%d</strong><h3>%s</h3>
    </button>`, toneClass(d.Tone), htmlAttr(d.ID), htmlAttr(d.Explain), counts[d.ID], htmlEscape(d.Label))
	}

	var statCardsHTML strings.Builder
	for _, s := range statCards {
		border := ""
		if s.Tone != "" {
			border = fmt.Sprintf(` style="border-top-color:var(--%s)"`, s.Tone)
		}
		fmt.Fprintf(&statCardsHTML, `
    <div class="stat"%s><strong>%s</strong><span>%s</span></div>`, border, htmlEscape(s.Value), htmlEscape(s.Label))
	}

	reposActiveClass := ""
	if ro.Instance == nil {
		reposActiveClass = " active"
	}

	reposNav := fmt.Sprintf(`<button class="report-tab%s" data-report-tab="repos">Repos</button>`, reposActiveClass)
	wavesNav := `<button class="report-tab" data-report-tab="waves">Wave Planning</button>`
	wavesPanel := "\n  <section class=\"report-panel\" data-report-panel=\"waves\">\n" + wavePlanningPanel + "\n  </section>\n"
	migrationNav := `<button class="report-tab" data-report-tab="migration">Migration Workflow</button>`
	migrationPanel := "\n  <section class=\"report-panel\" data-report-panel=\"migration\">\n" + migrationWorkflowPanel + "\n  </section>\n"

	navTabsHTML, extraPanelsHTML, err := renderInstanceTabs(ro.Instance)
	if err != nil {
		return "", err
	}

	var headerInstanceHTML string
	if ro.Instance != nil && ro.Instance.URL != "" {
		headerInstanceHTML = fmt.Sprintf(
			`<a class="header-instance" href="%s" target="_blank" rel="noopener noreferrer">%s</a>`,
			htmlAttr(ro.Instance.URL), htmlEscape(ro.Instance.URL))
	}

	if navTabsHTML != "" {
		overviewContent, err := renderOverviewPanel(ro.Instance, len(rows))
		if err != nil {
			return "", fmt.Errorf("render overview panel: %w", err)
		}
		overviewNav := `<button class="report-tab active" data-report-tab="overview">Overview</button>`
		overviewPanel := "\n  <section class=\"report-panel active\" data-report-panel=\"overview\">\n" + overviewContent + "\n  </section>\n"

		allFindingsContent, err := renderAllFindingsPanel(rows, defs, ro.Instance)
		if err != nil {
			return "", fmt.Errorf("render all-findings panel: %w", err)
		}
		allFindingsNav := `<button class="report-tab" data-report-tab="all-findings">All Findings</button>`
		allFindingsPanel := "\n  <section class=\"report-panel\" data-report-panel=\"all-findings\">\n" + allFindingsContent + "\n  </section>\n"

		relationshipsContent, err := renderRelationshipsPanel(rows, ro.Instance)
		if err != nil {
			return "", fmt.Errorf("render relationships panel: %w", err)
		}
		relationshipsNav := `<button class="report-tab" data-report-tab="relationships">Relationships</button>`
		relationshipsPanel := "\n  <section class=\"report-panel\" data-report-panel=\"relationships\">\n" + relationshipsContent + "\n  </section>\n"

		navTabsHTML = overviewNav + reposNav + navTabsHTML + allFindingsNav + relationshipsNav + wavesNav + migrationNav
		extraPanelsHTML = overviewPanel + extraPanelsHTML + allFindingsPanel + relationshipsPanel + wavesPanel + migrationPanel
	} else {
		navTabsHTML = reposNav + wavesNav + migrationNav
		extraPanelsHTML = wavesPanel + migrationPanel
	}

	replacer := strings.NewReplacer(
		"__CSS__", css,
		"__SOURCE__", htmlEscape(ro.SourceFile),
		"__STAT_CARDS__", statCardsHTML.String(),
		"__FINDING_CARDS__", findingCardsHTML.String(),
		"__ROW_COUNT__", fmt.Sprintf("%d", len(rows)),
		"__JSON_ROWS__", jsonRows,
		"__JSON_FINDINGS__", jsonFindingDefs,
		"__PAGE_SIZE__", fmt.Sprintf("%d", ro.PageSize),
		"__GENERATED_AT__", htmlEscape(ro.GeneratedAt.Format("2006-01-02 15:04:05 MST")),
		"__NAV_TABS__", navTabsHTML,
		"__EXTRA_PANELS__", extraPanelsHTML,
		"__REPOS_ACTIVE_CLASS__", reposActiveClass,
		"__HEADER_INSTANCE__", headerInstanceHTML,
	)
	return replacer.Replace(pageTemplate), nil
}

// generateIndexHTML creates a landing page listing all projects with links to their reports
func generateIndexHTML(projects []projectInfo, ro RenderOptions) string {
	var html strings.Builder
	html.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Discovery Analysis - Project Reports</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
      background: #f4f7fb;
      color: #172033;
      padding: 40px 20px;
    }
    .container { max-width: 1200px; margin: 0 auto; }
    header {
      background: linear-gradient(135deg, #1769e0 0%, #6941c6 100%);
      color: white;
      padding: 40px;
      border-radius: 12px;
      margin-bottom: 40px;
      text-align: center;
    }
    header h1 { font-size: 2em; margin-bottom: 10px; }
    header p { opacity: 0.9; margin: 5px 0; }
    .stats-summary {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
      gap: 15px;
      margin-top: 20px;
    }
    .stat-box {
      background: rgba(255,255,255,0.15);
      padding: 15px;
      border-radius: 8px;
      text-align: center;
    }
    .stat-box .value { font-size: 1.8em; font-weight: bold; }
    .stat-box .label { font-size: 0.9em; opacity: 0.9; }
    .projects-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
      gap: 20px;
    }
    .project-card {
      background: white;
      border: 1px solid #d8e0ec;
      border-radius: 12px;
      padding: 20px;
      transition: transform 0.2s, box-shadow 0.2s;
      cursor: pointer;
      text-decoration: none;
      color: inherit;
      display: block;
    }
    .project-card:hover {
      transform: translateY(-4px);
      box-shadow: 0 10px 25px rgba(0,0,0,0.1);
    }
    .project-card h3 {
      color: #1769e0;
      margin-bottom: 10px;
      font-size: 1.2em;
    }
    .project-card p {
      font-size: 0.9em;
      color: #667085;
      margin: 8px 0;
    }
    .project-key {
      font-family: monospace;
      font-size: 0.85em;
      background: #f0f3f7;
      padding: 4px 8px;
      border-radius: 4px;
      display: inline-block;
      margin-top: 10px;
    }
    .footer {
      text-align: center;
      color: #667085;
      margin-top: 40px;
      font-size: 0.9em;
    }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>Discovery Analysis - Project Reports</h1>
      <p>Select a project to view its repository analysis</p>
      <div class="stats-summary">
`)

	totalRepos := 0
	totalSize := 0.0
	totalFindings := 0
	for _, p := range projects {
		totalRepos += p.RepoCount
		totalSize += p.SizeGB
		totalFindings += p.Findings
	}

	html.WriteString(fmt.Sprintf(`
        <div class="stat-box">
          <div class="value">%d</div>
          <div class="label">Projects</div>
        </div>
        <div class="stat-box">
          <div class="value">%d</div>
          <div class="label">Repositories</div>
        </div>
        <div class="stat-box">
          <div class="value">%.2f TB</div>
          <div class="label">Total Size</div>
        </div>
        <div class="stat-box">
          <div class="value">%d</div>
          <div class="label">Findings</div>
        </div>
`, len(projects), totalRepos, totalSize/1024, totalFindings))

	html.WriteString(`
      </div>
    </header>

    <h2 style="margin-bottom: 20px;">Projects</h2>
    <div class="projects-grid">
`)

	for _, p := range projects {
		html.WriteString(fmt.Sprintf(`
      <a href="project-%s.html" class="project-card">
        <h3>%s</h3>
        <p><strong>%d</strong> repositories</p>
        <p><strong>%.2f GB</strong> logical size</p>
        <p><strong>%d</strong> findings</p>
        <div class="project-key">%s</div>
      </a>
`, sanitizeFilename(p.Key), htmlEscape(p.Name), p.RepoCount, p.SizeGB, p.Findings, htmlEscape(p.Key)))
	}

	html.WriteString(`
    </div>

    <div class="footer">
      <p>Generated on ` + ro.GeneratedAt.Format("2006-01-02 15:04:05 MST") + `</p>
      <p>JFrog PS CLI Discovery Analysis</p>
    </div>
  </div>
</body>
</html>`)

	return html.String()
}

// sanitizeFilename converts project keys to safe filenames
func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "_",
	)
	return replacer.Replace(s)
}

// writeFile writes content to a file
func writeFile(filename string, content string) error {
	return os.WriteFile(filename, []byte(content), 0644)
}
