package reposcmd

import "testing"

func TestRun_NoArgs(t *testing.T) {
	if code := Run(nil); code != 1 {
		t.Fatalf("expected exit code 1 for no args, got %d", code)
	}
}

func TestRun_Help(t *testing.T) {
	if code := Run([]string{"-h"}); code != 0 {
		t.Fatalf("expected exit code 0 for -h, got %d", code)
	}
	if code := Run([]string{"--help"}); code != 0 {
		t.Fatalf("expected exit code 0 for --help, got %d", code)
	}
	if code := Run([]string{"help"}); code != 0 {
		t.Fatalf("expected exit code 0 for help, got %d", code)
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	if code := Run([]string{"bogus"}); code != 1 {
		t.Fatalf("expected exit code 1 for unknown subcommand, got %d", code)
	}
}

func TestRun_ListMissingCredentials(t *testing.T) {
	// No server flags/env set, so resolving the client should fail cleanly
	// with exit code 1 rather than panicking.
	if code := Run([]string{"list", "--url", "http://example.invalid"}); code != 1 {
		t.Fatalf("expected exit code 1 when no credentials provided, got %d", code)
	}
}

func TestRun_DiffMissingType(t *testing.T) {
	if code := Run([]string{"diff"}); code != 1 {
		t.Fatalf("expected exit code 1 when --type is missing, got %d", code)
	}
}

func TestRun_DeleteMissingFile(t *testing.T) {
	if code := Run([]string{"delete"}); code != 1 {
		t.Fatalf("expected exit code 1 when --file is missing, got %d", code)
	}
}
