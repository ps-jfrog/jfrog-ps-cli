package compare

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

// aqlServer returns an httptest server that responds to
// POST /artifactory/api/search/aql with the given items as an AQL results
// payload, and fails the test if any mutating (non-GET/POST-to-AQL) request
// is received.
func aqlServer(t *testing.T, items []RepoItem) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/artifactory/api/search/aql" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		body := `{"results":[`
		for i, it := range items {
			if i > 0 {
				body += ","
			}
			body += fmt.Sprintf(`{"path":%q,"name":%q,"sha256":%q}`, it.Path, it.Name, it.SHA256)
		}
		body += `]}`
		_, _ = w.Write([]byte(body))
	}))
}

func TestShouldSkipItem(t *testing.T) {
	cases := []struct {
		path, name string
		want       bool
	}{
		{"a/b", "maven-metadata.xml", true},
		{"a/b", "Packages", true},
		{"a/b", "repomd.xml", true},
		{"a/.npm/pkg", "foo.tgz", true},
		{"a/.pypi/pkg", "foo.whl", true},
		{"a/.composer", "packages.json", true},
		{"a/b", "index.yaml", true},
		{"a/b", "some-index.yaml", true},
		{"a/b", "myfile.jar", false},
		{"a/b", "index.yamlx", false},
	}
	for _, c := range cases {
		got := shouldSkipItem(c.path, c.name)
		if got != c.want {
			t.Errorf("shouldSkipItem(%q, %q) = %v, want %v", c.path, c.name, got, c.want)
		}
	}
}

func TestFetchRepoItemsFiltersGeneratedFiles(t *testing.T) {
	srv := aqlServer(t, []RepoItem{
		{Path: "com/foo", Name: "foo-1.0.jar", SHA256: "abc"},
		{Path: "com/foo", Name: "maven-metadata.xml", SHA256: "def"},
	})
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	items, err := FetchRepoItems(client, "my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].Name != "foo-1.0.jar" {
		t.Fatalf("expected only foo-1.0.jar to remain, got %+v", items)
	}
}

func TestCompareRepoMissingAndDifferent(t *testing.T) {
	sourceItems := []RepoItem{
		{Path: "com/foo", Name: "foo-1.0.jar", SHA256: "sha-same"},
		{Path: "com/foo", Name: "foo-2.0.jar", SHA256: "sha-changed-source"},
		{Path: "com/foo", Name: "foo-3.0.jar", SHA256: "sha-missing"},
	}
	targetItems := []RepoItem{
		{Path: "com/foo", Name: "foo-1.0.jar", SHA256: "sha-same"},
		{Path: "com/foo", Name: "foo-2.0.jar", SHA256: "sha-changed-target"},
		{Path: "com/foo", Name: "foo-4.0.jar", SHA256: "target-only"},
	}

	sourceSrv := aqlServer(t, sourceItems)
	defer sourceSrv.Close()
	targetSrv := aqlServer(t, targetItems)
	defer targetSrv.Close()

	source := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	target := jfclient.New(targetSrv.URL, "tok", "", "", false)

	result, err := CompareRepo(source, target, "my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != 3 || result.Target != 3 {
		t.Fatalf("expected 3/3 items, got source=%d target=%d", result.Source, result.Target)
	}
	if len(result.Diffs) != 2 {
		t.Fatalf("expected 2 diffs (missing + different), got %+v", result.Diffs)
	}

	var sawMissing, sawDifferent bool
	for _, d := range result.Diffs {
		switch d.Status {
		case StatusMissing:
			sawMissing = true
			if d.Name != "foo-3.0.jar" {
				t.Errorf("expected missing item to be foo-3.0.jar, got %s", d.Name)
			}
		case StatusDifferent:
			sawDifferent = true
			if d.Name != "foo-2.0.jar" {
				t.Errorf("expected different item to be foo-2.0.jar, got %s", d.Name)
			}
			if d.SourceSHA256 != "sha-changed-source" || d.TargetSHA256 != "sha-changed-target" {
				t.Errorf("unexpected shas: %+v", d)
			}
		}
		// target-only item ("foo-4.0.jar") must never appear.
		if d.Name == "foo-4.0.jar" {
			t.Errorf("target-only item must not be reported: %+v", d)
		}
	}
	if !sawMissing || !sawDifferent {
		t.Fatalf("expected both a missing and a different diff, got %+v", result.Diffs)
	}
}

