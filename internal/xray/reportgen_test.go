package xray

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"jfrogpscli/internal/jfclient"
)

// zipCSV wraps csvBody as the single-file ZIP that
// GET /xray/api/v1/reports/export/{id} returns.
func zipCSV(t *testing.T, name, csvBody string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name + ".csv")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := f.Write([]byte(csvBody)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// reportLifecycleServer returns a mux handling the full
// create/poll/export/delete lifecycle for one report, plus reponame-scoped
// resolution (no watch/project calls needed). deleteCalled records whether
// the cleanup DELETE was actually attempted at the HTTP layer (it may still
// be rejected by the write guard before reaching here).
func reportLifecycleServer(t *testing.T, csvBody string, deleteCalled *bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/reports/violations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"report_id": 42}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/42", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"status":"completed","name":"violationreport_x"}`))
		case http.MethodDelete:
			*deleteCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method %s on /reports/42", r.Method)
		}
	})
	mux.HandleFunc("/xray/api/v1/reports/export/42", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipCSV(t, "violationreport_x", csvBody))
	})
	return httptest.NewServer(mux)
}

const sampleViolationsCSV = `repository,impacted_artifact,severity
repo-a,repo-a/path/one.jar,High
repo-a,repo-a/path/one.jar,Critical
repo-a,repo-a/path/two.jar,Low
`

