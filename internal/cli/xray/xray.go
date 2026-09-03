// Package xraycmd implements the "jfps xray ..." command group: Xray
// repository indexing, report listing/cleanup, and policy/watch/ignore-rule
// synchronization between two JFrog Platform Deployments.
//
// Dispatch is two levels deep: "jfps xray <group> <action> [flags]", e.g.
// "jfps xray index status", "jfps xray reports list", "jfps xray sync
// policies".
package xraycmd

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"jfrogpscli/internal/config"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/progress"
	"jfrogpscli/internal/registry"
	"jfrogpscli/internal/xray"
)

func init() {
	registry.Register("xray", "Manage Xray indexing, reports, and policy/watch/ignore-rule sync", Run)
}

// Run dispatches "jfps xray <group> <action> [flags]".
func Run(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		printUsage()
		if len(args) == 0 {
			return 1
		}
		return 0
	}

	group := args[0]
	rest := args[1:]
	switch group {
	case "index":
		return runIndex(rest)
	case "reports":
		return runReports(rest)
	case "sync":
		return dispatchSync(rest)
	default:
		fmt.Fprintf(os.Stderr, "jfps xray: unknown group %q\n\n", group)
		printUsage()
		return 1
	}
}

func isHelp(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps xray <group> <action> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Groups:")
	fmt.Fprintln(os.Stderr, "  index    status | toggle | reindex | jas-toggle")
	fmt.Fprintln(os.Stderr, "  reports  list | delete | generate | segregate-by-deployer")
	fmt.Fprintln(os.Stderr, "  sync     policies | watches | ignore-rules")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps xray <group> <action> -h' for flags on a specific action.")
}

// runIndex dispatches "jfps xray index <action> [flags]".
func runIndex(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprintln(os.Stderr, "Usage: jfps xray index <status|toggle|reindex|jas-toggle> [flags]")
		if len(args) == 0 {
			return 1
		}
		return 0
	}
	action := args[0]
	rest := args[1:]
	switch action {
	case "status":
		return runIndexStatus(rest)
	case "toggle":
		return runIndexToggle(rest)
	case "reindex":
		return runIndexReindex(rest)
	case "jas-toggle":
		return runIndexJASToggle(rest)
	default:
		fmt.Fprintf(os.Stderr, "jfps xray index: unknown action %q\n", action)
		return 1
	}
}

// runReports dispatches "jfps xray reports <action> [flags]".
func runReports(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprintln(os.Stderr, "Usage: jfps xray reports <list|delete|generate|segregate-by-deployer> [flags]")
		if len(args) == 0 {
			return 1
		}
		return 0
	}
	action := args[0]
	rest := args[1:]
	switch action {
	case "list":
		return runReportsList(rest)
	case "delete":
		return runReportsDelete(rest)
	case "generate":
		return runReportsGenerate(rest)
	case "segregate-by-deployer":
		return runReportsSegregateByDeployer(rest)
	default:
		fmt.Fprintf(os.Stderr, "jfps xray reports: unknown action %q\n", action)
		return 1
	}
}

// dispatchSync dispatches "jfps xray sync <action> [flags]".
func dispatchSync(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprintln(os.Stderr, "Usage: jfps xray sync <policies|watches|ignore-rules> [flags]")
		if len(args) == 0 {
			return 1
		}
		return 0
	}
	action := args[0]
	rest := args[1:]
	switch action {
	case "policies":
		return runSyncPolicies(rest)
	case "watches":
		return runSyncWatches(rest)
	case "ignore-rules":
		return runSyncIgnoreRules(rest)
	default:
		fmt.Fprintf(os.Stderr, "jfps xray sync: unknown action %q\n", action)
		return 1
	}
}

// ---------------------------------------------------------------------------
// index status
// ---------------------------------------------------------------------------

func runIndexStatus(args []string) int {
	fs := flag.NewFlagSet("xray index status", flag.ContinueOnError)
	repoType := fs.String("type", "local", "Repository type to check: local, remote, or federated")
	server := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index status:", err)
		return 1
	}

	statuses, err := xray.IndexStatus(client, *repoType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index status:", err)
		return 1
	}

	rows := make([][]string, 0, len(statuses))
	for _, s := range statuses {
		state := "NOT_INDEXED"
		if s.Indexed {
			state = "INDEXED"
		}
		rows = append(rows, []string{s.Repo, state})
	}
	output.Table(os.Stdout, []string{"REPO", "INDEXED"}, rows)
	return 0
}

