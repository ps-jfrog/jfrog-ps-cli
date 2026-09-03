package federation

import (
	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// ConvertResult is the outcome of attempting to convert one repo from local
// to federated.
type ConvertResult struct {
	Repo    string
	Success bool
	Err     error
}

// ConvertToFederated migrates each repo in repos from local to federated via
// POST /artifactory/api/federation/migrate/{repo}. This is irreversible on
// the server side. In dry-run mode no POST calls are made.
func ConvertToFederated(c *jfclient.Client, repos []string, dryRun bool, onProgress ...progress.Reporter) []ConvertResult {
	return concurrency.One(repos, 0, func(repo string) ConvertResult {
		if dryRun {
			return ConvertResult{Repo: repo, Success: true}
		}

		resp, err := c.Post("/artifactory/api/federation/migrate/"+repo, nil)
		if err != nil {
			return ConvertResult{Repo: repo, Err: err}
		}
		if err := resp.Error("convert " + repo + " to federated"); err != nil {
			return ConvertResult{Repo: repo, Err: err}
		}
		return ConvertResult{Repo: repo, Success: true}
	}, onProgress...)
}
