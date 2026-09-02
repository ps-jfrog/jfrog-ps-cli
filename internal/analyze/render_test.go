package analyze

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRender_ProducesValidEmbeddedJSONAndNoLeftoverPlaceholders(t *testing.T) {
	header := []string{"repo_name", "package_type", "repo_type", "files_count", "usage_gb"}
	rows := []Row{
		mkRow(map[string]string{"repo_name": "docker-local", "package_type": "Docker", "repo_type": "LOCAL", "files_count": "0", "usage_gb": "0"}),
	}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	html, err := Render(header, rows, RenderOptions{
		SourceFile:  "report.csv",
		Opts:        DefaultOptions(),
		GeneratedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if m := regexp.MustCompile(`__[A-Z_]+__`).FindString(html); m != "" {
		t.Fatalf("found unreplaced template placeholder: %q", m)
	}
	// The shared-helpers block (escapeHtml/mountTable/etc., which must load
	// before <main> - see the ordering bug this guards against), the Wave
	// Planning and Migration Workflow panels' own scripts (both always
	// present, even without instance data), and the main repos-panel
	// script - four total.
	if strings.Count(html, "</script>") != 4 {
		t.Fatalf("expected exactly four </script> tags, got %d", strings.Count(html, "</script>"))
	}

	dataJSON := extractBetween(t, html, "const DATA = ", ";\nconst FINDING_DEFS")
	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		t.Fatalf("embedded DATA is not valid JSON: %v\n%s", err, dataJSON)
	}
	if len(data) != 1 || data[0]["repo_name"] != "docker-local" {
		t.Fatalf("unexpected embedded DATA: %+v", data)
	}
}

func TestRender_SkipsUncomputedFindingDefs(t *testing.T) {
	header := []string{"repo_name", "files_count"} // no policy/activity columns
	rows := []Row{mkRow(map[string]string{"repo_name": "a", "files_count": "0"})}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	html, err := Render(header, rows, RenderOptions{SourceFile: "x.csv", Opts: DefaultOptions(), GeneratedAt: time.Now()})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if strings.Contains(html, `data-finding="no-cleanup-policy"`) {
		t.Fatalf("expected no-cleanup-policy finding card to be omitted without its source column")
	}
}

func TestRender_ReposTabHasProjectAndDateRangeFilters(t *testing.T) {
	header := []string{"repo_name", "package_type", "repo_type", "project_key", "files_count", "usage_gb", "last_activity_date"}
	rows := []Row{
		mkRow(map[string]string{"repo_name": "docker-local", "package_type": "Docker", "repo_type": "LOCAL", "project_key": "team-a", "files_count": "0", "usage_gb": "0", "last_activity_date": "2024-06-01T00:00:00Z"}),
	}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	html, err := Render(header, rows, RenderOptions{SourceFile: "report.csv", Opts: DefaultOptions(), GeneratedAt: time.Now()})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	for _, want := range []string{
		`id="project"`, `id="date-field"`, `id="date-from"`, `id="date-to"`,
		`["project_key", "Project"]`, "function rebuildProjectOptions", "function matchesDateRange",
		"const usesProjects", "projectSelect.addEventListener", "dateFromInput.addEventListener", "dateToInput.addEventListener",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected the Repos tab to contain %q, it did not", want)
		}
	}
}

func TestHTMLEscapeAndAttr(t *testing.T) {
	if got := htmlEscape(`<b>a & b</b>`); got != "&lt;b&gt;a &amp; b&lt;/b&gt;" {
		t.Errorf("htmlEscape unexpected: %q", got)
	}
	if got := htmlAttr(`say "hi" <script>`); !strings.Contains(got, "&quot;") || strings.Contains(got, "<script>") {
		t.Errorf("htmlAttr did not escape properly: %q", got)
	}
}

func extractBetween(t *testing.T, s, start, end string) string {
	t.Helper()
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatalf("marker %q not found in output", start)
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("end marker %q not found in output", end)
	}
	return rest[:j]
}
