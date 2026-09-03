package config

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// installFakeJF puts a stub "jf" executable on PATH for the duration of the
// test, so tests exercising the --server-id / JFROG_CLI_SERVER_ID path don't
// depend on the real JFrog CLI being installed or configured. It only
// understands "jf config export <id>": known IDs (from servers) print the
// base64-encoded JSON blob 'jf config export' really emits; anything else
// exits non-zero like the real command does for an unknown server ID.
func installFakeJF(t *testing.T, servers map[string]jfCLIServer) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake jf shim is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = config ] && [ \"$2\" = export ]; then\n  case \"$3\" in\n"
	for id, srv := range servers {
		data, err := json.Marshal(srv)
		if err != nil {
			t.Fatalf("marshal fake server %q: %v", id, err)
		}
		script += fmt.Sprintf("  %s) echo %s ;;\n", id, base64.StdEncoding.EncodeToString(data))
	}
	script += "  *) echo \"no such server ID\" >&2; exit 1 ;;\n  esac\n  exit 0\nfi\necho \"unsupported fake jf invocation: $*\" >&2\nexit 1\n"

	path := filepath.Join(dir, "jf")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake jf: %v", err)
	}
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
}

func TestAddLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servers.json")

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Servers) != 0 {
		t.Fatalf("expected empty store, got %v", s.Servers)
	}
	s.path = path
	if err := s.Add(Server{Name: "prod", URL: "https://prod.jfrog.io", Token: "tok"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := reloaded.Servers["prod"]
	if !ok {
		t.Fatalf("expected 'prod' profile to be saved")
	}
	if got.URL != "https://prod.jfrog.io" || got.Token != "tok" {
		t.Fatalf("unexpected saved profile: %+v", got)
	}
}

func TestResolveFromFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse([]string{"--url", "https://x.jfrog.io", "--token", "abc"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := f.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.BaseURL != "https://x.jfrog.io" || c.Token != "abc" {
		t.Fatalf("unexpected client: %+v", c)
	}
}

func TestResolveFromEnvWithPrefix(t *testing.T) {
	os.Setenv("JFPS_SOURCE_URL", "https://source.jfrog.io")
	os.Setenv("JFPS_SOURCE_ACCESS_TOKEN", "src-tok")
	defer os.Unsetenv("JFPS_SOURCE_URL")
	defer os.Unsetenv("JFPS_SOURCE_ACCESS_TOKEN")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "source-")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := f.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.BaseURL != "https://source.jfrog.io" || c.Token != "src-tok" {
		t.Fatalf("unexpected client: %+v", c)
	}
}

func TestResolveMissingURL(t *testing.T) {
	// Ensure no ambient env vars leak into this test.
	for _, k := range []string{"JFPS_URL", "JFPS_ACCESS_TOKEN", "JFPS_USER", "JFPS_PASSWORD"} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		if had {
			defer os.Setenv(k, old)
		}
	}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := f.Resolve(); err == nil {
		t.Fatalf("expected error when no URL is configured")
	}
}

func TestPeekURL_FromFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse([]string{"--url", "https://x.jfrog.io"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := f.PeekURL(); got != "https://x.jfrog.io" {
		t.Fatalf("PeekURL() = %q, want https://x.jfrog.io", got)
	}
}

func TestPeekURL_FromEnv_NoCredentialsNeeded(t *testing.T) {
	os.Setenv("JFPS_URL", "https://env.jfrog.io")
	defer os.Unsetenv("JFPS_URL")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := f.PeekURL(); got != "https://env.jfrog.io" {
		t.Fatalf("PeekURL() = %q, want https://env.jfrog.io", got)
	}
}

func TestPeekURL_Missing(t *testing.T) {
	for _, k := range []string{"JFPS_URL", "JFPS_ACCESS_TOKEN", "JFPS_USER", "JFPS_PASSWORD"} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		if had {
			defer os.Setenv(k, old)
		}
	}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := f.PeekURL(); got != "" {
		t.Fatalf("PeekURL() = %q, want empty when nothing is configured", got)
	}
}

