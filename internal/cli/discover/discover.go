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
	doAll := fs.Bool("all", false, "Shorthand for --repos --policies --instance --analyze")
	parallel := fs.Bool("parallel", true, "Run --repos/--policies/--instance concurrently instead of sequentially when more than one is selected - faster, since they're independent network calls against the same JPD. Progress bars are suppressed while running concurrently (multiple live bars would garble the terminal); set false to run sequentially with full progress bars")

	// --- output/input path flags (shared between generating and analyzing) ---
	outputDir := fs.String("output", "output", "Base output folder; a subfolder named after the JPD's hostname (e.g. 'output/psazuse') is created under it and used for any of the paths below left at their default")
	reposCSV := fs.String("repos-csv", "", "Path for the repo report CSV (written by --repos, read by --analyze) - default: <output>/<instance>/report.csv")
	policiesCSV := fs.String("policies-csv", "", "Path for the policy-mapping CSV (written by --policies) - default: <output>/<instance>/policies.csv")
	instanceJSON := fs.String("instance-json", "", "Path for the instance snapshot JSON (written by --instance, read by --analyze if present) - default: <output>/<instance>/instance.json")
	auditHTML := fs.String("audit-html", "", "Path for the rendered HTML audit (written by --analyze) - default: <output>/<instance>/audit.html")

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
	}
	if !*doRepos && !*doPolicies && !*doInstance && !*doAnalyze {
		printUsage()
		return 1
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

	selected := 0
	for _, on := range []bool{*doRepos, *doPolicies, *doInstance} {
		if on {
			selected++
		}
	}
	// Only worth parallelizing (and only safe to show live progress bars
	// for) when a single piece runs - two or more live \r-based bars
	// updating the same terminal concurrently would garble each other.
	runConcurrently := *parallel && selected > 1
	showProgress := !runConcurrently

	runRepos := func() int {
		return doReposReport(client, *reposCSV, *concurrency, *withPolicies, splitList(*include), splitList(*exclude), showProgress)
	}
	runPolicies := func() int { return doPoliciesReport(client, *policiesCSV) }
	runInstance := func() int {
		return doInstanceSnapshot(client, *instanceJSON, *concurrency, *maxUsers, *maxGroups, *maxPermissions, *projectDetail, showProgress)
	}

	if runConcurrently {
		fmt.Fprintln(os.Stderr, "discover: running repos/policies/instance concurrently (pass --parallel=false to run sequentially with full progress bars)")
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

	if *doAnalyze {
		if code := doAnalyzeReport(*reposCSV, *instanceJSON, *auditHTML, *staleDays, *hugeGB, *massiveFiles, *longNameChars); code != 0 {
			return code
		}
	}
	return 0
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps discover [--repos] [--policies] [--instance] [--analyze] [--all] [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags select which piece(s) to run - repos/policies/instance run concurrently by default when more")
	fmt.Fprintln(os.Stderr, "than one is selected (--parallel=false for sequentially, with full progress bars), and analyze")
	fmt.Fprintln(os.Stderr, "always runs last, so 'discover --repos --instance --analyze' generates the report and snapshot,")
	fmt.Fprintln(os.Stderr, "then renders the combined HTML audit, all in one invocation.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  --repos       Per-repo package type, file count, usage, project, last-push/last-pull, cleanup-policy detail -> --repos-csv")
	fmt.Fprintln(os.Stderr, "  --policies    Map every repo to its matching cleanup policy/policies (one row per repo x match) -> --policies-csv")
	fmt.Fprintln(os.Stderr, "  --instance    Versions, storage, auth/signing-keys, users/groups/permissions, projects, platform & Xray config -> --instance-json")
	fmt.Fprintln(os.Stderr, "  --analyze     Turn --repos-csv (+ --instance-json if present) into a self-contained interactive HTML audit -> --audit-html")
	fmt.Fprintln(os.Stderr, "  --all         Shorthand for --repos --policies --instance --analyze")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Output files land under --output (default \"output\"), in a subfolder named after the JPD's")
	fmt.Fprintln(os.Stderr, "hostname, e.g. 'output/psazuse/report.csv' - unless --repos-csv/--policies-csv/--instance-json/")
	fmt.Fprintln(os.Stderr, "--audit-html override an individual path explicitly.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps discover -h' to see every flag (output paths, concurrency, caps, analyze thresholds).")
	fmt.Fprintln(os.Stderr, "Everything here is read-only - none of it ever writes to Artifactory. --analyze makes no network calls at all.")
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

func doReposReport(client *jfclient.Client, csvPath string, concurrency int, withPolicies bool, include, exclude []string, showProgress bool) int {
	opts := discover.ReportOptions{
		Concurrency:  concurrency,
		WithPolicies: withPolicies,
		Include:      include,
		Exclude:      exclude,
	}
	if showProgress {
		opts.Progress = func(done, total int) { progressBar("Repos", done, total) }
	}

	rows, warnings, err := discover.Report(client, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "discover --repos: warning: %s\n", w)
	}

	if err := ensureParentDir(csvPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	f, err := os.Create(csvPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	if err := discover.WriteReportCSV(f, rows, withPolicies); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Wrote %d repo(s) to %s\n", len(rows), csvPath)
	return 0
}

// ---------------------------------------------------------------------------
// policies
// ---------------------------------------------------------------------------

func doPoliciesReport(client *jfclient.Client, csvPath string) int {
	repos, err := discover.ListRepoStorage(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	policies, err := discover.ListCleanupPolicies(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	if err := ensureParentDir(csvPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	f, err := os.Create(csvPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	if err := discover.WritePoliciesCSV(f, repos, policies); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Wrote %d repo(s), %d policy/policies to %s\n", len(repos), len(policies), csvPath)
	return 0
}

// ---------------------------------------------------------------------------
// instance
// ---------------------------------------------------------------------------

func doInstanceSnapshot(client *jfclient.Client, outputPath string, concurrency, maxUsers, maxGroups, maxPermissions int, projectDetail, showProgress bool) int {
	fmt.Fprintln(os.Stderr, "discover --instance: fetching repo list (for project repo-counts and federation targets)...")
	repos, err := discover.ListRepoStorage(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "discover --instance: collecting system, security, users, groups, permissions, projects, replication, federation, platform, xray...")
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
	if showProgress {
		collectOpts.Progress = func(stage string, done, total int) { progressBar(stage, done, total) }
	}

	snap, err := discover.CollectInstance(client, repos, collectOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	snap.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	for _, w := range snap.Warnings {
		fmt.Fprintf(os.Stderr, "discover --instance: warning: %s\n", w)
	}
	if snap.UsersTruncated {
		fmt.Fprintf(os.Stderr, "discover --instance: users list truncated at --max-users=%d\n", maxUsers)
	}
	if snap.GroupsTruncated {
		fmt.Fprintf(os.Stderr, "discover --instance: groups list truncated at --max-groups=%d\n", maxGroups)
	}
	if snap.PermissionsTruncated {
		fmt.Fprintf(os.Stderr, "discover --instance: permission targets truncated at --max-permissions=%d\n", maxPermissions)
	}

	if err := ensureParentDir(outputPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	f, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	if err := instance.WriteJSON(f, snap); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Wrote %s (%d users, %d groups, %d permission targets, %d projects, %d replications, %d federated repos)\n",
		outputPath, len(snap.Users), len(snap.Groups), len(snap.Permissions), len(snap.Projects), len(snap.Replications), len(snap.FederatedRepos))
	return 0
}

// ---------------------------------------------------------------------------
// analyze
// ---------------------------------------------------------------------------

func doAnalyzeReport(reposCSV, instanceJSONPath, outputPath string, staleDays int, hugeGB float64, massiveFiles, longNameChars int) int {
	f, err := os.Open(reposCSV)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening %s: %v (run with --repos first, or pass --repos-csv pointing at an existing report)\n", reposCSV, err)
		return 1
	}
	defer f.Close()

	header, rows, err := analyze.ParseCSV(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// --instance-json is optional for --analyze: if the file isn't there,
	// just render the single-panel repos report, same as never having run
	// --instance at all.
	var snap *instance.Snapshot
	if instF, err := os.Open(instanceJSONPath); err == nil {
		snap, err = instance.ReadJSON(instF)
		instF.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	}

	opts := analyze.Options{
		StaleDays:     staleDays,
		HugeGB:        hugeGB,
		MassiveFiles:  massiveFiles,
		LongNameChars: longNameChars,
	}
	now := time.Now()
	analyze.ComputeFindings(header, rows, opts, now)

	html, err := analyze.Render(header, rows, analyze.RenderOptions{
		SourceFile:  reposCSV,
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
	if snap == nil {
		suffix = fmt.Sprintf(" (no %s found - repos-only report; run --instance to add Overview/Access/Projects/Federation tabs)", instanceJSONPath)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s (%d repos analyzed)%s\n", outputPath, len(rows), suffix)
	printNextSteps(fmt.Sprintf("open %s in a browser to explore the report", outputPath))
	return 0
}
