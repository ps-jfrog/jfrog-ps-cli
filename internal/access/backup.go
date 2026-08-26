package access

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"jfrogpscli/internal/jfclient"
)

// BackupOptions configures Backup. If none of the *Only flags are set, all
// three artifact types are backed up.
type BackupOptions struct {
	OutputDir       string
	UsersOnly       bool
	GroupsOnly      bool
	PermissionsOnly bool
}

// BackupResult reports which files were written by Backup (a field is empty
// if that artifact type was skipped).
type BackupResult struct {
	UsersFile       string
	GroupsFile      string
	PermissionsFile string
}

// Backup fetches users, groups, and permissions from the Access API v2 and
// writes each as a pretty-printed JSON file into opts.OutputDir.
//
// ASSUMPTION (not verified against a live instance): the v2 list endpoints
// used here (GET /access/api/v2/users, GET /access/api/v2/groups) return
// simple, non-paginated {"users":[...]}/{"groups":[...]} shapes. Since this
// package does not need to inspect the contents of those two lists (only
// persist them), their raw JSON bodies are re-indented and written as-is
// rather than being decoded into a typed struct. Permissions ARE decoded
// (via ListPermissionsV2/GetPermissionV2) since a name + detail fetch is
// required per permission.
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
		resp, err := c.Get("/access/api/v2/users")
		if err != nil {
			return nil, err
		}
		if err := resp.Error("list users"); err != nil {
			return nil, err
		}
		path := filepath.Join(opts.OutputDir, "users.json")
		if err := writePrettyJSON(path, resp.Body); err != nil {
			return nil, err
		}
		result.UsersFile = path
	}

	if doGroups {
		resp, err := c.Get("/access/api/v2/groups")
		if err != nil {
			return nil, err
		}
		if err := resp.Error("list groups"); err != nil {
			return nil, err
		}
		path := filepath.Join(opts.OutputDir, "groups.json")
		if err := writePrettyJSON(path, resp.Body); err != nil {
			return nil, err
		}
		result.GroupsFile = path
	}

	if doPermissions {
		refs, err := ListPermissionsV2(c)
		if err != nil {
			return nil, err
		}
		details := make([]*V2Permission, 0, len(refs))
		for _, ref := range refs {
			p, err := GetPermissionV2(c, ref.Name)
			if err != nil {
				return nil, err
			}
			details = append(details, p)
		}
		data, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode permissions backup: %w", err)
		}
		path := filepath.Join(opts.OutputDir, "permissions.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
		result.PermissionsFile = path
	}

	return result, nil
}

// writePrettyJSON re-indents raw JSON bytes and writes them to path.
func writePrettyJSON(path string, raw []byte) error {
	var buf bytes.Buffer
	if len(raw) > 0 {
		if err := json.Indent(&buf, raw, "", "  "); err != nil {
			// Not valid JSON (shouldn't happen for a 2xx API response) --
			// fall back to writing the raw bytes rather than losing data.
			buf.Reset()
			buf.Write(raw)
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
