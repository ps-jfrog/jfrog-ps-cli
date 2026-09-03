// Package config resolves JFrog server connection details (URL + credentials)
// from CLI flags, environment variables, a saved profile file, or a server
// already configured in the JFrog CLI (--server-id), giving every jfps
// command the same connection options.
package config

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// Server is one saved connection profile.
type Server struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Token    string `json:"token,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
}

// Store is the on-disk collection of saved server profiles.
type Store struct {
	path    string
	Servers map[string]Server `json:"servers"`
}

// DefaultPath returns ~/.jfps/servers.json, honoring $JFPS_CONFIG_HOME if set.
func DefaultPath() string {
	if home := os.Getenv("JFPS_CONFIG_HOME"); home != "" {
		return filepath.Join(home, "servers.json")
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, ".jfps", "servers.json")
}

// Load reads the server profile store from path, returning an empty store if
// the file does not yet exist.
func Load(path string) (*Store, error) {
	s := &Store{path: path, Servers: map[string]Server{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	s.path = path
	return s, nil
}

// Save writes the store back to disk, creating parent directories as needed.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return os.WriteFile(s.path, data, 0o600)
}

// Add inserts or replaces a server profile and saves the store.
func (s *Store) Add(server Server) error {
	if server.Name == "" {
		return fmt.Errorf("server name is required")
	}
	s.Servers[server.Name] = server
	return s.Save()
}

// Remove deletes a server profile and saves the store.
func (s *Store) Remove(name string) error {
	delete(s.Servers, name)
	return s.Save()
}

// ServerFlags holds the raw flag values for one server (or server pair, using
// a prefix like "source-"/"target-") before resolution.
type ServerFlags struct {
	prefix   string
	Server   string
	ServerID string
	URL      string
	Token    string
	User     string
	Password string
	Insecure bool
}

// Register adds --{prefix}server, --{prefix}server-id, --{prefix}url,
// --{prefix}token, --{prefix}user, --{prefix}password, and
// --{prefix}insecure flags to fs. prefix may be "" for a single-server
// command, or e.g. "source-"/"target-" for commands that talk to two JPDs at
// once.
func Register(fs *flag.FlagSet, prefix string) *ServerFlags {
	f := &ServerFlags{prefix: prefix}
	fs.StringVar(&f.Server, prefix+"server", "", "Saved server profile name (see 'jfps config add')")
	fs.StringVar(&f.ServerID, prefix+"server-id", "", "Reuse a server already configured in the JFrog CLI ('jf config add'/'jf login'); requires 'jf' on PATH")
	fs.StringVar(&f.URL, prefix+"url", "", "JPD base URL, e.g. https://mycompany.jfrog.io")
	fs.StringVar(&f.Token, prefix+"token", "", "Access token (bearer auth)")
	fs.StringVar(&f.User, prefix+"user", "", "Username (basic auth, used if --token is empty)")
	fs.StringVar(&f.Password, prefix+"password", "", "Password or identity token (basic auth)")
	fs.BoolVar(&f.Insecure, prefix+"insecure", false, "Skip TLS certificate verification")
	return f
}

// jfCLIServer is the subset of jfrog-cli-core's ServerDetails fields carried
// in the JSON blob that `jf config export <server-id>` prints (base64
// encoded, not encrypted -- see jfrog-cli-core utils/config/config.go).
type jfCLIServer struct {
	URL         string `json:"url"`
	User        string `json:"user"`
	Password    string `json:"password"`
	AccessToken string `json:"accessToken"`
}

// resolveJFCLIServer shells out to the JFrog CLI to reuse a server already
// configured there (via 'jf config add' or 'jf login'), so jfps commands
// don't need their own copy of the same credentials.
func resolveJFCLIServer(serverID string) (jfCLIServer, error) {
	out, err := exec.Command("jf", "config", "export", serverID).Output()
	if err != nil {
		detail := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			detail = strings.TrimSpace(string(ee.Stderr))
		}
		return jfCLIServer{}, fmt.Errorf("run 'jf config export %s' (is the JFrog CLI installed, and is %q a configured server -- see 'jf c show'?): %s", serverID, serverID, detail)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return jfCLIServer{}, fmt.Errorf("decode 'jf config export %s' output: %w", serverID, err)
	}
	var srv jfCLIServer
	if err := json.Unmarshal(decoded, &srv); err != nil {
		return jfCLIServer{}, fmt.Errorf("parse 'jf config export %s' output: %w", serverID, err)
	}
	return srv, nil
}

// envSuffix upper-cases and underscores a flag prefix for env var lookups,
// e.g. "source-" -> "SOURCE_".
func envSuffix(prefix string) string {
	out := make([]byte, 0, len(prefix)+1)
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		if c == '-' {
			c = '_'
		} else if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}

// Resolve builds a jfclient.Client from the flags, falling back in this
// priority order: explicit flags > saved jfps profile (--server) > a
// server already configured in the JFrog CLI (--server-id, or the
// unprefixed JFROG_CLI_SERVER_ID env var it also honors) > jfps's own env
// vars (JFPS_URL / JFPS_ACCESS_TOKEN / JFPS_USER / JFPS_PASSWORD /
// JFPS_INSECURE, or their SOURCE_/TARGET_-prefixed equivalents) > the
// unprefixed JF_*/JFROG_* env vars used by 'setup-jfrog-cli' and similar CI
// tooling for ad hoc auth.
func (f *ServerFlags) Resolve() (*jfclient.Client, error) {
	url, token, user, password, insecure := f.URL, f.Token, f.User, f.Password, f.Insecure

	if f.Server != "" {
		store, err := Load(DefaultPath())
		if err != nil {
			return nil, err
		}
		srv, ok := store.Servers[f.Server]
		if !ok {
			return nil, fmt.Errorf("no saved server profile named %q (see 'jfps config add')", f.Server)
		}
		if url == "" {
			url = srv.URL
		}
		if token == "" {
			token = srv.Token
		}
		if user == "" {
			user = srv.User
		}
		if password == "" {
			password = srv.Password
		}
		if !insecure {
			insecure = srv.Insecure
		}
	}

	serverID := f.ServerID
	if serverID == "" && f.prefix == "" {
		serverID = os.Getenv("JFROG_CLI_SERVER_ID")
	}
	if serverID != "" && (url == "" || (token == "" && user == "")) {
		srv, err := resolveJFCLIServer(serverID)
		if err != nil {
			return nil, err
		}
		if url == "" {
			url = srv.URL
		}
		if token == "" {
			token = srv.AccessToken
		}
		if user == "" {
			user = srv.User
		}
		if password == "" {
			password = srv.Password
		}
	}

	env := envSuffix(f.prefix)
	if url == "" {
		url = firstNonEmptyEnv("JFPS_"+env+"URL", "JFPS_URL")
	}
	if token == "" {
		token = firstNonEmptyEnv("JFPS_"+env+"ACCESS_TOKEN", "JFPS_ACCESS_TOKEN")
	}
	if user == "" {
		user = firstNonEmptyEnv("JFPS_"+env+"USER", "JFPS_USER")
	}
	if password == "" {
		password = firstNonEmptyEnv("JFPS_"+env+"PASSWORD", "JFPS_PASSWORD")
	}
	if !insecure {
		insecure = os.Getenv("JFPS_INSECURE") == "true"
	}

	if f.prefix == "" {
		if url == "" {
			url = firstNonEmptyEnv("JF_URL", "JFROG_URL")
		}
		if token == "" {
			token = firstNonEmptyEnv("JF_ACCESS_TOKEN", "JFROG_ACCESS_TOKEN")
		}
		if user == "" {
			user = firstNonEmptyEnv("JF_USER", "JFROG_USER")
		}
		if password == "" {
			password = firstNonEmptyEnv("JF_PASSWORD", "JFROG_PASSWORD")
		}
	}

	if url == "" {
		flagName := f.prefix + "url"
		return nil, fmt.Errorf("no JPD URL provided: pass --%s, --%sserver <name>, --%sserver-id <jf CLI server>, or set JFPS_%sURL", flagName, f.prefix, f.prefix, env)
	}
	if token == "" && user == "" {
		return nil, fmt.Errorf("no credentials provided: pass --%stoken or --%suser/--%spassword", f.prefix, f.prefix, f.prefix)
	}

	return jfclient.New(url, token, user, password, insecure), nil
}

// PeekURL resolves just the base URL - flags > saved profile > env vars,
// the same priority order Resolve uses - without requiring credentials.
// Returns "" if no URL is configured anywhere. Useful for anything cosmetic
// that wants the target instance's URL (e.g. naming an output directory
// after it) even when a full client isn't being built; callers that need a
// real error for a missing/invalid profile should use Resolve instead.
func (f *ServerFlags) PeekURL() string {
	url := f.URL
	if url == "" && f.Server != "" {
		if store, err := Load(DefaultPath()); err == nil {
			if srv, ok := store.Servers[f.Server]; ok {
				url = srv.URL
			}
		}
	}
	if url == "" {
		serverID := f.ServerID
		if serverID == "" && f.prefix == "" {
			serverID = os.Getenv("JFROG_CLI_SERVER_ID")
		}
		if serverID != "" {
			if srv, err := resolveJFCLIServer(serverID); err == nil {
				url = srv.URL
			}
		}
	}
	if url == "" {
		url = firstNonEmptyEnv("JFPS_"+envSuffix(f.prefix)+"URL", "JFPS_URL")
	}
	if url == "" && f.prefix == "" {
		url = firstNonEmptyEnv("JF_URL", "JFROG_URL")
	}
	return url
}

func firstNonEmptyEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}
