package access

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestParseNamesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.txt")
	content := "alice\n\n# a comment\nbob\n  \ncarol\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := ParseNamesFile(path)
	if err != nil {
		t.Fatalf("ParseNamesFile: %v", err)
	}
	want := []string{"alice", "bob", "carol"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestFilterDeletable_ProtectsBuiltins(t *testing.T) {
	names := []string{"admin", "anonymous", "alice", "bob"}
	got := FilterDeletable(DeleteUsers, names, nil)
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("expected admin/anonymous to be protected, got %+v", got)
	}
}

func TestFilterDeletable_UserExcludeAndDedup(t *testing.T) {
	names := []string{"alice", "bob", "alice", "carol"}
	got := FilterDeletable(DeleteUsers, names, []string{"bob"})
	if len(got) != 2 || got[0] != "alice" || got[1] != "carol" {
		t.Fatalf("expected [alice carol], got %+v", got)
	}
}

func TestFilterDeletable_GroupsAndPermissionsProtectedSets(t *testing.T) {
	groups := FilterDeletable(DeleteGroups, []string{"readers", "Anyremote", "admin", "custom-group"}, nil)
	if len(groups) != 1 || groups[0] != "custom-group" {
		t.Fatalf("expected only custom-group to survive, got %+v", groups)
	}
	perms := FilterDeletable(DeletePermissions, []string{"Any Remote", "Anything", "INTERNAL", "my-perm"}, nil)
	if len(perms) != 1 || perms[0] != "my-perm" {
		t.Fatalf("expected only my-perm to survive, got %+v", perms)
	}
}

func TestDeleteItems_DryRunNoMutatingCalls(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no HTTP call should happen in dry-run mode, got %s %s", r.Method, r.URL.Path)
	})

	results := DeleteItems(c, DeleteUsers, []string{"alice", "bob"}, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected dry-run results to report success, got %+v", r)
		}
	}
}

func TestDeleteItems_Real(t *testing.T) {
	var calledPaths []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		calledPaths = append(calledPaths, r.URL.Path)
		if r.URL.Path == "/access/api/v2/users/bob" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	results := DeleteItems(c, DeleteUsers, []string{"alice", "bob"}, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %+v", results)
	}
	if !results[0].Success || results[0].Name != "alice" {
		t.Fatalf("expected alice to succeed, got %+v", results[0])
	}
	if results[1].Success || results[1].Name != "bob" {
		t.Fatalf("expected bob to fail (404), got %+v", results[1])
	}
	if len(calledPaths) != 2 || calledPaths[0] != "/access/api/v2/users/alice" || calledPaths[1] != "/access/api/v2/users/bob" {
		t.Fatalf("unexpected called paths: %+v", calledPaths)
	}
}
