package access

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
)

// RestoreOptions configures Restore. It reads the same file layout Backup
// writes (users.json/groups.json/permissions.json, each a JSON array --
// see Backup's doc comment); a missing file is treated as "nothing of that
// kind to restore", not an error, so a partial backup (e.g. --users-only)
// still restores cleanly.
type RestoreOptions struct {
	InputDir string

	// UserSuffix/GroupSuffix, if set, are appended to every restored
	// username/group name (and to every user/group reference inside
	// restored group memberships and permission principals), e.g. to merge
	// two orgs' identities without colliding with the target's existing
	// names. A username already containing "@" has its domain replaced
	// (matching the source script's behavior for already-suffixed names).
	UserSuffix  string
	GroupSuffix string

	// IncludeUsers/IncludeGroups/IncludePermissions, if non-empty, restrict
	// restoration to just those (original, pre-suffix) names. An empty list
	// means "all".
	IncludeUsers       []string
	IncludeGroups      []string
	IncludePermissions []string

	// Progress, if non-nil, is called as each of the three stages
	// (Users/Groups/Permissions) is restored: stage names it, done/total
	// drive a progress bar.
	Progress func(stage string, done, total int)
}

// RestoreAction describes what happened (or, in dry-run mode, would happen)
// to one restored entity.
type RestoreAction string

const (
	RestoreCreated RestoreAction = "created"
	RestoreSkipped RestoreAction = "skipped (already exists)"
	RestoreFailed  RestoreAction = "failed"
)

// RestoreItemResult is the outcome for one user/group/permission.
type RestoreItemResult struct {
	Name   string // the name actually used on the target (after suffixing)
	Action RestoreAction
	Error  string
}

// RestoreResult is the full outcome of a Restore call.
type RestoreResult struct {
	Users       []RestoreItemResult
	Groups      []RestoreItemResult
	Permissions []RestoreItemResult
}

// LoadBackupDir reads users.json/groups.json/permissions.json from dir (any
// of the three may be absent, yielding an empty slice for that kind).
func LoadBackupDir(dir string) (users []V2User, groups []V2Group, perms []V2Permission, err error) {
	if err := readJSONFileIfExists(filepath.Join(dir, "users.json"), &users); err != nil {
		return nil, nil, nil, err
	}
	if err := readJSONFileIfExists(filepath.Join(dir, "groups.json"), &groups); err != nil {
		return nil, nil, nil, err
	}
	if err := readJSONFileIfExists(filepath.Join(dir, "permissions.json"), &perms); err != nil {
		return nil, nil, nil, err
	}
	return users, groups, perms, nil
}

