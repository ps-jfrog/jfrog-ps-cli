package repos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func writeCSV(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "virtual_repos.csv")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	return path
}

func TestReadVirtualRepoSpecsCSV_Valid(t *testing.T) {
	path := writeCSV(t, "new-virtual-repo,remote-name,local-name\nmy-virtual,my-remote,my-local\n")
	specs, err := ReadVirtualRepoSpecsCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	want := VirtualRepoSpec{VirtualRepo: "my-virtual", RemoteRepo: "my-remote", LocalRepo: "my-local"}
	if specs[0] != want {
		t.Fatalf("unexpected spec: %+v, want %+v", specs[0], want)
	}
}

func TestReadVirtualRepoSpecsCSV_MissingColumn(t *testing.T) {
	path := writeCSV(t, "new-virtual-repo,remote-name\nmy-virtual,my-remote\n")
	if _, err := ReadVirtualRepoSpecsCSV(path); err == nil {
		t.Fatal("expected an error for a CSV missing the local-name column")
	}
}

func TestReadVirtualRepoSpecsCSV_NoDataRows(t *testing.T) {
	path := writeCSV(t, "new-virtual-repo,remote-name,local-name\n")
	if _, err := ReadVirtualRepoSpecsCSV(path); err == nil {
		t.Fatal("expected an error for a CSV with no data rows")
	}
}

func TestPlanVirtualRepos_MissingMemberIsError(t *testing.T) {
	srv := httptest.NewServer(reposHandler([]Repo{
		{Key: "my-local", Type: "local", PackageType: "npm"},
	}))
	defer srv.Close()

	plans, err := PlanVirtualRepos(testClient(t, srv), []VirtualRepoSpec{
		{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 || plans[0].Action != VirtualActionError {
		t.Fatalf("expected an error plan, got %+v", plans)
	}
}

func TestPlanVirtualRepos_PackageTypeMismatchIsError(t *testing.T) {
	srv := httptest.NewServer(reposHandler([]Repo{
		{Key: "my-local", Type: "local", PackageType: "npm"},
		{Key: "my-remote", Type: "remote", PackageType: "maven"},
	}))
	defer srv.Close()

	plans, err := PlanVirtualRepos(testClient(t, srv), []VirtualRepoSpec{
		{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 || plans[0].Action != VirtualActionError {
		t.Fatalf("expected a package-type-mismatch error plan, got %+v", plans)
	}
}

func TestPlanVirtualRepos_CreateWhenVirtualMissing(t *testing.T) {
	srv := httptest.NewServer(reposHandler([]Repo{
		{Key: "my-local", Type: "local", PackageType: "npm"},
		{Key: "my-remote", Type: "remote", PackageType: "npm"},
	}))
	defer srv.Close()

	plans, err := PlanVirtualRepos(testClient(t, srv), []VirtualRepoSpec{
		{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 || plans[0].Action != VirtualActionCreate || plans[0].PackageType != "npm" {
		t.Fatalf("expected a create plan with packageType npm, got %+v", plans)
	}
}

func TestPlanVirtualRepos_NoChangeWhenMembersAlreadyMatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Repo{
			{Key: "my-local", Type: "local", PackageType: "npm"},
			{Key: "my-remote", Type: "remote", PackageType: "npm"},
			{Key: "my-virtual", Type: "virtual", PackageType: "npm"},
		})
	})
	mux.HandleFunc("/artifactory/api/repositories/my-virtual", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"key":          "my-virtual",
			"repositories": []string{"my-remote", "my-local"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	plans, err := PlanVirtualRepos(testClient(t, srv), []VirtualRepoSpec{
		{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 || plans[0].Action != VirtualActionNoChange {
		t.Fatalf("expected a no-change plan, got %+v", plans)
	}
}

func TestPlanVirtualRepos_UpdateWhenMembersDiffer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Repo{
			{Key: "my-local", Type: "local", PackageType: "npm"},
			{Key: "my-remote", Type: "remote", PackageType: "npm"},
			{Key: "my-virtual", Type: "virtual", PackageType: "npm"},
		})
	})
	mux.HandleFunc("/artifactory/api/repositories/my-virtual", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"key":          "my-virtual",
			"repositories": []string{"some-other-repo"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	plans, err := PlanVirtualRepos(testClient(t, srv), []VirtualRepoSpec{
		{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans) != 1 || plans[0].Action != VirtualActionUpdate {
		t.Fatalf("expected an update plan, got %+v", plans)
	}
}

func TestApplyVirtualRepos_DryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not issue any call, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	plans := []VirtualRepoPlan{{Spec: VirtualRepoSpec{VirtualRepo: "v"}, Action: VirtualActionCreate}}
	if results := ApplyVirtualRepos(testClient(t, srv), plans, true); results != nil {
		t.Fatalf("expected nil results for dry-run, got %+v", results)
	}
}

func TestApplyVirtualRepos_CreatesForReal(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var putMethod, putPath string
	var putBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		putMethod = r.Method
		putPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&putBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	plans := []VirtualRepoPlan{{
		Spec:        VirtualRepoSpec{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
		PackageType: "npm",
		Action:      VirtualActionCreate,
	}}
	results := ApplyVirtualRepos(testClient(t, srv), plans, false)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
	if putMethod != http.MethodPut || putPath != "/artifactory/api/repositories/my-virtual" {
		t.Fatalf("unexpected create request: %s %s", putMethod, putPath)
	}
	if putBody["packageType"] != "npm" {
		t.Fatalf("unexpected create body: %+v", putBody)
	}
}

func TestApplyVirtualRepos_UpdatesForReal(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	plans := []VirtualRepoPlan{{
		Spec:   VirtualRepoSpec{VirtualRepo: "my-virtual", LocalRepo: "my-local", RemoteRepo: "my-remote"},
		Action: VirtualActionUpdate,
	}}
	results := ApplyVirtualRepos(testClient(t, srv), plans, false)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
	if method != http.MethodPost {
		t.Fatalf("expected POST for update, got %s", method)
	}
}

func TestApplyVirtualRepos_SkipsNoChangeAndError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("expected no HTTP calls for no-change/error plans, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	plans := []VirtualRepoPlan{
		{Spec: VirtualRepoSpec{VirtualRepo: "a"}, Action: VirtualActionNoChange},
		{Spec: VirtualRepoSpec{VirtualRepo: "b"}, Action: VirtualActionError},
	}
	if results := ApplyVirtualRepos(testClient(t, srv), plans, false); len(results) != 0 {
		t.Fatalf("expected no results, got %+v", results)
	}
}
