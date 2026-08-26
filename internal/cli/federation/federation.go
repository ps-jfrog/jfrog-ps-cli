// Package federationcmd implements the `jfps federation` command group:
// converting repos to/from federated, checking federation member health,
// and resyncing federation config.
package federationcmd

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"jfrogpscli/internal/config"
	"jfrogpscli/internal/federation"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("federation", "Convert, check, and resync federated repositories", Run)
}

// Run dispatches a `jfps federation <subcommand>` invocation.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	if isHelp(args[0]) {
		printUsage()
		return 0
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "convert-to-federated":
		return runConvertToFederated(rest)
	case "check-members":
		return runCheckMembers(rest)
	case "resync":
		return runResync(rest)
	case "convert-to-local":
		return runConvertToLocal(rest)
	default:
		fmt.Fprintf(os.Stderr, "federation: unknown subcommand %q\n\n", sub)
		printUsage()
		return 1
	}
}

func isHelp(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: jfps federation <subcommand> [flags]

Subcommands:
  convert-to-federated  Convert local repos to federated repos (irreversible)
  check-members         Report federation member health for federated repos
  resync                Trigger a federation config resync (config only, not artifact data)
  convert-to-local      Convert a federated repo back to local, preserving its content

Run 'jfps federation <subcommand> -h' for subcommand-specific flags.`)
}

func parseOrExit(fs *flag.FlagSet, args []string) (code int, stop bool) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, true
		}
		return 1, true
	}
	return 0, false
}

func requireRepoXorAll(repo string, all bool) error {
	if repo == "" && !all {
		return fmt.Errorf("one of --repo or --all is required")
	}
	if repo != "" && all {
		return fmt.Errorf("--repo and --all are mutually exclusive")
	}
	return nil
}

func runConvertToFederated(args []string) int {
	fs := flag.NewFlagSet("federation convert-to-federated", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Local repo to convert to federated")
	all := fs.Bool("all", false, "Convert every local repo to federated")
	dryRun := fs.Bool("dry-run", true, "List what would be converted without applying")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if err := requireRepoXorAll(*repo, *all); err != nil {
		fmt.Fprintln(os.Stderr, "federation convert-to-federated:", err)
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "federation convert-to-federated:", err)
		return 1
	}

	var repos []string
	if *all {
		repos, err = federation.ListLocalRepos(client)
		if err != nil {
			fmt.Fprintln(os.Stderr, "federation convert-to-federated:", err)
			return 1
		}
	} else {
		repos = []string{*repo}
	}
	if len(repos) == 0 {
		fmt.Println("No local repos found.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf(
			"convert the following repo(s) to federated (THIS IS IRREVERSIBLE): %s",
			strings.Join(repos, ", ")))
		return 0
	}

	results := federation.ConvertToFederated(client, repos, false)
	failed := 0
	for _, r := range results {
		if r.Success {
			fmt.Printf("OK    %s\n", r.Repo)
		} else {
			failed++
			fmt.Printf("FAIL  %s: %v\n", r.Repo, r.Err)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func runCheckMembers(args []string) int {
	fs := flag.NewFlagSet("federation check-members", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Check only this federated repo (default: all federated repos)")
	expected := fs.Int("expected-members", 2, "Expected number of federation members per repo")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "federation check-members:", err)
		return 1
	}

	reports, err := federation.CheckAllMembers(client, *repo, *expected)
	if err != nil {
		fmt.Fprintln(os.Stderr, "federation check-members:", err)
		return 1
	}

	rows := make([][]string, 0, len(reports))
	unhealthy := 0
	for _, r := range reports {
		healthy := "yes"
		if !r.Healthy {
			healthy = "no"
			unhealthy++
		}
		rows = append(rows, []string{r.Repo, strconv.Itoa(r.MemberCount), healthy, strings.Join(r.Issues, "; ")})
	}
	output.Table(os.Stdout, []string{"REPO", "MEMBER_COUNT", "HEALTHY", "ISSUES"}, rows)
	if unhealthy > 0 {
		return 1
	}
	return 0
}

func runResync(args []string) int {
	fs := flag.NewFlagSet("federation resync", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Federated repo to resync")
	all := fs.Bool("all", false, "Resync every federated repo")
	dryRun := fs.Bool("dry-run", true, "List what would be resynced without applying")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if err := requireRepoXorAll(*repo, *all); err != nil {
		fmt.Fprintln(os.Stderr, "federation resync:", err)
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "federation resync:", err)
		return 1
	}

	var repos []string
	if *all {
		repos, err = federation.ListFederatedRepos(client)
		if err != nil {
			fmt.Fprintln(os.Stderr, "federation resync:", err)
			return 1
		}
	} else {
		repos = []string{*repo}
	}
	if len(repos) == 0 {
		fmt.Println("No federated repos found.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf(
			"resync federation config (config only, not artifact data) for: %s",
			strings.Join(repos, ", ")))
		return 0
	}

	results := federation.Resync(client, repos, false)
	failed := 0
	for _, r := range results {
		if r.Success {
			fmt.Printf("OK    %s\n", r.Repo)
		} else {
			failed++
			fmt.Printf("FAIL  %s: %v\n", r.Repo, r.Err)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func runConvertToLocal(args []string) int {
	fs := flag.NewFlagSet("federation convert-to-local", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Federated repo to convert to local (required)")
	tempRepo := fs.String("temp-repo", "", "Temp repo name to use during migration (default: <repo>-temp)")
	dryRun := fs.Bool("dry-run", true, "Print the migration plan without applying it (read-only)")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if *repo == "" {
		fmt.Fprintln(os.Stderr, "federation convert-to-local: --repo is required")
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "federation convert-to-local:", err)
		return 1
	}

	if *dryRun {
		plan, err := federation.PlanConvertToLocal(client, *repo, *tempRepo)
		if err != nil {
			fmt.Fprintln(os.Stderr, "federation convert-to-local:", err)
			return 1
		}
		output.DryRunBanner(os.Stdout, fmt.Sprintf(
			"convert %s to local: create temp repo %s, copy %d artifact(s) into it, delete %s and recreate it as local, then copy the artifacts back",
			plan.Repo, plan.TempRepo, len(plan.Items), plan.Repo))
		if len(plan.Items) > 0 {
			fmt.Println("Artifacts that would be migrated:")
			for _, item := range plan.Items {
				fmt.Printf("  %s\n", item)
			}
		}
		return 0
	}

	result, err := federation.ConvertToLocal(client, *repo, *tempRepo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "federation convert-to-local:", err)
		if result.Message != "" {
			fmt.Fprintln(os.Stderr, result.Message)
		}
		for _, f := range result.CopyFailures {
			fmt.Fprintf(os.Stderr, "  copy failed: %s: %v\n", f.Item, f.Err)
		}
		return 1
	}

	fmt.Println(result.Message)
	return 0
}
