// Package discovercmd wires the internal/discover and internal/analyze
// business logic into the "jfps discover" command group: per-repo
// storage/activity/cleanup-policy reporting and turning a report CSV into a
// self-contained interactive HTML audit. Every subcommand here is read-only
// (no dry-run flag - there is nothing to preview, since nothing is ever
// mutated).
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
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("discover", "Repo storage/activity/cleanup-policy reporting and HTML audit generation", Run)
}

// Run dispatches to the discover subcommands.
func Run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage()
		if len(args) == 0 {
			return 1
		}
		return 0
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "repos":
		return runRepos(rest)
	case "policies":
		return runPolicies(rest)
	case "instance":
		return runInstance(rest)
	case "analyze":
		return runAnalyze(rest)
	default:
		fmt.Fprintf(os.Stderr, "discover: unknown subcommand %q\n\n", sub)
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps discover <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  repos      Per-repo package type, file count, usage, last-push/last-pull, and matched cleanup-policy detail -> CSV")
	fmt.Fprintln(os.Stderr, "  policies   Map every repo to its matching cleanup policy/policies (one row per repo x match) -> CSV")
	fmt.Fprintln(os.Stderr, "  instance   Component versions, storage, auth/signing-key config, users/groups/permissions, projects, replication & federation -> JSON")
	fmt.Fprintln(os.Stderr, "  analyze    Turn a 'discover repos' CSV (optionally + a 'discover instance' JSON) into a self-contained interactive HTML audit")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps discover <subcommand> -h' for subcommand-specific flags.")
	fmt.Fprintln(os.Stderr, "All four subcommands are read-only - none of them ever writes to Artifactory. 'analyze' makes no network calls at all.")
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

