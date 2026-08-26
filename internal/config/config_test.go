package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

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
