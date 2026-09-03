package cleanupcmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

// captureStdout redirects os.Stdout for the duration of f and returns
// whatever was written to it.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return string(data)
}

// withStdin redirects os.Stdin to feed input for the duration of f.
func withStdin(t *testing.T, input string, f func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	done := make(chan struct{})
	go func() {
		_, _ = w.WriteString(input)
		w.Close()
		close(done)
	}()
	f()
	os.Stdin = old
	<-done
}

func TestRunNoArgs(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected exit code 1 for no args, got %d", code)
	}
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		if code := Run(args); code != 0 {
			t.Fatalf("expected exit code 0 for %v, got %d", args, code)
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	if code := Run([]string{"does-not-exist"}); code != 1 {
		t.Fatalf("expected exit code 1 for unknown subcommand, got %d", code)
	}
}

// --- artifacts ---------------------------------------------------------

func TestArtifactsRequiresExactlyOneAQLSource(t *testing.T) {
	if code := Run([]string{"artifacts", "--url", "http://example.invalid", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit 1 when neither --aql nor --aql-file given, got %d", code)
	}
	if code := Run([]string{"artifacts", "--url", "http://example.invalid", "--token", "t", "--aql", "x", "--aql-file", "f"}); code != 1 {
		t.Fatalf("expected exit 1 when both --aql and --aql-file given, got %d", code)
	}
}

func TestArtifactsMissingServer(t *testing.T) {
	if code := Run([]string{"artifacts", "--aql", "items.find({})"}); code != 1 {
		t.Fatalf("expected exit 1 without server credentials, got %d", code)
	}
}

func artifactsAQLDeleteServer(t *testing.T, deleteCalled *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[{"repo":"my-repo","path":"a","name":"f1.txt","size":10}]}`))
		case r.Method == http.MethodDelete:
			*deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestArtifactsDryRunDefaultNeverDeletes(t *testing.T) {
	deleteCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dry-run is true (default)")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"repo":"my-repo","path":"a","name":"f1.txt","size":10}]}`))
	}))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"artifacts", "--url", srv.URL, "--token", "tok", "--aql", `items.find({"repo":"my-repo"})`})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner in output, got %q", out)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called")
	}
}

func TestArtifactsAQLFileFlag(t *testing.T) {
	deleteCalled := false
	srv := artifactsAQLDeleteServer(t, &deleteCalled)
	defer srv.Close()

	dir := t.TempDir()
	aqlPath := filepath.Join(dir, "query.aql")
	if err := os.WriteFile(aqlPath, []byte(`items.find({"repo":"my-repo"})`), 0o600); err != nil {
		t.Fatalf("write aql file: %v", err)
	}

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"artifacts", "--url", srv.URL, "--token", "tok", "--aql-file", aqlPath})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called in dry-run")
	}
}

func TestArtifactsRealDeleteWhenDryRunFalse(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleteCalled := false
	srv := artifactsAQLDeleteServer(t, &deleteCalled)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"artifacts", "--url", srv.URL, "--token", "tok", "--aql", `items.find({"repo":"my-repo"})`, "--dry-run=false"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE to be called when --dry-run=false")
	}
}

func TestArtifactsCSVOutput(t *testing.T) {
	deleteCalled := false
	srv := artifactsAQLDeleteServer(t, &deleteCalled)
	defer srv.Close()

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "out.csv")

	var code int
	captureStdout(t, func() {
		code = Run([]string{"artifacts", "--url", srv.URL, "--token", "tok", "--aql", `items.find({"repo":"my-repo"})`, "--csv", csvPath})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("expected csv file to be written: %v", err)
	}
	if !strings.Contains(string(data), "f1.txt") {
		t.Fatalf("expected csv to contain match, got %q", data)
	}
}

func TestArtifactsBuilderFlagsRequireBoth(t *testing.T) {
	if code := Run([]string{"artifacts", "--url", "http://example.invalid", "--token", "t", "--repo", "my-repo"}); code != 1 {
		t.Fatalf("expected exit 1 with --repo but no --not-downloaded-since, got %d", code)
	}
	if code := Run([]string{"artifacts", "--url", "http://example.invalid", "--token", "t", "--not-downloaded-since", "3mo"}); code != 1 {
		t.Fatalf("expected exit 1 with --not-downloaded-since but no --repo, got %d", code)
	}
}

