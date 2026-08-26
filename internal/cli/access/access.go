// Package accesscmd is the thin CLI wrapper around internal/access: it
// parses flags, resolves JFrog server connections, calls into the business
// logic, and prints results. It intentionally contains no HTTP calls or
// business rules of its own.
package accesscmd

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"jfrogpscli/internal/access"
	"jfrogpscli/internal/config"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/output"
	"jfrogpscli/internal/registry"
)

func init() {
	registry.Register("access", "Manage users, groups, and permission targets", Run)
}

type subcommand struct {
	name string
	help string
	fn   func(args []string) int
}

var subcommands = []subcommand{
	{"list-permissions", "List all permission targets", runListPermissions},
	{"show-permission", "Show one permission target's detail", runShowPermission},
	{"find-empty-permissions", "Find permission targets with no repositories", runFindEmptyPermissions},
	{"list-users-in-permission", "List users assigned to a permission target", runListUsersInPermission},
	{"find-empty-groups", "Find groups with no members", runFindEmptyGroups},
	{"token-expiry", "Find access tokens expiring soon", runTokenExpiry},
	{"create-permission", "Create a new permission target (Access API v2)", runCreatePermission},
	{"grant-path", "Create or update a permission target's repo/path grants", runGrantPath},
	{"add-repos", "Add repositories to a permission target", runAddRepos},
	{"remove-repos", "Remove repositories from a permission target", runRemoveRepos},
	{"backup", "Back up users, groups, and permissions to JSON files", runBackup},
	{"delete-users", "Delete users listed in a file", runDeleteUsers},
	{"delete-groups", "Delete groups listed in a file", runDeleteGroups},
	{"delete-permissions", "Delete permission targets listed in a file", runDeletePermissions},
	{"audit", "Audit permission targets against a repo pattern/type", runAudit},
}

// Run dispatches to the requested "jfps access <subcommand>" handler.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	switch args[0] {
	case "-h", "--help", "help":
		printUsage()
		return 0
	}
	for _, sc := range subcommands {
		if sc.name == args[0] {
			return sc.fn(args[1:])
		}
	}
	fmt.Fprintf(os.Stderr, "access: unknown subcommand %q\n\n", args[0])
	printUsage()
	return 1
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: jfps access <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "\nSubcommands:")
	for _, sc := range subcommands {
		fmt.Fprintf(os.Stderr, "  %-24s %s\n", sc.name, sc.help)
	}
}

// ---- read-only subcommands -------------------------------------------------

func runListPermissions(args []string) int {
	fs := flag.NewFlagSet("access list-permissions", flag.ContinueOnError)
	sf := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access list-permissions", err)
	}
	perms, err := access.ListPermissions(client)
	if err != nil {
		return fail("access list-permissions", err)
	}
	rows := make([][]string, 0, len(perms))
	for _, p := range perms {
		rows = append(rows, []string{p.Name, p.URI})
	}
	output.Table(os.Stdout, []string{"NAME", "URI"}, rows)
	return 0
}

func runShowPermission(args []string) int {
	fs := flag.NewFlagSet("access show-permission", flag.ContinueOnError)
	sf := config.Register(fs, "")
	name := fs.String("name", "", "Permission target name (required)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *name == "" {
		return failf("access show-permission", "--name is required")
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access show-permission", err)
	}
	perm, err := access.ShowPermission(client, *name)
	if err != nil {
		return fail("access show-permission", err)
	}
	if err := output.JSON(os.Stdout, perm); err != nil {
		return fail("access show-permission", err)
	}
	return 0
}

func runFindEmptyPermissions(args []string) int {
	fs := flag.NewFlagSet("access find-empty-permissions", flag.ContinueOnError)
	sf := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access find-empty-permissions", err)
	}
	names, err := access.FindEmptyPermissions(client)
	if err != nil {
		return fail("access find-empty-permissions", err)
	}
	if len(names) == 0 {
		fmt.Println("No empty permission targets found.")
		return 0
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return 0
}

