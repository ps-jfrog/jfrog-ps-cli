package main

import (
	"testing"

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