func TestArtifactsBuilderFlagsCannotCombineWithAQL(t *testing.T) {
	code := Run([]string{
		"artifacts", "--url", "http://example.invalid", "--token", "t",
		"--repo", "my-repo", "--not-downloaded-since", "3mo", "--aql", "items.find({})",
	})
	if code != 1 {
		t.Fatalf("expected exit 1 when combining builder flags with --aql, got %d", code)
	}
}

func TestArtifactsRepoNotDownloadedSinceBuildsQuery(t *testing.T) {
	deleteCalled := false
	srv := artifactsAQLDeleteServer(t, &deleteCalled)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"artifacts", "--url", srv.URL, "--token", "tok", "--repo", "my-repo", "--not-downloaded-since", "3mo"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "totaling") {
		t.Fatalf("expected size summary in output, got %q", out)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called in dry-run")
	}
}

// --- docker-tags -----------------------------------------------------------

func dockerTagsServer(t *testing.T, deleteCalled *bool, deletedPaths *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			if strings.Contains(string(body), "list.manifest.json") {
				_, _ = w.Write([]byte(`{"results":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"results":[{"repo":"docker-local","path":"my-image/1.0","name":"manifest.json","size":300}]}`))
		case r.Method == http.MethodDelete:
			*deleteCalled = true
			*deletedPaths = append(*deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestDockerTagsRequiresRepoAndOlderThan(t *testing.T) {
	if code := Run([]string{"docker-tags", "--url", "http://example.invalid", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit 1 without --repo/--older-than, got %d", code)
	}
}

func TestDockerTagsDryRunDefaultNeverDeletes(t *testing.T) {
	deleteCalled := false
	var deletedPaths []string
	srv := dockerTagsServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"docker-tags", "--url", srv.URL, "--token", "tok", "--repo", "docker-local", "--older-than", "3mo"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner, got %q", out)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called in dry-run")
	}
}

func TestDockerTagsYesFlagDeletesFullTagDirectory(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleteCalled := false
	var deletedPaths []string
	srv := dockerTagsServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"docker-tags", "--url", srv.URL, "--token", "tok", "--repo", "docker-local", "--older-than", "3mo", "--dry-run=false", "--yes"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE to be called with --yes")
	}
	if len(deletedPaths) != 1 || deletedPaths[0] != "/artifactory/docker-local/my-image/1.0/" {
		t.Fatalf("expected delete of the full tag directory, got %v", deletedPaths)
	}
}

// --- remote-cache ------------------------------------------------------------

func remoteCacheServer(t *testing.T, deleteCalled *bool, deletedPaths *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/artifactory/api/repositories/npm-remote":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"packageType":"npm","type":"remote"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/artifactory/api/search/aql":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[
				{"repo":"npm-remote-cache","path":"pkg","name":"stale-indexed.tgz","size":100},
				{"repo":"npm-remote-cache","path":"pkg","name":"stale-expired.tgz","size":200}
			]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/xray/api/v1/summary/artifact":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"artifacts":[{"general":{"path":"npm-remote-cache/pkg/stale-indexed.tgz"}}],"errors":[]}`))
		case r.Method == http.MethodDelete:
			*deleteCalled = true
			*deletedPaths = append(*deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRemoteCacheRequiresRepo(t *testing.T) {
	if code := Run([]string{"remote-cache", "--url", "http://example.invalid", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit 1 without --repo, got %d", code)
	}
}

func TestRemoteCacheDryRunOnlyKeepsXrayExpiredCandidates(t *testing.T) {
	deleteCalled := false
	var deletedPaths []string
	srv := remoteCacheServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"remote-cache", "--url", srv.URL, "--token", "tok", "--repo", "npm-remote"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner, got %q", out)
	}
	if !strings.Contains(out, "stale-expired.tgz") {
		t.Fatalf("expected the not-indexed candidate in output, got %q", out)
	}
	if strings.Contains(out, "stale-indexed.tgz") {
		t.Fatalf("still-indexed candidate must be excluded from cleanup, got %q", out)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called in dry-run")
	}
}

func TestRemoteCacheYesFlagDeletesOnlyExpiredCandidate(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleteCalled := false
	var deletedPaths []string
	srv := remoteCacheServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"remote-cache", "--url", srv.URL, "--token", "tok", "--repo", "npm-remote", "--dry-run=false", "--yes"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE to be called with --yes")
	}
	if len(deletedPaths) != 1 || deletedPaths[0] != "/artifactory/npm-remote-cache/pkg/stale-expired.tgz" {
		t.Fatalf("expected exactly one delete of the not-indexed candidate, got %v", deletedPaths)
	}
}

// --- repos ---------------------------------------------------------------

func writeRepoFile(t *testing.T, repos ...string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "repos.txt")
	if err := os.WriteFile(p, []byte(strings.Join(repos, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	return p
}

func TestReposRequiresFile(t *testing.T) {
	if code := Run([]string{"repos", "--url", "http://example.invalid", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit 1 without --file, got %d", code)
	}
}

func TestReposDryRunDefaultNeverDeletes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dry-run is true (default)")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	file := writeRepoFile(t, "repo-a", "repo-b")
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"repos", "--url", srv.URL, "--token", "tok", "--file", file})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner, got %q", out)
	}
}

func TestReposConfirmDeclinedAbortsWithoutDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when confirmation is declined")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	file := writeRepoFile(t, "repo-a")
	var code int
	withStdin(t, "n\n", func() {
		captureStdout(t, func() {
			code = Run([]string{"repos", "--url", srv.URL, "--token", "tok", "--file", file, "--dry-run=false"})
		})
	})
	if code != 1 {
		t.Fatalf("expected exit 1 when confirmation declined, got %d", code)
	}
}

