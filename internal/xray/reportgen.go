package xray

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// Polling cadence for report generation -- vars (not consts) so tests can
// shrink them instead of waiting on real wall-clock time.
var (
	reportPollInterval = 10 * time.Second
	reportPollTimeout  = 120 * time.Minute
)

// GeneratedReport is the outcome of generating and exporting one report (one
// per date-range chunk when --days-per-chunk is used).
type GeneratedReport struct {
	Name       string
	ReportID   int
	ChunkStart string
	ChunkEnd   string
	CSVPath    string // "" if the report produced no data rows
	CSVData    []byte // in-memory copy of the exported CSV, "" if empty
	RowCount   int
	Warning    string // e.g. a failed best-effort cleanup delete, or "no rows"

	GitRepoSegregatedCSVPath string   // set when --build-gitrepo-report produced output
	UploadedPaths            []string // set when --report-upload-repo was given
}

// appendWarning joins add onto existing with "; ", or just returns add if
// existing is empty.
func appendWarning(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "; " + add
}

// createReport issues POST /xray/api/v1/reports/{reportType} and returns the
// new report's ID.
func createReport(client *jfclient.Client, reportType string, req reportRequest) (int, error) {
	resp, err := client.Post("/xray/api/v1/reports/"+reportType, req)
	if err != nil {
		return 0, err
	}
	if !resp.OK() {
		switch resp.StatusCode {
		case 401:
			return 0, fmt.Errorf("create %s report: invalid or expired token (HTTP 401)", reportType)
		case 403:
			return 0, fmt.Errorf("create %s report: token lacks Advanced Security entitlement or Manage Reports permission (HTTP 403)", reportType)
		case 404:
			return 0, fmt.Errorf("create %s report: Advanced Security is not enabled on this instance (HTTP 404)", reportType)
		default:
			return 0, resp.Error("create " + reportType + " report")
		}
	}
	var out createReportResponse
	if err := resp.Unmarshal(&out); err != nil {
		return 0, fmt.Errorf("decode create-report response: %w", err)
	}
	return out.ReportID, nil
}

