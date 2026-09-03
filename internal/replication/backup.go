package replication

import (
	"encoding/json"
	"fmt"
	"os"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// repoBackup pairs one repo's key with its fetched raw config, so
// Backup's concurrent fetch pass can hand results back as a plain slice
// (safe to write one-per-goroutine) and only build the result map
// afterwards, sequentially.
type repoBackup struct {
	repo string
	body json.RawMessage
}

// Backup fetches the raw replication config for each repo and returns it as
// a map of repo key -> raw JSON config, suitable for writing to a backup
// file and later restoring with Restore. Fetches run with up to
// concurrency.DefaultParallelism in flight at once (each is an independent,
// read-only GET); if any repo's fetch failed, Backup returns that error
// instead of a partial map, matching the previous sequential behavior of
// stopping at the first failure -- just detected once every fetch already
// in flight finishes, rather than before starting the next one.
func Backup(c *jfclient.Client, repos []string, onProgress ...progress.Reporter) (map[string]json.RawMessage, error) {
	fetched, err := concurrency.MapOrErr(repos, 0, func(repo string) (repoBackup, error) {
		resp, err := c.Get("/artifactory/api/replications/" + repo)
		if err != nil {
			return repoBackup{}, fmt.Errorf("get replication config for %s: %w", repo, err)
		}
		if err := resp.Error("get replication config for " + repo); err != nil {
			return repoBackup{}, err
		}
		return repoBackup{repo: repo, body: json.RawMessage(resp.Body)}, nil
	}, onProgress...)
	if err != nil {
		return nil, err
	}

	out := make(map[string]json.RawMessage, len(fetched))
	for _, f := range fetched {
		out[f.repo] = f.body
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