func TestResolveFromJFCLIServerIDFlag(t *testing.T) {
	installFakeJF(t, map[string]jfCLIServer{
		"myserver": {URL: "https://jf-cli.jfrog.io", User: "admin", AccessToken: "jf-cli-tok"},
	})

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse([]string{"--server-id", "myserver"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := f.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.BaseURL != "https://jf-cli.jfrog.io" || c.Token != "jf-cli-tok" {
		t.Fatalf("unexpected client: %+v", c)
	}
}

func TestResolveFromJFCLIServerIDFlag_UnknownServer(t *testing.T) {
	installFakeJF(t, map[string]jfCLIServer{})

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse([]string{"--server-id", "does-not-exist"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := f.Resolve(); err == nil {
		t.Fatalf("expected error for unknown jf CLI server ID")
	}
}

func TestResolveFromJFROGCLIServerIDEnv(t *testing.T) {
	installFakeJF(t, map[string]jfCLIServer{
		"envserver": {URL: "https://env-server.jfrog.io", AccessToken: "env-tok"},
	})
	os.Setenv("JFROG_CLI_SERVER_ID", "envserver")
	defer os.Unsetenv("JFROG_CLI_SERVER_ID")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := f.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.BaseURL != "https://env-server.jfrog.io" || c.Token != "env-tok" {
		t.Fatalf("unexpected client: %+v", c)
	}
}

func TestResolveFromJFROGCLIServerIDEnv_IgnoredWithPrefix(t *testing.T) {
	for _, k := range []string{"JFPS_URL", "JFPS_ACCESS_TOKEN", "JFPS_USER", "JFPS_PASSWORD", "JF_URL", "JFROG_URL"} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		if had {
			defer os.Setenv(k, old)
		}
	}
	installFakeJF(t, map[string]jfCLIServer{
		"envserver": {URL: "https://env-server.jfrog.io", AccessToken: "env-tok"},
	})
	os.Setenv("JFROG_CLI_SERVER_ID", "envserver")
	defer os.Unsetenv("JFROG_CLI_SERVER_ID")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "source-")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := f.Resolve(); err == nil {
		t.Fatalf("expected JFROG_CLI_SERVER_ID to be ignored for a prefixed (two-server) command")
	}
}

func TestResolveFromJFEcosystemEnvVars(t *testing.T) {
	// JFPS_* takes priority over the JF_*/JFROG_* fallback, so make sure no
	// ambient JFPS_* env vars are present to shadow what this test sets.
	for _, k := range []string{"JFPS_URL", "JFPS_ACCESS_TOKEN", "JFPS_USER", "JFPS_PASSWORD"} {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		if had {
			defer os.Setenv(k, old)
		}
	}
	os.Setenv("JF_URL", "https://jf-env.jfrog.io")
	os.Setenv("JF_ACCESS_TOKEN", "jf-env-tok")
	defer os.Unsetenv("JF_URL")
	defer os.Unsetenv("JF_ACCESS_TOKEN")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := f.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.BaseURL != "https://jf-env.jfrog.io" || c.Token != "jf-env-tok" {
		t.Fatalf("unexpected client: %+v", c)
	}
}

func TestResolveFromSavedProfile(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("JFPS_CONFIG_HOME", dir)
	defer os.Unsetenv("JFPS_CONFIG_HOME")

	s, err := Load(DefaultPath())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := s.Add(Server{Name: "prod", URL: "https://prod.jfrog.io", Token: "prod-tok"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	f := Register(fs, "")
	if err := fs.Parse([]string{"--server", "prod"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := f.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if c.BaseURL != "https://prod.jfrog.io" || c.Token != "prod-tok" {
		t.Fatalf("unexpected client: %+v", c)
	}
}
