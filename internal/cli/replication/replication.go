// Package replicationcmd implements the `jfps replication` command group:
// managing Artifactory push replication configuration.
package replicationcmd

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"jfrogpscli/internal/config"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/progress"
	"jfrogpscli/internal/registry"
	"jfrogpscli/internal/replication"
)

func init() {
	registry.Register("replication", "Manage Artifactory push replication", Run)
}

// Run dispatches a `jfps replication <subcommand>` invocation.
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
	case "list":
		return runList(rest)
	case "create":
		return runCreate(rest)
	case "delete":
		return runDelete(rest)
	case "disable":
		return runDisable(rest)
	case "backup":
		return runBackup(rest)
	case "restore":
		return runRestore(rest)
	default:
		fmt.Fprintf(os.Stderr, "replication: unknown subcommand %q\n\n", sub)
		printUsage()
		return 1
	}
}

func isHelp(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: jfps replication <subcommand> [flags]

Subcommands:
  list      List push replication configuration for one or all local repos
  create    Create push replication for one or more repos
  delete    Delete push replication for one or more repos
  disable   Disable (without deleting) push replication for one or more repos
  backup    Back up push replication configs to a JSON file
  restore   Restore push replication configs from a JSON backup file

Run 'jfps replication <subcommand> -h' for subcommand-specific flags.`)
}

// parseOrExit parses fs and returns the exit code to use if parsing failed
// or -h/--help was requested, plus a bool indicating whether the caller
// should stop and return that code.
func parseOrExit(fs *flag.FlagSet, args []string) (code int, stop bool) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, true
		}
		return 1, true
	}
	return 0, false
}

func readRepoFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open repo file %s: %w", path, err)
	}
	defer f.Close()

	var repos []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		repos = append(repos, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read repo file %s: %w", path, err)
	}
	return repos, nil
}

func runList(args []string) int {
	fs := flag.NewFlagSet("replication list", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Check only this repo (default: all local repos)")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication list:", err)
		return 1
	}

	bar := progress.New()
	statuses, err := replication.List(client, *repo, func(done, total int) { bar.Update("Checking replication", done, total) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication list:", err)
		return 1
	}

	rows := make([][]string, 0, len(statuses))
	for _, st := range statuses {
		configured := "no"
		if st.Configured {
			configured = "yes"
		}
		rows = append(rows, []string{st.Repo, configured, strings.Join(st.TargetURLs, ", ")})
	}
	output.Table(os.Stdout, []string{"REPO", "CONFIGURED", "TARGET_URL(S)"}, rows)
	return 0
}

func runCreate(args []string) int {
	fs := flag.NewFlagSet("replication create", flag.ContinueOnError)
	sf := config.Register(fs, "")
	// The target connection details (--target-url/--target-user/--target-password/...)
	// are registered via the target- prefix per the shared config foundation's
	// convention for commands that reference a second JPD. --target-user and
	// --target-password double as the username/password embedded in the
	// replication payload -- bare --username/--password flags would collide
	// with the source connection's own --password flag registered above.
	tf := config.Register(fs, "target-")
	repo := fs.String("repo", "", "Repo to create replication for")
	repoFile := fs.String("repo-file", "", "File with newline-delimited repo keys (# comments and blank lines skipped)")
	targetRepo := fs.String("target-repo", "", "Target repo key (default: same as source repo)")
	cron := fs.String("cron", "0 0/6 * * * ?", "Cron expression for the replication schedule")
	syncDeletes := fs.Bool("sync-deletes", false, "Propagate deletes to the target")
	enabled := fs.Bool("enabled", true, "Enable the replication immediately")
	dryRun := fs.Bool("dry-run", true, "Print the payload that would be sent without applying it")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if *repo == "" && *repoFile == "" {
		fmt.Fprintln(os.Stderr, "replication create: one of --repo or --repo-file is required")
		return 1
	}
	if *repo != "" && *repoFile != "" {
		fmt.Fprintln(os.Stderr, "replication create: --repo and --repo-file are mutually exclusive")
		return 1
	}
	if tf.URL == "" {
		fmt.Fprintln(os.Stderr, "replication create: --target-url is required")
		return 1
	}

	var repos []string
	if *repoFile != "" {
		var err error
		repos, err = readRepoFile(*repoFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "replication create:", err)
			return 1
		}
	} else {
		repos = []string{*repo}
	}
	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "replication create: no repos to process")
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication create:", err)
		return 1
	}

	opts := replication.CreateOptions{
		TargetURL:   tf.URL,
		TargetRepo:  *targetRepo,
		Cron:        *cron,
		Username:    tf.User,
		Password:    tf.Password,
		SyncDeletes: *syncDeletes,
		Enabled:     *enabled,
	}

	bar := progress.New()
	results := replication.Create(client, repos, opts, *dryRun, func(done, total int) { bar.Update("Creating replication", done, total) })

	failed := 0
	for _, r := range results {
		if *dryRun {
			output.DryRunBanner(os.Stdout, fmt.Sprintf("create replication for %s -> %s", r.Repo, r.Payload.URL))
			_ = output.JSON(os.Stdout, r.Payload)
			continue
		}
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

func runDelete(args []string) int {
	fs := flag.NewFlagSet("replication delete", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Repo to delete replication for")
	all := fs.Bool("all", false, "Delete replication for every local repo that has it configured")
	dryRun := fs.Bool("dry-run", true, "List what would be deleted without applying")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if err := requireRepoXorAll(*repo, *all); err != nil {
		fmt.Fprintln(os.Stderr, "replication delete:", err)
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication delete:", err)
		return 1
	}

	var candidates []string
	if *all {
		candidates, err = replication.ListLocalRepos(client)
	} else {
		candidates = []string{*repo}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication delete:", err)
		return 1
	}

	targets, err := replication.ReposWithReplication(client, candidates)
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication delete:", err)
		return 1
	}
	if len(targets) == 0 {
		fmt.Println("No matching repos with replication configured.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete replication for: %s", strings.Join(targets, ", ")))
		return 0
	}

	if !*yes {
		if !output.Confirm(fmt.Sprintf("Delete replication for %d repo(s): %s?", len(targets), strings.Join(targets, ", "))) {
			fmt.Println("Aborted.")
			return 1
		}
	}

	bar := progress.New()
	results := replication.Delete(client, targets, false, func(done, total int) { bar.Update("Deleting replication", done, total) })
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

func runDisable(args []string) int {
	fs := flag.NewFlagSet("replication disable", flag.ContinueOnError)
	sf := config.Register(fs, "")
	repo := fs.String("repo", "", "Repo to disable replication for")
	all := fs.Bool("all", false, "Disable replication for every local repo that has it configured")
	dryRun := fs.Bool("dry-run", true, "List what would be disabled without applying")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if err := requireRepoXorAll(*repo, *all); err != nil {
		fmt.Fprintln(os.Stderr, "replication disable:", err)
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication disable:", err)
		return 1
	}

	var candidates []string
	if *all {
		candidates, err = replication.ListLocalRepos(client)
	} else {
		candidates = []string{*repo}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication disable:", err)
		return 1
	}

	targets, err := replication.ReposWithReplication(client, candidates)
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication disable:", err)
		return 1
	}
	if len(targets) == 0 {
		fmt.Println("No matching repos with replication configured.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("disable replication for: %s", strings.Join(targets, ", ")))
		return 0
	}

	bar := progress.New()
	results := replication.Disable(client, targets, false, func(done, total int) { bar.Update("Disabling replication", done, total) })
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

func runBackup(args []string) int {
	fs := flag.NewFlagSet("replication backup", flag.ContinueOnError)
	sf := config.Register(fs, "")
	outputFile := fs.String("output", "", "Output JSON file path (required)")
	repoFile := fs.String("repo-file", "", "File with newline-delimited repo keys (default: all local repos)")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if *outputFile == "" {
		fmt.Fprintln(os.Stderr, "replication backup: --output is required")
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication backup:", err)
		return 1
	}

	var candidates []string
	if *repoFile != "" {
		candidates, err = readRepoFile(*repoFile)
	} else {
		candidates, err = replication.ListLocalRepos(client)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication backup:", err)
		return 1
	}

	targets, err := replication.ReposWithReplication(client, candidates)
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication backup:", err)
		return 1
	}
	if len(targets) == 0 {
		fmt.Println("No repos with replication configured found.")
		return 0
	}

	bar := progress.New()
	data, err := replication.Backup(client, targets, func(done, total int) { bar.Update("Backing up replication", done, total) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication backup:", err)
		return 1
	}
	if err := replication.WriteBackupFile(*outputFile, data); err != nil {
		fmt.Fprintln(os.Stderr, "replication backup:", err)
		return 1
	}

	fmt.Printf("Backed up replication config for %d repo(s) to %s\n", len(data), *outputFile)
	return 0
}

func runRestore(args []string) int {
	fs := flag.NewFlagSet("replication restore", flag.ContinueOnError)
	sf := config.Register(fs, "")
	input := fs.String("input", "", "Input JSON backup file (required)")
	dryRun := fs.Bool("dry-run", true, "List what would be restored without applying")
	if code, stop := parseOrExit(fs, args); stop {
		return code
	}

	if *input == "" {
		fmt.Fprintln(os.Stderr, "replication restore: --input is required")
		return 1
	}

	client, err := sf.Resolve()
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication restore:", err)
		return 1
	}

	data, err := replication.LoadBackupFile(*input)
	if err != nil {
		fmt.Fprintln(os.Stderr, "replication restore:", err)
		return 1
	}
	if len(data) == 0 {
		fmt.Println("Backup file contains no repos.")
		return 0
	}

	bar := progress.New()
	results := replication.Restore(client, data, *dryRun, func(done, total int) { bar.Update("Restoring replication", done, total) })

	if *dryRun {
		names := make([]string, 0, len(results))
		for _, r := range results {
			names = append(names, r.Repo)
		}
		output.DryRunBanner(os.Stdout, fmt.Sprintf("restore replication for: %s", strings.Join(names, ", ")))
		return 0
	}

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

func requireRepoXorAll(repo string, all bool) error {
	if repo == "" && !all {
		return fmt.Errorf("one of --repo or --all is required")
	}
	if repo != "" && all {
		return fmt.Errorf("--repo and --all are mutually exclusive")
	}
	return nil
}
