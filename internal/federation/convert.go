package federation

import (
	"jfrogpscli/internal/jfclient"
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
func ConvertToFederated(c *jfclient.Client, repos []string, dryRun bool) []ConvertResult {
	results := make([]ConvertResult, 0, len(repos))
	for _, repo := range repos {
		if dryRun {
			results = append(results, ConvertResult{Repo: repo, Success: true})
			continue
		}

		resp, err := c.Post("/artifactory/api/federation/migrate/"+repo, nil)
		if err != nil {
			results = append(results, ConvertResult{Repo: repo, Err: err})
			continue
		}
		if err := resp.Error("convert " + repo + " to federated"); err != nil {
			results = append(results, ConvertResult{Repo: repo, Err: err})
			continue
		}
		results = append(results, ConvertResult{Repo: repo, Success: true})
	}
	return results
}
