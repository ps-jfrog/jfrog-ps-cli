package xray

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

// ---------------------------------------------------------------------------
// Repo-deployer segregation
// ---------------------------------------------------------------------------

func TestSegregateByDeployerHappyPath(t *testing.T) {
	const csvBody = `repository,impacted_artifact,severity
repo-a,path/one.jar,High
repo-a,path/one.jar,Critical
repo-a,path/two.jar,Low
`
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v2/watches/w1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"repository","name":"repo-a"}]}}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/violations", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"report_id": 1}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed","name":"x"}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/export/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipCSV(t, "x", csvBody))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/path/one.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"createdBy":"alice"}`))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/path/two.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"createdBy":"bob"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	result, err := SegregateByDeployer(client, DeployerSegregationOptions{
		Watchname:  "w1",
		ReportType: ReportTypeViolations,
		OutputDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 grouped rows, got %d: %+v", len(result.Rows), result.Rows)
	}
	byDeployer := map[string]DeployerSegregationRow{}
	for _, r := range result.Rows {
		byDeployer[r.Deployer] = r
	}
	if byDeployer["alice"].ViolationCount != 2 {
		t.Fatalf("expected alice to have violation_count=2, got %+v", byDeployer["alice"])
	}
	if byDeployer["bob"].ViolationCount != 1 {
		t.Fatalf("expected bob to have violation_count=1, got %+v", byDeployer["bob"])
	}
	if result.SegregatedCSVPath == "" {
		t.Fatalf("expected a segregated CSV path to be recorded")
	}
}

func TestSegregateByDeployerRejectsBuildWatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xray/api/v2/watches/w1":
			_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"build","name":"b1"}]}}`))
		case "/artifactory/api/storage/artifactory-build-info/b1":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected report-generation call for a rejected watch: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	if _, err := SegregateByDeployer(client, DeployerSegregationOptions{Watchname: "w1", ReportType: ReportTypeViolations}); err == nil {
		t.Fatalf("expected an error for a build-scoped watch")
	}
}

func TestSegregateByDeployerChecksumLookup(t *testing.T) {
	const csvBody = `impacted_artifact,severity
sha256:deadbeef,High
`
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v2/watches/w1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"repository","name":"repo-a"}]}}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/violations", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"report_id": 2}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed","name":"x"}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/export/2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipCSV(t, "x", csvBody))
	})
	mux.HandleFunc("/artifactory/api/search/checksum", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sha256"); got != "deadbeef" {
			t.Fatalf("expected sha256=deadbeef, got %q", got)
		}
		_, _ = w.Write([]byte(`{"results":[{"uri":"https://x.jfrog.io/artifactory/api/storage/repo-a/checksum/art.jar"}]}`))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/checksum/art.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"createdBy":"carol"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	result, err := SegregateByDeployer(client, DeployerSegregationOptions{Watchname: "w1", ReportType: ReportTypeViolations, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0].Deployer != "carol" {
		t.Fatalf("expected a single row attributed to carol, got %+v", result.Rows)
	}
}

func TestSegregateByDeployerLookupErrorOn404(t *testing.T) {
	const csvBody = `repository,impacted_artifact,severity
repo-a,missing/art.jar,High
`
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v2/watches/w1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"repository","name":"repo-a"}]}}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/violations", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"report_id": 3}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/3", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed","name":"x"}`))
	})
	mux.HandleFunc("/xray/api/v1/reports/export/3", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipCSV(t, "x", csvBody))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/missing/art.jar", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	result, err := SegregateByDeployer(client, DeployerSegregationOptions{Watchname: "w1", ReportType: ReportTypeViolations, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0].LookupError != "404_not_found" || result.Rows[0].Deployer != "UNKNOWN" {
		t.Fatalf("expected a 404_not_found lookup error with deployer=UNKNOWN, got %+v", result.Rows)
	}
}

// ---------------------------------------------------------------------------
// Build-gitrepo segregation
// ---------------------------------------------------------------------------

func buildGitRepoResources(names []string) reportResources {
	bwr := make([]buildWithRepo, len(names))
	for i, n := range names {
		bwr[i] = buildWithRepo{Build: n, Repo: "artifactory-build-info"}
	}
	return reportResources{Builds: &buildsResource{Names: names, NumberOfLatestVersions: 5, BuildsWithRepos: bwr}}
}

func TestSegregateByBuildGitRepoHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/search/checksum", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"uri":"https://x.jfrog.io/artifactory/api/storage/repo-a/path/art.jar"}]}`))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/path/art.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"createdBy":"dave","checksums":{"sha256":"deadbeef"}}`))
	})
	mux.HandleFunc("/artifactory/api/build/build1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"buildsNumbers":[{"uri":"/5","started":"2024-01-05T00:00:00.000Z"}]}`))
	})
	mux.HandleFunc("/artifactory/api/build/build1/5", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"buildInfo":{"modules":[{"id":"build1:5","artifacts":[{"name":"art.jar","sha256":"deadbeef"}]}],"vcs":[{"url":"https://github.com/org/repo.git","branch":"main","revision":"abc123"}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	records := [][]string{
		{"impacted_artifact", "severity"},
		{"sha256:deadbeef", "High"},
	}
	rows, ran, err := SegregateByBuildGitRepo(client, buildGitRepoResources([]string{"build1"}), "w1", records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Fatalf("expected segregation to run for a build-scoped report")
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Deployer != "dave" || r.BuildName != "build1" || r.GitURL != "https://github.com/org/repo.git" || r.VCSBranch != "main" || r.VCSRevision != "abc123" {
		t.Fatalf("unexpected row: %+v", r)
	}
	if r.LookupError != "" {
		t.Fatalf("expected no lookup error, got %q", r.LookupError)
	}
}

func TestSegregateByBuildGitRepoSkippedWhenNotBuildScoped(t *testing.T) {
	client := jfclient.New("http://example.invalid", "tok", "", "", false)
	records := [][]string{{"impacted_artifact"}, {"repo-a/path.jar"}}
	rows, ran, err := SegregateByBuildGitRepo(client, reportResources{Repositories: []repositoryResource{{Name: "repo-a"}}}, "w1", records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ran || rows != nil {
		t.Fatalf("expected a no-op for a non-build-scoped report, got ran=%v rows=%v", ran, rows)
	}
}

func TestSegregateByBuildGitRepoChecksumUnresolved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/artifactory/api/search/checksum" {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		t.Fatalf("unexpected call: %s", r.URL.Path)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	records := [][]string{
		{"impacted_artifact"},
		{"sha256:notfound"},
	}
	rows, ran, err := SegregateByBuildGitRepo(client, buildGitRepoResources([]string{"build1"}), "w1", records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran || len(rows) != 1 || rows[0].LookupError != "checksum_unresolved" {
		t.Fatalf("expected a checksum_unresolved row, got ran=%v rows=%+v", ran, rows)
	}
}

func TestSegregateByBuildGitRepoBuildVCSMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/search/checksum", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"uri":"https://x.jfrog.io/artifactory/api/storage/repo-a/path/art.jar"}]}`))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/path/art.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"createdBy":"dave","checksums":{"sha256":"deadbeef"}}`))
	})
	mux.HandleFunc("/artifactory/api/build/build1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"buildsNumbers":[{"uri":"/5","started":"2024-01-05T00:00:00.000Z"}]}`))
	})
	mux.HandleFunc("/artifactory/api/build/build1/5", func(w http.ResponseWriter, r *http.Request) {
		// Build info with a matching checksum, but no VCS data at all.
		_, _ = w.Write([]byte(`{"buildInfo":{"modules":[{"id":"build1:5","artifacts":[{"name":"art.jar","sha256":"deadbeef"}]}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	records := [][]string{
		{"impacted_artifact"},
		{"sha256:deadbeef"},
	}
	rows, ran, err := SegregateByBuildGitRepo(client, buildGitRepoResources([]string{"build1"}), "w1", records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran || len(rows) != 1 {
		t.Fatalf("expected 1 row, got ran=%v rows=%+v", ran, rows)
	}
	if rows[0].BuildName != "build1" || rows[0].LookupError != "build_vcs_missing" {
		t.Fatalf("expected build_vcs_missing with build resolved, got %+v", rows[0])
	}
}

func TestSegregateByBuildGitRepoArtifactUnmatchedToAnyBuild(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/search/checksum", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"uri":"https://x.jfrog.io/artifactory/api/storage/repo-a/path/art.jar"}]}`))
	})
	mux.HandleFunc("/artifactory/api/storage/repo-a/path/art.jar", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"createdBy":"dave","checksums":{"sha256":"unrelated"}}`))
	})
	mux.HandleFunc("/artifactory/api/build/build1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"buildsNumbers":[{"uri":"/5","started":"2024-01-05T00:00:00.000Z"}]}`))
	})
	mux.HandleFunc("/artifactory/api/build/build1/5", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"buildInfo":{"modules":[{"id":"build1:5","artifacts":[{"name":"other.jar","sha256":"deadbeef"}]}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	records := [][]string{
		{"impacted_artifact"},
		{"sha256:deadbeef"},
	}
	rows, ran, err := SegregateByBuildGitRepo(client, buildGitRepoResources([]string{"build1"}), "w1", records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran || len(rows) != 1 || rows[0].LookupError != "build_artifact_unmatched" || rows[0].BuildName != "UNKNOWN" {
		t.Fatalf("expected build_artifact_unmatched with BuildName=UNKNOWN, got ran=%v rows=%+v", ran, rows)
	}
}

// ---------------------------------------------------------------------------
// CSV grouping helpers
// ---------------------------------------------------------------------------

func TestGroupRowsDeduplicatesAndCountsSeverities(t *testing.T) {
	records := [][]string{
		{"repository", "impacted_artifact", "severity"},
		{"repo-a", "path/one.jar", "High"},
		{"repo-a", "path/one.jar", "High"},
		{"repo-a", "path/one.jar", "Critical"},
	}
	groups, ok := groupRows(records)
	if !ok {
		t.Fatalf("expected groupRows to find the impacted_artifact column")
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 unique group, got %d", len(groups))
	}
	if groups[0].Count != 3 {
		t.Fatalf("expected count=3, got %d", groups[0].Count)
	}
	if len(groups[0].Severities) != 2 {
		t.Fatalf("expected 2 distinct severities, got %v", groups[0].Severities)
	}
}

func TestGroupRowsNoImpactedArtifactColumn(t *testing.T) {
	records := [][]string{{"foo", "bar"}, {"1", "2"}}
	if _, ok := groupRows(records); ok {
		t.Fatalf("expected groupRows to fail when there is no impacted-artifact-like column")
	}
}
