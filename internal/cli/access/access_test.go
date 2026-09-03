package accesscmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_NoArgsReturnsUsageError(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected exit code 1 for no args, got %d", code)
	}
}

func TestRun_HelpReturnsZero(t *testing.T) {
	if code := Run([]string{"help"}); code != 0 {
		t.Fatalf("expected exit code 0 for help, got %d", code)
	}
	if code := Run([]string{"-h"}); code != 0 {
		t.Fatalf("expected exit code 0 for -h, got %d", code)
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	if code := Run([]string{"does-not-exist"}); code != 1 {
		t.Fatalf("expected exit code 1 for unknown subcommand, got %d", code)
	}
}

func TestRun_ListPermissions_MissingConnectionFlags(t *testing.T) {
	// No --url/--token/--server, so client resolution must fail cleanly.
	if code := Run([]string{"list-permissions"}); code != 1 {
		t.Fatalf("expected exit code 1 when no connection info is supplied, got %d", code)
	}
}

func TestRun_ShowPermission_MissingName(t *testing.T) {
	if code := Run([]string{"show-permission", "--url", "https://example.test", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit code 1 when --name is missing, got %d", code)
	}
}

func TestRun_CreatePermission_MissingFlags(t *testing.T) {
	if code := Run([]string{"create-permission"}); code != 1 {
		t.Fatalf("expected exit code 1 when required flags are missing, got %d", code)
	}
}

func TestRun_ListPermissions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"name":"p1","uri":"/p1"}]`))
	}))
	defer srv.Close()

	code := Run([]string{"list-permissions", "--url", srv.URL, "--token", "tkn"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestRun_DeleteUsers_DryRunDoesNotRequireConnection(t *testing.T) {
	dir := t.TempDir()
	namesFile := filepath.Join(dir, "names.txt")
	if err := os.WriteFile(namesFile, []byte("alice\nbob\nadmin\n"), 0o644); err != nil {
		t.Fatalf("write names file: %v", err)
	}

	// No --url/--token provided at all: dry-run must succeed without ever
	// needing to resolve a client, since it makes no HTTP calls.
	code := Run([]string{"delete-users", "--file", namesFile})
	if code != 0 {
		t.Fatalf("expected exit code 0 for dry-run delete, got %d", code)
	}
}

func TestRun_DeleteUsers_NothingToDelete(t *testing.T) {
	dir := t.TempDir()
	namesFile := filepath.Join(dir, "names.txt")
	if err := os.WriteFile(namesFile, []byte("admin\nanonymous\n"), 0o644); err != nil {
		t.Fatalf("write names file: %v", err)
	}

	code := Run([]string{"delete-users", "--file", namesFile})
	if code != 0 {
		t.Fatalf("expected exit code 0 when all names are protected, got %d", code)
	}
}

func TestParseActionsFlag(t *testing.T) {
	got, err := parseActionsFlag("g1:READ+WRITE,g2:READ")
	if err != nil {
		t.Fatalf("parseActionsFlag: %v", err)
	}
	if len(got["g1"]) != 2 || got["g1"][0] != "READ" || got["g1"][1] != "WRITE" {
		t.Fatalf("unexpected g1 actions: %+v", got["g1"])
	}
	if len(got["g2"]) != 1 || got["g2"][0] != "READ" {
		t.Fatalf("unexpected g2 actions: %+v", got["g2"])
	}
}

func TestParseActionsFlag_Empty(t *testing.T) {
	got, err := parseActionsFlag("")
	if err != nil {
		t.Fatalf("parseActionsFlag: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %+v", got)
	}
}

func TestParseActionsFlag_Invalid(t *testing.T) {
	if _, err := parseActionsFlag("no-colon-here"); err == nil {
		t.Fatal("expected error for malformed entry")
	}
}

func TestSplitCSVAndSemi(t *testing.T) {
	if got := splitCSV(" a, b ,,c "); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected splitCSV result: %+v", got)
	}
	if got := splitCSV(""); got != nil {
		t.Fatalf("expected nil for empty input, got %+v", got)
	}
	if got := splitSemi("pat1; pat2 ;;pat3"); len(got) != 3 || got[0] != "pat1" || got[1] != "pat2" || got[2] != "pat3" {
		t.Fatalf("unexpected splitSemi result: %+v", got)
	}
}

func TestRun_EffectivePermissions_MissingRepos(t *testing.T) {
	if code := Run([]string{"effective-permissions", "--url", "https://example.test", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit code 1 when --repos is missing, got %d", code)
	}
}

func TestRun_EffectivePermissions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/artifactpermissions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"userEffectivePermissions":[{"principal":"alice","admin":false,"permission":{"read":true},"permissionTargets":["p1"]}],"groupEffectivePermissions":[]}`))
	}))
	defer srv.Close()

	code := Run([]string{"effective-permissions", "--repos", "repo1", "--url", srv.URL, "--token", "tkn"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestRun_Restore_MissingInput(t *testing.T) {
	if code := Run([]string{"restore", "--url", "https://example.test", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit code 1 when --input is missing, got %d", code)
	}
}

func TestRun_Restore_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte(`[{"username":"alice"}]`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("dry-run must never issue a write, got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	code := Run([]string{"restore", "--input", dir, "--url", srv.URL, "--token", "tkn"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestRun_Restore_NothingToRestore(t *testing.T) {
	dir := t.TempDir() // no backup files at all

	code := Run([]string{"restore", "--input", dir, "--url", "https://example.test", "--token", "t", "--dry-run=false", "--yes"})
	if code != 0 {
		t.Fatalf("expected exit code 0 when there is nothing to restore, got %d", code)
	}
}
