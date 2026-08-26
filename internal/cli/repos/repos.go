// Package reposcmd is the CLI wrapper around internal/repos: list, diff,
// sync-config, and delete subcommands for Artifactory repositories.
package reposcmd

import (
	"flag"
	"fmt"
	"os"

	"jfrogpscli/internal/config"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/registry"
	"jfrogpscli/internal/repos"
)

func init() {
	registry.Register("repos", "List, diff, sync, and delete Artifactory repositories", Run)
}

// Run dispatches to the repos subcommands.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return 0
	case "list":
		return runList(args[1:])
	case "diff":
		return runDiff(args[1:])
	case "sync-config":
		return runSyncConfig(args[1:])
	case "delete":
		return runDelete(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "jfps repos: unknown subcommand %q\n\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps repos <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  list         List repositories on a single server")
	fmt.Fprintln(os.Stderr, "  diff         Diff repositories between source and target, optionally create missing ones on target")
	fmt.Fprintln(os.Stderr, "  sync-config  Compare and optionally sync full repository configuration for repos present on both servers")
	fmt.Fprintln(os.Stderr, "  delete       Delete repositories listed in a file")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps repos <subcommand> -h' for subcommand-specific flags.")
}

func parseOrHelp(fs *flag.FlagSet, args []string) (ok bool, exitCode int) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return false, 0
		}
		return false, 1
	}
	return true, 0
}

func runList(args []string) int {
	fs := flag.NewFlagSet("repos list", flag.ContinueOnError)
	serverFlags := config.Register(fs, "")
	repoType := fs.String("type", "all", "Repository type: local, remote, virtual, federated, or all")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	list, err := repos.List(client, *repoType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	rows := make([][]string, 0, len(list))
	for _, r := range list {
		rows = append(rows, []string{r.Key, r.Type, r.URL})
	}
	output.Table(os.Stdout, []string{"KEY", "TYPE", "URL"}, rows)
	return 0
}

func runDiff(args []string) int {
	fs := flag.NewFlagSet("repos diff", flag.ContinueOnError)
	srcFlags := config.Register(fs, "source-")
	tgtFlags := config.Register(fs, "target-")
	repoType := fs.String("type", "", "Repository type to diff: local, remote, virtual, federated, or all (required)")
	dryRun := fs.Bool("dry-run", true, "Preview changes without applying them")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
	}
	if *repoType == "" {
		fmt.Fprintln(os.Stderr, "error: --type is required")
		return 1
	}

	source, err := srcFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	target, err := tgtFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	result, err := repos.Diff(source, target, *repoType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	if len(result.MissingOnTarget) == 0 {
		fmt.Println("No repositories missing on target.")
		return 0
	}

	fmt.Printf("Repositories missing on target (%d):\n", len(result.MissingOnTarget))
	rows := make([][]string, 0, len(result.MissingOnTarget))
	for _, r := range result.MissingOnTarget {
		rows = append(rows, []string{r.Key, r.Type, r.URL})
	}
	output.Table(os.Stdout, []string{"KEY", "TYPE", "URL"}, rows)

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("create %d repositories on target", len(result.MissingOnTarget)))
		return 0
	}

	createResults := repos.CopyMissing(source, target, result.MissingOnTarget, *dryRun)
	failed := 0
	for _, r := range createResults {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s on target: %v\n", r.Key, r.Err)
			failed++
			continue
		}
		fmt.Printf("created %s on target\n", r.Key)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func runSyncConfig(args []string) int {
	fs := flag.NewFlagSet("repos sync-config", flag.ContinueOnError)
	srcFlags := config.Register(fs, "source-")
	tgtFlags := config.Register(fs, "target-")
	repoType := fs.String("type", "", "Repository type to sync: local, remote, virtual, federated, or all (required)")
	dryRun := fs.Bool("dry-run", true, "Preview changes without applying them")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
	}
	if *repoType == "" {
		fmt.Fprintln(os.Stderr, "error: --type is required")
		return 1
	}

	source, err := srcFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	target, err := tgtFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	result, err := repos.SyncConfigDiff(source, target, *repoType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	fmt.Printf("Repositories present on both servers: %d\n", len(result.Common))
	if len(result.Differing) == 0 {
		fmt.Println("No configuration differences found.")
		return 0
	}

	fmt.Printf("Repositories with differing configuration (%d):\n", len(result.Differing))
	for _, key := range result.Differing {
		fmt.Println("  " + key)
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("sync configuration for %d repositories to target", len(result.Differing)))
		return 0
	}

	updateResults := repos.ApplyConfigSync(source, target, result.Differing, *dryRun)
	failed := 0
	for _, r := range updateResults {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "failed to sync %s to target: %v\n", r.Key, r.Err)
			failed++
			continue
		}
		fmt.Printf("synced %s to target\n", r.Key)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func runDelete(args []string) int {
	fs := flag.NewFlagSet("repos delete", flag.ContinueOnError)
	serverFlags := config.Register(fs, "")
	file := fs.String("file", "", "Path to a file with repo keys to delete, one per line (required)")
	dryRun := fs.Bool("dry-run", true, "Preview deletions without applying them")
	yes := fs.Bool("yes", false, "Skip interactive confirmation")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "error: --file is required")
		return 1
	}

	keys, err := repos.ReadKeysFromFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(keys) == 0 {
		fmt.Println("No repository keys found in file; nothing to do.")
		return 0
	}

	fmt.Printf("Repositories to delete (%d):\n", len(keys))
	for _, key := range keys {
		fmt.Println("  " + key)
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d repositories", len(keys)))
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d repositories?", len(keys))) {
		fmt.Println("Aborted.")
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	results := repos.DeleteRepos(client, keys, *dryRun)
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "failed to delete %s (HTTP %d): %v\n", r.Key, r.StatusCode, r.Err)
			failed++
			continue
		}
		fmt.Printf("deleted %s (HTTP %d)\n", r.Key, r.StatusCode)
	}
	if failed > 0 {
		return 1
	}
	return 0
}
