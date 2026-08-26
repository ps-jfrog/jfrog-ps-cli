package xraycmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever was written to it. Subcommands under test print directly via
// output.Table/DryRunBanner to os.Stdout, so tests need this to make
// assertions on CLI-visible output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestRunNoArgsReturnsError(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRunUnknownGroupReturnsError(t *testing.T) {
	if code := Run([]string{"bogus"}); code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestRunHelpReturnsZero(t *testing.T) {
	if code := Run([]string{"help"}); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestIndexUnknownActionReturnsError(t *testing.T) {
	if code := Run([]string{"index", "bogus"}); code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

func TestIndexStatusReadOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/binMgr/default/repos", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"indexed_repos":["repo-a"]}`))
	})
	mux.HandleFunc("/artifactory/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"key":"repo-a","type":"local"},{"key":"repo-b","type":"local"}]`))
	})
	srv := httptest.NewServer(guardNoMutations(t, mux))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"index", "status", "--type", "local", "--url", srv.URL, "--token", "tok"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if !strings.Contains(out, "repo-a") || !strings.Contains(out, "INDEXED") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "repo-b") || !strings.Contains(out, "NOT_INDEXED") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestIndexToggleRequiresExactlyOneOfEnableDisable(t *testing.T) {
	code := Run([]string{"index", "toggle", "--repo", "repo-a", "--url", "http://example.invalid", "--token", "tok"})
	if code != 1 {
		t.Fatalf("expected exit code 1 when neither --enable nor --disable given, got %d", code)
	}
	code = Run([]string{"index", "toggle", "--repo", "repo-a", "--enable", "--disable", "--url", "http://example.invalid", "--token", "tok"})
	if code != 1 {
		t.Fatalf("expected exit code 1 when both --enable and --disable given, got %d", code)
	}
}

func TestIndexToggleRequiresExactlyOneOfRepoAll(t *testing.T) {
	code := Run([]string{"index", "toggle", "--enable", "--url", "http://example.invalid", "--token", "tok"})
	if code != 1 {
		t.Fatalf("expected exit code 1 when neither --repo nor --all given, got %d", code)
	}
	code = Run([]string{"index", "toggle", "--repo", "repo-a", "--all", "--type", "local", "--enable", "--url", "http://example.invalid", "--token", "tok"})
	if code != 1 {
		t.Fatalf("expected exit code 1 when both --repo and --all given, got %d", code)
	}
}

func TestIndexToggleDryRunDefaultMakesNoMutatingCalls(t *testing.T) {
	srv := httptest.NewServer(guardNoMutations(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected GET call in single-repo toggle test: %s", r.URL.Path)
	})))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"index", "toggle", "--repo", "repo-a", "--enable", "--url", srv.URL, "--token", "tok"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if !strings.Contains(out, "DRY RUN") {
		t.Fatalf("expected dry-run banner, got: %s", out)
	}
}

func TestIndexReindexRequiresModeAndRepo(t *testing.T) {
	if code := Run([]string{"index", "reindex", "--url", "http://example.invalid", "--token", "tok"}); code != 1 {
		t.Fatalf("expected exit code 1 without --repo/--mode, got %d", code)
	}
	if code := Run([]string{"index", "reindex", "--repo", "repo-a", "--mode", "bogus", "--url", "http://example.invalid", "--token", "tok"}); code != 1 {
		t.Fatalf("expected exit code 1 for invalid mode, got %d", code)
	}
}

func TestIndexReindexDryRunDefaultMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(guardNoMutations(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected call: %s %s", r.Method, r.URL.Path)
	})))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"index", "reindex", "--repo", "repo-a", "--mode", "force", "--url", srv.URL, "--token", "tok"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if !strings.Contains(out, "DRY RUN") {
		t.Fatalf("expected dry-run banner, got: %s", out)
	}
}

func TestReportsListReadOnly(t *testing.T) {
	srv := httptest.NewServer(guardOnlyPost(t, "/xray/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"reports":[{"id":1,"name":"r1","status":"Completed","report_type":"license","end_time":"2024-01-01T00:00:00Z"}],"total_reports":1}`))
	}))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"reports", "list", "--url", srv.URL, "--token", "tok"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if !strings.Contains(out, "r1") || !strings.Contains(out, "Completed") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsDeleteRequiresEndTime(t *testing.T) {
	if code := Run([]string{"reports", "delete", "--url", "http://example.invalid", "--token", "tok"}); code != 1 {
		t.Fatalf("expected exit code 1 without --end-time, got %d", code)
	}
	if code := Run([]string{"reports", "delete", "--end-time", "not-a-date", "--url", "http://example.invalid", "--token", "tok"}); code != 1 {
		t.Fatalf("expected exit code 1 for invalid --end-time, got %d", code)
	}
}

func TestReportsDeleteDryRunDefaultMakesNoDeleteCalls(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v1/reports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			return
		}
		_, _ = w.Write([]byte(`{"reports":[{"id":1,"name":"old","status":"Completed","report_type":"license","end_time":"2020-01-01T00:00:00Z"}],"total_reports":1}`))
	})
	srv := httptest.NewServer(guardNoDeletes(t, mux))
	defer srv.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"reports", "delete", "--end-time", "2024-01-01T00:00:00Z", "--url", srv.URL, "--token", "tok"})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if !strings.Contains(out, "DRY RUN") || !strings.Contains(out, "old") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSyncPoliciesDryRunDefaultMakesNoTargetPost(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"pol1"}]`))
	}))
	defer source.Close()
	target := httptest.NewServer(guardNoMutations(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected call to target: %s %s", r.Method, r.URL.Path)
	})))
	defer target.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"sync", "policies",
			"--source-url", source.URL, "--source-token", "tok",
			"--target-url", target.URL, "--target-token", "tok",
		})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if !strings.Contains(out, "DRY RUN") || !strings.Contains(out, "pol1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSyncWatchesAppliesWithDryRunFalse(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"watch1"}]`))
	}))
	defer source.Close()
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{
			"sync", "watches", "--dry-run=false",
			"--source-url", source.URL, "--source-token", "tok",
			"--target-url", target.URL, "--target-token", "tok",
		})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d, output=%s", code, out)
	}
	if hits != 1 {
		t.Fatalf("expected 1 POST to target, got %d", hits)
	}
	if !strings.Contains(out, "OK") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestSyncUnknownActionReturnsError(t *testing.T) {
	if code := Run([]string{"sync", "bogus"}); code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}

// guardNoMutations wraps h so the test fails if a POST/PUT/PATCH/DELETE
// request reaches it -- used to assert dry-run modes issue zero mutating
// calls.
func guardNoMutations(t *testing.T, h http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			t.Fatalf("dry-run must not issue mutating call: %s %s", r.Method, r.URL.Path)
		}
		h.ServeHTTP(w, r)
	})
}

// guardNoDeletes wraps h so the test fails if a DELETE request reaches it.
func guardNoDeletes(t *testing.T, h http.Handler) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("dry-run must not issue DELETE call: %s", r.URL.Path)
		}
		h.ServeHTTP(w, r)
	})
}

// guardOnlyPost serves handler only for POST requests to path and fails the
// test on any other method, so read-only commands are proven not to issue
// mutating calls.
func guardOnlyPost(t *testing.T, path string, handler http.HandlerFunc) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		handler(w, r)
	})
}
