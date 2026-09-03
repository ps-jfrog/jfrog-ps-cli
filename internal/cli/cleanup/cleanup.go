// Package cleanupcmd wires the internal/cleanup business logic into the
// "jfps cleanup" command group: bulk-deleting artifacts and repositories,
// pruning inactive users, diffing/pruning artifacts between two servers,
// cleaning up stale Docker/OCI tags, and cleaning up remote-repository cache
// artifacts that are both download-stale and no longer Xray-indexed. Every
// mutating subcommand defaults to --dry-run=true.
package cleanupcmd

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"jfrogpscli/internal/cleanup"
	"jfrogpscli/internal/config"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/progress"
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("cleanup", "Bulk-delete repos/artifacts and prune inactive users (dry-run by default)", Run)
}

// Run dispatches to the cleanup subcommands.
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
	case "artifacts":
		return runArtifacts(rest)
	case "repos":
		return runRepos(rest)
	case "inactive-users":
		return runInactiveUsers(rest)
	case "diff-delete-artifacts":
		return runDiffDeleteArtifacts(rest)
	case "docker-tags":
		return runDockerTags(rest)
	case "remote-cache":
		return runRemoteCache(rest)
	case "empty-folders":
		return runEmptyFolders(rest)
	default:
		fmt.Fprintf(os.Stderr, "cleanup: unknown subcommand %q\n\n", sub)
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps cleanup <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  artifacts               Bulk-delete artifacts matched by an AQL query (or --repo/--not-downloaded-since)")
	fmt.Fprintln(os.Stderr, "  repos                   Bulk-delete repositories listed in a file")
	fmt.Fprintln(os.Stderr, "  inactive-users          Find/delete users inactive for N+ months")
	fmt.Fprintln(os.Stderr, "  diff-delete-artifacts   Delete target-only artifacts to match source")
	fmt.Fprintln(os.Stderr, "  docker-tags             Delete stale Docker/OCI tags (full tag dir, not just manifest.json)")
	fmt.Fprintln(os.Stderr, "  remote-cache            Delete remote-cache artifacts that are download-stale AND not Xray-indexed")
	fmt.Fprintln(os.Stderr, "  empty-folders           Delete recursively-empty folders in a repo (never the repo root itself)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'jfps cleanup <subcommand> -h' for subcommand-specific flags.")
	fmt.Fprintln(os.Stderr, "Every mutating subcommand defaults to --dry-run=true.")
}

// ---------------------------------------------------------------------------
// artifacts
// ---------------------------------------------------------------------------

func runArtifacts(args []string) int {
	fs := flag.NewFlagSet("cleanup artifacts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	aqlFile := fs.String("aql-file", "", "Path to a file containing the AQL query to run")
	aqlLiteral := fs.String("aql", "", "AQL query to run, as a literal string")
	repo := fs.String("repo", "", "Repository to search (used with --not-downloaded-since instead of --aql/--aql-file)")
	notDownloadedSince := fs.String("not-downloaded-since", "", "Build the query for you: match artifacts in --repo never downloaded, or not downloaded since this AQL duration (e.g. 3mo, 6mo, 1y, 90d)")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show what would be deleted")
	csvPath := fs.String("csv", "", "Optional path to also write matches as CSV")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	query, err := resolveArtifactsQuery(*aqlFile, *aqlLiteral, *repo, *notDownloadedSince)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup artifacts: %v\n", err)
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup artifacts: %v\n", err)
		return 1
	}

	matches, err := cleanup.FindArtifacts(client, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup artifacts: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Found %d matching artifact(s), totaling %s.\n", len(matches), cleanup.FormatBytes(cleanup.TotalSize(matches)))
	printArtifactTable(matches)

	if *csvPath != "" {
		if err := cleanup.WriteArtifactsCSV(*csvPath, matches); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup artifacts: write csv: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "Wrote %d row(s) to %s\n", len(matches), *csvPath)
	}

	if len(matches) == 0 {
		return 0
	}

	bar := progress.New()
	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d artifact(s)", len(matches)))
		cleanup.DeleteArtifacts(client, matches, true, func(done, total int) { bar.Update("Deleting artifacts", done, total) })
		return 0
	}

	results := cleanup.DeleteArtifacts(client, matches, false, func(done, total int) { bar.Update("Deleting artifacts", done, total) })
	return reportArtifactDeleteResults(results)
}

