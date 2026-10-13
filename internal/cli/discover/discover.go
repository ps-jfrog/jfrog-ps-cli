// Package discovercmd wires the internal/discover and internal/analyze
// business logic into the "jfps discover" command: per-repo storage/
// activity/cleanup-policy/project reporting, instance-wide discovery
// (versions, storage, auth/signing-key config, users/groups/permissions,
// projects, replication/federation, platform config, Xray policy/watch/
// ignore-rule/curation config), and turning that into a self-contained
// interactive HTML audit. It is entirely read-only (no dry-run flag -
// there is nothing to preview, since nothing is ever mutated); `analyze`
// makes no network calls at all.
//
// Rather than subcommands, one piece per invocation, "discover" takes
// boolean flags selecting which piece(s) to run - so a single invocation
// can fetch the repo report, the instance snapshot, and render the HTML
// audit together. Each piece's own flags (concurrency, caps, thresholds,
// output paths) are all registered on the same flag set.
package discovercmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"jfrogpscli/internal/analyze"
	"jfrogpscli/internal/config"
	"jfrogpscli/internal/discover"
	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("discover", "Repo/instance/policy reporting and HTML audit generation (flags select which pieces to run)", Run)
}

// Run parses one combined flag set and runs whichever of
// --repos/--policies/--instance/--analyze (or --all) were selected.
// --repos/--policies/--instance run concurrently by default when more than
// one is selected (--parallel=false for sequentially instead); --analyze
// always runs last, so it picks up whatever --repos/--instance just wrote
// in the same invocation.
func Run(args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printUsage()
		return 0
	}

	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = printUsage
	serverFlags := config.Register(fs, "")

	// --- selection flags: which piece(s) to run ---
	doRepos := fs.Bool("repos", false, "Fetch the per-repo storage/activity/cleanup-policy/project report -> --repos-csv")
	doPolicies := fs.Bool("policies", false, "Fetch the repo -> cleanup-policy mapping (one row per repo x match) -> --policies-csv")
	doInstance := fs.Bool("instance", false, "Fetch the instance-wide snapshot (versions, security, access, projects, platform, Xray) -> --instance-json")
	doAnalyze := fs.Bool("analyze", false, "Render the self-contained HTML audit from --repos-csv (+ --instance-json if present) -> --audit-html")
	doCrossEnvAnalyze := fs.Bool("cross-env-analyze", false, "Analyze multiple environment outputs and report on duplicate repositories across environments -> --cross-env-report")
	doProjectRoles := fs.Bool("project-roles", false, "List all project roles (one row per project x role) -> --project-roles-csv")
	doProjectRepos := fs.Bool("project-repos", false, "List all project-to-repository mappings -> --project-repos-csv")
	doAppTrust := fs.Bool("apptrust", false, "List all AppTrust applications/policies/rules/templates/stages/evidence/release-bundles -> --apptrust-csv")
	doAll := fs.Bool("all", false, "Shorthand for --repos --policies --instance --analyze")
	parallel := fs.Bool("parallel", true, "Run --repos/--policies/--instance concurrently instead of sequentially when more than one is selected - faster, since they're independent network calls against the same JPD (each still gets its own live progress bar); set false to run sequentially instead")

	// --- output/input path flags (shared between generating and analyzing) ---
	outputDir := fs.String("output", "output", "Base output folder; a subfolder named after the JPD's hostname (e.g. 'output/psazuse') is created under it and used for any of the paths below left at their default")
	reposCSV := fs.String("repos-csv", "", "Path for the repo report CSV (written by --repos, read by --analyze) - default: <output>/<instance>/report.csv")
	policiesCSV := fs.String("policies-csv", "", "Path for the policy-mapping CSV (written by --policies) - default: <output>/<instance>/policies.csv")
	instanceJSON := fs.String("instance-json", "", "Path for the instance snapshot JSON (written by --instance, read by --analyze if present) - default: <output>/<instance>/instance.json")
	auditHTML := fs.String("audit-html", "", "Path for the rendered HTML audit (written by --analyze) - default: <output>/<instance>/audit.html")
	crossEnvReport := fs.String("cross-env-report", "", "--cross-env-analyze: output path for the cross-environment analysis report (default: <output>/cross-env-analysis.txt; also generates .json and .html versions)")
	projectRolesCSV := fs.String("project-roles-csv", "", "Path for the project roles CSV (written by --project-roles) - default: <output>/<instance>/project-roles.csv")
	projectReposCSV := fs.String("project-repos-csv", "", "Path for the project repositories CSV (written by --project-repos) - default: <output>/<instance>/project-repos.csv")
	appTrustCSV := fs.String("apptrust-csv", "", "Path for the AppTrust CSV (written by --apptrust) - default: <output>/<instance>/apptrust.csv")

	// --- --repos-specific flags ---
	concurrency := fs.Int("concurrency", 8, "Repos/permission-targets/federated-repos/projects processed concurrently (be considerate on shared instances)")
	withPolicies := fs.Bool("cleanup-policies", true, "--repos: include matched cleanup-policy detail columns (one extra API call)")
	include := fs.String("include", "", "--repos: comma-separated glob patterns - only report repos matching at least one")
	exclude := fs.String("exclude", "", "--repos: comma-separated glob patterns - skip repos matching any of these")

	// --- --instance-specific flags ---
	maxUsers := fs.Int("max-users", discover.DefaultCollectOptions().MaxUsers, "--instance: cap on users collected (0 = unlimited)")
	maxGroups := fs.Int("max-groups", discover.DefaultCollectOptions().MaxGroups, "--instance: cap on groups collected (0 = unlimited)")
	maxPermissions := fs.Int("max-permissions", discover.DefaultCollectOptions().MaxPermissions, "--instance: cap on permission targets collected (0 = unlimited)")
	projectDetail := fs.Bool("project-detail", discover.DefaultCollectOptions().ProjectDetail, "--instance: also fetch each project's roles/environments/members (2-4 extra calls per project)")

	// --- --analyze-specific flags ---
	staleDays := fs.Int("stale-days", analyze.DefaultOptions().StaleDays, "--analyze: flag non-empty repos with no push/pull inside this many days")
	hugeGB := fs.Float64("huge-gb", analyze.DefaultOptions().HugeGB, "--analyze: flag repos using more than this many GB")
	massiveFiles := fs.Int("massive-files", analyze.DefaultOptions().MassiveFiles, "--analyze: flag repos with more than this many files")
	longNameChars := fs.Int("long-name-chars", analyze.DefaultOptions().LongNameChars, "--analyze: flag repo keys longer than this many characters")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *doAll {
		*doRepos, *doPolicies, *doInstance, *doAnalyze = true, true, true, true
		*doProjectRoles, *doProjectRepos, *doAppTrust = true, true, true
	}
	if !*doRepos && !*doPolicies && !*doInstance && !*doAnalyze && !*doCrossEnvAnalyze && !*doProjectRoles && !*doProjectRepos && !*doAppTrust {
		printUsage()
		return 1
	}

	// --project-roles, --project-repos, and --apptrust require instance data,
	// so if any of them are requested without --instance, enable it.
	if (*doProjectRoles || *doProjectRepos || *doAppTrust) && !*doInstance {
		*doInstance = true
	}

	// Any path flag left at its default ("") lands under <output>/<instance>/
	// instead of the current directory, so repeated runs against different
	// JPDs don't overwrite each other's reports. An explicitly-set path flag
	// is used exactly as given, ignoring --output entirely.
	targetDir := filepath.Join(*outputDir, outputSubdir(serverFlags.PeekURL()))
	if *reposCSV == "" {
		*reposCSV = filepath.Join(targetDir, "report.csv")
	}
	if *policiesCSV == "" {
		*policiesCSV = filepath.Join(targetDir, "policies.csv")
	}
	if *instanceJSON == "" {
		*instanceJSON = filepath.Join(targetDir, "instance.json")
	}
	if *auditHTML == "" {
		*auditHTML = filepath.Join(targetDir, "audit.html")
	}
	if *crossEnvReport == "" {
		*crossEnvReport = filepath.Join(*outputDir, "cross-env-analysis.txt")
	}
	if *projectRolesCSV == "" {
		*projectRolesCSV = filepath.Join(targetDir, "project-roles.csv")
	}
	if *projectReposCSV == "" {
		*projectReposCSV = filepath.Join(targetDir, "project-repos.csv")
	}
	if *appTrustCSV == "" {
		*appTrustCSV = filepath.Join(targetDir, "apptrust.csv")
	}

	// Only the pieces that talk to Artifactory need a resolved client;
	// --analyze alone makes no network calls at all.
	var client *jfclient.Client
	if *doRepos || *doPolicies || *doInstance {
		c, err := serverFlags.Resolve()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		client = c
	}

	// Count how many network operations we'll run
	// Project-roles/repos/apptrust are derived from instance data, not separate calls
	selected := 0
	for _, on := range []bool{*doRepos, *doPolicies, *doInstance} {
		if on {
			selected++
		}
	}
	runConcurrently := *parallel && selected > 1

	// One shared bar block for the whole invocation: each piece gets its
	// own labeled line, stacked and redrawn together, so running several
	// pieces concurrently doesn't garble the terminal the way independent
	// \r-based bars would.
	mb := progress.New()
	errs := &runErrors{}

	runRepos := func() int {
		return doReposReport(client, *reposCSV, *concurrency, *withPolicies, splitList(*include), splitList(*exclude), mb, errs)
	}
	runPolicies := func() int { return doPoliciesReport(client, *policiesCSV, mb, errs) }
	runInstance := func() int {
		return doInstanceSnapshot(client, *instanceJSON, *concurrency, *maxUsers, *maxGroups, *maxPermissions, *projectDetail, mb, errs)
	}

	if runConcurrently {
		mb.Log("discover: running repos/policies/instance concurrently (pass --parallel=false to run sequentially instead)")
		var wg sync.WaitGroup
		codes := make(chan int, selected)
		launch := func(fn func() int) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes <- fn()
			}()
		}
		if *doRepos {
			launch(runRepos)
		}
		if *doPolicies {
			launch(runPolicies)
		}
		if *doInstance {
			launch(runInstance)
		}
		wg.Wait()
		close(codes)
		for code := range codes {
			if code != 0 {
				return code
			}
		}
	} else {
		if *doRepos {
			if code := runRepos(); code != 0 {
				return code
			}
		}
		if *doPolicies {
			if code := runPolicies(); code != 0 {
				return code
			}
		}
		if *doInstance {
			if code := runInstance(); code != 0 {
				return code
			}
		}
	}

	// Extract and write CSV files from the snapshot for project roles,
	// project repos, and AppTrust data (requires --instance to have run)
	var snap *instance.Snapshot
	if *doInstance {
		// Try to load the instance snapshot that was just created
		if instF, err := os.Open(*instanceJSON); err == nil {
			snap, _ = instance.ReadJSON(instF)
			instF.Close()
		}
	}

	if *doProjectRoles && snap != nil {
		if code := doProjectRolesCSV(snap, *projectRolesCSV, mb, errs); code != 0 {
			return code
		}
	} else if *doProjectRoles {
		mb.Log("discover --project-roles: not writing CSV (--instance was not run)")
	}

	if *doProjectRepos && snap != nil {
		if code := doProjectReposCSV(snap, *projectReposCSV, mb, errs); code != 0 {
			return code
		}
	} else if *doProjectRepos {
		mb.Log("discover --project-repos: not writing CSV (--instance was not run)")
	}

	if *doAppTrust && snap != nil {
		if code := doAppTrustCSVs(snap, *appTrustCSV, mb, errs); code != 0 {
			return code
		}
	} else if *doAppTrust {
		mb.Log("discover --apptrust: not writing CSV (--instance was not run)")
	}

	analyzeCode := 0
	if *doAnalyze {
		analyzeCode = doAnalyzeReport(*reposCSV, *instanceJSON, *auditHTML, *staleDays, *hugeGB, *massiveFiles, *longNameChars)
	}

	if *doCrossEnvAnalyze {
		if code := doCrossEnvAnalyzeReport(*outputDir, *crossEnvReport, mb, errs); code != 0 {
			analyzeCode = code
		}
	}

	if errList := errs.list(); len(errList) > 0 {
		fmt.Fprintf(os.Stderr, "\nDiscovery completed with %d error(s) (continuing rather than aborting):\n", len(errList))
		for _, e := range errList {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
	}
	return analyzeCode
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps discover [--repos] [--policies] [--instance] [--project-roles] [--project-repos] [--apptrust] [--analyze] [--cross-env-analyze] [--all] [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags select which piece(s) to run - most operations run concurrently by default when more")
	fmt.Fprintln(os.Stderr, "than one is selected, each with its own live progress bar (--parallel=false to run sequentially instead), and analyze")
	fmt.Fprintln(os.Stderr, "always runs last, so 'discover --repos --instance --analyze' generates the report and snapshot,")
	fmt.Fprintln(os.Stderr, "then renders the combined HTML audit, all in one invocation.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  --repos               Per-repo package type, file count, usage, project, last-push/last-pull, cleanup-policy detail -> --repos-csv")
	fmt.Fprintln(os.Stderr, "  --policies            Map every repo to its matching cleanup policy/policies (one row per repo x match) -> --policies-csv")
	fmt.Fprintln(os.Stderr, "  --instance            Versions, storage, auth/signing-keys, users/groups/permissions, projects, platform & Xray config -> --instance-json")
	fmt.Fprintln(os.Stderr, "  --project-roles       List all project roles (one row per project x role) -> --project-roles-csv")
	fmt.Fprintln(os.Stderr, "  --project-repos       List all project-to-repository mappings -> --project-repos-csv")
	fmt.Fprintln(os.Stderr, "  --apptrust            List all AppTrust applications/policies/rules/templates/stages/evidence/release-bundles -> --apptrust-csv")
	fmt.Fprintln(os.Stderr, "  --analyze             Turn --repos-csv (+ --instance-json if present) into a self-contained interactive HTML audit -> --audit-html")
	fmt.Fprintln(os.Stderr, "  --cross-env-analyze   Scan --output directory for multiple environment outputs and report duplicate repositories -> --cross-env-report")
	fmt.Fprintln(os.Stderr, "  --all                 Comprehensive discovery: --repos --policies --instance --project-roles --project-repos --apptrust --analyze")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Output files land under --output (default \"output\"), in a subfolder named after the JPD's")
	fmt.Fprintln(os.Stderr, "hostname, e.g. 'output/psazuse/report.csv' - unless --repos-csv/--policies-csv/--instance-json/")
	fmt.Fprintln(os.Stderr, "--project-roles-csv/--project-repos-csv/--apptrust-csv override an individual path explicitly.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps discover -h' to see every flag (output paths, concurrency, caps, analyze thresholds).")
	fmt.Fprintln(os.Stderr, "Everything here is read-only - none of it ever writes to Artifactory. --analyze and --cross-env-analyze make no network calls at all.")
}

// runErrors collects errors hit while running the selected discovery
// pieces (--repos/--policies/--instance), so one broken API call - even a
// transient 5xx on the very first request - gets logged and the piece
// skipped, rather than aborting pieces that would otherwise succeed. All
// collected errors are summarized to stderr at the end of Run.
type runErrors struct {
	mu   sync.Mutex
	errs []string
}

func (e *runErrors) add(format string, args ...interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, fmt.Sprintf(format, args...))
}