func runListUsersInPermission(args []string) int {
	fs := flag.NewFlagSet("access list-users-in-permission", flag.ContinueOnError)
	sf := config.Register(fs, "")
	name := fs.String("name", "", "Permission target name (required)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *name == "" {
		return failf("access list-users-in-permission", "--name is required")
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access list-users-in-permission", err)
	}
	users, err := access.ListUsersInPermission(client, *name)
	if err != nil {
		return fail("access list-users-in-permission", err)
	}
	if len(users) == 0 {
		fmt.Println("No users assigned.")
		return 0
	}
	for _, u := range users {
		fmt.Println(u)
	}
	return 0
}

func runFindEmptyGroups(args []string) int {
	fs := flag.NewFlagSet("access find-empty-groups", flag.ContinueOnError)
	sf := config.Register(fs, "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access find-empty-groups", err)
	}
	names, err := access.FindEmptyGroups(client)
	if err != nil {
		return fail("access find-empty-groups", err)
	}
	if len(names) == 0 {
		fmt.Println("No empty groups found.")
		return 0
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return 0
}

func runTokenExpiry(args []string) int {
	fs := flag.NewFlagSet("access token-expiry", flag.ContinueOnError)
	sf := config.Register(fs, "")
	days := fs.Int("days", 30, "Report tokens expiring within this many days")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access token-expiry", err)
	}
	tokens, err := access.TokenExpiry(client, *days, time.Now())
	if err != nil {
		return fail("access token-expiry", err)
	}
	rows := make([][]string, 0, len(tokens))
	for _, t := range tokens {
		rows = append(rows, []string{t.TokenID, t.Subject, t.ExpiresAt.Format(time.RFC3339)})
	}
	output.Table(os.Stdout, []string{"TOKEN_ID", "SUBJECT", "EXPIRES_AT"}, rows)
	return 0
}

func runAudit(args []string) int {
	fs := flag.NewFlagSet("access audit", flag.ContinueOnError)
	sf := config.Register(fs, "")
	pattern := fs.String("repo-pattern", "", "Regex to match repo keys (required)")
	repoType := fs.String("repo-type", "", "Repo type filter: local|remote|virtual|federated")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *pattern == "" {
		return failf("access audit", "--repo-pattern is required")
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access audit", err)
	}
	matches, err := access.AuditPermissions(client, *pattern, *repoType)
	if err != nil {
		return fail("access audit", err)
	}
	rows := make([][]string, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, []string{m.Permission, m.Matched})
	}
	output.Table(os.Stdout, []string{"PERMISSION", "MATCHED_REPO/WILDCARD"}, rows)
	return 0
}

// ---- mutating subcommands ---------------------------------------------------

func runCreatePermission(args []string) int {
	fs := flag.NewFlagSet("access create-permission", flag.ContinueOnError)
	sf := config.Register(fs, "")
	name := fs.String("name", "", "Permission target name (required)")
	repos := fs.String("repos", "", "Comma-separated repo keys (required)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *name == "" || *repos == "" {
		return failf("access create-permission", "--name and --repos are required")
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access create-permission", err)
	}
	repoList := splitCSV(*repos)
	if err := access.CreatePermission(client, *name, repoList); err != nil {
		return fail("access create-permission", err)
	}
	fmt.Printf("Created permission %q with repos: %s\n", *name, strings.Join(repoList, ", "))
	return 0
}

