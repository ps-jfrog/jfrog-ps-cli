package access

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestCreatePermission(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var captured V2Permission
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/access/api/v2/permissions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	})

	if err := CreatePermission(c, "my-perm", []string{"repo1", "repo2"}); err != nil {
		t.Fatalf("CreatePermission: %v", err)
	}
	if captured.Name != "my-perm" {
		t.Fatalf("unexpected name: %+v", captured)
	}
	targets := captured.Resources["artifact"].Targets
	if _, ok := targets["repo1"]; !ok {
		t.Fatalf("expected repo1 target: %+v", targets)
	}
	if _, ok := targets["repo2"]; !ok {
		t.Fatalf("expected repo2 target: %+v", targets)
	}
}

func TestCreatePermission_APIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"errors":[{"message":"already exists"}]}`))
	})
	err := CreatePermission(c, "dup", []string{"repo1"})
	if err == nil {
		t.Fatal("expected error for duplicate permission")
	}
}

func TestGetPermissionV2_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := GetPermissionV2(c, "missing")
	if !errors.Is(err, ErrPermissionNotFound) {
		t.Fatalf("expected ErrPermissionNotFound, got %v", err)
	}
}

func TestGrantPath_CreatesWhenMissing(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var posted V2Permission
	patchCalled := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/access/api/v2/permissions":
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch:
			patchCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	opts := GrantPathOptions{
		Permission: "new-perm",
		Repos:      []string{"repo1", "repo2"},
		Paths:      []string{"pat1", "pat2"},
		Groups:     map[string][]string{"g1": {"READ", "WRITE"}},
		Users:      map[string][]string{"u1": {"READ"}},
	}
	result, err := GrantPath(c, opts, false)
	if err != nil {
		t.Fatalf("GrantPath: %v", err)
	}
	if !result.WillCreate {
		t.Fatalf("expected WillCreate=true, got %+v", result)
	}
	if patchCalled {
		t.Fatal("PATCH should not be called on the create path")
	}
	if posted.Name != "new-perm" {
		t.Fatalf("unexpected posted body: %+v", posted)
	}
	res := posted.Resources["artifact"]
	if len(res.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %+v", res.Targets)
	}
	for repo, target := range res.Targets {
		if len(target.IncludePatterns) != 2 {
			t.Fatalf("repo %s: expected 2 include patterns, got %+v", repo, target.IncludePatterns)
		}
	}
	if res.Actions == nil || res.Actions.Groups["g1"] == nil {
		t.Fatalf("expected group actions to be set: %+v", res.Actions)
	}
}

func TestGrantPath_MergesWhenExists(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	existing := V2Permission{
		Name: "existing-perm",
		Resources: map[string]V2Resource{
			"artifact": {
				Targets: map[string]V2Target{
					"repo1": {IncludePatterns: []string{"old-pattern"}},
				},
				Actions: &V2Actions{
					Groups: map[string][]string{"g1": {"READ"}},
				},
			},
		},
	}
	var patched V2Resource
	postCalled := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existing)
		case r.Method == http.MethodPost:
			postCalled = true
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch:
			if r.URL.Path != "/access/api/v2/permissions/existing-perm/artifact" {
				t.Fatalf("unexpected PATCH path: %s", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	opts := GrantPathOptions{
		Permission: "existing-perm",
		Repos:      []string{"repo1", "repo2"}, // repo1 already present, repo2 new
		Paths:      []string{"new-pattern"},
		Groups:     map[string][]string{"g1": {"WRITE"}}, // merge into existing READ
	}
	result, err := GrantPath(c, opts, false)
	if err != nil {
		t.Fatalf("GrantPath: %v", err)
	}
	if result.WillCreate {
		t.Fatal("expected WillCreate=false on merge path")
	}
	if postCalled {
		t.Fatal("POST should not be called on the merge path")
	}
	if len(patched.Targets) != 2 {
		t.Fatalf("expected union of 2 targets, got %+v", patched.Targets)
	}
	// repo1's original pattern must survive untouched (existing keys kept).
	if got := patched.Targets["repo1"].IncludePatterns; len(got) != 1 || got[0] != "old-pattern" {
		t.Fatalf("expected repo1 to keep its original pattern, got %+v", got)
	}
	acts := patched.Actions.Groups["g1"]
	if len(acts) != 2 {
		t.Fatalf("expected merged actions [READ WRITE], got %+v", acts)
	}
}

func TestGrantPath_DryRunNoMutatingCalls(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("mutating call %s %s must not happen in dry-run mode", r.Method, r.URL.Path)
		}
	})

	opts := GrantPathOptions{Permission: "p", Repos: []string{"repo1"}, Paths: []string{"pat"}}
	result, err := GrantPath(c, opts, true)
	if err != nil {
		t.Fatalf("GrantPath dry-run: %v", err)
	}
	if !result.DryRun || !result.WillCreate {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAddRepos(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	existing := V2Permission{
		Name: "p",
		Resources: map[string]V2Resource{
			"artifact": {Targets: map[string]V2Target{"repo1": {}}},
		},
	}
	var put V2Permission
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existing)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result, err := AddRepos(c, "p", []string{"repo2"}, false)
	if err != nil {
		t.Fatalf("AddRepos: %v", err)
	}
	if len(result.After) != 2 {
		t.Fatalf("expected 2 targets after add, got %+v", result.After)
	}
	if _, ok := put.Resources["artifact"].Targets["repo2"]; !ok {
		t.Fatalf("expected repo2 in PUT body: %+v", put)
	}
}

func TestAddRepos_DryRunNoMutatingCalls(t *testing.T) {
	existing := V2Permission{
		Name:      "p",
		Resources: map[string]V2Resource{"artifact": {Targets: map[string]V2Target{"repo1": {}}}},
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("mutating call %s %s must not happen in dry-run mode", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existing)
	})

	result, err := AddRepos(c, "p", []string{"repo2"}, true)
	if err != nil {
		t.Fatalf("AddRepos dry-run: %v", err)
	}
	if len(result.After) != 2 {
		t.Fatalf("expected computed after-state even in dry-run: %+v", result)
	}
}

func TestRemoveRepos_ExactKeyMatch(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	// Regression test for the substring-matching bug in the original bash
	// script: removing "repo1" must not also remove "repo10".
	existing := V2Permission{
		Name: "p",
		Resources: map[string]V2Resource{
			"artifact": {Targets: map[string]V2Target{
				"repo1":  {},
				"repo10": {},
			}},
		},
	}
	var put V2Permission
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existing)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Fatalf("decode: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	result, err := RemoveRepos(c, "p", []string{"repo1"}, false)
	if err != nil {
		t.Fatalf("RemoveRepos: %v", err)
	}
	if len(result.After) != 1 || result.After[0] != "repo10" {
		t.Fatalf("expected only repo10 to remain, got %+v", result.After)
	}
	if _, ok := put.Resources["artifact"].Targets["repo10"]; !ok {
		t.Fatalf("expected repo10 preserved in PUT body: %+v", put)
	}
}

func TestRemoveRepos_DryRunNoMutatingCalls(t *testing.T) {
	existing := V2Permission{
		Name:      "p",
		Resources: map[string]V2Resource{"artifact": {Targets: map[string]V2Target{"repo1": {}, "repo2": {}}}},
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("mutating call %s %s must not happen in dry-run mode", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(existing)
	})

	result, err := RemoveRepos(c, "p", []string{"repo1"}, true)
	if err != nil {
		t.Fatalf("RemoveRepos dry-run: %v", err)
	}
	if len(result.After) != 1 || result.After[0] != "repo2" {
		t.Fatalf("unexpected computed after-state: %+v", result)
	}
}

func TestAuditPermissions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v2/permissions":
			w.Write([]byte(`{"permissions":[{"name":"p1"},{"name":"p2"},{"name":"p3"}]}`))
		case "/access/api/v2/permissions/p1":
			w.Write([]byte(`{"name":"p1","resources":{"artifact":{"targets":{"docker-local":{}}}}}`))
		case "/access/api/v2/permissions/p2":
			w.Write([]byte(`{"name":"p2","resources":{"artifact":{"targets":{"ANY LOCAL":{}}}}}`))
		case "/access/api/v2/permissions/p3":
			w.Write([]byte(`{"name":"p3","resources":{"artifact":{"targets":{"maven-local":{}}}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	matches, err := AuditPermissions(c, "^docker-", "local")
	if err != nil {
		t.Fatalf("AuditPermissions: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected p1 (regex) and p2 (ANY LOCAL wildcard), got %+v", matches)
	}
	names := map[string]bool{}
	for _, m := range matches {
		names[m.Permission] = true
	}
	if !names["p1"] || !names["p2"] {
		t.Fatalf("expected p1 and p2 in matches, got %+v", matches)
	}
}

func TestAuditPermissions_WildcardRespectsRepoType(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v2/permissions":
			w.Write([]byte(`{"permissions":[{"name":"p1"}]}`))
		case "/access/api/v2/permissions/p1":
			w.Write([]byte(`{"name":"p1","resources":{"artifact":{"targets":{"ANY REMOTE":{}}}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	matches, err := AuditPermissions(c, "^nomatch$", "local")
	if err != nil {
		t.Fatalf("AuditPermissions: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("ANY REMOTE should not match repo-type=local, got %+v", matches)
	}
}