func TestCompareRepoNoDifferences(t *testing.T) {
	items := []RepoItem{{Path: "a", Name: "b.jar", SHA256: "same"}}
	sourceSrv := aqlServer(t, items)
	defer sourceSrv.Close()
	targetSrv := aqlServer(t, items)
	defer targetSrv.Close()

	source := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	target := jfclient.New(targetSrv.URL, "tok", "", "", false)

	result, err := CompareRepo(source, target, "my-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Diffs) != 0 {
		t.Fatalf("expected no diffs, got %+v", result.Diffs)
	}
}

func TestListLocalRepos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/artifactory/api/repositories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("type") != "local" {
			t.Fatalf("expected type=local query param, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"key":"repo-a","type":"LOCAL"},{"key":"repo-b","type":"LOCAL"}]`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	keys, err := ListLocalRepos(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 || keys[0] != "repo-a" || keys[1] != "repo-b" {
		t.Fatalf("unexpected keys: %v", keys)
	}
}

func TestCompareRepoNeverMutates(t *testing.T) {
	// Any non-GET/POST-to-AQL request should fail the test -- repos
	// comparison must be strictly read-only.
	mutationGuard := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		t.Fatalf("unexpected mutating/unknown request: %s %s", r.Method, r.URL.Path)
	}
	sourceSrv := httptest.NewServer(http.HandlerFunc(mutationGuard))
	defer sourceSrv.Close()
	targetSrv := httptest.NewServer(http.HandlerFunc(mutationGuard))
	defer targetSrv.Close()

	source := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	target := jfclient.New(targetSrv.URL, "tok", "", "", false)

	if _, err := CompareRepo(source, target, "my-repo"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteRepoReports(t *testing.T) {
	dir := t.TempDir()
	results := []RepoDiffResult{
		{
			Repo:   "repo-a",
			Source: 2,
			Target: 1,
			Diffs: []ItemDiff{
				{Repo: "repo-a", Path: "com/foo", Name: "missing.jar", Status: StatusMissing, SourceSHA256: "abc"},
				{Repo: "repo-a", Path: ".", Name: "root.jar", Status: StatusDifferent, SourceSHA256: "111", TargetSHA256: "222"},
			},
		},
		{Repo: "repo-b", Source: 1, Target: 1},
	}

	if err := WriteRepoReports(dir, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repoAReport, err := os.ReadFile(filepath.Join(dir, "repo-a.txt"))
	if err != nil {
		t.Fatalf("expected repo-a.txt to exist: %v", err)
	}
	if len(repoAReport) == 0 {
		t.Fatalf("expected non-empty repo-a.txt")
	}

	repoBReport, err := os.ReadFile(filepath.Join(dir, "repo-b.txt"))
	if err != nil {
		t.Fatalf("expected repo-b.txt to exist: %v", err)
	}
	if len(repoBReport) == 0 {
		t.Fatalf("expected non-empty repo-b.txt")
	}

	plan, err := os.ReadFile(filepath.Join(dir, "transfer-plan.txt"))
	if err != nil {
		t.Fatalf("expected transfer-plan.txt to exist: %v", err)
	}
	planStr := string(plan)
	if !strings.Contains(planStr, "jf rt dl") || !strings.Contains(planStr, "jf rt u") {
		t.Fatalf("expected transfer plan to contain jf rt dl/u commands, got %q", planStr)
	}
	// Two diffs in repo-a, zero in repo-b => exactly two plan lines.
	if got := strings.Count(planStr, "\n"); got != 2 {
		t.Fatalf("expected 2 transfer plan lines, got %d: %q", got, planStr)
	}
}