func (e *runErrors) list() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.errs...)
}

// printNextSteps prints a "Next:" hint block to stderr so it's visible
// alongside progress output but never mixed into a CSV/JSON/HTML payload
// written to stdout.
func printNextSteps(lines ...string) {
	fmt.Fprintln(os.Stderr, "\nNext:")
	for _, l := range lines {
		fmt.Fprintf(os.Stderr, "  %s\n", l)
	}
}

// outputSubdir derives the --output directory name for a JPD base URL. It's
// the same instance.NameFromURL used to label the Overview tab and stamp
// Snapshot.Name, so an instance's output folder and its displayed name in
// the report always match (e.g. "https://psazuse.jfrog.io" -> "psazuse").
func outputSubdir(rawURL string) string {
	return instance.NameFromURL(rawURL)
}

// ensureParentDir creates path's parent directory (and any missing
// ancestors) so os.Create/os.WriteFile can't fail with ENOENT - covers both
// the computed <output>/<instance>/ default and an explicit nested path the
// user passed directly.
func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// splitList splits a comma-separated flag value into a trimmed, non-empty
// slice, e.g. "a, b ,,c" -> ["a","b","c"]. Returns nil for an empty input.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// repos
// ---------------------------------------------------------------------------

func doReposReport(client *jfclient.Client, csvPath string, concurrency int, withPolicies bool, include, exclude []string, mb *progress.Bars, errs *runErrors) int {
	opts := discover.ReportOptions{
		Concurrency:  concurrency,
		WithPolicies: withPolicies,
		Include:      include,
		Exclude:      exclude,
		Progress:     func(done, total int) { mb.Update("Repos", done, total) },
	}

	rows, warnings, err := discover.Report(client, opts)
	if err != nil {
		mb.Log("error:", err)
		errs.add("repos: %v", err)
		return 0
	}
	for _, w := range warnings {
		mb.Logf("discover --repos: warning: %s\n", w)
	}

	if err := ensureParentDir(csvPath); err != nil {
		mb.Log("error:", err)
		errs.add("repos: %v", err)
		return 0
	}
	f, err := os.Create(csvPath)
	if err != nil {
		mb.Log("error:", err)
		errs.add("repos: %v", err)
		return 0
	}
	defer f.Close()
	if err := discover.WriteReportCSV(f, rows, withPolicies); err != nil {
		mb.Log("error:", err)
		errs.add("repos: %v", err)
		return 0
	}
	mb.Logf("Wrote %d repo(s) to %s\n", len(rows), csvPath)
	return 0
}

