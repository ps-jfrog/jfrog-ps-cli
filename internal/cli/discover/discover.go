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
	"strings"
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
// --repos/--policies/--instance/--analyze (or --all) were selected, in
// that fixed order, so a repo report or instance snapshot generated
// earlier in the same invocation is what --analyze picks up.
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

	// --- output/input path flags (shared between generating and analyzing) ---
	reposCSV := fs.String("repos-csv", "report.csv", "Path for the repo report CSV (written by --repos, read by --analyze)")
	policiesCSV := fs.String("policies-csv", "policies.csv", "Path for the policy-mapping CSV (written by --policies)")
	instanceJSON := fs.String("instance-json", "instance.json", "Path for the instance snapshot JSON (written by --instance, read by --analyze if present)")
	auditHTML := fs.String("audit-html", "audit.html", "Path for the rendered HTML audit (written by --analyze)")

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

	if *doRepos {
		if code := doReposReport(client, *reposCSV, *concurrency, *withPolicies, splitList(*include), splitList(*exclude)); code != 0 {
			return code
		}
	}
	if *doPolicies {
		if code := doPoliciesReport(client, *policiesCSV); code != 0 {
			return code
		}
	}
	if *doInstance {
		if code := doInstanceSnapshot(client, *instanceJSON, *concurrency, *maxUsers, *maxGroups, *maxPermissions, *projectDetail); code != 0 {
			return code
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
	fmt.Fprintln(os.Stderr, "Flags select which piece(s) to run, in this fixed order: repos, policies, instance, analyze -")
	fmt.Fprintln(os.Stderr, "so 'discover --repos --instance --analyze' generates the report and snapshot, then renders")
	fmt.Fprintln(os.Stderr, "the combined HTML audit, all in one invocation.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  --repos       Per-repo package type, file count, usage, project, last-push/last-pull, cleanup-policy detail -> --repos-csv")
	fmt.Fprintln(os.Stderr, "  --policies    Map every repo to its matching cleanup policy/policies (one row per repo x match) -> --policies-csv")
	fmt.Fprintln(os.Stderr, "  --instance    Versions, storage, auth/signing-keys, users/groups/permissions, projects, platform & Xray config -> --instance-json")
	fmt.Fprintln(os.Stderr, "  --analyze     Turn --repos-csv (+ --instance-json if present) into a self-contained interactive HTML audit -> --audit-html")
	fmt.Fprintln(os.Stderr, "  --all         Shorthand for --repos --policies --instance --analyze")
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

func doReposReport(client *jfclient.Client, csvPath string, concurrency int, withPolicies bool, include, exclude []string) int {
	opts := discover.ReportOptions{
		Concurrency:  concurrency,
		WithPolicies: withPolicies,
		Include:      include,
		Exclude:      exclude,
		Progress:     func(done, total int) { progressBar("Repos", done, total) },
	}

	rows, warnings, err := discover.Report(client, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "discover --repos: warning: %s\n", w)
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

func doInstanceSnapshot(client *jfclient.Client, outputPath string, concurrency, maxUsers, maxGroups, maxPermissions int, projectDetail bool) int {
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
	collectOpts.Progress = func(stage string, done, total int) { progressBar(stage, done, total) }

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
