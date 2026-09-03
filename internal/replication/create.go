package replication

import (
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// CreateOptions holds the parameters used to build a push replication
// payload for one or more repos.
type CreateOptions struct {
	TargetURL   string
	TargetRepo  string // optional; defaults to the source repo key
	Cron        string
	Username    string
	Password    string
	SyncDeletes bool
	Enabled     bool
}

// Payload is the JSON body sent to
// PUT /artifactory/api/replications/{repo} to create push replication.
type Payload struct {
	RepoKey     string `json:"repoKey"`
	URL         string `json:"url"`
	CronExp     string `json:"cronExp"`
	Enabled     bool   `json:"enabled"`
	SyncDeletes bool   `json:"syncDeletes"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

// BuildPayload constructs the replication-create payload for repo.
func BuildPayload(repo string, opts CreateOptions) Payload {
	targetRepo := opts.TargetRepo
	if targetRepo == "" {
		targetRepo = repo
	}
	url := strings.TrimRight(opts.TargetURL, "/") + "/artifactory/" + targetRepo
	return Payload{
		RepoKey:     repo,
		URL:         url,
		CronExp:     opts.Cron,
		Enabled:     opts.Enabled,
		SyncDeletes: opts.SyncDeletes,
		Username:    opts.Username,
		Password:    opts.Password,
	}
}

// CreateResult is the outcome of attempting to create replication for one
// repo.
type CreateResult struct {
	Repo    string
	Payload Payload
	Success bool
	Err     error
}

// Create builds and (unless dryRun) applies push replication configuration
// for each repo in repos. In dry-run mode no PUT calls are made; each result
// carries the payload that would have been sent.
func Create(c *jfclient.Client, repos []string, opts CreateOptions, dryRun bool, onProgress ...progress.Reporter) []CreateResult {
	return concurrency.One(repos, 0, func(repo string) CreateResult {
		payload := BuildPayload(repo, opts)
		if dryRun {
			return CreateResult{Repo: repo, Payload: payload, Success: true}
		}

		resp, err := c.Put("/artifactory/api/replications/"+repo, payload)
		if err != nil {
			return CreateResult{Repo: repo, Payload: payload, Err: err}
		}
		if err := resp.Error("create replication for " + repo); err != nil {
			return CreateResult{Repo: repo, Payload: payload, Err: err}
		}
		return CreateResult{Repo: repo, Payload: payload, Success: true}
	}, onProgress...)
}
