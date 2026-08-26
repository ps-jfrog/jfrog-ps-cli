package discovercmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRun_NoArgs(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected exit code 1 for no args, got %d", code)
	}
}

func TestRun_Help(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		if code := Run(args); code != 0 {
			t.Fatalf("expected exit code 0 for %v, got %d", args, code)
		}
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	if code := Run([]string{"bogus"}); code != 1 {
		t.Fatalf("expected exit code 1 for unknown subcommand, got %d", code)
	}
}

func TestRun_ReposMissingCredentials(t *testing.T) {
	if code := Run([]string{"repos", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_PoliciesMissingCredentials(t *testing.T) {
	if code := Run([]string{"policies", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_AnalyzeMissingInput(t *testing.T) {
	if code := Run([]string{"analyze"}); code != 1 {
		t.Fatalf("expected exit code 1 when --input is missing, got %d", code)
	}
}

func TestRun_AnalyzeHappyPath(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "report.csv")
	htmlPath := filepath.Join(dir, "audit.html")

	csvContent := "repo_name,package_type,repo_type,files_count,usage_gb,created_date,last_push_date,last_pull_date,last_activity_date,cleanup_policy_keys,cleanup_policy_enabled,cleanup_policy_cron_exps,cleanup_policy_created_before_days,cleanup_policy_created_before_months,cleanup_policy_last_downloaded_before_days,cleanup_policy_last_downloaded_before_months,cleanup_policy_keep_last_n_versions\n" +
		"docker-local,Docker,LOCAL,0,0,,,,,,,,,,,,\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	code := Run([]string{"analyze", "--input", csvPath, "--output", htmlPath})
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