func runGrantPath(args []string) int {
	fs := flag.NewFlagSet("access grant-path", flag.ContinueOnError)
	sf := config.Register(fs, "")
	permission := fs.String("permission", "", "Permission target name (required)")
	repos := fs.String("repos", "", "Comma-separated repo keys (required)")
	path := fs.String("path", "", "Semicolon-separated include patterns (required)")
	groupsFlag := fs.String("groups", "", `Comma-separated "group:ACT+ACT" entries`)
	usersFlag := fs.String("users", "", `Comma-separated "user:ACT+ACT" entries`)
	resource := fs.String("resource", "artifact", "Resource type")
	dryRun := fs.Bool("dry-run", true, "Preview changes without applying them")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *permission == "" || *repos == "" || *path == "" {
		return failf("access grant-path", "--permission, --repos, and --path are required")
	}
	groups, err := parseActionsFlag(*groupsFlag)
	if err != nil {
		return fail("access grant-path", err)
	}
	users, err := parseActionsFlag(*usersFlag)
	if err != nil {
		return fail("access grant-path", err)
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access grant-path", err)
	}
	opts := access.GrantPathOptions{
		Permission: *permission,
		Repos:      splitCSV(*repos),
		Paths:      splitSemi(*path),
		Groups:     groups,
		Users:      users,
		Resource:   *resource,
	}
	result, err := access.GrantPath(client, opts, *dryRun)
	if err != nil {
		return fail("access grant-path", err)
	}
	verb := "update"
	if result.WillCreate {
		verb = "create"
	}
	action := fmt.Sprintf("%s permission %q with targets: %s", verb, *permission, strings.Join(result.Targets, ", "))
	if *dryRun {
		output.DryRunBanner(os.Stdout, action)
		return 0
	}
	fmt.Printf("Permission %q %sd. Targets: %s\n", *permission, verb, strings.Join(result.Targets, ", "))
	return 0
}

func runAddRepos(args []string) int {
	return runReposChange(args, "add-repos", access.AddRepos)
}

func runRemoveRepos(args []string) int {
	return runReposChange(args, "remove-repos", access.RemoveRepos)
}

func runReposChange(args []string, cmdName string, fn func(*jfclient.Client, string, []string, bool) (*access.ReposChangeResult, error)) int {
	fs := flag.NewFlagSet("access "+cmdName, flag.ContinueOnError)
	sf := config.Register(fs, "")
	permission := fs.String("permission", "", "Permission target name (required)")
	repos := fs.String("repos", "", "Comma-separated repo keys (required)")
	dryRun := fs.Bool("dry-run", true, "Preview changes without applying them")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *permission == "" || *repos == "" {
		return failf("access "+cmdName, "--permission and --repos are required")
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access "+cmdName, err)
	}
	result, err := fn(client, *permission, splitCSV(*repos), *dryRun)
	if err != nil {
		return fail("access "+cmdName, err)
	}
	action := fmt.Sprintf("%s on permission %q (resulting targets: %s)", cmdName, *permission, strings.Join(result.After, ", "))
	if *dryRun {
		output.DryRunBanner(os.Stdout, action)
		return 0
	}
	fmt.Printf("Permission %q updated. Targets: %s\n", *permission, strings.Join(result.After, ", "))
	return 0
}

func runBackup(args []string) int {
	fs := flag.NewFlagSet("access backup", flag.ContinueOnError)
	sf := config.Register(fs, "")
	outDir := fs.String("output", "", "Output directory (required)")
	usersOnly := fs.Bool("users-only", false, "Only back up users")
	groupsOnly := fs.Bool("groups-only", false, "Only back up groups")
	permissionsOnly := fs.Bool("permissions-only", false, "Only back up permissions")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *outDir == "" {
		return failf("access backup", "--output is required")
	}
	client, err := sf.Resolve()
	if err != nil {
		return fail("access backup", err)
	}
	result, err := access.Backup(client, access.BackupOptions{
		OutputDir:       *outDir,
		UsersOnly:       *usersOnly,
		GroupsOnly:      *groupsOnly,
		PermissionsOnly: *permissionsOnly,
	})
	if err != nil {
		return fail("access backup", err)
	}
	if result.UsersFile != "" {
		fmt.Println("Wrote", result.UsersFile)
	}
	if result.GroupsFile != "" {
		fmt.Println("Wrote", result.GroupsFile)
	}
	if result.PermissionsFile != "" {
		fmt.Println("Wrote", result.PermissionsFile)
	}
	return 0
}

