package access

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"jfrogpscli/internal/jfclient"
)

// backupFetchConcurrency bounds how many per-item detail GETs (one user,
// group, or permission target at a time) Backup issues at once, so backing
// up a large instance doesn't take one sequential round trip per entity.
const backupFetchConcurrency = 8

// BackupOptions configures Backup. If none of the *Only flags are set, all
// three artifact types are backed up.
type BackupOptions struct {
	OutputDir       string
	UsersOnly       bool
	GroupsOnly      bool
	PermissionsOnly bool

	// Progress, if non-nil, is called as each of the three stages
	// (Users/Groups/Permissions) fetches per-item detail: stage names it,
	// done/total drive a progress bar. Stages that are skipped (see the
	// *Only fields) never call it.
	Progress func(stage string, done, total int)
}

// BackupResult reports which files were written by Backup (a field is empty
// if that artifact type was skipped). Warnings lists individual users/
// groups/permission targets whose detail fetch failed -- they are omitted
// from the corresponding file rather than aborting the whole backup.
type BackupResult struct {
	UsersFile       string
	GroupsFile      string
	PermissionsFile string
	Warnings        []string
}

// fetchDetailsConcurrently calls fetch(name) for each entry in names with up
// to backupFetchConcurrency in flight at once. A single name's failure is
// appended to warnings and that entry is omitted from the result, rather
// than aborting every other in-flight or pending fetch.
func fetchDetailsConcurrently[T any](names []string, fetch func(name string) (*T, error), progress func(done, total int)) (results []*T, warnings []string) {
	items := make([]*T, len(names))
	sem := make(chan struct{}, backupFetchConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var done atomic.Int64

	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			v, err := fetch(name)
			mu.Lock()
			if err != nil {
				warnings = append(warnings, err.Error())
			} else {
				items[i] = v
			}
			mu.Unlock()
			if progress != nil {
				progress(int(done.Add(1)), len(names))
			}
		}(i, name)
	}
	wg.Wait()

	for _, v := range items {
		if v != nil {
			results = append(results, v)
		}
	}
	return results, warnings
}

// Backup fetches users, groups, and permissions from the Access API v2 and
// writes each as a pretty-printed JSON file into opts.OutputDir: users.json
// and groups.json each hold a JSON array of full per-item detail
// ([]V2User / []V2Group -- see ListUsersV2/GetUserV2 and
// ListGroupsV2/GetGroupV2), and permissions.json holds a JSON array of
// []V2Permission. All three are typed, round-trippable shapes so `access
// restore` can recreate them on another JPD.
func Backup(c *jfclient.Client, opts BackupOptions) (*BackupResult, error) {
	doUsers, doGroups, doPermissions := true, true, true
	if opts.UsersOnly || opts.GroupsOnly || opts.PermissionsOnly {
		doUsers, doGroups, doPermissions = opts.UsersOnly, opts.GroupsOnly, opts.PermissionsOnly
	}

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir %s: %w", opts.OutputDir, err)
	}

	result := &BackupResult{}

	if doUsers {
		refs, err := ListUsersV2(c)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(refs))
		for i, ref := range refs {
			names[i] = ref.Username
		}
		users, warnings := fetchDetailsConcurrently(names, func(name string) (*V2User, error) {
			return GetUserV2(c, name)
		}, stageProgress(opts.Progress, "Users"))
		result.Warnings = append(result.Warnings, warnings...)
		path := filepath.Join(opts.OutputDir, "users.json")
		if err := writeIndentedJSON(path, users); err != nil {
			return nil, err
		}
		result.UsersFile = path
	}

	if doGroups {
		refs, err := ListGroupsV2(c)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(refs))
		for i, ref := range refs {
			names[i] = ref.GroupName
		}
		groups, warnings := fetchDetailsConcurrently(names, func(name string) (*V2Group, error) {
			return GetGroupV2(c, name)
		}, stageProgress(opts.Progress, "Groups"))
		result.Warnings = append(result.Warnings, warnings...)
		path := filepath.Join(opts.OutputDir, "groups.json")
		if err := writeIndentedJSON(path, groups); err != nil {
			return nil, err
		}
		result.GroupsFile = path
	}

	if doPermissions {
		refs, err := ListPermissionsV2(c)
		if err != nil {
			return nil, err
		}
		names := make([]string, len(refs))
		for i, ref := range refs {
			names[i] = ref.Name
		}
		details, warnings := fetchDetailsConcurrently(names, func(name string) (*V2Permission, error) {
			return GetPermissionV2(c, name)
		}, stageProgress(opts.Progress, "Permissions"))
		result.Warnings = append(result.Warnings, warnings...)
		path := filepath.Join(opts.OutputDir, "permissions.json")
		if err := writeIndentedJSON(path, details); err != nil {
			return nil, err
		}
		result.PermissionsFile = path
	}

	return result, nil
}

// stageProgress adapts a BackupOptions.Progress callback (which names the
// stage) into the plain done/total callback fetchDetailsConcurrently takes,
// or returns nil if overall is nil.
func stageProgress(overall func(stage string, done, total int), stage string) func(done, total int) {
	if overall == nil {
		return nil
	}
	return func(done, total int) { overall(stage, done, total) }
}

// writeIndentedJSON marshals v as pretty-printed JSON and writes it to path.
func writeIndentedJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
