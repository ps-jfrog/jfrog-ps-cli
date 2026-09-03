// Package testutil holds small test helpers shared across this module's
// _test.go files, so each package doesn't need its own copy.
package testutil

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// CaptureStderr redirects os.Stderr for the duration of fn and returns
// whatever was written to it. os.Pipe() gives a plain file, never a
// character device, so anything checking stderrIsTerminal() (e.g.
// internal/progress.Bars) takes its non-tty fallback path under this.
func CaptureStderr(t *testing.T, fn func()) string {
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
