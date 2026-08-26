// Package projectscmd is the CLI wrapper around internal/projects: list,
// diff, and delete subcommands for JFrog Projects.
package projectscmd

import (
	"flag"
	"fmt"
	"os"

	"jfrogpscli/internal/config"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/projects"
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("projects", "List, diff, and delete JFrog Projects", Run)
}

// Run dispatches to the projects subcommands.
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
	case "delete":
		return runDelete(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "jfps projects: unknown subcommand %q\n\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps projects <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  list    List JFrog Projects on a single server")
	fmt.Fprintln(os.Stderr, "  diff    Diff JFrog Projects between source and target, optionally create missing / delete extra")
	fmt.Fprintln(os.Stderr, "  delete  Delete JFrog Projects listed in a file")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps projects <subcommand> -h' for subcommand-specific flags.")
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
	fs := flag.NewFlagSet("projects list", flag.ContinueOnError)
	serverFlags := config.Register(fs, "")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	list, err := projects.List(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	rows := make([][]string, 0, len(list))
	for _, p := range list {
		rows = append(rows, []string{p.ProjectKey, p.DisplayName})
	}
	output.Table(os.Stdout, []string{"PROJECT_KEY", "DISPLAY_NAME"}, rows)
	return 0
}

func runDiff(args []string) int {
	fs := flag.NewFlagSet("projects diff", flag.ContinueOnError)
	srcFlags := config.Register(fs, "source-")
	tgtFlags := config.Register(fs, "target-")
	dryRun := fs.Bool("dry-run", true, "Preview changes without applying them")
	deleteExtra := fs.Bool("delete-extra", false, "Also delete projects present on target but not on source (requires --dry-run=false)")
	yes := fs.Bool("yes", false, "Skip interactive confirmation before deleting extra projects")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
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

	result, err := projects.Diff(source, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// Always print a summary, dry-run or not.
	fmt.Printf("Missing on target (%d):\n", len(result.MissingOnTarget))
	for _, p := range result.MissingOnTarget {
		fmt.Printf("  %s (%s)\n", p.ProjectKey, p.DisplayName)
	}
	fmt.Printf("Extra on target (%d):\n", len(result.ExtraOnTarget))
	for _, p := range result.ExtraOnTarget {
		fmt.Printf("  %s (%s)\n", p.ProjectKey, p.DisplayName)
	}

	failed := 0

	// Missing-on-target: created whenever --dry-run=false, unconditionally.
	if len(result.MissingOnTarget) > 0 {
		if *dryRun {
			output.DryRunBanner(os.Stdout, fmt.Sprintf("create %d projects on target", len(result.MissingOnTarget)))
		} else {
			createResults := projects.CreateMissing(source, target, result.MissingOnTarget, *dryRun)
			for _, r := range createResults {
				if r.Err != nil {
					fmt.Fprintf(os.Stderr, "failed to create %s on target: %v\n", r.Key, r.Err)
					failed++
					continue
				}
				fmt.Printf("created %s on target\n", r.Key)
			}
		}
	}

	// Extra-on-target: only acted on if --delete-extra was explicitly passed
	// AND --dry-run=false.
	if len(result.ExtraOnTarget) > 0 {
		switch {
		case !*deleteExtra:
			fmt.Println("Extra projects on target left untouched (pass --delete-extra --dry-run=false to remove them).")
		case *dryRun:
			output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d extra projects from target", len(result.ExtraOnTarget)))
		default:
			if !*yes && !output.Confirm(fmt.Sprintf("Delete %d extra projects from target?", len(result.ExtraOnTarget))) {
				fmt.Println("Aborted deletion of extra projects.")
			} else {
				deleteResults := projects.DeleteExtra(target, result.ExtraOnTarget, *dryRun)
				for _, r := range deleteResults {
					if r.Err != nil {
						fmt.Fprintf(os.Stderr, "failed to delete %s from target: %v\n", r.Key, r.Err)
						failed++
						continue
					}
					fmt.Printf("deleted %s from target\n", r.Key)
				}
			}
		}
	}

	if failed > 0 {
		return 1
	}
	return 0
}

func runDelete(args []string) int {
	fs := flag.NewFlagSet("projects delete", flag.ContinueOnError)
	serverFlags := config.Register(fs, "")
	file := fs.String("file", "", "Path to a file with project keys to delete, one per line (required)")
	yes := fs.Bool("yes", false, "Skip interactive confirmation")
	ok, code := parseOrHelp(fs, args)
	if !ok {
		return code
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "error: --file is required")
		return 1
	}

	keys, err := projects.ReadKeysFromFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(keys) == 0 {
		fmt.Println("No project keys found in file; nothing to do.")
		return 0
	}

	fmt.Printf("Projects to delete (%d):\n", len(keys))
	for _, key := range keys {
		fmt.Println("  " + key)
	}

	// projects delete has no --dry-run flag, matching the original script's
	// "confirm once, then just do it" behavior -- but confirmation (or
	// --yes) is still mandatory before any DELETE call is issued.
	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d projects?", len(keys))) {
		fmt.Println("Aborted.")
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	results := projects.DeleteProjects(client, keys)
	var failures []string
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "failed to delete %s (HTTP %d): %v\n", r.Key, r.StatusCode, r.Err)
			failures = append(failures, r.Key)
			continue
		}
		fmt.Printf("deleted %s (HTTP %d)\n", r.Key, r.StatusCode)
	}

	if len(failures) > 0 {
		fmt.Printf("Failed to delete %d project(s): %v\n", len(failures), failures)
		return 1
	}
	return 0
}