func TestReposYesFlagDeletesForReal(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleteCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	file := writeRepoFile(t, "repo-a")
	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"repos", "--url", srv.URL, "--token", "tok", "--file", file, "--dry-run=false", "--yes"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE to be called with --yes")
	}
}

// --- inactive-users --------------------------------------------------------

func TestInactiveUsersDryRunDefaultNeverDeletes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dry-run is true (default)")
		}
		if r.URL.Path == "/artifactory/api/security/users" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"name":"alice","realm":"ldap","lastLoggedIn":0}]`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"inactive-users", "--url", srv.URL, "--token", "tok", "--months", "3"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner, got %q", out)
	}
	if !strings.Contains(out, "alice") {
		t.Fatalf("expected candidate alice in output, got %q", out)
	}
}

func TestInactiveUsersYesFlagDeletesForReal(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleteCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/artifactory/api/security/users":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"name":"alice","realm":"ldap","lastLoggedIn":0}]`))
		case r.Method == http.MethodDelete:
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"inactive-users", "--url", srv.URL, "--token", "tok", "--months", "3", "--dry-run=false", "--yes"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE to be called with --yes")
	}
}

func TestInactiveUsersExcludesInternalRealmByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called in dry-run")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"name":"admin","realm":"internal","lastLoggedIn":0}]`))
	}))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"inactive-users", "--url", srv.URL, "--token", "tok", "--months", "3"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.Contains(out, "admin") {
		t.Fatalf("expected internal-realm user to be excluded by default, got %q", out)
	}
}

// --- diff-delete-artifacts --------------------------------------------------

