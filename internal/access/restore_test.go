package access

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func decodeJSONBody(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	var v map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		t.Fatalf("decode request body for %s: %v", r.URL.Path, err)
	}
	return v
}

func writeBackupFixture(t *testing.T, dir string) {
	t.Helper()
	users := `[{"username":"alice","email":"alice@example.com","realm":"internal"},{"username":"bob","realm":"internal"}]`
	groups := `[{"name":"g1","members":["alice","bob"]}]`
	perms := `[{"name":"p1","resources":{"artifact":{"targets":{"repo1":{}},"actions":{"users":{"alice":["read"]},"groups":{"g1":["read"]}}}}}]`
	for name, content := range map[string]string{
		"users.json":       users,
		"groups.json":      groups,
		"permissions.json": perms,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
}

func TestRestore_DryRunIssuesNoWrites(t *testing.T) {
	dir := t.TempDir()
	writeBackupFixture(t, dir)

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound) // nothing exists on target yet
		default:
			t.Fatalf("dry-run must never issue a write, got %s %s", r.Method, r.URL.Path)
		}
	})

	result, err := Restore(c, RestoreOptions{InputDir: dir}, true)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Users) != 2 || len(result.Groups) != 1 || len(result.Permissions) != 1 {
		t.Fatalf("unexpected result shape: %+v", result)
	}
	for _, r := range result.Users {
		if r.Action != RestoreCreated {
			t.Fatalf("expected all users marked would-create, got %+v", r)
		}
	}
}

func TestRestore_SkipsExistingEntities(t *testing.T) {
	dir := t.TempDir()
	writeBackupFixture(t, dir)

	created := map[string]bool{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/access/api/v2/users/alice":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"username":"alice"}`))
		case r.Method == http.MethodGet && (r.URL.Path == "/access/api/v2/users/bob" ||
			r.URL.Path == "/access/api/v2/groups/g1" ||
			r.URL.Path == "/access/api/v2/permissions/p1"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost:
			created[r.URL.Path] = true
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	result, err := Restore(c, RestoreOptions{InputDir: dir}, false)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	var aliceResult, bobResult RestoreItemResult
	for _, u := range result.Users {
		switch u.Name {
		case "alice":
			aliceResult = u
		case "bob":
			bobResult = u
		}
	}
	if aliceResult.Action != RestoreSkipped {
		t.Fatalf("expected alice to be skipped (already exists), got %+v", aliceResult)
	}
	if bobResult.Action != RestoreCreated {
		t.Fatalf("expected bob to be created, got %+v", bobResult)
	}
	if !created["/access/api/v2/users"] {
		t.Fatalf("expected a POST to create bob")
	}
	if !created["/access/api/v2/groups"] || !created["/access/api/v2/permissions"] {
		t.Fatalf("expected group and permission to be created, got %+v", created)
	}
}

func TestRestore_AppliesSuffixesToNamesAndReferences(t *testing.T) {
	dir := t.TempDir()
	writeBackupFixture(t, dir)

	var createdUser, createdGroup, createdPerm map[string]interface{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/access/api/v2/users":
			createdUser = decodeJSONBody(t, r)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && r.URL.Path == "/access/api/v2/groups":
			createdGroup = decodeJSONBody(t, r)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && r.URL.Path == "/access/api/v2/permissions":
			createdPerm = decodeJSONBody(t, r)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	opts := RestoreOptions{
		InputDir:      dir,
		UserSuffix:    "@corp.example",
		GroupSuffix:   "-merged",
		IncludeUsers:  []string{"alice"},
		IncludeGroups: []string{"g1"},
	}
	result, err := Restore(c, opts, false)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Users) != 1 || result.Users[0].Name != "alice@corp.example" {
		t.Fatalf("expected only suffixed alice, got %+v", result.Users)
	}
	if createdUser["username"] != "alice@corp.example" {
		t.Fatalf("expected created user payload to carry suffixed username, got %+v", createdUser)
	}
	if createdGroup["name"] != "g1-merged" {
		t.Fatalf("expected created group payload to carry suffixed name, got %+v", createdGroup)
	}
	members, _ := createdGroup["members"].([]interface{})
	if len(members) != 2 || members[0] != "alice@corp.example" || members[1] != "bob@corp.example" {
		t.Fatalf("expected group members suffixed too, got %+v", createdGroup["members"])
	}
	if createdPerm["name"] != "p1" {
		t.Fatalf("expected permission name unchanged, got %+v", createdPerm["name"])
	}
	resources, _ := createdPerm["resources"].(map[string]interface{})
	artifact, _ := resources["artifact"].(map[string]interface{})
	actions, _ := artifact["actions"].(map[string]interface{})
	users, _ := actions["users"].(map[string]interface{})
	groups, _ := actions["groups"].(map[string]interface{})
	if _, ok := users["alice@corp.example"]; !ok {
		t.Fatalf("expected permission user reference suffixed, got %+v", users)
	}
	if _, ok := groups["g1-merged"]; !ok {
		t.Fatalf("expected permission group reference suffixed, got %+v", groups)
	}
}

func TestRestore_MissingBackupFilesAreNotAnError(t *testing.T) {
	dir := t.TempDir() // no users.json/groups.json/permissions.json at all
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected no HTTP calls when the backup dir is empty, got %s %s", r.Method, r.URL.Path)
	})

	result, err := Restore(c, RestoreOptions{InputDir: dir}, true)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(result.Users) != 0 || len(result.Groups) != 0 || len(result.Permissions) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}
