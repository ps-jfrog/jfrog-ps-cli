package discovercmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// whatever was written to it. os.Pipe() gives a plain file, never a
// character device, so multiBar always takes its non-tty fallback path
// under this - which is exactly the path these tests exercise.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestMultiBar_NonTTY_OnlyLogsOnCompletion(t *testing.T) {
	out := captureStderr(t, func() {
		mb := newMultiBar()
		if mb.tty {
			t.Fatal("expected tty=false when stderr is a pipe")
		}
		mb.Update("Repos", 1, 10)
		mb.Update("Repos", 5, 10)
		mb.Update("Repos", 10, 10)
	})
	if out != "Repos: 10/10\n" {
		t.Fatalf("expected exactly one completion line, got %q", out)
	}
}

func TestMultiBar_NonTTY_ZeroTotalIsNoOp(t *testing.T) {
	out := captureStderr(t, func() {
		mb := newMultiBar()
		mb.Update("Federated repos", 0, 0)
	})
	if out != "" {
		t.Fatalf("expected no output for a total<=0 update, got %q", out)
	}
}

func TestMultiBar_NonTTY_LogPassesThrough(t *testing.T) {
	out := captureStderr(t, func() {
		mb := newMultiBar()
		mb.Log("error:", "boom")
		mb.Logf("wrote %d items\n", 3)
	})
	want := "error: boom\nwrote 3 items\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestMultiBar_NonTTY_MultipleLabelsEachReportOnCompletion(t *testing.T) {
	out := captureStderr(t, func() {
		mb := newMultiBar()
		mb.Update("Repos", 3, 3)
		mb.Update("Permissions", 2, 5)
		mb.Update("Permissions", 5, 5)
	})
	want := "Repos: 3/3\nPermissions: 5/5\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