// ---------------------------------------------------------------------------
// index toggle
// ---------------------------------------------------------------------------

func runIndexToggle(args []string) int {
	fs := flag.NewFlagSet("xray index toggle", flag.ContinueOnError)
	repo := fs.String("repo", "", "Repository key to toggle (mutually exclusive with --all)")
	all := fs.Bool("all", false, "Toggle every repository of --type (mutually exclusive with --repo)")
	repoType := fs.String("type", "", "Repository type: local, remote, virtual, or federated (required with --all)")
	enable := fs.Bool("enable", false, "Enable Xray indexing (mutually exclusive with --disable)")
	disable := fs.Bool("disable", false, "Disable Xray indexing (mutually exclusive with --enable)")
	dryRun := fs.Bool("dry-run", true, "Preview changes without applying them")
	server := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if (*repo == "" && !*all) || (*repo != "" && *all) {
		fmt.Fprintln(os.Stderr, "jfps xray index toggle: exactly one of --repo or --all is required")
		return 1
	}
	if *enable == *disable {
		fmt.Fprintln(os.Stderr, "jfps xray index toggle: exactly one of --enable or --disable is required")
		return 1
	}
	if *all && *repoType == "" {
		fmt.Fprintln(os.Stderr, "jfps xray index toggle: --type is required with --all")
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index toggle:", err)
		return 1
	}

	var repos []string
	if *all {
		repos, err = xray.ListRepositories(client, *repoType)
		if err != nil {
			fmt.Fprintln(os.Stderr, "jfps xray index toggle:", err)
			return 1
		}
	} else {
		repos = []string{*repo}
	}

	if len(repos) == 0 {
		fmt.Fprintln(os.Stdout, "No repositories matched.")
		return 0
	}

	action := fmt.Sprintf("enable Xray indexing on %d repo(s)", len(repos))
	if *disable {
		action = fmt.Sprintf("disable Xray indexing on %d repo(s)", len(repos))
	}
	if *dryRun {
		output.DryRunBanner(os.Stdout, action)
	}

	bar := progress.New()
	results := xray.ToggleIndexing(client, repos, *enable, *dryRun, func(done, total int) { bar.Update("Toggling indexing", done, total) })
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		status := "WOULD_TOGGLE"
		if !r.DryRun {
			if r.Success {
				status = "OK"
			} else {
				status = "FAILED: " + r.Error
			}
		}
		rows = append(rows, []string{r.Repo, status})
	}
	output.Table(os.Stdout, []string{"REPO", "RESULT"}, rows)
	return 0
}

// ---------------------------------------------------------------------------
// index reindex
// ---------------------------------------------------------------------------

func runIndexReindex(args []string) int {
	fs := flag.NewFlagSet("xray index reindex", flag.ContinueOnError)
	repo := fs.String("repo", "", "Repository key to reindex (required)")
	mode := fs.String("mode", "", "Reindex mode: index, force, or scan-now. "+
		"'force' and 'scan-now' require Xray Admin privileges and can be expensive on large repos.")
	dryRun := fs.Bool("dry-run", true, "Preview the call without executing it")
	server := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *repo == "" {
		fmt.Fprintln(os.Stderr, "jfps xray index reindex: --repo is required")
		return 1
	}
	if *mode == "" {
		fmt.Fprintln(os.Stderr, "jfps xray index reindex: --mode is required (index, force, or scan-now)")
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index reindex:", err)
		return 1
	}

	res, err := xray.Reindex(client, *repo, *mode, *dryRun)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index reindex:", err)
		return 1
	}

	if res.DryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("POST %s (mode=%s, repo=%s)", res.Endpoint, res.Mode, res.Repo))
		return 0
	}
	if res.Success {
		fmt.Fprintf(os.Stdout, "OK: reindex mode=%s repo=%s (HTTP %d)\n", res.Mode, res.Repo, res.StatusCode)
		return 0
	}
	fmt.Fprintf(os.Stderr, "jfps xray index reindex: FAILED mode=%s repo=%s: %s\n", res.Mode, res.Repo, res.Error)
	return 1
}

