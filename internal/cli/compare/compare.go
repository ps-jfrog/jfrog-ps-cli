// Package comparecmd wires the "jfps compare" command group -- verifying
// migrations between two JFrog Platform Deployments by comparing repository
// contents, build-info, and generic JSON/config files -- to the pure
// business logic in internal/compare.
package comparecmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"jfrogpscli/internal/compare"
	"jfrogpscli/internal/config"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("compare", "Compare repo contents, build-info, and JSON configs between two JPDs", Run)
}

// Run dispatches to the compare subcommands.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "repos":
		return runRepos(args[1:])
	case "builds":
		return runBuilds(args[1:])
	case "json":
		return runJSON(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "jfps compare: unknown subcommand %q\n\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps compare <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  repos    Compare repository contents (checksums) between two JPDs")
	fmt.Fprintln(os.Stderr, "  builds   Compare build-info between two JPDs, optionally migrate what's missing")
	fmt.Fprintln(os.Stderr, "  json     Diff two local JSON files (added/removed/changed keys)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps compare <subcommand> -h' for subcommand-specific flags.")
}

// isHelp reports whether args starts with a help request.
func isHelp(args []string) bool {
	return len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help")
}

// ---------------------------------------------------------------------------
// repos
// ---------------------------------------------------------------------------

func runRepos(args []string) int {
	fs := flag.NewFlagSet("compare repos", flag.ContinueOnError)
	source := config.Register(fs, "source-")
	target := config.Register(fs, "target-")
	repo := fs.String("repo", "", "Repository key to compare (mutually exclusive with --all)")
	all := fs.Bool("all", false, "Compare every local-type repository on the source server")
	reportDir := fs.String("report-dir", "", "Directory to write per-repo reports and a transfer-plan.txt into, instead of printing tables")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: jfps compare repos --source-server <name> --target-server <name> (--repo <repo> | --all) [--report-dir <dir>]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Compares repository contents (path/name/sha256) between two JPDs.")
		fmt.Fprintln(fs.Output(), "Source is ground truth: reports items missing from target (MISSING) and")
		fmt.Fprintln(fs.Output(), "items present on both sides with a different sha256 (DIFFERENT).")
		fmt.Fprintln(fs.Output(), "This command is read-only: it never modifies either server.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	if isHelp(args) {
		fs.Usage()
		return 0
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *repo == "" && !*all {
		fmt.Fprintln(os.Stderr, "jfps compare repos: one of --repo or --all is required")
		return 1
	}
	if *repo != "" && *all {
		fmt.Fprintln(os.Stderr, "jfps compare repos: --repo and --all are mutually exclusive")
		return 1
	}

	sourceClient, err := source.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare repos: %v\n", err)
		return 1
	}
	targetClient, err := target.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare repos: %v\n", err)
		return 1
	}

	var repos []string
	if *all {
		repos, err = compare.ListLocalRepos(sourceClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jfps compare repos: %v\n", err)
			return 1
		}
	} else {
		repos = []string{*repo}
	}

	results, err := compare.CompareRepos(sourceClient, targetClient, repos)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare repos: %v\n", err)
		return 1
	}

	if *reportDir != "" {
		if err := compare.WriteRepoReports(*reportDir, results); err != nil {
			fmt.Fprintf(os.Stderr, "jfps compare repos: %v\n", err)
			return 1
		}
		total := 0
		for _, r := range results {
			total += len(r.Diffs)
		}
		fmt.Printf("Wrote reports for %d repo(s) (%d item(s) need sync) to %s\n", len(results), total, *reportDir)
		return 0
	}

	printRepoResults(results)
	return 0
}

func printRepoResults(results []compare.RepoDiffResult) {
	for _, res := range results {
		fmt.Printf("\nRepo: %s (source items: %d, target items: %d)\n", res.Repo, res.Source, res.Target)
		if len(res.Diffs) == 0 {
			fmt.Println("No differences found.")
			continue
		}
		rows := make([][]string, 0, len(res.Diffs))
		for _, d := range res.Diffs {
			rows = append(rows, []string{
				string(d.Status), d.Path, d.Name, d.SourceSHA256, d.TargetSHA256,
			})
		}
		output.Table(os.Stdout, []string{"STATUS", "PATH", "NAME", "SOURCE_SHA256", "TARGET_SHA256"}, rows)
	}
}

// ---------------------------------------------------------------------------
// builds
// ---------------------------------------------------------------------------