func runRepos(args []string) int {
	fs := flag.NewFlagSet("discover repos", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	concurrency := fs.Int("concurrency", 8, "Repos processed concurrently (be considerate on shared instances)")
	withPolicies := fs.Bool("cleanup-policies", true, "Include matched cleanup-policy detail columns (one extra API call)")
	include := fs.String("include", "", "Comma-separated glob patterns: only report repos matching at least one")
	exclude := fs.String("exclude", "", "Comma-separated glob patterns: skip repos matching any of these")
	csvPath := fs.String("csv", "", "Path to write the CSV to (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	opts := discover.ReportOptions{
		Concurrency:  *concurrency,
		WithPolicies: *withPolicies,
		Include:      splitList(*include),
		Exclude:      splitList(*exclude),
		Progress: func(done, total int) {
			fmt.Fprintf(os.Stderr, "\rdiscover repos: %d/%d", done, total)
			if done == total {
				fmt.Fprintln(os.Stderr)
			}
		},
	}

	rows, warnings, err := discover.Report(client, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "discover repos: warning: %s\n", w)
	}

	out := os.Stdout
	if *csvPath != "" {
		f, err := os.Create(*csvPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	if err := discover.WriteReportCSV(out, rows, *withPolicies); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *csvPath != "" {
		fmt.Fprintf(os.Stderr, "Wrote %d repo(s) to %s\n", len(rows), *csvPath)
	}
	return 0
}

// ---------------------------------------------------------------------------
// policies
// ---------------------------------------------------------------------------

func runPolicies(args []string) int {
	fs := flag.NewFlagSet("discover policies", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	csvPath := fs.String("csv", "", "Path to write the CSV to (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

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

	out := os.Stdout
	if *csvPath != "" {
		f, err := os.Create(*csvPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	if err := discover.WritePoliciesCSV(out, repos, policies); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if *csvPath != "" {
		fmt.Fprintf(os.Stderr, "Wrote %d repo(s), %d policy/policies to %s\n", len(repos), len(policies), *csvPath)
	}
	return 0
}

// ---------------------------------------------------------------------------
// instance
// ---------------------------------------------------------------------------

func runInstance(args []string) int {
	fs := flag.NewFlagSet("discover instance", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	concurrency := fs.Int("concurrency", 8, "Permission-target and federated-repo config fetches processed concurrently")
	maxUsers := fs.Int("max-users", discover.DefaultCollectOptions().MaxUsers, "Cap on users collected (0 = unlimited)")
	maxGroups := fs.Int("max-groups", discover.DefaultCollectOptions().MaxGroups, "Cap on groups collected (0 = unlimited)")
	maxPermissions := fs.Int("max-permissions", discover.DefaultCollectOptions().MaxPermissions, "Cap on permission targets collected (0 = unlimited)")
	output := fs.String("output", "instance.json", "Path to write the instance snapshot JSON")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "discover instance: fetching repo list (for project repo-counts and federation targets)...")
	repos, err := discover.ListRepoStorage(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "discover instance: collecting system, security, users, groups, permissions, projects, replication, federation...")
	snap, err := discover.CollectInstance(client, repos, discover.CollectOptions{
		Concurrency:    *concurrency,
		MaxUsers:       *maxUsers,
		MaxGroups:      *maxGroups,
		MaxPermissions: *maxPermissions,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	snap.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	for _, w := range snap.Warnings {
		fmt.Fprintf(os.Stderr, "discover instance: warning: %s\n", w)
	}
	if snap.UsersTruncated {
		fmt.Fprintf(os.Stderr, "discover instance: users list truncated at --max-users=%d\n", *maxUsers)
	}
	if snap.GroupsTruncated {
		fmt.Fprintf(os.Stderr, "discover instance: groups list truncated at --max-groups=%d\n", *maxGroups)
	}
	if snap.PermissionsTruncated {
		fmt.Fprintf(os.Stderr, "discover instance: permission targets truncated at --max-permissions=%d\n", *maxPermissions)
	}

	f, err := os.Create(*output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()
	if err := instance.WriteJSON(f, snap); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Wrote %s (%d users, %d groups, %d permission targets, %d projects, %d replications, %d federated repos)\n",
		*output, len(snap.Users), len(snap.Groups), len(snap.Permissions), len(snap.Projects), len(snap.Replications), len(snap.FederatedRepos))
	return 0
}

// ---------------------------------------------------------------------------
// analyze
// ---------------------------------------------------------------------------

func runAnalyze(args []string) int {
	fs := flag.NewFlagSet("discover analyze", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	input := fs.String("input", "", "Path to a 'discover repos' CSV (required)")
	instancePath := fs.String("instance", "", "Optional path to a 'discover instance' JSON snapshot to embed system/security/access/projects/federation sections")
	output := fs.String("output", "repo-audit.html", "Path to write the HTML report")
	staleDays := fs.Int("stale-days", analyze.DefaultOptions().StaleDays, "Flag non-empty repos with no push/pull inside this many days")
	hugeGB := fs.Float64("huge-gb", analyze.DefaultOptions().HugeGB, "Flag repos using more than this many GB")
	massiveFiles := fs.Int("massive-files", analyze.DefaultOptions().MassiveFiles, "Flag repos with more than this many files")
	longNameChars := fs.Int("long-name-chars", analyze.DefaultOptions().LongNameChars, "Flag repo keys longer than this many characters")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *input == "" {
		fmt.Fprintln(os.Stderr, "error: --input is required")
		return 1
	}

	f, err := os.Open(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer f.Close()

	header, rows, err := analyze.ParseCSV(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	var snap *instance.Snapshot
	if *instancePath != "" {
		instF, err := os.Open(*instancePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		snap, err = instance.ReadJSON(instF)
		instF.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
	}

	opts := analyze.Options{
		StaleDays:     *staleDays,
		HugeGB:        *hugeGB,
		MassiveFiles:  *massiveFiles,
		LongNameChars: *longNameChars,
	}
	now := time.Now()
	analyze.ComputeFindings(header, rows, opts, now)

	html, err := analyze.Render(header, rows, analyze.RenderOptions{
		SourceFile:  *input,
		Opts:        opts,
		GeneratedAt: now,
		Instance:    snap,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if err := os.WriteFile(*output, []byte(html), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Wrote %s (%d repos analyzed)\n", *output, len(rows))
	return 0
}