func runDeleteUsers(args []string) int {
	return runDelete(args, "delete-users", access.DeleteUsers, "user")
}

func runDeleteGroups(args []string) int {
	return runDelete(args, "delete-groups", access.DeleteGroups, "group")
}

func runDeletePermissions(args []string) int {
	return runDelete(args, "delete-permissions", access.DeletePermissions, "permission")
}

func runDelete(args []string, cmdName string, kind access.DeleteKind, label string) int {
	fs := flag.NewFlagSet("access "+cmdName, flag.ContinueOnError)
	sf := config.Register(fs, "")
	file := fs.String("file", "", "Path to newline-delimited names file (required)")
	excludeFlag := fs.String("exclude", "", "Comma-separated additional names to protect from deletion")
	dryRun := fs.Bool("dry-run", true, "Preview deletions without applying them")
	yes := fs.Bool("yes", false, "Skip the interactive confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *file == "" {
		return failf("access "+cmdName, "--file is required")
	}

	names, err := access.ParseNamesFile(*file)
	if err != nil {
		return fail("access "+cmdName, err)
	}
	targets := access.FilterDeletable(kind, names, splitCSV(*excludeFlag))
	if len(targets) == 0 {
		fmt.Println("Nothing to delete after applying protections/exclusions.")
		return 0
	}

	if *dryRun {
		output.DryRunBanner(os.Stdout, fmt.Sprintf("delete %d %s(s): %s", len(targets), label, strings.Join(targets, ", ")))
		return 0
	}

	if !*yes {
		prompt := fmt.Sprintf("Delete %d %s(s)? This is irreversible.", len(targets), label)
		if !output.Confirm(prompt) {
			fmt.Println("Aborted.")
			return 1
		}
	}

	client, err := sf.Resolve()
	if err != nil {
		return fail("access "+cmdName, err)
	}
	results := access.DeleteItems(client, kind, targets, false)
	failures := 0
	for _, r := range results {
		if r.Success {
			fmt.Printf("Deleted %s %s\n", label, r.Name)
		} else {
			failures++
			fmt.Fprintf(os.Stderr, "Failed to delete %s %s: %s\n", label, r.Name, r.Error)
		}
	}
	if failures > 0 {
		return 1
	}
	return 0
}

// ---- flag parsing helpers ---------------------------------------------------

// splitCSV splits a comma-separated flag value into a trimmed, non-empty
// slice. An empty/blank input yields nil.
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitSemi splits a semicolon-separated flag value the same way splitCSV
// splits on commas (used for --path include patterns).
func splitSemi(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseActionsFlag parses a comma-separated list of "name:ACT+ACT" entries
// (e.g. "g1:READ+WRITE,g2:READ") into a map of name -> actions.
func parseActionsFlag(s string) (map[string][]string, error) {
	result := map[string][]string{}
	s = strings.TrimSpace(s)
	if s == "" {
		return result, nil
	}
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid entry %q, expected format name:ACT+ACT", entry)
		}
		name := strings.TrimSpace(parts[0])
		rawActs := strings.Split(parts[1], "+")
		acts := make([]string, 0, len(rawActs))
		for _, a := range rawActs {
			a = strings.TrimSpace(a)
			if a != "" {
				acts = append(acts, a)
			}
		}
		result[name] = acts
	}
	return result, nil
}

// ---- error helpers -----------------------------------------------------

func fail(context string, err error) int {
	fmt.Fprintf(os.Stderr, "%s: %v\n", context, err)
	return 1
}

func failf(context, format string, a ...interface{}) int {
	fmt.Fprintf(os.Stderr, "%s: %s\n", context, fmt.Sprintf(format, a...))
	return 1
}