// resolveArtifactsQuery picks the AQL query to run: either the query given
// directly (--aql/--aql-file), or one built for the caller from
// --repo/--not-downloaded-since, so common cleanup searches -- "find
// artifacts never downloaded, or not downloaded in N months" -- don't
// require hand-writing AQL.
func resolveArtifactsQuery(aqlFile, aqlLiteral, repo, notDownloadedSince string) (string, error) {
	explicitCount := 0
	if aqlFile != "" {
		explicitCount++
	}
	if aqlLiteral != "" {
		explicitCount++
	}
	builderRequested := repo != "" || notDownloadedSince != ""

	switch {
	case explicitCount > 1:
		return "", fmt.Errorf("only one of --aql-file or --aql may be given")
	case explicitCount == 1 && builderRequested:
		return "", fmt.Errorf("--repo/--not-downloaded-since cannot be combined with --aql or --aql-file")
	case explicitCount == 1:
		if aqlFile != "" {
			data, err := os.ReadFile(aqlFile)
			if err != nil {
				return "", fmt.Errorf("read --aql-file: %w", err)
			}
			return string(data), nil
		}
		return aqlLiteral, nil
	case builderRequested:
		if repo == "" || notDownloadedSince == "" {
			return "", fmt.Errorf("--repo and --not-downloaded-since must be given together")
		}
		return cleanup.BuildDownloadStaleAQL(cleanup.DownloadStaleQuery{Repo: repo, OlderThan: notDownloadedSince}), nil
	default:
		return "", fmt.Errorf("exactly one of --aql-file, --aql, or --repo/--not-downloaded-since is required")
	}
}

func printArtifactTable(matches []cleanup.ArtifactMatch) {
	rows := make([][]string, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, []string{m.Repo, m.Path, m.Name, strconv.FormatInt(m.Size, 10)})
	}
	output.Table(os.Stdout, []string{"REPO", "PATH", "NAME", "SIZE"}, rows)
}