func TestGenerateReportsHappyPathDefaultWritesDisabled(t *testing.T) {
	// Deliberately do NOT flip jfclient.WritesDisabled here: report
	// generation must work even with the default-safe write guard on,
	// since POST /reports/violations is exempted and the cleanup DELETE
	// failing is non-fatal.
	var deleteAttempted bool
	srv := reportLifecycleServer(t, sampleViolationsCSV, &deleteAttempted)
	defer srv.Close()

	outDir := t.TempDir()
	client := jfclient.New(srv.URL, "tok", "", "", false)
	results, warnings, err := GenerateReports(client, ReportGenOptions{
		ReportType: ReportTypeViolations,
		Scope:      ReportScope{Reponame: "repo-a"},
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if res.RowCount != 3 {
		t.Fatalf("expected 3 data rows, got %d", res.RowCount)
	}
	if res.CSVPath == "" {
		t.Fatalf("expected a CSV path to be recorded")
	}
	if _, err := os.Stat(res.CSVPath); err != nil {
		t.Fatalf("expected CSV file to exist at %s: %v", res.CSVPath, err)
	}
	if deleteAttempted {
		t.Fatalf("write guard should have blocked the cleanup DELETE from reaching the server")
	}
	if !strings.Contains(res.Warning, "could not delete transient report") {
		t.Fatalf("expected a non-fatal delete-cleanup warning, got %q", res.Warning)
	}
}

func TestGenerateReportsDeletesCleanlyWhenWritesEnabled(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var deleteAttempted bool
	srv := reportLifecycleServer(t, sampleViolationsCSV, &deleteAttempted)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results, _, err := GenerateReports(client, ReportGenOptions{
		ReportType: ReportTypeViolations,
		Scope:      ReportScope{Reponame: "repo-a"},
		OutputDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleteAttempted {
		t.Fatalf("expected the cleanup DELETE to reach the server once writes are enabled")
	}
	if results[0].Warning != "" {
		t.Fatalf("expected no warning, got %q", results[0].Warning)
	}
}

func TestGenerateReportsEmptyCSVProducesNoFile(t *testing.T) {
	var deleteAttempted bool
	srv := reportLifecycleServer(t, "repository,impacted_artifact,severity\n", &deleteAttempted)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results, _, err := GenerateReports(client, ReportGenOptions{
		ReportType: ReportTypeViolations,
		Scope:      ReportScope{Reponame: "repo-a"},
		OutputDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].CSVPath != "" {
		t.Fatalf("expected no CSV file for a header-only export, got %q", results[0].CSVPath)
	}
	if !strings.Contains(results[0].Warning, "no data rows") {
		t.Fatalf("expected a 'no data rows' warning, got %q", results[0].Warning)
	}
}

func TestGenerateReportsRequiresExactlyOneScope(t *testing.T) {
	client := jfclient.New("http://example.invalid", "tok", "", "", false)
	if _, _, err := GenerateReports(client, ReportGenOptions{ReportType: ReportTypeViolations}); err == nil {
		t.Fatalf("expected an error when no scope is given")
	}
	if _, _, err := GenerateReports(client, ReportGenOptions{
		ReportType: ReportTypeViolations,
		Scope:      ReportScope{Reponame: "a", Project: "b"},
	}); err == nil {
		t.Fatalf("expected an error when reponame and project are both given")
	}
}

func TestGenerateReportsNonCompletedStatusIsAnError(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var deleteAttempted bool
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/reports/violations", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"report_id": 7}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/7", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteAttempted = true
			return
		}
		_, _ = w.Write([]byte(`{"status":"failed","name":"x"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	_, _, err := GenerateReports(client, ReportGenOptions{
		ReportType: ReportTypeViolations,
		Scope:      ReportScope{Reponame: "repo-a"},
	})
	if err == nil || !strings.Contains(err.Error(), `status "failed"`) {
		t.Fatalf("expected a status=failed error, got %v", err)
	}
	if !deleteAttempted {
		t.Fatalf("expected best-effort cleanup delete to be attempted even for a non-completed report")
	}
}

func TestGenerateReportsChunksByDaysPerChunk(t *testing.T) {
	var createCount int
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/reports/violations", func(w http.ResponseWriter, r *http.Request) {
		var req reportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Filters.Updated == nil || req.Filters.Updated.Start == "" || req.Filters.Updated.End == "" {
			t.Fatalf("expected a populated Updated range in chunk %d, got %+v", createCount, req.Filters.Updated)
		}
		createCount++
		fmt.Fprintf(w, `{"report_id": %d}`, createCount)
	})
	mux.HandleFunc("/xray/api/v1/reports/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/xray/api/v1/reports/export/"):
			_, _ = w.Write(zipCSV(t, "chunk", sampleViolationsCSV))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			_, _ = w.Write([]byte(`{"status":"completed","name":"chunk"}`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results, _, err := GenerateReports(client, ReportGenOptions{
		ReportType:   ReportTypeViolations,
		Scope:        ReportScope{Reponame: "repo-a"},
		UpdatedStart: "2024-01-01T00:00:00Z",
		UpdatedEnd:   "2024-01-21T00:00:00Z",
		DaysPerChunk: 7,
		OutputDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 chunks (21 days / 7-day chunks), got %d", len(results))
	}
	if createCount != 3 {
		t.Fatalf("expected 3 report-creation calls, got %d", createCount)
	}
	for i, r := range results {
		if r.ChunkStart == "" || r.ChunkEnd == "" {
			t.Fatalf("chunk %d missing start/end: %+v", i, r)
		}
	}
}

func TestChunkWindowsNoChunkingWhenDaysPerChunkZero(t *testing.T) {
	windows, err := chunkWindows("2024-01-01T00:00:00Z", "2024-01-21T00:00:00Z", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
}

func TestChunkWindowsSplitsEvenly(t *testing.T) {
	windows, err := chunkWindows("2024-01-01T00:00:00Z", "2024-01-15T00:00:00Z", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d: %v", len(windows), windows)
	}
	if windows[1][1] != "2024-01-15T00:00:00Z" {
		t.Fatalf("expected last window to end exactly at the requested end, got %v", windows[1])
	}
}

func TestBuildFiltersWarnsOnIgnoredExposuresFlags(t *testing.T) {
	opts := ReportGenOptions{
		ReportType:       ReportTypeExposures,
		ExposureCategory: "secrets",
		Severities:       []string{"High"},
		Scope:            ReportScope{Watchname: "w1"},
	}
	filters, warnings := buildFilters(opts, "", "")
	if filters.Category != "secrets" {
		t.Fatalf("expected category=secrets, got %q", filters.Category)
	}
	if filters.Severities != nil || filters.WatchNames != nil {
		t.Fatalf("expected no severities/watch_names fields on an exposures filter, got %+v", filters)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings (severities + watchname ignored), got %v", warnings)
	}
}

func TestResolveReportScopeReponame(t *testing.T) {
	client := jfclient.New("http://example.invalid", "tok", "", "", false)
	resources, err := ResolveReportScope(client, ReportScope{Reponame: "repo-a"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources.Repositories) != 1 || resources.Repositories[0].Name != "repo-a" {
		t.Fatalf("unexpected resources: %+v", resources)
	}
}

func TestResolveReportScopeBuildnameRequiresBuildRepo(t *testing.T) {
	client := jfclient.New("http://example.invalid", "tok", "", "", false)
	if _, err := ResolveReportScope(client, ReportScope{Buildname: "b1"}, 5); err == nil {
		t.Fatalf("expected an error when --buildname is given without --build-repo")
	}
}

func TestResolveReportScopeProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("project"); got != "proj1" {
			t.Fatalf("expected project=proj1, got %q", got)
		}
		_, _ = w.Write([]byte(`[{"key":"repo-a"},{"key":"repo-b"}]`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	resources, err := ResolveReportScope(client, ReportScope{Project: "proj1"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources.Repositories) != 2 {
		t.Fatalf("expected 2 repos, got %+v", resources.Repositories)
	}
}

func TestResolveReportScopeWatchRejectsUnsupportedType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"all-repos","name":"*"}]}}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	if _, err := ResolveReportScope(client, ReportScope{Watchname: "w1"}, 5); err == nil {
		t.Fatalf("expected an error for a watch scoped to an unsupported wildcard type")
	}
}

func TestResolveReportScopeWatchProjectResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v2/watches/w1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"project","name":"proj1"}]}}`))
	})
	mux.HandleFunc("/artifactory/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"key":"repo-a"}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	resources, err := ResolveReportScope(client, ReportScope{Watchname: "w1"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources.Repositories) != 1 || resources.Repositories[0].Name != "repo-a" {
		t.Fatalf("unexpected resources: %+v", resources)
	}
}

func TestResolveReportScopeWatchRepositoryResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"repository","name":"repo-a"},{"type":"repository","name":"repo-b"}]}}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	resources, err := ResolveReportScope(client, ReportScope{Watchname: "w1"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources.Repositories) != 2 {
		t.Fatalf("expected 2 repositories, got %+v", resources.Repositories)
	}
}

func TestPollReportRetriesUntilTerminal(t *testing.T) {
	orig := reportPollInterval
	reportPollInterval = time.Millisecond
	t.Cleanup(func() { reportPollInterval = orig })

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			_, _ = w.Write([]byte(`{"status":"running","name":"x"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed","name":"x"}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	status, err := pollReport(client, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "completed" {
		t.Fatalf("expected completed, got %q", status)
	}
	if calls != 3 {
		t.Fatalf("expected 3 polls, got %d", calls)
	}
}
