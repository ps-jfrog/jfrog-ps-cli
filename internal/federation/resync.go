package federation

import (
	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
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
func Resync(c *jfclient.Client, repos []string, dryRun bool, onProgress ...progress.Reporter) []ResyncResult {
	return concurrency.One(repos, 0, func(repo string) ResyncResult {
		if dryRun {
			return ResyncResult{Repo: repo, Success: true}
		}

		resp, err := c.Post("/artifactory/api/federation/configSync/"+repo, nil)
		if err != nil {
			return ResyncResult{Repo: repo, Err: err}
		}
		if err := resp.Error("resync " + repo); err != nil {
			return ResyncResult{Repo: repo, Err: err}
		}
		return ResyncResult{Repo: repo, Success: true}
	}, onProgress...)
}