func reportArtifactDeleteResults(results []cleanup.DeleteResult) int {
	rows := make([][]string, 0, len(results))
	failures := 0
	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAILED: " + r.Error
			failures++
		}
		rows = append(rows, []string{r.Path, status})
	}
	output.Table(os.Stdout, []string{"PATH", "RESULT"}, rows)
	fmt.Fprintf(os.Stdout, "Deleted %d/%d artifact(s).\n", len(results)-failures, len(results))
	if failures > 0 {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// repos
// ---------------------------------------------------------------------------

func runRepos(args []string) int {
	fs := flag.NewFlagSet("cleanup repos", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	file := fs.String("file", "", "Path to a file with newline-delimited repo keys to delete")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show what would be deleted")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *file == "" {
		fmt.Fprintln(os.Stderr, "cleanup repos: --file is required")
		return 1
	}

	repos, err := cleanup.ReadRepoKeys(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup repos: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Loaded %d repo(s) from %s.\n", len(repos), *file)
	if len(repos) == 0 {
		return 0
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup repos: %v\n", err)
		return 1
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d repo(s)", len(repos)))
		rows := make([][]string, 0, len(repos))
		for _, r := range repos {
			rows = append(rows, []string{r})
		}
		output.Table(os.Stdout, []string{"REPO"}, rows)
		bar := progress.New()
		cleanup.DeleteRepos(client, repos, true, func(done, total int) { bar.Update("Deleting repos", done, total) })
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d repo(s) from %s?", len(repos), describeServer(serverFlags))) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := cleanup.DeleteRepos(client, repos, false, func(done, total int) { bar.Update("Deleting repos", done, total) })
	rows := make([][]string, 0, len(results))
	failures := 0
	for _, r := range results {
		status := fmt.Sprintf("%d OK", r.StatusCode)
		if !r.Success {
			status = fmt.Sprintf("FAILED (%d): %s", r.StatusCode, r.Error)
			failures++
		}
		rows = append(rows, []string{r.Repo, status})
	}
	output.Table(os.Stdout, []string{"REPO", "RESULT"}, rows)
	fmt.Fprintf(os.Stdout, "Deleted %d/%d repo(s).\n", len(results)-failures, len(results))
	if failures > 0 {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// empty-folders
// ---------------------------------------------------------------------------

func runEmptyFolders(args []string) int {
	fs := flag.NewFlagSet("cleanup empty-folders", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	repo := fs.String("repo", "", "Repository to scan for recursively-empty folders (required)")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show which folders would be deleted")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *repo == "" {
		fmt.Fprintln(os.Stderr, "cleanup empty-folders: --repo is required")
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup empty-folders: %v\n", err)
		return 1
	}

	folders, err := cleanup.FindEmptyFolders(client, *repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup empty-folders: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Found %d recursively-empty folder(s) in %s.\n", len(folders), *repo)
	if len(folders) == 0 {
		return 0
	}
	rows := make([][]string, 0, len(folders))
	for _, f := range folders {
		rows = append(rows, []string{f})
	}
	output.Table(os.Stdout, []string{"FOLDER"}, rows)

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d empty folder(s) from %s", len(folders), *repo))
		bar := progress.New()
		cleanup.DeleteEmptyFolders(client, *repo, folders, true, func(done, total int) { bar.Update("Deleting folders", done, total) })
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d empty folder(s) from %q?", len(folders), *repo)) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := cleanup.DeleteEmptyFolders(client, *repo, folders, false, func(done, total int) { bar.Update("Deleting folders", done, total) })
	rows = make([][]string, 0, len(results))
	failures := 0
	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAILED: " + r.Error
			failures++
		}
		rows = append(rows, []string{r.Path, status})
	}
	output.Table(os.Stdout, []string{"FOLDER", "RESULT"}, rows)
	fmt.Fprintf(os.Stdout, "Deleted %d/%d folder(s).\n", len(results)-failures, len(results))
	if failures > 0 {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// inactive-users
// ---------------------------------------------------------------------------

func runInactiveUsers(args []string) int {
	fs := flag.NewFlagSet("cleanup inactive-users", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	months := fs.Int("months", 6, "Users inactive for at least this many months are candidates")
	excludeRealm := fs.String("exclude-realm", "internal", "Skip users in this realm")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show which users would be deleted")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *months <= 0 {
		fmt.Fprintln(os.Stderr, "cleanup inactive-users: --months must be positive")
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup inactive-users: %v\n", err)
		return 1
	}

	users, err := cleanup.FetchUsers(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup inactive-users: %v\n", err)
		return 1
	}

	candidates := cleanup.InactiveCandidates(users, *months, *excludeRealm, time.Now())
	fmt.Fprintf(os.Stdout, "Found %d inactive user candidate(s) out of %d total user(s).\n", len(candidates), len(users))

	rows := make([][]string, 0, len(candidates))
	for _, u := range candidates {
		rows = append(rows, []string{u.Name, u.Realm, formatLastLoggedIn(u.LastLoggedIn)})
	}
	output.Table(os.Stdout, []string{"NAME", "REALM", "LAST_LOGGED_IN"}, rows)

	if len(candidates) == 0 {
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d user(s)", len(candidates)))
		bar := progress.New()
		cleanup.DeleteUsers(client, candidates, true, func(done, total int) { bar.Update("Deleting users", done, total) })
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d inactive user(s)?", len(candidates))) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := cleanup.DeleteUsers(client, candidates, false, func(done, total int) { bar.Update("Deleting users", done, total) })
	rows = make([][]string, 0, len(results))
	failures := 0
	for _, r := range results {
		status := "OK"
		if !r.Success {
			status = "FAILED: " + r.Error
			failures++
		}
		rows = append(rows, []string{r.Name, status})
	}
	output.Table(os.Stdout, []string{"NAME", "RESULT"}, rows)
	fmt.Fprintf(os.Stdout, "Deleted %d/%d user(s).\n", len(results)-failures, len(results))
	if failures > 0 {
		return 1
	}
	return 0
}

func formatLastLoggedIn(millis int64) string {
	if millis == 0 {
		return "never"
	}
	return time.UnixMilli(millis).UTC().Format(time.RFC3339)
}

// ---------------------------------------------------------------------------
// diff-delete-artifacts
// ---------------------------------------------------------------------------

func runDiffDeleteArtifacts(args []string) int {
	fs := flag.NewFlagSet("cleanup diff-delete-artifacts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sourceFlags := config.Register(fs, "source-")
	targetFlags := config.Register(fs, "target-")
	repo := fs.String("repo", "", "Repository key to compare between source and target")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show what would be deleted from target")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *repo == "" {
		fmt.Fprintln(os.Stderr, "cleanup diff-delete-artifacts: --repo is required")
		return 1
	}

	source, err := sourceFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup diff-delete-artifacts: %v\n", err)
		return 1
	}
	target, err := targetFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup diff-delete-artifacts: %v\n", err)
		return 1
	}

	candidates, err := cleanup.TargetOnlyArtifacts(source, target, *repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup diff-delete-artifacts: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Found %d artifact(s) in target repo %q not present in source.\n", len(candidates), *repo)
	printArtifactTable(candidates)

	if len(candidates) == 0 {
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d artifact(s) from target", len(candidates)))
		bar := progress.New()
		cleanup.DeleteArtifacts(target, candidates, true, func(done, total int) { bar.Update("Deleting artifacts", done, total) })
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d target-only artifact(s) from repo %q?", len(candidates), *repo)) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := cleanup.DeleteArtifacts(target, candidates, false, func(done, total int) { bar.Update("Deleting artifacts", done, total) })
	return reportArtifactDeleteResults(results)
}

// ---------------------------------------------------------------------------
// docker-tags
// ---------------------------------------------------------------------------

func runDockerTags(args []string) int {
	fs := flag.NewFlagSet("cleanup docker-tags", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	repo := fs.String("repo", "", "Docker/OCI repository to search (required)")
	olderThan := fs.String("older-than", "", "AQL duration (e.g. 3mo, 6mo, 1y): tags never downloaded, or not downloaded since this long ago, are candidates (required)")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show which tags would be deleted")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	csvPath := fs.String("csv", "", "Optional path to also write matches as CSV")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *repo == "" || *olderThan == "" {
		fmt.Fprintln(os.Stderr, "cleanup docker-tags: --repo and --older-than are required")
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup docker-tags: %v\n", err)
		return 1
	}

	matches, err := cleanup.FindStaleDockerTags(client, *repo, *olderThan)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup docker-tags: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Found %d stale Docker tag(s), totaling %s (manifest size only).\n", len(matches), cleanup.FormatBytes(cleanup.TotalSize(matches)))
	rows := make([][]string, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, []string{cleanup.DockerTagPath(m), m.Name})
	}
	output.Table(os.Stdout, []string{"TAG", "MANIFEST"}, rows)

	if *csvPath != "" {
		if err := cleanup.WriteArtifactsCSV(*csvPath, matches); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup docker-tags: write csv: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "Wrote %d row(s) to %s\n", len(matches), *csvPath)
	}

	if len(matches) == 0 {
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d Docker tag(s) (full tag directory, not just manifest.json)", len(matches)))
		bar := progress.New()
		cleanup.DeleteDockerTags(client, matches, true, func(done, total int) { bar.Update("Deleting tags", done, total) })
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d Docker tag(s) from %q?", len(matches), *repo)) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := cleanup.DeleteDockerTags(client, matches, false, func(done, total int) { bar.Update("Deleting tags", done, total) })
	return reportArtifactDeleteResults(results)
}

// ---------------------------------------------------------------------------
// remote-cache
// ---------------------------------------------------------------------------

func runRemoteCache(args []string) int {
	fs := flag.NewFlagSet("cleanup remote-cache", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	serverFlags := config.Register(fs, "")
	repo := fs.String("repo", "", "Remote repository key, e.g. 'npm-remote' (its '-cache' repo is searched; required)")
	olderThan := fs.String("older-than", "6mo", "AQL duration: cached artifacts never downloaded, or not downloaded since this long ago, are candidates")
	packageType := fs.String("package-type", "auto", "auto|docker|other -- auto detects from the remote repo's configured package type")
	batchSize := fs.Int("batch-size", 50, "Number of paths to check per Xray Artifact Summary call")
	dryRun := fs.Bool("dry-run", true, "If true (default), only show which artifacts would be deleted")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	csvPath := fs.String("csv", "", "Optional path to also write candidates as CSV")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *repo == "" {
		fmt.Fprintln(os.Stderr, "cleanup remote-cache: --repo is required")
		return 1
	}

	client, err := serverFlags.Resolve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup remote-cache: %v\n", err)
		return 1
	}

	pkg := *packageType
	if pkg == "" || pkg == "auto" {
		rawPkg, err := cleanup.RepoPackageType(client, *repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cleanup remote-cache: detect package type: %v\n", err)
			return 1
		}
		pkg = cleanup.NormalizePackageType("auto", rawPkg)
	} else {
		pkg = cleanup.NormalizePackageType(pkg, "")
	}

	cacheRepo := cleanup.ResolveCacheRepo(*repo)
	fmt.Fprintf(os.Stdout, "Searching %s (packageType=%s) for artifacts not downloaded since %s...\n", cacheRepo, pkg, *olderThan)

	staleCandidates, err := cleanup.FindRemoteCacheCandidates(client, cacheRepo, *olderThan, pkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup remote-cache: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Found %d download-stale candidate(s); checking Xray index status...\n", len(staleCandidates))

	classifyBar := progress.New()
	expired, err := cleanup.ClassifyByXrayIndex(client, staleCandidates, *batchSize, func(done, total int) { classifyBar.Update("Checking Xray index", done, total) })
	if err != nil {
		fmt.Fprintf(os.Stderr, "cleanup remote-cache: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "%d of %d download-stale candidate(s) are also not Xray-indexed (retention expired or never indexed), totaling %s.\n",
		len(expired), len(staleCandidates), cleanup.FormatBytes(cleanup.TotalSize(expired)))
	rows := make([][]string, 0, len(expired))
	for _, m := range expired {
		rows = append(rows, []string{cleanup.RemoteCacheDeleteTarget(m, pkg), strconv.FormatInt(m.Size, 10)})
	}
	output.Table(os.Stdout, []string{"DELETE_TARGET", "SIZE"}, rows)

	if *csvPath != "" {
		if err := cleanup.WriteArtifactsCSV(*csvPath, expired); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup remote-cache: write csv: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "Wrote %d row(s) to %s\n", len(expired), *csvPath)
	}

	if len(expired) == 0 {
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d remote-cache artifact(s) from %s", len(expired), cacheRepo))
		bar := progress.New()
		cleanup.DeleteRemoteCacheCandidates(client, expired, pkg, true, func(done, total int) { bar.Update("Deleting artifacts", done, total) })
		return 0
	}

	if !*yes && !output.Confirm(fmt.Sprintf("Delete %d artifact(s) from %q?", len(expired), cacheRepo)) {
		fmt.Fprintln(os.Stdout, "Aborted.")
		return 1
	}

	bar := progress.New()
	results := cleanup.DeleteRemoteCacheCandidates(client, expired, pkg, false, func(done, total int) { bar.Update("Deleting artifacts", done, total) })
	return reportArtifactDeleteResults(results)
}

// describeServer returns a short human-readable label for a resolved server,
// for use in confirmation prompts.
func describeServer(f *config.ServerFlags) string {
	switch {
	case f.URL != "":
		return f.URL
	case f.Server != "":
		return f.Server
	default:
		return "the configured server"
	}
}
