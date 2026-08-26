package replication

import (
	"jfrogpscli/internal/jfclient"
)

// DeleteResult is the outcome of attempting to delete replication for one
// repo.
type DeleteResult struct {
	Repo    string
	Success bool
	Err     error
}

// Delete removes push replication configuration for each repo in repos. In
// dry-run mode no DELETE calls are made.
func Delete(c *jfclient.Client, repos []string, dryRun bool) []DeleteResult {
	results := make([]DeleteResult, 0, len(repos))
	for _, repo := range repos {
		if dryRun {
			results = append(results, DeleteResult{Repo: repo, Success: true})
			continue
		}

		resp, err := c.Delete("/artifactory/api/replications/" + repo)
		if err != nil {
			results = append(results, DeleteResult{Repo: repo, Err: err})
			continue
		}
		if err := resp.Error("delete replication for " + repo); err != nil {
			results = append(results, DeleteResult{Repo: repo, Err: err})
			continue
		}
		results = append(results, DeleteResult{Repo: repo, Success: true})
	}
	return results
}
