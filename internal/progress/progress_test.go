package progress

import (
	"testing"

	"jfrogpscli/internal/testutil"
)

func TestBars_NonTTY_OnlyLogsOnCompletion(t *testing.T) {
	out := testutil.CaptureStderr(t, func() {
		b := New()
		if b.tty {
			t.Fatal("expected tty=false when stderr is a pipe")
		}
		b.Update("Repos", 1, 10)
		b.Update("Repos", 5, 10)
		b.Update("Repos", 10, 10)
	})
	if out != "Repos: 10/10\n" {
		t.Fatalf("expected exactly one completion line, got %q", out)
	}
}

func TestBars_NonTTY_ZeroTotalIsNoOp(t *testing.T) {
	out := testutil.CaptureStderr(t, func() {
		b := New()
		b.Update("Federated repos", 0, 0)
	})
	if out != "" {
		t.Fatalf("expected no output for a total<=0 update, got %q", out)
	}
}

func TestBars_NonTTY_LogPassesThrough(t *testing.T) {
	out := testutil.CaptureStderr(t, func() {
		b := New()
		b.Log("error:", "boom")
		b.Logf("wrote %d items\n", 3)
	})
	want := "error: boom\nwrote 3 items\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestBars_NonTTY_MultipleLabelsEachReportOnCompletion(t *testing.T) {
	out := testutil.CaptureStderr(t, func() {
		b := New()
		b.Update("Repos", 3, 3)
		b.Update("Permissions", 2, 5)
		b.Update("Permissions", 5, 5)
	})
	want := "Repos: 3/3\nPermissions: 5/5\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
