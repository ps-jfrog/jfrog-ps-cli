package comparecmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever was written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
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
	if code := Run([]string{"bogus"}); code != 1 {
		t.Fatalf("expected exit code 1 for unknown subcommand, got %d", code)
	}
}

func TestRunReposRequiresRepoOrAll(t *testing.T) {
	code := Run([]string{"repos", "--source-url", "https://source.example", "--source-token", "t",
		"--target-url", "https://target.example", "--target-token", "t"})
	if code != 1 {
		t.Fatalf("expected exit code 1 when neither --repo nor --all given, got %d", code)
	}
}

func TestRunReposRejectsBothRepoAndAll(t *testing.T) {
	code := Run([]string{"repos", "--repo", "my-repo", "--all",
		"--source-url", "https://source.example", "--source-token", "t",
		"--target-url", "https://target.example", "--target-token", "t"})
	if code != 1 {
		t.Fatalf("expected exit code 1 when both --repo and --all given, got %d", code)
	}
}

func TestRunReposHelp(t *testing.T) {
	if code := Run([]string{"repos", "-h"}); code != 0 {
		t.Fatalf("expected exit code 0 for repos help, got %d", code)
	}
}

func TestRunBuildsHelp(t *testing.T) {
	if code := Run([]string{"builds", "-h"}); code != 0 {
		t.Fatalf("expected exit code 0 for builds help, got %d", code)
	}
}

func TestRunBuildsMissingConnection(t *testing.T) {
	if code := Run([]string{"builds"}); code != 1 {
		t.Fatalf("expected exit code 1 when no connection flags given, got %d", code)
	}
}

func TestRunJSONMissingFlags(t *testing.T) {
	if code := Run([]string{"json"}); code != 1 {
		t.Fatalf("expected exit code 1 when --file1/--file2 missing, got %d", code)
	}
}

func TestRunJSONNoDifferences(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.json")
	file2 := filepath.Join(dir, "b.json")
	writeTestFile(t, file1, `{"a": 1}`)
	writeTestFile(t, file2, `{"a": 1}`)

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"json", "--file1", file1, "--file2", file2})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !bytes.Contains([]byte(out), []byte("no differences found")) {
		t.Fatalf("expected 'no differences found' in output, got %q", out)
	}
}

func TestRunJSONWithDifferences(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.json")
	file2 := filepath.Join(dir, "b.json")
	writeTestFile(t, file1, `{"a": 1, "removed": true}`)
	writeTestFile(t, file2, `{"a": 2, "added": true}`)

	var code int
	out := captureStdout(t, func() {
		code = Run([]string{"json", "--file1", file1, "--file2", file2})
	})
	if code != 0 {
		t.Fatalf("expected exit code 0 even with differences (report command), got %d", code)
	}
	for _, want := range []string{"Added", "Removed", "Changed"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Fatalf("expected output to mention %q, got %q", want, out)
		}
	}
}

func TestRunJSONReadError(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.json")
	writeTestFile(t, file1, `{"a": 1}`)

	code := Run([]string{"json", "--file1", file1, "--file2", filepath.Join(dir, "nope.json")})
	if code != 1 {
		t.Fatalf("expected exit code 1 for unreadable file2, got %d", code)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