func readJSONFileIfExists(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// Restore reads a backup directory (see LoadBackupDir) and recreates on c
// whatever users/groups/permissions from it don't already exist there --
// in that order, since groups reference users and permissions reference
// both. Existing entities are always left untouched (skipped), never
// updated: this is a purely additive operation. When dryRun is true, only
// existence-check GETs are issued -- no entity is ever created.
func Restore(c *jfclient.Client, opts RestoreOptions, dryRun bool) (*RestoreResult, error) {
	users, groups, perms, err := LoadBackupDir(opts.InputDir)
	if err != nil {
		return nil, err
	}
	result := &RestoreResult{
		Users:       restoreUsers(c, users, opts, dryRun),
		Groups:      restoreGroups(c, groups, opts, dryRun),
		Permissions: restorePermissions(c, perms, opts, dryRun),
	}
	return result, nil
}

// restoreUsers checks/creates each user independently, with up to
// concurrency.DefaultParallelism in flight at once.
func restoreUsers(c *jfclient.Client, users []V2User, opts RestoreOptions, dryRun bool) []RestoreItemResult {
	include := toSet(opts.IncludeUsers)
	onProgress := stageProgress(opts.Progress, "Users")
	return concurrency.Map(users, 0, func(u V2User) []RestoreItemResult {
		if len(include) > 0 && !include[u.Username] {
			return nil
		}
		newName := applySuffix(u.Username, opts.UserSuffix)

		exists, err := userExists(c, newName)
		if err != nil {
			return []RestoreItemResult{{Name: newName, Action: RestoreFailed, Error: err.Error()}}
		}
		if exists {
			return []RestoreItemResult{{Name: newName, Action: RestoreSkipped}}
		}
		if dryRun {
			return []RestoreItemResult{{Name: newName, Action: RestoreCreated}}
		}

		create := u
		create.Username = newName
		create.LastLoggedIn = ""
		if create.Realm == "" {
			create.Realm = "internal"
		}
		create.Groups = nil // group membership is (re)established when groups are restored
		if create.Email == "" {
			create.Email = newName
		}
		// A backup never carries a raw password (the API doesn't return
		// one), so a recreated user can never authenticate with its
		// original internal password -- always disable it, regardless of
		// realm, so the account is created in a consistent, expected state
		// rather than a silently-unusable "enabled but password-less" one.
		create.InternalPasswordDisabled = true

		if err := CreateUserV2(c, &create); err != nil {
			return []RestoreItemResult{{Name: newName, Action: RestoreFailed, Error: err.Error()}}
		}
		return []RestoreItemResult{{Name: newName, Action: RestoreCreated}}
	}, onProgress)
}

// restoreGroups checks/creates each group independently, with up to
// concurrency.DefaultParallelism in flight at once.
func restoreGroups(c *jfclient.Client, groups []V2Group, opts RestoreOptions, dryRun bool) []RestoreItemResult {
	include := toSet(opts.IncludeGroups)
	onProgress := stageProgress(opts.Progress, "Groups")
	return concurrency.Map(groups, 0, func(g V2Group) []RestoreItemResult {
		if len(include) > 0 && !include[g.Name] {
			return nil
		}
		newName := applySuffix(g.Name, opts.GroupSuffix)

		exists, err := groupExists(c, newName)
		if err != nil {
			return []RestoreItemResult{{Name: newName, Action: RestoreFailed, Error: err.Error()}}
		}
		if exists {
			return []RestoreItemResult{{Name: newName, Action: RestoreSkipped}}
		}
		if dryRun {
			return []RestoreItemResult{{Name: newName, Action: RestoreCreated}}
		}

		create := g
		create.Name = newName
		create.Members = applySuffixToAll(g.Members, opts.UserSuffix)

		if err := CreateGroupV2(c, &create); err != nil {
			return []RestoreItemResult{{Name: newName, Action: RestoreFailed, Error: err.Error()}}
		}
		return []RestoreItemResult{{Name: newName, Action: RestoreCreated}}
	}, onProgress)
}

// restorePermissions checks/creates each permission target independently,
// with up to concurrency.DefaultParallelism in flight at once.
func restorePermissions(c *jfclient.Client, perms []V2Permission, opts RestoreOptions, dryRun bool) []RestoreItemResult {
	include := toSet(opts.IncludePermissions)
	onProgress := stageProgress(opts.Progress, "Permissions")
	return concurrency.Map(perms, 0, func(p V2Permission) []RestoreItemResult {
		// Permission names are never suffixed (they aren't identities being
		// merged across orgs -- only the user/group principals inside them
		// are), matching the source script's behavior.
		if len(include) > 0 && !include[p.Name] {
			return nil
		}

		_, err := GetPermissionV2(c, p.Name)
		if err != nil && !errors.Is(err, ErrPermissionNotFound) {
			return []RestoreItemResult{{Name: p.Name, Action: RestoreFailed, Error: err.Error()}}
		}
		if err == nil {
			return []RestoreItemResult{{Name: p.Name, Action: RestoreSkipped}}
		}
		if dryRun {
			return []RestoreItemResult{{Name: p.Name, Action: RestoreCreated}}
		}

		create := p
		create.Resources = make(map[string]V2Resource, len(p.Resources))
		for resType, res := range p.Resources {
			create.Resources[resType] = suffixResourceActions(res, opts.UserSuffix, opts.GroupSuffix)
		}

		if err := createFullPermission(c, &create); err != nil {
			return []RestoreItemResult{{Name: p.Name, Action: RestoreFailed, Error: err.Error()}}
		}
		return []RestoreItemResult{{Name: p.Name, Action: RestoreCreated}}
	}, onProgress)
}

// createFullPermission creates a permission target from a full
// V2Permission object (unlike CreatePermission, which only grants bare repo
// access with no principals) via POST /access/api/v2/permissions.
func createFullPermission(c *jfclient.Client, p *V2Permission) error {
	resp, err := c.Post("/access/api/v2/permissions", p)
	if err != nil {
		return err
	}
	return resp.Error("create permission " + p.Name)
}

// suffixResourceActions returns a copy of res with every user/group
// principal key in its Actions map suffixed.
func suffixResourceActions(res V2Resource, userSuffix, groupSuffix string) V2Resource {
	if res.Actions == nil {
		return res
	}
	out := res
	actions := &V2Actions{}
	if len(res.Actions.Users) > 0 {
		actions.Users = make(map[string][]string, len(res.Actions.Users))
		for u, acts := range res.Actions.Users {
			actions.Users[applySuffix(u, userSuffix)] = acts
		}
	}
	if len(res.Actions.Groups) > 0 {
		actions.Groups = make(map[string][]string, len(res.Actions.Groups))
		for g, acts := range res.Actions.Groups {
			actions.Groups[applySuffix(g, groupSuffix)] = acts
		}
	}
	out.Actions = actions
	return out
}

func userExists(c *jfclient.Client, username string) (bool, error) {
	_, err := GetUserV2(c, username)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrUserNotFound) {
		return false, nil
	}
	return false, err
}

func groupExists(c *jfclient.Client, name string) (bool, error) {
	_, err := GetGroupV2(c, name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrGroupNotFound) {
		return false, nil
	}
	return false, err
}

// applySuffix appends suffix to name (replacing any existing "@domain" part
// first), or returns name unchanged if suffix is empty.
func applySuffix(name, suffix string) string {
	if suffix == "" {
		return name
	}
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	return name + suffix
}

func applySuffixToAll(names []string, suffix string) []string {
	if suffix == "" {
		return names
	}
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = applySuffix(n, suffix)
	}
	return out
}

func toSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}
