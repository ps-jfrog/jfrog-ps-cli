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
	"strings"

	"jfrogpscli/internal/jfclient"
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

// stableCommands lists the command groups that have been fully exercised
// against a real JPD and are safe to hand to users in a tagged release.
// Everything else stays registered and fully usable in dev builds
// (version == "dev") so work on it can continue, but a tagged binary hides
// it from help output and refuses to run it until it earns a spot here.
var stableCommands = map[string]bool{
	"discover": true,
}

// stableSubcommands promotes individual subcommands of a command group that
// is otherwise dev-build-only: a tagged binary runs (and lists) only the
// subcommands named here for that group, and refuses the rest. A group in
// stableCommands is fully available and needs no entry here. A promoted
// subcommand that writes must also opt its endpoint out of the release write
// guard (see jfclient.AllowWrite); promotion alone does not enable writes.
var stableSubcommands = map[string]map[string]bool{
	"repos": {"resolve-merge-diff": true},
}

func isStable(group string, args []string) (ok bool, partial bool) {
	if stableCommands[group] {
		return true, false
	}
	subs, has := stableSubcommands[group]
	if !has {
		return false, false
	}
	return len(args) > 0 && subs[args[0]], true
}

func releasedSubcommands(group string) []string {
	var out []string
	for s := range stableSubcommands[group] {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func isDevBuild() bool {
	return version == "dev"
}

func main() {
	configureWritesGate()
	os.Exit(run(os.Args[1:]))
}

// configureWritesGate re-enables writes/deletes for dev builds. They stay
// disabled by default (see jfclient.WritesDisabled) as a safety net for
// tagged releases, which only expose stableCommands anyway; dev builds need
// them on so the untested commands they expose can actually be run and
// tested end-to-end.
func configureWritesGate() {
	if isDevBuild() {
		jfclient.WritesDisabled = false
	}
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
	if !isDevBuild() {
		ok, partial := isStable(args[0], args[1:])
		if !ok && partial {
			// Group-level help, or a subcommand that has not been promoted.
			help := len(args) == 1 || args[1] == "-h" || args[1] == "--help" || args[1] == "help"
			w := os.Stderr
			if help {
				fmt.Fprintf(w, "jfps %s: only these subcommands are available in this release:\n", args[0])
			} else {
				fmt.Fprintf(w, "jfps: \"%s %s\" is not available in this release yet (still under test). Available:\n", args[0], args[1])
			}
			for _, sub := range releasedSubcommands(args[0]) {
				fmt.Fprintf(w, "  jfps %s %s\n", args[0], sub)
			}
			fmt.Fprintln(w, "\nRun 'jfps "+args[0]+" <subcommand> -h' for flags.")
			if help {
				return 0
			}
			return 1
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "jfps: %q is not available in this release yet (still under test)\n\n", args[0])
			printUsage()
			return 1
		}
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
	if !isDevBuild() {
		visible := names[:0]
		for _, name := range names {
			if stableCommands[name] || len(stableSubcommands[name]) > 0 {
				visible = append(visible, name)
			}
		}
		names = visible
	}
	sort.Strings(names)
	for _, name := range names {
		help := registry.Help(name)
		if !isDevBuild() && !stableCommands[name] {
			help = "Released subcommands only: " + strings.Join(releasedSubcommands(name), ", ")
		}
		fmt.Fprintf(os.Stderr, "  %-14s %s\n", name, help)
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps <command>' with no subcommand, or 'jfps <command> <subcommand> -h', for details.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Connection flags (on every subcommand): --url, --token, --user, --password, --insecure, --server <name>, --server-id <jf CLI server>")
	fmt.Fprintln(os.Stderr, "Two-server commands use --source-url/--source-token/... and --target-url/--target-token/...")
	fmt.Fprintln(os.Stderr, "Environment variables: JFPS_URL, JFPS_ACCESS_TOKEN, JFPS_USER, JFPS_PASSWORD, JFPS_INSECURE (and JFPS_SOURCE_*/JFPS_TARGET_* variants)")
	fmt.Fprintln(os.Stderr, "Also honored (single-server commands only): JFROG_CLI_SERVER_ID, JF_URL/JF_ACCESS_TOKEN, JFROG_URL/JFROG_ACCESS_TOKEN")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Every mutating subcommand defaults to --dry-run=true. Pass --dry-run=false to apply changes.")
}