// ---------------------------------------------------------------------------
// index jas-toggle
// ---------------------------------------------------------------------------

func runIndexJASToggle(args []string) int {
	fs := flag.NewFlagSet("xray index jas-toggle", flag.ContinueOnError)
	repoFlag := fs.String("repo", "", "Repository key (mutually exclusive with --project/--repos-file)")
	project := fs.String("project", "", "Fetch repos from this JFrog Project (mutually exclusive with --repo/--repos-file)")
	reposFile := fs.String("repos-file", "", "Path to a newline-delimited repo-key file (mutually exclusive with --repo/--project)")
	enableExposures := fs.Bool("enable-exposures", false, "Enable every present Exposures scanner category")
	disableExposures := fs.Bool("disable-exposures", false, "Disable every present Exposures scanner category")
	enableCA := fs.Bool("enable-contextual-analysis", false, "Enable Contextual Analysis")
	disableCA := fs.Bool("disable-contextual-analysis", false, "Disable Contextual Analysis")
	retentionDays := fs.Int("retention-days", 0, "Set scan-result retention in days (0 = leave unchanged)")
	dryRun := fs.Bool("dry-run", true, "Preview the current configuration without applying changes")
	server := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	scopeCount := 0
	for _, s := range []string{*repoFlag, *project, *reposFile} {
		if s != "" {
			scopeCount++
		}
	}
	if scopeCount != 1 {
		fmt.Fprintln(os.Stderr, "jfps xray index jas-toggle: exactly one of --repo, --project, or --repos-file is required")
		return 1
	}
	if *enableExposures && *disableExposures {
		fmt.Fprintln(os.Stderr, "jfps xray index jas-toggle: --enable-exposures and --disable-exposures are mutually exclusive")
		return 1
	}
	if *enableCA && *disableCA {
		fmt.Fprintln(os.Stderr, "jfps xray index jas-toggle: --enable-contextual-analysis and --disable-contextual-analysis are mutually exclusive")
		return 1
	}

	opts := xray.JASToggleOptions{}
	switch {
	case *enableExposures:
		v := true
		opts.EnableExposures = &v
	case *disableExposures:
		v := false
		opts.EnableExposures = &v
	}
	switch {
	case *enableCA:
		v := true
		opts.EnableContextualAnalysis = &v
	case *disableCA:
		v := false
		opts.EnableContextualAnalysis = &v
	}
	if *retentionDays > 0 {
		opts.RetentionInDays = retentionDays
	}
	if opts.EnableExposures == nil && opts.EnableContextualAnalysis == nil && opts.RetentionInDays == nil {
		fmt.Fprintln(os.Stderr, "jfps xray index jas-toggle: nothing to do -- pass at least one of --enable/disable-exposures, --enable/disable-contextual-analysis, or --retention-days")
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index jas-toggle:", err)
		return 1
	}

	var repos []string
	switch {
	case *repoFlag != "":
		repos = []string{*repoFlag}
	case *project != "":
		repos, err = xray.ListRepositoriesForProject(client, *project)
	case *reposFile != "":
		repos, err = xray.ReadRepoNamesFile(*reposFile)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray index jas-toggle:", err)
		return 1
	}
	if len(repos) == 0 {
		fmt.Fprintln(os.Stdout, "No repositories matched.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("update JAS/Contextual Analysis config on %d repo(s)", len(repos)))
	}

	bar := progress.New()
	results := xray.ToggleJAS(client, repos, opts, *dryRun, func(done, total int) { bar.Update("Toggling JAS", done, total) })
	rows := make([][]string, 0, len(results))
	failed := 0
	for _, r := range results {
		status := "OK"
		switch {
		case r.Error != "":
			status = "FAILED: " + r.Error
			failed++
		case r.DryRun:
			status = "CURRENT"
		case !r.Changed:
			status = "NO_CHANGE"
		}
		rows = append(rows, []string{
			r.Repo,
			boolPtrStr(r.Before.VulnContextualAnalysis), boolPtrStr(r.After.VulnContextualAnalysis),
			boolPtrStr(r.Before.SecretsScan), boolPtrStr(r.After.SecretsScan),
			boolPtrStr(r.Before.ServicesScan), boolPtrStr(r.After.ServicesScan),
			boolPtrStr(r.Before.ApplicationsScan), boolPtrStr(r.After.ApplicationsScan),
			boolPtrStr(r.Before.IacScan), boolPtrStr(r.After.IacScan),
			status,
		})
	}
	output.Table(os.Stdout, []string{
		"REPO",
		"CA_BEFORE", "CA_AFTER",
		"SECRETS_BEFORE", "SECRETS_AFTER",
		"SERVICES_BEFORE", "SERVICES_AFTER",
		"APPS_BEFORE", "APPS_AFTER",
		"IAC_BEFORE", "IAC_AFTER",
		"RESULT",
	}, rows)
	if failed > 0 {
		return 1
	}
	return 0
}

func boolPtrStr(b *bool) string {
	if b == nil {
		return "-"
	}
	if *b {
		return "true"
	}
	return "false"
}

// ---------------------------------------------------------------------------
// reports list
// ---------------------------------------------------------------------------

func runReportsList(args []string) int {
	fs := flag.NewFlagSet("xray reports list", flag.ContinueOnError)
	status := fs.String("status", "", "Filter by report status")
	reportType := fs.String("report-type", "", "Filter by report type")
	server := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports list:", err)
		return 1
	}

	reports, err := xray.ListReports(client, xray.ReportFilters{Status: *status, ReportType: *reportType})
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports list:", err)
		return 1
	}

	printReportsTable(reports)
	return 0
}

