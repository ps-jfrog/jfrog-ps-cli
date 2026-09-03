package replication

import (
	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
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
func Delete(c *jfclient.Client, repos []string, dryRun bool, onProgress ...progress.Reporter) []DeleteResult {
	return concurrency.One(repos, 0, func(repo string) DeleteResult {
		if dryRun {
			return DeleteResult{Repo: repo, Success: true}
		}

		resp, err := c.Delete("/artifactory/api/replications/" + repo)
		if err != nil {
			return DeleteResult{Repo: repo, Err: err}
		}
		if err := resp.Error("delete replication for " + repo); err != nil {
			return DeleteResult{Repo: repo, Err: err}
		}
		return DeleteResult{Repo: repo, Success: true}
	}, onProgress...)
}