func diffServers(t *testing.T, targetDeleteCalled *bool) (source, target *httptest.Server) {
	t.Helper()
	source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"repo":"my-repo","path":"a","name":"shared.txt","sha256":"s1"}]}`))
	}))
	target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			*targetDeleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/artifactory/api/search/aql":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"results":[
				{"repo":"my-repo","path":"a","name":"shared.txt","sha256":"s1"},
				{"repo":"my-repo","path":"b","name":"extra.txt","sha256":"s2"}
			]}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	return source, target
}

func TestDiffDeleteArtifactsRequiresRepo(t *testing.T) {
	if code := Run([]string{"diff-delete-artifacts", "--source-url", "http://a.invalid", "--source-token", "t", "--target-url", "http://b.invalid", "--target-token", "t"}); code != 1 {
		t.Fatalf("expected exit 1 without --repo, got %d", code)
	}
}

func TestDiffDeleteArtifactsDryRunDefaultNeverDeletes(t *testing.T) {
	var targetDeleteCalled bool
	source, target := diffServers(t, &targetDeleteCalled)
	defer source.Close()
	defer target.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"diff-delete-artifacts",
			"--source-url", source.URL, "--source-token", "tok",
			"--target-url", target.URL, "--target-token", "tok",
			"--repo", "my-repo",
		})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner, got %q", out)
	}
	if !strings.Contains(out, "extra.txt") {
		t.Fatalf("expected target-only candidate in output, got %q", out)
	}
	if targetDeleteCalled {
		t.Fatalf("expected DELETE to never be called in dry-run")
	}
}

func TestDiffDeleteArtifactsYesFlagDeletesForReal(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var targetDeleteCalled bool
	source, target := diffServers(t, &targetDeleteCalled)
	defer source.Close()
	defer target.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"diff-delete-artifacts",
			"--source-url", source.URL, "--source-token", "tok",
			"--target-url", target.URL, "--target-token", "tok",
			"--repo", "my-repo",
			"--dry-run=false", "--yes",
		})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !targetDeleteCalled {
		t.Fatalf("expected DELETE to be called against target with --yes")
	}
}

// --- empty-folders ---------------------------------------------------------

// emptyFoldersServer serves a fixed fake tree for repo "my-repo": root has
// an empty "emptydir" folder and a "keep" folder containing a file, so the
// only empty-folder candidate is "emptydir".
func emptyFoldersServer(t *testing.T, deleteCalled *bool, deletedPaths *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/storage/my-repo":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"children":[{"uri":"/emptydir","folder":true},{"uri":"/keep","folder":true}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/storage/my-repo/emptydir":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"children":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/storage/my-repo/keep":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"children":[{"uri":"/file.txt","folder":false}]}`))
		case r.Method == http.MethodDelete:
			*deleteCalled = true
			*deletedPaths = append(*deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestEmptyFoldersRequiresRepo(t *testing.T) {
	if code := Run([]string{"empty-folders", "--url", "http://example.invalid", "--token", "t"}); code != 1 {
		t.Fatalf("expected exit 1 without --repo, got %d", code)
	}
}

func TestEmptyFoldersDryRunDefaultNeverDeletes(t *testing.T) {
	deleteCalled := false
	var deletedPaths []string
	srv := emptyFoldersServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"empty-folders", "--url", srv.URL, "--token", "tok", "--repo", "my-repo"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !strings.Contains(out, "[DRY RUN]") {
		t.Fatalf("expected dry-run banner, got %q", out)
	}
	if !strings.Contains(out, "emptydir") {
		t.Fatalf("expected emptydir to be listed as a candidate, got %q", out)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called when dry-run is true (default)")
	}
}

func TestEmptyFoldersYesFlagDeletesForReal(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	deleteCalled := false
	var deletedPaths []string
	srv := emptyFoldersServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"empty-folders", "--url", srv.URL, "--token", "tok", "--repo", "my-repo", "--dry-run=false", "--yes"})
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output=%s", code, out)
	}
	if !deleteCalled {
		t.Fatalf("expected DELETE to be called with --yes")
	}
	if len(deletedPaths) != 1 || deletedPaths[0] != "/artifactory/my-repo/emptydir" {
		t.Fatalf("expected exactly one delete of /artifactory/my-repo/emptydir, got %v", deletedPaths)
	}
}

func TestEmptyFoldersConfirmDeclinedAbortsWithoutDelete(t *testing.T) {
	deleteCalled := false
	var deletedPaths []string
	srv := emptyFoldersServer(t, &deleteCalled, &deletedPaths)
	defer srv.Close()

	var code int
	withStdin(t, "n\n", func() {
		captureStdout(t, func() {
			code = Run([]string{"empty-folders", "--url", srv.URL, "--token", "tok", "--repo", "my-repo", "--dry-run=false"})
		})
	})
	if code != 1 {
		t.Fatalf("expected exit 1 when confirmation declined, got %d", code)
	}
	if deleteCalled {
		t.Fatalf("expected DELETE to never be called when confirmation is declined")
	}
}
