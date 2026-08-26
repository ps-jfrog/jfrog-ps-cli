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
	"time"

	"jfrogpscli/internal/config"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/output"
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
	fmt.Fprintln(os.Stderr, "  index    status | toggle | reindex")
	fmt.Fprintln(os.Stderr, "  reports  list | delete")
	fmt.Fprintln(os.Stderr, "  sync     policies | watches | ignore-rules")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps xray <group> <action> -h' for flags on a specific action.")
}

// runIndex dispatches "jfps xray index <action> [flags]".
func runIndex(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprintln(os.Stderr, "Usage: jfps xray index <status|toggle|reindex> [flags]")
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
	default:
		fmt.Fprintf(os.Stderr, "jfps xray index: unknown action %q\n", action)
		return 1
	}
}

// runReports dispatches "jfps xray reports <action> [flags]".
func runReports(args []string) int {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Fprintln(os.Stderr, "Usage: jfps xray reports <list|delete> [flags]")
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

	results := xray.ToggleIndexing(client, repos, *enable, *dryRun)
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

	results := xray.DeleteReports(client, matches, false)
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
// sync policies / watches / ignore-rules
// ---------------------------------------------------------------------------

// syncFunc is the shared shape of xray.SyncPolicies / SyncWatches /
// SyncIgnoreRules, letting runSync drive all three from one flag-parsing and
// reporting implementation.
type syncFunc func(source, target *jfclient.Client, dryRun bool) ([]xray.SyncResult, error)

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

	results, err := fn(sourceClient, targetClient, *dryRun)
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
