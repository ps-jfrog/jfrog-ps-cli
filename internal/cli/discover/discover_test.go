package discovercmd

import (
	"net/http"
	"net/http/httptest"
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

// emptyRepoServer mocks just enough of Artifactory for --repos and
// --policies to succeed with zero repos and zero policies: no AQL calls
// happen for an empty repo list, so this covers both pieces' network
// surface without a much heavier full-instance mock.
func emptyRepoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artifactory/api/storageinfo":
			w.Write([]byte(`{"repositoriesSummaryList":[]}`))
		case "/artifactory/api/cleanup/packages/policies":
			w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_ParallelRunsReposAndPoliciesConcurrently(t *testing.T) {
	srv := emptyRepoServer(t)
	dir := t.TempDir()

	code := Run([]string{
		"--repos", "--policies", "--url", srv.URL, "--token", "tok",
		"--repos-csv", filepath.Join(dir, "report.csv"),
		"--policies-csv", filepath.Join(dir, "policies.csv"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.csv")); err != nil {
		t.Errorf("expected report.csv to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "policies.csv")); err != nil {
		t.Errorf("expected policies.csv to exist: %v", err)
	}
}

func TestRun_ParallelFalseStillSucceeds(t *testing.T) {
	srv := emptyRepoServer(t)
	dir := t.TempDir()

	code := Run([]string{
		"--repos", "--policies", "--parallel=false", "--url", srv.URL, "--token", "tok",
		"--repos-csv", filepath.Join(dir, "report.csv"),
		"--policies-csv", filepath.Join(dir, "policies.csv"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.csv")); err != nil {
		t.Errorf("expected report.csv to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "policies.csv")); err != nil {
		t.Errorf("expected policies.csv to exist: %v", err)
	}
}

func TestRun_SinglePieceNeverRunsConcurrently(t *testing.T) {
	// Only one of --repos/--policies/--instance is selected, so --parallel's
	// default (true) must not change behavior or suppress the progress bar.
	srv := emptyRepoServer(t)
	dir := t.TempDir()

	code := Run([]string{
		"--repos", "--url", srv.URL, "--token", "tok",
		"--repos-csv", filepath.Join(dir, "report.csv"),
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.csv")); err != nil {
		t.Errorf("expected report.csv to exist: %v", err)
	}
}

// outputSubdir just delegates to instance.NameFromURL (exhaustively tested
// in internal/instance) so the --output directory naming and the Overview
// tab's displayed instance name always agree - this only checks the
// delegation itself.
func TestOutputSubdir_DelegatesToNameFromURL(t *testing.T) {
	if got := outputSubdir("https://psazuse.jfrog.io"); got != "psazuse" {
		t.Errorf("outputSubdir(...) = %q, want psazuse", got)
	}
	if got := outputSubdir(""); got != "default" {
		t.Errorf("outputSubdir(\"\") = %q, want default", got)
	}
}

func TestRun_AnalyzeDefaultsUnderOutputDir(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "report.csv")
	csvContent := "repo_name,package_type,repo_type,files_count,usage_gb,created_date,last_push_date,last_pull_date,last_activity_date,cleanup_policy_keys,cleanup_policy_enabled,cleanup_policy_cron_exps,cleanup_policy_created_before_days,cleanup_policy_created_before_months,cleanup_policy_last_downloaded_before_days,cleanup_policy_last_downloaded_before_months,cleanup_policy_keep_last_n_versions\n" +
		"docker-local,Docker,LOCAL,0,0,,,,,,,,,,,,\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	outputBase := filepath.Join(dir, "myoutput")
	// --repos-csv is explicit, but --audit-html is left at its default, so it
	// should land under <output>/<url-instance>/audit.html.
	code := Run([]string{
		"--analyze", "--repos-csv", csvPath,
		"--output", outputBase, "--url", "https://myinstance.jfrog.io",
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	wantHTML := filepath.Join(outputBase, "myinstance", "audit.html")
	if _, err := os.Stat(wantHTML); err != nil {
		t.Fatalf("expected audit HTML at %s: %v", wantHTML, err)
	}
}

func TestRun_AnalyzeDefaultOutputDirIsOutput(t *testing.T) {
	// A stray JFPS_URL in the shell (common after this project's live-testing
	// sessions) would otherwise change the subfolder name and break this test.
	if old, had := os.LookupEnv("JFPS_URL"); had {
		os.Unsetenv("JFPS_URL")
		defer os.Setenv("JFPS_URL", old)
	}

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "report.csv")
	csvContent := "repo_name,files_count\ndocker-local,0\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	// Run from a temp cwd so a bare --output default ("output") doesn't
	// touch this package's own source directory.
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origWD)

	code := Run([]string{"--analyze", "--repos-csv", csvPath})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	wantHTML := filepath.Join(dir, "output", "default", "audit.html")
	if _, err := os.Stat(wantHTML); err != nil {
		t.Fatalf("expected audit HTML at %s: %v", wantHTML, err)
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
