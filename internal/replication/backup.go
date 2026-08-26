package replication

import (
	"encoding/json"
	"fmt"
	"os"

	"jfrogpscli/internal/jfclient"
)

// Backup fetches the raw replication config for each repo and returns it as
// a map of repo key -> raw JSON config, suitable for writing to a backup
// file and later restoring with Restore.
func Backup(c *jfclient.Client, repos []string) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(repos))
	for _, repo := range repos {
		resp, err := c.Get("/artifactory/api/replications/" + repo)
		if err != nil {
			return nil, fmt.Errorf("get replication config for %s: %w", repo, err)
		}
		if err := resp.Error("get replication config for " + repo); err != nil {
			return nil, err
		}
		out[repo] = json.RawMessage(resp.Body)
	}
	return out, nil
}

// WriteBackupFile writes data (as produced by Backup) to path as indented
// JSON.
func WriteBackupFile(path string, data map[string]json.RawMessage) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backup data: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write backup file %s: %w", path, err)
	}
	return nil
}
