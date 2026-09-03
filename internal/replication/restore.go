package replication

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// LoadBackupFile reads a backup file produced by WriteBackupFile.
func LoadBackupFile(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backup file %s: %w", path, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse backup file %s: %w", path, err)
	}
	return m, nil
}

// RestoreResult is the outcome of attempting to restore replication for one
// repo.
type RestoreResult struct {
	Repo    string
	Success bool
	Err     error
}

// Restore writes each repo's config from data back via PUT
// /artifactory/api/replications/{repo}. Results are returned in a stable,
// sorted-by-repo order. In dry-run mode no PUT calls are made.
func Restore(c *jfclient.Client, data map[string]json.RawMessage, dryRun bool, onProgress ...progress.Reporter) []RestoreResult {
	repos := make([]string, 0, len(data))
	for repo := range data {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	return concurrency.One(repos, 0, func(repo string) RestoreResult {
		if dryRun {
			return RestoreResult{Repo: repo, Success: true}
		}

		resp, err := c.Put("/artifactory/api/replications/"+repo, data[repo])
		if err != nil {
			return RestoreResult{Repo: repo, Err: err}
		}
		if err := resp.Error("restore replication for " + repo); err != nil {
			return RestoreResult{Repo: repo, Err: err}
		}
		return RestoreResult{Repo: repo, Success: true}
	}, onProgress...)
}