func printReportsTable(reports []xray.Report) {
	rows := make([][]string, 0, len(reports))
	for _, r := range reports {
		rows = append(rows, []string{fmt.Sprintf("%d", r.ID), r.Name, r.Status, r.ReportType, r.EndTime})
	}
	output.Table(os.Stdout, []string{"ID", "NAME", "STATUS", "TYPE", "END_TIME"}, rows)
}

// ---------------------------------------------------------------------------
// reports delete
// ---------------------------------------------------------------------------

func runReportsDelete(args []string) int {
	fs := flag.NewFlagSet("xray reports delete", flag.ContinueOnError)
	status := fs.String("status", "", "Filter by report status")
	reportType := fs.String("report-type", "", "Filter by report type")
	endTime := fs.String("end-time", "", "Delete reports with end_time before this RFC3339 cutoff (required)")
	dryRun := fs.Bool("dry-run", true, "Preview deletions without applying them")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	server := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *endTime == "" {
		fmt.Fprintln(os.Stderr, "jfps xray reports delete: --end-time is required (RFC3339, e.g. 2024-01-01T00:00:00Z)")
		return 1
	}
	cutoff, err := time.Parse(time.RFC3339, *endTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps xray reports delete: --end-time %q is not RFC3339: %v\n", *endTime, err)
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports delete:", err)
		return 1
	}

	reports, err := xray.ListReports(client, xray.ReportFilters{Status: *status, ReportType: *reportType})
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports delete:", err)
		return 1
	}

	matches, skipped := xray.ReportsBeforeEndTime(reports, cutoff)
	for _, s := range skipped {
		fmt.Fprintf(os.Stdout, "skipping report id=%d name=%q: end_time %q is not RFC3339\n", s.ID, s.Name, s.EndTime)
	}

	if len(matches) == 0 {
		fmt.Fprintln(os.Stdout, "No reports matched.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d report(s)", len(matches)))
		printReportsTable(matches)
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d report(s)?", len(matches))) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := xray.DeleteReports(client, matches, false, func(done, total int) { bar.Update("Deleting reports", done, total) })
	rows := make([][]string, 0, len(results))
	failed := 0
	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAILED: " + r.Error
			failed++
		}
		rows = append(rows, []string{fmt.Sprintf("%d", r.ID), r.Name, status})
	}
	output.Table(os.Stdout, []string{"ID", "NAME", "RESULT"}, rows)
	if failed > 0 {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// reports generate
// ---------------------------------------------------------------------------

func runReportsGenerate(args []string) int {
	fs := flag.NewFlagSet("xray reports generate", flag.ContinueOnError)
	server := config.Register(fs, "")

	reponame := fs.String("reponame", "", "Scope: a single repository (mutually exclusive with --project/--buildname/--resource-type/--watchname)")
	project := fs.String("project", "", "Scope: every repository in this JFrog Project")
	buildname := fs.String("buildname", "", "Scope: a single build (requires --build-repo)")
	buildRepo := fs.String("build-repo", "", "The build-info repository --buildname lives in")
	resourceType := fs.String("resource-type", "", "Scope: 'builds' alone means instance-wide build discovery")
	watchname := fs.String("watchname", "", "Scope: derive resources from this watch's own resources")
	repoRClass := fs.String("repo-rclass", "", "With --project: filter to repos of this class (local/remote/virtual/federated)")

	reportType := fs.String("report-type", ReportTypeViolationsFlag, "Report type: violations, vulnerabilities, or exposures")
	severities := fs.String("severities", "", "Comma-separated severities (violations/vulnerabilities only)")
	updatedStart := fs.String("updated-start", "", "Violations: RFC3339 start of the 'updated' filter range")
	updatedEnd := fs.String("updated-end", "", "Violations: RFC3339 end of the 'updated' filter range")
	daysPerChunk := fs.Int("days-per-chunk", 0, "Split the date range into this many days per report (0 = one report for the whole range)")
	buildVersions := fs.Int("build-number-of-versions", 5, "Number of latest build versions to scan when resolving a build resource")

	exposureCategory := fs.String("exposure-category", "", "Exposures: category (secrets|services|applications|iac) -- required for --report-type=exposures")
	exposureImpactedArtifact := fs.String("exposure-impacted-artifact", "", "Exposures: wildcard filter on the impacted artifact")
	scanDateStart := fs.String("scan-date-start", "", "Vulnerabilities/exposures: RFC3339 start of the 'scan_date' filter range")
	scanDateEnd := fs.String("scan-date-end", "", "Vulnerabilities/exposures: RFC3339 end of the 'scan_date' filter range")

	hasRemediation := fs.String("has-remediation", "", "Vulnerabilities: 'true' or 'false' (empty = don't filter)")
	cve := fs.String("cve", "", "Vulnerabilities: filter by CVE")
	issueID := fs.String("issue-id", "", "Vulnerabilities: filter by Xray issue ID")
	cvssMin := fs.Float64("cvss-min-score", 0, "Vulnerabilities: minimum CVSS score")
	cvssMax := fs.Float64("cvss-max-score", 0, "Vulnerabilities: maximum CVSS score")
	publishedStart := fs.String("published-start", "", "Vulnerabilities: RFC3339 start of the 'published' filter range")
	publishedEnd := fs.String("published-end", "", "Vulnerabilities: RFC3339 end of the 'published' filter range")

	buildGitRepoReport := fs.Bool("build-gitrepo-report", false, "For a build-scoped report: also segregate findings by the producing build's Git repo/VCS revision")
	reportUploadRepo := fs.String("report-upload-repo", "", "Upload the generated CSV(s) to this repo (PUT; subject to the write guard)")
	outputDir := fs.String("output-dir", "", "Directory to write CSV(s) to (default: current directory)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	scopeCount := 0
	for _, s := range []string{*reponame, *project, *buildname} {
		if s != "" {
			scopeCount++
		}
	}
	instanceWideBuilds := scopeCount == 0 && *resourceType == "builds"
	watchOnly := scopeCount == 0 && !instanceWideBuilds && *watchname != ""
	if scopeCount > 1 || (scopeCount == 1 && (*resourceType == "builds" || *watchname != "")) || (*resourceType == "builds" && *watchname != "") || (!instanceWideBuilds && !watchOnly && scopeCount != 1) {
		fmt.Fprintln(os.Stderr, "jfps xray reports generate: exactly one of --reponame, --project, --buildname, --resource-type=builds (alone), or --watchname (alone) is required")
		return 1
	}
	if *buildname != "" && *buildRepo == "" {
		fmt.Fprintln(os.Stderr, "jfps xray reports generate: --build-repo is required with --buildname")
		return 1
	}
	if *reportType == "exposures" && *exposureCategory == "" {
		fmt.Fprintln(os.Stderr, "jfps xray reports generate: --exposure-category is required with --report-type=exposures")
		return 1
	}

	var hasRemediationPtr *bool
	switch *hasRemediation {
	case "":
	case "true":
		v := true
		hasRemediationPtr = &v
	case "false":
		v := false
		hasRemediationPtr = &v
	default:
		fmt.Fprintln(os.Stderr, "jfps xray reports generate: --has-remediation must be 'true' or 'false'")
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports generate:", err)
		return 1
	}

	opts := xray.ReportGenOptions{
		ReportType: *reportType,
		Scope: xray.ReportScope{
			Reponame:     *reponame,
			Project:      *project,
			Buildname:    *buildname,
			BuildRepo:    *buildRepo,
			Watchname:    *watchname,
			ResourceType: *resourceType,
			RepoRClass:   *repoRClass,
		},
		Severities:               splitCSVFlag(*severities),
		UpdatedStart:             *updatedStart,
		UpdatedEnd:               *updatedEnd,
		DaysPerChunk:             *daysPerChunk,
		BuildNumberOfVersions:    *buildVersions,
		ExposureCategory:         *exposureCategory,
		ExposureImpactedArtifact: *exposureImpactedArtifact,
		ScanDateStart:            *scanDateStart,
		ScanDateEnd:              *scanDateEnd,
		HasRemediation:           hasRemediationPtr,
		CVE:                      *cve,
		IssueID:                  *issueID,
		CVSSMinScore:             *cvssMin,
		CVSSMaxScore:             *cvssMax,
		PublishedStart:           *publishedStart,
		PublishedEnd:             *publishedEnd,
		BuildGitRepoReport:       *buildGitRepoReport,
		ReportUploadRepo:         *reportUploadRepo,
		OutputDir:                *outputDir,
	}

	bar := progress.New()
	results, warnings, err := xray.GenerateReports(client, opts, func(done, total int) { bar.Update("Generating reports", done, total) })
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		csvCol := r.CSVPath
		if csvCol == "" {
			csvCol = "(empty)"
		}
		rows = append(rows, []string{r.Name, fmt.Sprintf("%d", r.RowCount), csvCol, r.GitRepoSegregatedCSVPath, r.Warning})
	}
	output.Table(os.Stdout, []string{"REPORT", "ROWS", "CSV", "GITREPO_CSV", "NOTE"}, rows)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports generate:", err)
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// reports segregate-by-deployer
// ---------------------------------------------------------------------------

func runReportsSegregateByDeployer(args []string) int {
	fs := flag.NewFlagSet("xray reports segregate-by-deployer", flag.ContinueOnError)
	server := config.Register(fs, "")
	watchname := fs.String("watchname", "", "Repository- or project-scoped watch to report on (required)")
	reportType := fs.String("report-type", ReportTypeViolationsFlag, "Report type: violations or exposures")
	exposureCategory := fs.String("exposure-category", "", "Exposures: category (secrets|services|applications|iac) -- required for --report-type=exposures")
	exposureImpactedArtifact := fs.String("exposure-impacted-artifact", "", "Exposures: wildcard filter on the impacted artifact")
	severities := fs.String("severities", "", "Comma-separated severities (violations only)")
	updatedStart := fs.String("updated-start", "", "Violations: RFC3339 start of the 'updated' filter range")
	updatedEnd := fs.String("updated-end", "", "Violations: RFC3339 end of the 'updated' filter range")
	reportUploadRepo := fs.String("report-upload-repo", "", "Upload the source and segregated CSVs to this repo (PUT; subject to the write guard)")
	outputDir := fs.String("output-dir", "", "Directory to write CSVs to (default: current directory)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *watchname == "" {
		fmt.Fprintln(os.Stderr, "jfps xray reports segregate-by-deployer: --watchname is required")
		return 1
	}
	if *reportType == "exposures" && *exposureCategory == "" {
		fmt.Fprintln(os.Stderr, "jfps xray reports segregate-by-deployer: --exposure-category is required with --report-type=exposures")
		return 1
	}

	client, err := server.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports segregate-by-deployer:", err)
		return 1
	}

	bar := progress.New()
	result, err := xray.SegregateByDeployer(client, xray.DeployerSegregationOptions{
		Watchname:                *watchname,
		ReportType:               *reportType,
		ExposureCategory:         *exposureCategory,
		ExposureImpactedArtifact: *exposureImpactedArtifact,
		Severities:               splitCSVFlag(*severities),
		UpdatedStart:             *updatedStart,
		UpdatedEnd:               *updatedEnd,
		ReportUploadRepo:         *reportUploadRepo,
		OutputDir:                *outputDir,
	}, func(done, total int) { bar.Update("Segregating by deployer", done, total) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "jfps xray reports segregate-by-deployer:", err)
		return 1
	}
	if len(result.Rows) == 0 {
		fmt.Fprintln(os.Stdout, "Source report produced no rows; nothing to segregate.")
		return 0
	}

	rows := make([][]string, 0, len(result.Rows))
	for _, r := range result.Rows {
		rows = append(rows, []string{r.Deployer, r.Repo, r.ArtifactPath, fmt.Sprintf("%d", r.ViolationCount), r.Severities, r.LookupError})
	}
	output.Table(os.Stdout, []string{"DEPLOYER", "REPO", "ARTIFACT_PATH", "COUNT", "SEVERITIES", "LOOKUP_ERROR"}, rows)
	fmt.Fprintf(os.Stdout, "\nWrote %s\n", result.SegregatedCSVPath)
	return 0
}

// splitCSVFlag splits a comma-separated flag value into a trimmed,
// non-empty slice. An empty/blank input yields nil.
func splitCSVFlag(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ReportTypeViolationsFlag is the default --report-type value.
const ReportTypeViolationsFlag = "violations"

// ---------------------------------------------------------------------------
// sync policies / watches / ignore-rules
// ---------------------------------------------------------------------------

// syncFunc is the shared shape of xray.SyncPolicies / SyncWatches /
// SyncIgnoreRules, letting runSync drive all three from one flag-parsing and
// reporting implementation.
type syncFunc func(source, target *jfclient.Client, dryRun bool, onProgress ...progress.Reporter) ([]xray.SyncResult, error)

func runSyncPolicies(args []string) int {
	return runSync(args, "xray sync policies", xray.SyncPolicies)
}

func runSyncWatches(args []string) int {
	return runSync(args, "xray sync watches", xray.SyncWatches)
}

func runSyncIgnoreRules(args []string) int {
	return runSync(args, "xray sync ignore-rules", xray.SyncIgnoreRules)
}

// runSync is the shared flag-parsing/execution/reporting shell for the three
// sync actions ("policies", "watches", "ignore-rules"), which differ only in
// which xray.Sync* function drives them and which entities they copy.
//
// Per the spec, a --dry-run flag (defaulting true) is supported uniformly
// across all three, even though the original ad hoc script for "policies"
// had no such guard.
func runSync(args []string, name string, fn syncFunc) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", true, "Preview the copy without applying it")
	source := config.Register(fs, "source-")
	target := config.Register(fs, "target-")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	sourceClient, err := source.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return 1
	}
	targetClient, err := target.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return 1
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, "copy entries from source to target")
	}

	bar := progress.New()
	results, err := fn(sourceClient, targetClient, *dryRun, func(done, total int) { bar.Update("Syncing", done, total) })
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return 1
	}

	rows := make([][]string, 0, len(results))
	failed := 0
	for _, r := range results {
		status := "WOULD_COPY"
		if !r.DryRun {
			if r.Success {
				status = "OK"
			} else {
				status = "FAILED: " + r.Error
				failed++
			}
		}
		rows = append(rows, []string{r.Name, r.ProjectKey, status})
	}
	output.Table(os.Stdout, []string{"NAME", "PROJECT_KEY", "RESULT"}, rows)
	if failed > 0 {
		return 1
	}
	return 0
}