func runBuilds(args []string) int {
	fs := flag.NewFlagSet("compare builds", flag.ContinueOnError)
	source := config.Register(fs, "source-")
	target := config.Register(fs, "target-")
	migrate := fs.Bool("migrate", false, "Migrate missing build-info from source to target (see --dry-run)")
	dryRun := fs.Bool("dry-run", true, "Preview the migration without uploading anything; pass --dry-run=false to apply")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: jfps compare builds --source-server <name> --target-server <name> [--migrate] [--dry-run=true]")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Compares build names and build numbers between two JPDs. Source is ground")
		fmt.Fprintln(fs.Output(), "truth: reports build names and numbers present on source but missing from target.")
		fmt.Fprintln(fs.Output(), "With --migrate --dry-run=false, uploads each missing build-info to target.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	if isHelp(args) {
		fs.Usage()
		return 0
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	sourceClient, err := source.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare builds: %v\n", err)
		return 1
	}
	targetClient, err := target.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare builds: %v\n", err)
		return 1
	}

	diff, err := compare.CompareBuilds(sourceClient, targetClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare builds: %v\n", err)
		return 1
	}

	printBuildDiff(diff)

	if !*migrate {
		return 0
	}

	pairs, err := compare.MissingPairs(sourceClient, diff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare builds: %v\n", err)
		return 1
	}
	if len(pairs) == 0 {
		fmt.Println("\nNothing to migrate.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("migrate %d build(s)", len(pairs)))
	}

	results := compare.MigrateBuildPairs(sourceClient, targetClient, pairs, *dryRun)
	rows := make([][]string, 0, len(results))
	failed := false
	for _, r := range results {
		status := "OK"
		if r.DryRun {
			status = "DRY-RUN"
		} else if !r.Success {
			status = "FAILED"
			failed = true
		}
		rows = append(rows, []string{r.Name, r.Number, status, r.Error})
	}
	fmt.Println()
	output.Table(os.Stdout, []string{"BUILD", "NUMBER", "STATUS", "ERROR"}, rows)

	if failed {
		return 1
	}
	return 0
}

func printBuildDiff(diff compare.BuildDiff) {
	sort.Strings(diff.MissingBuildNames)
	fmt.Println("Missing build names (present on source, absent on target):")
	if len(diff.MissingBuildNames) == 0 {
		fmt.Println("  none")
	} else {
		for _, name := range diff.MissingBuildNames {
			fmt.Printf("  %s\n", name)
		}
	}

	sharedWithMissing := make([]string, 0, len(diff.MissingNumbers))
	for name := range diff.MissingNumbers {
		sharedWithMissing = append(sharedWithMissing, name)
	}
	sort.Strings(sharedWithMissing)

	fmt.Println("\nMissing build numbers on shared build names:")
	if len(sharedWithMissing) == 0 {
		fmt.Println("  none")
		return
	}
	rows := make([][]string, 0)
	for _, name := range sharedWithMissing {
		numbers := diff.MissingNumbers[name]
		for _, num := range numbers {
			rows = append(rows, []string{name, num})
		}
	}
	output.Table(os.Stdout, []string{"BUILD", "MISSING_NUMBER"}, rows)
}

// ---------------------------------------------------------------------------
// json
// ---------------------------------------------------------------------------

func runJSON(args []string) int {
	fs := flag.NewFlagSet("compare json", flag.ContinueOnError)
	file1 := fs.String("file1", "", "Path to the first JSON file")
	file2 := fs.String("file2", "", "Path to the second JSON file")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: jfps compare json --file1 <path> --file2 <path>")
		fmt.Fprintln(fs.Output(), "")
		fmt.Fprintln(fs.Output(), "Diffs two local JSON object files: keys only in file1 are 'removed'")
		fmt.Fprintln(fs.Output(), "(relative to file2), keys only in file2 are 'added', and keys in both")
		fmt.Fprintln(fs.Output(), "with different values are 'changed'. Purely local; no server connection needed.")
		fmt.Fprintln(fs.Output(), "")
		fs.PrintDefaults()
	}
	if isHelp(args) {
		fs.Usage()
		return 0
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *file1 == "" || *file2 == "" {
		fmt.Fprintln(os.Stderr, "jfps compare json: --file1 and --file2 are required")
		return 1
	}

	diff, err := compare.CompareJSONFiles(*file1, *file2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jfps compare json: %v\n", err)
		return 1
	}

	printJSONDiff(diff)
	return 0
}

func printJSONDiff(diff compare.JSONDiff) {
	if diff.Empty() {
		fmt.Println("no differences found")
		return
	}

	if len(diff.Added) > 0 {
		fmt.Println("Added (present only in file2):")
		rows := make([][]string, 0, len(diff.Added))
		for _, c := range diff.Added {
			rows = append(rows, []string{c.Key, formatValue(c.NewValue)})
		}
		output.Table(os.Stdout, []string{"KEY", "VALUE"}, rows)
		fmt.Println()
	}

	if len(diff.Removed) > 0 {
		fmt.Println("Removed (present only in file1):")
		rows := make([][]string, 0, len(diff.Removed))
		for _, c := range diff.Removed {
			rows = append(rows, []string{c.Key, formatValue(c.OldValue)})
		}
		output.Table(os.Stdout, []string{"KEY", "VALUE"}, rows)
		fmt.Println()
	}

	if len(diff.Changed) > 0 {
		fmt.Println("Changed (different values):")
		rows := make([][]string, 0, len(diff.Changed))
		for _, c := range diff.Changed {
			rows = append(rows, []string{c.Key, formatValue(c.OldValue), formatValue(c.NewValue)})
		}
		output.Table(os.Stdout, []string{"KEY", "OLD_VALUE", "NEW_VALUE"}, rows)
	}
}

// formatValue renders a decoded JSON value as a compact single-line string
// for table display.
func formatValue(v interface{}) string {
	if v == nil {
		return "null"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
