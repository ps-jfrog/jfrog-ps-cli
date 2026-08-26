package replication

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"jfrogpscli/internal/jfclient"
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
func Restore(c *jfclient.Client, data map[string]json.RawMessage, dryRun bool) []RestoreResult {
	repos := make([]string, 0, len(data))
	for repo := range data {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	results := make([]RestoreResult, 0, len(repos))
	for _, repo := range repos {
		if dryRun {
			results = append(results, RestoreResult{Repo: repo, Success: true})
			continue
		}

		resp, err := c.Put("/artifactory/api/replications/"+repo, data[repo])
		if err != nil {
			results = append(results, RestoreResult{Repo: repo, Err: err})
			continue
		}
		if err := resp.Error("restore replication for " + repo); err != nil {
			results = append(results, RestoreResult{Repo: repo, Err: err})
			continue
		}
		results = append(results, RestoreResult{Repo: repo, Success: true})
	}
	return results
}
