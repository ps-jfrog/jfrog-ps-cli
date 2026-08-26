package discovercmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun_NoFlags(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected exit code 1 when no selection flag is given, got %d", code)
	}
}

func TestRun_Help(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		if code := Run(args); code != 0 {
			t.Fatalf("expected exit code 0 for %v, got %d", args, code)
		}
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	if code := Run([]string{"--bogus"}); code != 1 {
		t.Fatalf("expected exit code 1 for an unknown flag, got %d", code)
	}
}

func TestRun_ReposMissingCredentials(t *testing.T) {
	if code := Run([]string{"--repos", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_PoliciesMissingCredentials(t *testing.T) {
	if code := Run([]string{"--policies", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_InstanceMissingCredentials(t *testing.T) {
	if code := Run([]string{"--instance", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_AllRequiresCredentialsOnlyOnce(t *testing.T) {
	// --all selects repos/policies/instance/analyze together; it should
	// fail fast on the first network-touching piece (repos) rather than
	// requiring --analyze's non-network path to run first.
	if code := Run([]string{"--all", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_AnalyzeMissingReposCSV(t *testing.T) {
	dir := t.TempDir()
	code := Run([]string{"--analyze", "--repos-csv", filepath.Join(dir, "does-not-exist.csv")})
	if code != 1 {
		t.Fatalf("expected exit code 1 when --repos-csv doesn't exist, got %d", code)
	}
}

func TestRun_AnalyzeHappyPath_NoNetworkCallsRequired(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "report.csv")
	htmlPath := filepath.Join(dir, "audit.html")
	missingInstancePath := filepath.Join(dir, "instance.json") // deliberately absent

	csvContent := "repo_name,package_type,repo_type,files_count,usage_gb,created_date,last_push_date,last_pull_date,last_activity_date,cleanup_policy_keys,cleanup_policy_enabled,cleanup_policy_cron_exps,cleanup_policy_created_before_days,cleanup_policy_created_before_months,cleanup_policy_last_downloaded_before_days,cleanup_policy_last_downloaded_before_months,cleanup_policy_keep_last_n_versions\n" +
		"docker-local,Docker,LOCAL,0,0,,,,,,,,,,,,\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	// No --url/--token at all: --analyze alone must never try to resolve a
	// client or make a network call.
	code := Run([]string{"--analyze", "--repos-csv", csvPath, "--instance-json", missingInstancePath, "--audit-html", htmlPath})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("expected HTML output file to exist: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty HTML output")
	}
}

func TestSplitList(t *testing.T) {
	cases := map[string][]string{
		"":         nil,
		"a":        {"a"},
		"a,b":      {"a", "b"},
		"a, b ,,c": {"a", "b", "c"},
	}
	for in, want := range cases {
		got := splitList(in)
		if len(got) != len(want) {
			t.Fatalf("splitList(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("splitList(%q) = %v, want %v", in, got, want)
			}
		}
	}
}
