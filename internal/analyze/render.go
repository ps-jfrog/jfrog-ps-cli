package analyze

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"jfrogpscli/internal/instance"
)

// RenderOptions carries the rendering-time context Render needs beyond the
// parsed rows themselves.
type RenderOptions struct {
	SourceFile  string
	Opts        Options
	GeneratedAt time.Time
	// Instance, if non-nil, adds Overview/Access/Projects/Federation &
	// Replication tabs built from a "discover instance" snapshot. Nil
	// renders exactly the single-panel report this package always has.
	Instance *instance.Snapshot
}

// Render builds a self-contained interactive HTML page from header/rows
// (as returned by ParseCSV, after ComputeFindings has run).
func Render(header []string, rows []Row, ro RenderOptions) (string, error) {
	defs := ro.Opts.Defs(header)
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

	navTabsHTML, extraPanelsHTML, err := renderInstanceTabs(ro.Instance)
	if err != nil {
		return "", err
	}
	if navTabsHTML != "" {
		// Order: Overview, Repos, [Projects/Access/Fedrep/Platform/Xray from
		// renderInstanceTabs], All Findings, Relationships. Overview is built
		// here rather than by renderInstanceTabs because it needs the repo
		// count, which isn't part of an instance.Snapshot.
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

		navTabsHTML = overviewNav +
			`<button class="report-tab" data-report-tab="repos">Repos</button>` +
			navTabsHTML + allFindingsNav + relationshipsNav
		extraPanelsHTML = overviewPanel + extraPanelsHTML + allFindingsPanel + relationshipsPanel
	}

	// Overview is the default tab once there's an instance snapshot to show
	// one for; without it, Repos (the only tab that always exists) stays
	// the default, exactly as before this report had an Overview tab.
	reposActiveClass := ""
	if ro.Instance == nil {
		reposActiveClass = " active"
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