// pollReport polls GET /xray/api/v1/reports/{id} every reportPollInterval
// until it reaches a terminal status (completed/aborted/failed) or
// reportPollTimeout elapses. A transient error on a single poll (including
// the request's own server-side ?timeout=30) is retried at the same
// interval rather than aborting the whole wait.
func pollReport(client *jfclient.Client, reportID int) (string, error) {
	deadline := time.Now().Add(reportPollTimeout)
	for {
		resp, err := client.Get(fmt.Sprintf("/xray/api/v1/reports/%d?timeout=30", reportID))
		if err == nil {
			if statusErr := resp.Error("poll report"); statusErr == nil {
				var out reportStatusResponse
				if decErr := resp.Unmarshal(&out); decErr == nil {
					switch out.Status {
					case reportStatusCompleted, reportStatusAborted, reportStatusFailed:
						return out.Status, nil
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("timed out after %s waiting for report %d to complete", reportPollTimeout, reportID)
		}
		time.Sleep(reportPollInterval)
	}
}

// exportReportZIP issues GET /xray/api/v1/reports/export/{id} and returns
// the raw ZIP response body.
func exportReportZIP(client *jfclient.Client, reportID int, fileName string) ([]byte, error) {
	path := fmt.Sprintf("/xray/api/v1/reports/export/%d?file_name=%s&format=csv", reportID, url.QueryEscape(fileName))
	resp, err := client.Get(path)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("export report"); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// unzipAndParseCSV extracts the single CSV file inside a report export ZIP
// and parses it with encoding/csv. A CSV with a header row only (or no
// rows at all) is treated as empty (nil data, 0 rows), matching the source
// script's "discard header-only exports" behavior.
func unzipAndParseCSV(zipData []byte) (data []byte, records [][]string, err error) {
	if len(zipData) == 0 {
		return nil, nil, nil
	}
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, fmt.Errorf("open report zip: %w", err)
	}
	var f *zip.File
	for _, zf := range zr.File {
		if strings.HasSuffix(strings.ToLower(zf.Name), ".csv") {
			f = zf
			break
		}
	}
	if f == nil && len(zr.File) > 0 {
		f = zr.File[0]
	}
	if f == nil {
		return nil, nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("open csv in zip: %w", err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, fmt.Errorf("read csv in zip: %w", err)
	}

	reader := csv.NewReader(bytes.NewReader(raw))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	parsed, err := reader.ReadAll()
	if err != nil {
		// Malformed CSV: surface the raw bytes rather than silently
		// dropping the export, but report zero parsed rows.
		return raw, nil, nil
	}
	if len(parsed) <= 1 {
		return nil, nil, nil
	}
	return raw, parsed, nil
}

// deleteReportBestEffort deletes a transient report via
// DELETE /xray/api/v1/reports/{id}. This goes through the normal write
// guard; a failure (including "writes disabled") is never fatal to the
// caller -- it's reported as a warning string, matching the source
// script's "always attempt cleanup, never fail the run over it" behavior.
func deleteReportBestEffort(client *jfclient.Client, reportID int) string {
	resp, err := client.Delete(fmt.Sprintf("/xray/api/v1/reports/%d", reportID))
	if err != nil {
		return fmt.Sprintf("could not delete transient report %d (%v) -- it will remain on the server", reportID, err)
	}
	if !resp.OK() {
		return fmt.Sprintf("could not delete transient report %d (%v) -- it will remain on the server", reportID, resp.Error("delete report"))
	}
	return ""
}

// sanitizeForFilename replaces characters that are awkward in filenames
// (notably ":" in RFC3339 timestamps) with "-".
func sanitizeForFilename(s string) string {
	return strings.NewReplacer(":", "-", "/", "-").Replace(s)
}

// reportName builds a report name following the source scripts' convention:
// "<prefix>_<start>_to_<end>" when both a start and end are given, else
// "<prefix>_<YYYYMMDDHHMMSS>".
func reportName(reportType, start, end string) string {
	prefix := reportNamePrefix(reportType)
	if start != "" && end != "" {
		return fmt.Sprintf("%s_%s_to_%s", prefix, sanitizeForFilename(start), sanitizeForFilename(end))
	}
	return fmt.Sprintf("%s_%s", prefix, time.Now().UTC().Format("20060102150405"))
}

// chunkableRange returns the (start, end) date-range flags that
// --days-per-chunk chunks: --updated-start/--updated-end for violations,
// --scan-date-start/--scan-date-end for vulnerabilities and exposures.
func chunkableRange(opts ReportGenOptions) (string, string) {
	if opts.ReportType == ReportTypeViolations {
		return opts.UpdatedStart, opts.UpdatedEnd
	}
	return opts.ScanDateStart, opts.ScanDateEnd
}

// chunkWindows splits [startStr,endStr] into daysPerChunk-day windows. If
// daysPerChunk <= 0, or either bound is empty, it returns the single window
// [startStr,endStr] unchanged (chunking is a no-op).
func chunkWindows(startStr, endStr string, daysPerChunk int) ([][2]string, error) {
	if daysPerChunk <= 0 || startStr == "" || endStr == "" {
		return [][2]string{{startStr, endStr}}, nil
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, fmt.Errorf("parse start date %q: %w", startStr, err)
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, fmt.Errorf("parse end date %q: %w", endStr, err)
	}
	if !end.After(start) {
		return nil, fmt.Errorf("end date %q must be after start date %q", endStr, startStr)
	}

	var windows [][2]string
	step := time.Duration(daysPerChunk) * 24 * time.Hour
	for cur := start; cur.Before(end); {
		next := cur.Add(step)
		if next.After(end) {
			next = end
		}
		windows = append(windows, [2]string{cur.Format(time.RFC3339), next.Format(time.RFC3339)})
		cur = next
	}
	return windows, nil
}

// buildFilters constructs the "filters" object for one report-generation
// request, applying rangeStart/rangeEnd to whichever date field this report
// type actually uses (see chunkableRange). It returns any user-visible
// warnings about flags that don't apply to opts.ReportType (e.g.
// --severities on an exposures report).
func buildFilters(opts ReportGenOptions, rangeStart, rangeEnd string) (reportFilters, []string) {
	var warnings []string
	switch opts.ReportType {
	case ReportTypeExposures:
		f := reportFilters{Category: opts.ExposureCategory, ImpactedArtifact: opts.ExposureImpactedArtifact}
		if rangeStart != "" || rangeEnd != "" {
			f.ScanDate = &dateRange{Start: rangeStart, End: rangeEnd}
		}
		if len(opts.Severities) > 0 {
			warnings = append(warnings, "--severities is ignored for exposures reports (no matching filter field)")
		}
		if opts.Scope.Watchname != "" {
			warnings = append(warnings, "--watchname is used only to resolve scope for exposures reports (no matching filter field to apply it as)")
		}
		return f, warnings
	case ReportTypeVulnerabilities:
		f := reportFilters{Type: "security"}
		if len(opts.Severities) > 0 {
			f.Severities = opts.Severities
		}
		if opts.HasRemediation != nil {
			f.HasRemediation = opts.HasRemediation
		}
		f.CVE = opts.CVE
		f.IssueID = opts.IssueID
		if opts.CVSSMinScore != 0 || opts.CVSSMaxScore != 0 {
			f.CVSSScore = &cvssRange{MinScore: opts.CVSSMinScore, MaxScore: opts.CVSSMaxScore}
		}
		if opts.PublishedStart != "" || opts.PublishedEnd != "" {
			f.Published = &dateRange{Start: opts.PublishedStart, End: opts.PublishedEnd}
		}
		if rangeStart != "" || rangeEnd != "" {
			f.ScanDate = &dateRange{Start: rangeStart, End: rangeEnd}
		}
		return f, warnings
	default: // violations
		f := reportFilters{Type: "security"}
		if len(opts.Severities) > 0 {
			f.Severities = opts.Severities
		}
		if opts.Scope.Watchname != "" {
			f.WatchNames = []string{opts.Scope.Watchname}
		}
		if rangeStart != "" || rangeEnd != "" {
			f.Updated = &dateRange{Start: rangeStart, End: rangeEnd}
		}
		return f, warnings
	}
}

// writeCSVFile writes data to <outputDir>/<filename> (outputDir defaults to
// the current directory), creating outputDir if needed.
func writeCSVFile(outputDir, filename string, data []byte) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", outputDir, err)
	}
	path := filepath.Join(outputDir, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// generateOneReport runs the full generate -> poll -> export -> (best
// effort) delete lifecycle for a single report/chunk and writes its CSV to
// disk unless it came back empty.
func generateOneReport(client *jfclient.Client, opts ReportGenOptions, resources reportResources, filters reportFilters, name, chunkStart, chunkEnd string) (GeneratedReport, error) {
	result := GeneratedReport{Name: name, ChunkStart: chunkStart, ChunkEnd: chunkEnd}

	reportID, err := createReport(client, opts.ReportType, reportRequest{Name: name, Resources: resources, Filters: filters})
	if err != nil {
		return result, err
	}
	result.ReportID = reportID

	status, err := pollReport(client, reportID)
	if err != nil {
		result.Warning = deleteReportBestEffort(client, reportID)
		return result, err
	}
	if status != reportStatusCompleted {
		result.Warning = deleteReportBestEffort(client, reportID)
		return result, fmt.Errorf("report %q ended with status %q", name, status)
	}

	zipData, exportErr := exportReportZIP(client, reportID, name)
	deleteWarning := deleteReportBestEffort(client, reportID)
	if exportErr != nil {
		result.Warning = deleteWarning
		return result, fmt.Errorf("export report %q: %w", name, exportErr)
	}

	raw, records, err := unzipAndParseCSV(zipData)
	if err != nil {
		result.Warning = deleteWarning
		return result, err
	}
	result.Warning = deleteWarning

	if len(records) <= 1 {
		note := "report produced no data rows"
		if result.Warning != "" {
			result.Warning += "; " + note
		} else {
			result.Warning = note
		}
		return result, nil
	}

	result.CSVData = raw
	result.RowCount = len(records) - 1
	path, err := writeCSVFile(opts.OutputDir, name+".csv", raw)
	if err != nil {
		return result, err
	}
	result.CSVPath = path
	return result, nil
}

// GenerateReports resolves scope, builds filters, and runs
// generateOneReport once per date-range chunk (a single "chunk" covering
// the whole requested range, or the whole (unbounded) request, when
// --days-per-chunk isn't used). It returns every chunk's result even if a
// later chunk fails, plus any type-vs-flag warnings from buildFilters.
func GenerateReports(client *jfclient.Client, opts ReportGenOptions, onProgress ...progress.Reporter) ([]GeneratedReport, []string, error) {
	resources, err := ResolveReportScope(client, opts.Scope, opts.BuildNumberOfVersions)
	if err != nil {
		return nil, nil, err
	}

	rangeStart, rangeEnd := chunkableRange(opts)
	windows, err := chunkWindows(rangeStart, rangeEnd, opts.DaysPerChunk)
	if err != nil {
		return nil, nil, err
	}

	var allWarnings []string
	var results []GeneratedReport
	for i, w := range windows {
		filters, warnings := buildFilters(opts, w[0], w[1])
		if len(warnings) > 0 && len(results) == 0 {
			// Only surface these once, not once per chunk.
			allWarnings = append(allWarnings, warnings...)
		}
		name := reportName(opts.ReportType, w[0], w[1])
		res, genErr := generateOneReport(client, opts, resources, filters, name, w[0], w[1])
		if genErr == nil && len(res.CSVData) > 0 {
			applyPostGenerateSteps(client, opts, resources, &res)
		}
		results = append(results, res)
		if genErr != nil {
			return results, allWarnings, fmt.Errorf("chunk %q: %w", name, genErr)
		}
		progress.Report(onProgress, i+1, len(windows))
	}
	return results, allWarnings, nil
}

// applyPostGenerateSteps runs --build-gitrepo-report segregation and
// --report-upload-repo upload against one already-generated, non-empty
// report, recording any failures as warnings on res rather than failing the
// whole run -- a generated report is still useful even if a downstream
// enrichment step fails.
func applyPostGenerateSteps(client *jfclient.Client, opts ReportGenOptions, resources reportResources, res *GeneratedReport) {
	if opts.BuildGitRepoReport {
		records, err := parseCSVRecords(res.CSVData)
		if err != nil {
			res.Warning = appendWarning(res.Warning, "build-gitrepo segregation skipped: parse CSV: "+err.Error())
		} else {
			rows, ran, serr := SegregateByBuildGitRepo(client, resources, opts.Scope.Watchname, records)
			switch {
			case serr != nil:
				res.Warning = appendWarning(res.Warning, "build-gitrepo segregation failed: "+serr.Error())
			case !ran:
				res.Warning = appendWarning(res.Warning, "build-gitrepo segregation skipped: report is not build-scoped, or its CSV has no impacted-artifact column")
			default:
				path, werr := writeGitRepoSegregatedCSV(opts.OutputDir, res.Name, rows)
				if werr != nil {
					res.Warning = appendWarning(res.Warning, "write build-gitrepo CSV failed: "+werr.Error())
				} else {
					res.GitRepoSegregatedCSVPath = path
				}
			}
		}
	}

	if opts.ReportUploadRepo != "" {
		if target, uerr := UploadReportCSV(client, opts.ReportUploadRepo, opts.Scope.Project, res.Name+".csv", res.CSVData); uerr != nil {
			res.Warning = appendWarning(res.Warning, "upload source CSV failed: "+uerr.Error())
		} else {
			res.UploadedPaths = append(res.UploadedPaths, target)
		}
		if res.GitRepoSegregatedCSVPath != "" {
			if data, rerr := os.ReadFile(res.GitRepoSegregatedCSVPath); rerr == nil {
				if target, uerr := UploadReportCSV(client, opts.ReportUploadRepo, opts.Scope.Project, filepath.Base(res.GitRepoSegregatedCSVPath), data); uerr == nil {
					res.UploadedPaths = append(res.UploadedPaths, target)
				}
			}
		}
	}
}

// UploadReportCSV uploads data (a generated report's CSV) to
// <repo>/<projectKey>/<YYYY-MM-DD>/<filename> via a plain
// PUT /artifactory/<target path> (no `jf` CLI shellout -- jfps already has
// its own HTTP client). projectKey may be empty, in which case that path
// segment is omitted.
func UploadReportCSV(client *jfclient.Client, repo, projectKey, filename string, data []byte) (string, error) {
	segments := []string{strings.Trim(repo, "/")}
	if projectKey != "" {
		segments = append(segments, projectKey)
	}
	segments = append(segments, time.Now().UTC().Format("2006-01-02"), filename)
	target := strings.Join(segments, "/")

	resp, err := client.Put("/artifactory/"+target, data)
	if err != nil {
		return "", err
	}
	if err := resp.Error("upload report to " + target); err != nil {
		return "", err
	}
	return target, nil
}
