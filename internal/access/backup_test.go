package access

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestBackup_All(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v2/users":
			w.Write([]byte(`{"users":[{"name":"alice"}]}`))
		case "/access/api/v2/groups":
			w.Write([]byte(`{"groups":[{"name":"g1"}]}`))
		case "/access/api/v2/permissions":
			w.Write([]byte(`{"permissions":[{"name":"p1"}]}`))
		case "/access/api/v2/permissions/p1":
			w.Write([]byte(`{"name":"p1","resources":{"artifact":{"targets":{"repo1":{}}}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	result, err := Backup(c, BackupOptions{OutputDir: dir})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	for _, f := range []string{result.UsersFile, result.GroupsFile, result.PermissionsFile} {
		if f == "" {
			t.Fatalf("expected all three files to be written, got %+v", result)
		}
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected file %s to exist: %v", f, err)
		}
	}

	var perms []V2Permission
	data, err := os.ReadFile(filepath.Join(dir, "permissions.json"))
	if err != nil {
		t.Fatalf("read permissions.json: %v", err)
	}
	if err := json.Unmarshal(data, &perms); err != nil {
		t.Fatalf("unmarshal permissions.json: %v", err)
	}
	if len(perms) != 1 || perms[0].Name != "p1" {
		t.Fatalf("unexpected permissions.json content: %+v", perms)
	}
}

func TestBackup_UsersOnly(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/access/api/v2/users" {
			t.Fatalf("only /access/api/v2/users should be called, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"users":[]}`))
	})

	result, err := Backup(c, BackupOptions{OutputDir: dir, UsersOnly: true})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if result.UsersFile == "" || result.GroupsFile != "" || result.PermissionsFile != "" {
		t.Fatalf("expected only users file written, got %+v", result)
	}
}
