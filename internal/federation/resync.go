package federation

import (
	"jfrogpscli/internal/jfclient"
)

// ResyncResult is the outcome of attempting to resync federation config for
// one repo.
type ResyncResult struct {
	Repo    string
	Success bool
	Err     error
}

// Resync triggers a federation config resync for each repo in repos via
// POST /artifactory/api/federation/configSync/{repo}. Note this only
// resyncs configuration (member list, ACLs, etc.), not artifact data. In
// dry-run mode no POST calls are made.
func Resync(c *jfclient.Client, repos []string, dryRun bool) []ResyncResult {
	results := make([]ResyncResult, 0, len(repos))
	for _, repo := range repos {
		if dryRun {
			results = append(results, ResyncResult{Repo: repo, Success: true})
			continue
		}

		resp, err := c.Post("/artifactory/api/federation/configSync/"+repo, nil)
		if err != nil {
			results = append(results, ResyncResult{Repo: repo, Err: err})
			continue
		}
		if err := resp.Error("resync " + repo); err != nil {
			results = append(results, ResyncResult{Repo: repo, Err: err})
			continue
		}
		results = append(results, ResyncResult{Repo: repo, Success: true})
	}
	return results
}
