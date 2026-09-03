package access

import (
	"net/http"
	"sync"
	"testing"
)

func TestGetEffectivePermissions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/artifactory/api/artifactpermissions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("repoKey"); got != "repo1" {
			t.Fatalf("expected repoKey=repo1, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userEffectivePermissions": [
				{"principal":"alice","admin":true,"permission":{"delete":true,"deploy":true,"annotate":true,"read":true,"distribute":false,"managedXrayMeta":false},"permissionTargets":["p1"]}
			],
			"groupEffectivePermissions": [
				{"principal":"readers","admin":false,"permission":{"delete":false,"deploy":false,"annotate":false,"read":true,"distribute":false,"managedXrayMeta":false},"permissionTargets":["p1","p2"]}
			]
		}`))
	})

	result, err := GetEffectivePermissions(c, "repo1")
	if err != nil {
		t.Fatalf("GetEffectivePermissions: %v", err)
	}
	if result.Repo != "repo1" {
		t.Fatalf("expected Repo=repo1, got %q", result.Repo)
	}
	if len(result.Users) != 1 || result.Users[0].Principal != "alice" || !result.Users[0].Admin || !result.Users[0].Delete {
		t.Fatalf("unexpected users: %+v", result.Users)
	}
	if len(result.Groups) != 1 || result.Groups[0].Principal != "readers" || result.Groups[0].Delete {
		t.Fatalf("unexpected groups: %+v", result.Groups)
	}
	if len(result.Groups[0].PermissionTargets) != 2 {
		t.Fatalf("expected 2 permission targets, got %+v", result.Groups[0].PermissionTargets)
	}
}

func TestGetEffectivePermissionsForRepos(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Query().Get("repoKey")] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"userEffectivePermissions":[],"groupEffectivePermissions":[]}`))
	})

	results, err := GetEffectivePermissionsForRepos(c, []string{"repo1", "repo2"})
	if err != nil {
		t.Fatalf("GetEffectivePermissionsForRepos: %v", err)
	}
	if len(results) != 2 || results[0].Repo != "repo1" || results[1].Repo != "repo2" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if !seen["repo1"] || !seen["repo2"] {
		t.Fatalf("expected both repos queried, got %+v", seen)
	}
}
