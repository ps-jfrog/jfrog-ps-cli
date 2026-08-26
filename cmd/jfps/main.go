// Command jfps is a consolidated CLI for JFrog Professional Services
// automation: access/permission management, cleanup, repo & project sync,
// replication & federation, compare/diff, repo/cleanup-policy discovery and
// HTML audit reporting, and Xray operations, generalized from the
// shell/Python scripts in the ps_jfrog_scripts repository.
//
// Each feature area lives in its own internal/cli/<group> package and
// self-registers with internal/registry via a blank import below, so
// features can be developed and tested in complete isolation from one
// another before being wired together here.
package main

import (
	"fmt"
	"os"
	"sort"

	"jfrogpscli/internal/registry"

	_ "jfrogpscli/internal/cli/access"
	_ "jfrogpscli/internal/cli/cleanup"
	_ "jfrogpscli/internal/cli/compare"
	_ "jfrogpscli/internal/cli/discover"
	_ "jfrogpscli/internal/cli/federation"
	_ "jfrogpscli/internal/cli/projects"
	_ "jfrogpscli/internal/cli/replication"
	_ "jfrogpscli/internal/cli/repos"
	_ "jfrogpscli/internal/cli/xray"
)

// version is set at build time via -ldflags "-X main.version=...", which
// goreleaser does automatically from the git tag (see .goreleaser.yaml). A
// binary built with plain "go build" (no ldflags) reports "dev".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage()
		return 0
	}
	if args[0] == "-v" || args[0] == "--version" || args[0] == "version" {
		fmt.Println("jfps version " + version)
		return 0
	}

	fn, ok := registry.Get(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "jfps: unknown command %q\n\n", args[0])
		printUsage()
		return 1
	}
	return fn(args[1:])
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "jfps - JFrog Professional Services CLI")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: jfps <command> <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	names := registry.Names()
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %-14s %s\n", name, registry.Help(name))
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps <command>' with no subcommand, or 'jfps <command> <subcommand> -h', for details.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Connection flags (on every subcommand): --url, --token, --user, --password, --insecure, --server <name>")
	fmt.Fprintln(os.Stderr, "Two-server commands use --source-url/--source-token/... and --target-url/--target-token/...")
	fmt.Fprintln(os.Stderr, "Environment variables: JFPS_URL, JFPS_ACCESS_TOKEN, JFPS_USER, JFPS_PASSWORD, JFPS_INSECURE (and JFPS_SOURCE_*/JFPS_TARGET_* variants)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Every mutating subcommand defaults to --dry-run=true. Pass --dry-run=false to apply changes.")
}