// ---------------------------------------------------------------------------
// policies
// ---------------------------------------------------------------------------

func doPoliciesReport(client *jfclient.Client, csvPath string, mb *progress.Bars, errs *runErrors) int {
	repos, err := discover.ListRepoStorage(client)
	if err != nil {
		mb.Log("error:", err)
		errs.add("policies: %v", err)
		return 0
	}
	policies, err := discover.ListCleanupPolicies(client)
	if err != nil {
		mb.Log("error:", err)
		errs.add("policies: %v", err)
		return 0
	}

	if err := ensureParentDir(csvPath); err != nil {
		mb.Log("error:", err)
		errs.add("policies: %v", err)
		return 0
	}
	f, err := os.Create(csvPath)
	if err != nil {
		mb.Log("error:", err)
		errs.add("policies: %v", err)
		return 0
	}
	defer f.Close()
	if err := discover.WritePoliciesCSV(f, repos, policies); err != nil {
		mb.Log("error:", err)
		errs.add("policies: %v", err)
		return 0
	}
	mb.Logf("Wrote %d repo(s), %d policy/policies to %s\n", len(repos), len(policies), csvPath)
	return 0
}

// ---------------------------------------------------------------------------
// instance
// ---------------------------------------------------------------------------

func doInstanceSnapshot(client *jfclient.Client, outputPath string, concurrency, maxUsers, maxGroups, maxPermissions int, projectDetail bool, mb *progress.Bars, errs *runErrors) int {
	mb.Log("discover --instance: fetching repo list (for project repo-counts and federation targets)...")
	repos, err := discover.ListRepoStorage(client)
	if err != nil {
		mb.Log("error:", err)
		errs.add("instance: %v", err)
		mb.Log("discover --instance: continuing without a repo cross-reference (project repo-counts and federated-repo detection will be incomplete)")
		repos = nil
	}

	mb.Log("discover --instance: collecting system, security, users, groups, permissions, projects, replication, federation, platform, xray...")
	// Start from the defaults and override just what the flags set, rather
	// than a bespoke literal - so a future CollectOptions field picks up a
	// sane default here automatically instead of silently zero-valuing
	// until someone remembers to wire a flag for it too.
	collectOpts := discover.DefaultCollectOptions()
	collectOpts.Concurrency = concurrency
	collectOpts.MaxUsers = maxUsers
	collectOpts.MaxGroups = maxGroups
	collectOpts.MaxPermissions = maxPermissions
	collectOpts.ProjectDetail = projectDetail
	collectOpts.Progress = func(stage string, done, total int) { mb.Update(stage, done, total) }

	snap, err := discover.CollectInstance(client, repos, collectOpts)
	if err != nil {
		mb.Log("error:", err)
		errs.add("instance: %v", err)
		return 0
	}
	snap.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	for _, w := range snap.Warnings {
		mb.Logf("discover --instance: warning: %s\n", w)
	}
	if snap.UsersTruncated {
		mb.Logf("discover --instance: users list truncated at --max-users=%d\n", maxUsers)
	}
	if snap.GroupsTruncated {
		mb.Logf("discover --instance: groups list truncated at --max-groups=%d\n", maxGroups)
	}
	if snap.PermissionsTruncated {
		mb.Logf("discover --instance: permission targets truncated at --max-permissions=%d\n", maxPermissions)
	}

	if err := ensureParentDir(outputPath); err != nil {
		mb.Log("error:", err)
		errs.add("instance: %v", err)
		return 0
	}
	f, err := os.Create(outputPath)
	if err != nil {
		mb.Log("error:", err)
		errs.add("instance: %v", err)
		return 0
	}
	defer f.Close()
	if err := instance.WriteJSON(f, snap); err != nil {
		mb.Log("error:", err)
		errs.add("instance: %v", err)
		return 0
	}
	mb.Logf("Wrote %s (%d users, %d groups, %d permission targets, %d projects, %d replications, %d federated repos)\n",
		outputPath, len(snap.Users), len(snap.Groups), len(snap.Permissions), len(snap.Projects), len(snap.Replications), len(snap.FederatedRepos))
	return 0
}

