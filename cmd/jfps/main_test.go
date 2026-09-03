package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/registry"
)

func TestRunNoArgsReturnsError(t *testing.T) {
	if code := run(nil); code != 1 {
		t.Fatalf("expected exit code 1 for no args, got %d", code)
	}
}

func TestRunHelpReturnsZero(t *testing.T) {
	if code := run([]string{"help"}); code != 0 {
		t.Fatalf("expected exit code 0 for help, got %d", code)
	}
}

func TestRunUnknownCommandReturnsError(t *testing.T) {
	if code := run([]string{"does-not-exist"}); code != 1 {
		t.Fatalf("expected exit code 1 for unknown command, got %d", code)
	}
}

func TestRunVersionReturnsZero(t *testing.T) {
	for _, args := range [][]string{{"-v"}, {"--version"}, {"version"}} {
		if code := run(args); code != 0 {
			t.Fatalf("expected exit code 0 for %v, got %d", args, code)
		}
	}
}

func TestAllRegisteredCommandsHaveHelp(t *testing.T) {
	for _, name := range registry.Names() {
		if registry.Help(name) == "" {
			t.Errorf("command %q registered without help text", name)
		}
	}
}

// TestExpectedFeatureCommandsRegistered guards against a feature package
// forgetting to call registry.Register in its init().
func TestExpectedFeatureCommandsRegistered(t *testing.T) {
	want := []string{"access", "cleanup", "discover", "repos", "projects", "replication", "federation", "compare", "xray"}
	got := map[string]bool{}
	for _, n := range registry.Names() {
		got[n] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("expected command group %q to be registered, but it wasn't", w)
		}
	}
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

func TestRunTaggedBuildOnlyAllowsStableCommands(t *testing.T) {
	withVersion(t, "v1.2.3")

	for name := range stableCommands {
		if code := run([]string{name, "-h"}); code != 0 {
			t.Errorf("expected stable command %q to run in a tagged build, got exit code %d", name, code)
		}
	}

	for _, name := range registry.Names() {
		if stableCommands[name] {
			continue
		}
		if code := run([]string{name}); code != 1 {
			t.Errorf("expected unstable command %q to be refused in a tagged build, got exit code %d", name, code)
		}
	}
}

func TestPrintUsageTaggedBuildHidesUnstableCommands(t *testing.T) {
	withVersion(t, "v1.2.3")

	out := captureStderr(t, printUsage)
	for name := range stableCommands {
		if !strings.Contains(out, name) {
			t.Errorf("expected stable command %q in tagged usage output, got:\n%s", name, out)
		}
	}
	for _, name := range registry.Names() {
		if stableCommands[name] {
			continue
		}
		if strings.Contains(out, "  "+name+" ") {
			t.Errorf("expected unstable command %q to be hidden from tagged usage output, got:\n%s", name, out)
		}
	}
}

func TestPrintUsageDevBuildShowsAllCommands(t *testing.T) {
	withVersion(t, "dev")

	out := captureStderr(t, printUsage)
	for _, name := range registry.Names() {
		if !strings.Contains(out, name) {
			t.Errorf("expected command %q in dev usage output, got:\n%s", name, out)
		}
	}
}

func TestConfigureWritesGate(t *testing.T) {
	old := jfclient.WritesDisabled
	t.Cleanup(func() { jfclient.WritesDisabled = old })

	withVersion(t, "dev")
	jfclient.WritesDisabled = true
	configureWritesGate()
	if jfclient.WritesDisabled {
		t.Error("expected dev build to re-enable writes")
	}

	withVersion(t, "v1.2.3")
	jfclient.WritesDisabled = true
	configureWritesGate()
	if !jfclient.WritesDisabled {
		t.Error("expected tagged build to leave the default write gate untouched")
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	os.Stderr = old

	buf, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(buf)
}