// ---------------------------------------------------------------------------
// analyze
// ---------------------------------------------------------------------------

func doAnalyzeReport(reposCSV, instanceJSONPath, outputPath string, staleDays int, hugeGB float64, massiveFiles, longNameChars int) int {
	// Both inputs are optional independently of each other: a repos-only run
	// renders the single-panel Repos report, an instance-only run (e.g.
	// "discover --instance" without "--repos") renders the instance tabs
	// with an empty Repos tab, and either alone is enough to proceed. Only
	// the combination of neither existing is a real usage error.
	reposFound := false
	var header []string
	var rows []analyze.Row
	if f, err := os.Open(reposCSV); err == nil {
		reposFound = true
		defer f.Close()
		header, rows, err = analyze.ParseCSV(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	}

	instanceFound := false
	var snap *instance.Snapshot
	if instF, err := os.Open(instanceJSONPath); err == nil {
		instanceFound = true
		snap, err = instance.ReadJSON(instF)
		instF.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	}

	if !reposFound && !instanceFound {
		fmt.Fprintf(os.Stderr, "error: neither %s nor %s exists (run --repos and/or --instance first, or pass explicit --repos-csv/--instance-json paths)\n", reposCSV, instanceJSONPath)
		return 1
	}

	opts := analyze.Options{
		StaleDays:     staleDays,
		HugeGB:        hugeGB,
		MassiveFiles:  massiveFiles,
		LongNameChars: longNameChars,
	}
	now := time.Now()
	analyze.ComputeFindings(header, rows, opts, now)

	sourceFile := reposCSV
	if !reposFound {
		sourceFile = instanceJSONPath
	}
	html, err := analyze.Render(header, rows, analyze.RenderOptions{
		SourceFile:  sourceFile,
		Opts:        opts,
		GeneratedAt: now,
		Instance:    snap,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := ensureParentDir(outputPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := os.WriteFile(outputPath, []byte(html), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	suffix := ""
	switch {
	case snap == nil:
		suffix = fmt.Sprintf(" (no %s found - repos-only report; run --instance to add Overview/Access/Projects/Federation tabs)", instanceJSONPath)
	case !reposFound:
		suffix = fmt.Sprintf(" (no %s found - instance-only report; run --repos to add per-repo storage/activity data)", reposCSV)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s (%d repos analyzed)%s\n", outputPath, len(rows), suffix)
	printNextSteps(fmt.Sprintf("open %s in a browser to explore the report", outputPath))
	return 0
}

// ---------------------------------------------------------------------------
// cross-env-analyze
// ---------------------------------------------------------------------------

func doCrossEnvAnalyzeReport(outputDir, reportPath string, mb *progress.Bars, errs *runErrors) int {
	mb.Log("discover --cross-env-analyze: scanning environments in", outputDir)

	report, err := analyze.AnalyzeCrossEnvironments(outputDir)
	if err != nil {
		mb.Log("error:", err)
		errs.add("cross-env-analyze: %v", err)
		return 1
	}

	report.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	if err := ensureParentDir(reportPath); err != nil {
		mb.Log("error:", err)
		errs.add("cross-env-analyze: %v", err)
		return 1
	}

	if err := analyze.WriteCrossEnvReport(report, reportPath); err != nil {
		mb.Log("error:", err)
		errs.add("cross-env-analyze: %v", err)
		return 1
	}

	baseDir := filepath.Dir(reportPath)
	baseName := strings.TrimSuffix(filepath.Base(reportPath), filepath.Ext(reportPath))
	jsonPath := filepath.Join(baseDir, baseName+".json")
	htmlPath := filepath.Join(baseDir, baseName+".html")

	mb.Logf("Cross-environment analysis complete: %d environment(s), %d total repo(s), %d duplicate(s)\n",
		len(report.Environments), report.TotalRepos, report.DuplicateCount)
	mb.Logf("Wrote %s\n", reportPath)
	mb.Logf("Wrote %s\n", jsonPath)
	mb.Logf("Wrote %s\n", htmlPath)

	printNextSteps(
		fmt.Sprintf("open %s in a browser to explore the interactive report", htmlPath),
		fmt.Sprintf("view text summary: %s", reportPath),
		fmt.Sprintf("view JSON data: %s", jsonPath),
	)
	return 0
}

// ---------------------------------------------------------------------------
// project roles, project repos, and apptrust (derived from instance data)
// ---------------------------------------------------------------------------

func doProjectRolesCSV(snap *instance.Snapshot, csvPath string, mb *progress.Bars, errs *runErrors) int {
	roles := discover.FlattenProjectRoles(snap.Projects)
	if err := ensureParentDir(csvPath); err != nil {
		mb.Log("error:", err)
		errs.add("project-roles: %v", err)
		return 0
	}
	f, err := os.Create(csvPath)
	if err != nil {
		mb.Log("error:", err)
		errs.add("project-roles: %v", err)
		return 0
	}
	defer f.Close()
	if err := discover.WriteProjectRolesCSV(f, roles); err != nil {
		mb.Log("error:", err)
		errs.add("project-roles: %v", err)
		return 0
	}
	mb.Logf("Wrote %d project role(s) to %s\n", len(roles), csvPath)
	return 0
}

func doProjectReposCSV(snap *instance.Snapshot, csvPath string, mb *progress.Bars, errs *runErrors) int {
	// We need the repos list for detailed mapping info; for now, build from projects
	mappings := discover.BuildProjectRepositoryMapping(snap.Projects, nil)
	if err := ensureParentDir(csvPath); err != nil {
		mb.Log("error:", err)
		errs.add("project-repos: %v", err)
		return 0
	}
	f, err := os.Create(csvPath)
	if err != nil {
		mb.Log("error:", err)
		errs.add("project-repos: %v", err)
		return 0
	}
	defer f.Close()
	if err := discover.WriteProjectRepositoriesCSV(f, mappings); err != nil {
		mb.Log("error:", err)
		errs.add("project-repos: %v", err)
		return 0
	}
	mb.Logf("Wrote %d project-repository mapping(s) to %s\n", len(mappings), csvPath)
	return 0
}

func doAppTrustCSVs(snap *instance.Snapshot, csvPathPattern string, mb *progress.Bars, errs *runErrors) int {
	// Replace .csv with .<resource>.csv to generate multiple files from one pattern
	// e.g. /path/to/apptrust.csv -> /path/to/apptrust.applications.csv, etc.
	dir := filepath.Dir(csvPathPattern)
	ext := filepath.Ext(csvPathPattern)
	base := strings.TrimSuffix(filepath.Base(csvPathPattern), ext)

	resources := []struct {
		name   string
		count  int
		write  func(path string) error
	}{
		{"applications", len(snap.AppTrustApplications), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustApplicationsCSV(f, snap.AppTrustApplications)
		}},
		{"policies", len(snap.AppTrustPolicies), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustPoliciesCSV(f, snap.AppTrustPolicies)
		}},
		{"rules", len(snap.AppTrustRules), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustRulesCSV(f, snap.AppTrustRules)
		}},
		{"templates", len(snap.AppTrustTemplates), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustTemplatesCSV(f, snap.AppTrustTemplates)
		}},
		{"stages", len(snap.AppTrustStages), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustStagesCSV(f, snap.AppTrustStages)
		}},
		{"evidence", len(snap.AppTrustEvidence), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustEvidenceCSV(f, snap.AppTrustEvidence)
		}},
		{"release-bundles", len(snap.AppTrustReleaseBundles), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustReleaseBundlesCSV(f, snap.AppTrustReleaseBundles)
		}},
		{"application-policies", len(snap.AppTrustApplicationPolicies), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustApplicationPoliciesCSV(f, snap.AppTrustApplicationPolicies)
		}},
		{"policy-rules", len(snap.AppTrustPolicyRules), func(path string) error {
			f, err := os.Create(path)
			if err != nil {
				return err
			}
			defer f.Close()
			return discover.WriteAppTrustPolicyRulesCSV(f, snap.AppTrustPolicyRules)
		}},
	}

	if err := ensureParentDir(csvPathPattern); err != nil {
		mb.Log("error:", err)
		errs.add("apptrust: %v", err)
		return 0
	}

	for _, r := range resources {
		if r.count == 0 {
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("%s.%s%s", base, r.name, ext))
		if err := r.write(path); err != nil {
			mb.Log("error:", err)
			errs.add("apptrust %s: %v", r.name, err)
			return 0
		}
		mb.Logf("Wrote %d AppTrust %s to %s\n", r.count, r.name, path)
	}
	return 0
}

