package replication

import (
	"encoding/json"
	"fmt"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// Status describes whether push replication is configured for a repo, and if
// so, the target URL(s) it replicates to.
type Status struct {
	Repo       string
	Configured bool
	TargetURLs []string
}

type replicationEntry struct {
	URL string `json:"url"`
}

// Check queries the push replication configuration for a single repo. A
// non-2xx response (typically 404) means replication is not configured and
// is not treated as an error.
func Check(c *jfclient.Client, repo string) (Status, error) {
	status := Status{Repo: repo}

	resp, err := c.Get("/artifactory/api/replications/" + repo)
	if err != nil {
		return status, fmt.Errorf("get replication config for %s: %w", repo, err)
	}
	if !resp.OK() {
		return status, nil
	}

	body := strings.TrimSpace(string(resp.Body))
	if body == "" || body == "[]" || body == "{}" || body == "null" {
		return status, nil
	}

	// Push replication config responses are typically an array of entries,
	// but tolerate a single object too.
	if strings.HasPrefix(body, "[") {
		var entries []replicationEntry
		if err := json.Unmarshal(resp.Body, &entries); err != nil {
			return status, fmt.Errorf("parse replication config for %s: %w", repo, err)
		}
		status.Configured = len(entries) > 0
		for _, e := range entries {
			if e.URL != "" {
				status.TargetURLs = append(status.TargetURLs, e.URL)
			}
		}
		return status, nil
	}

	var single replicationEntry
	if err := json.Unmarshal(resp.Body, &single); err != nil {
		return status, fmt.Errorf("parse replication config for %s: %w", repo, err)
	}
	status.Configured = true
	if single.URL != "" {
		status.TargetURLs = append(status.TargetURLs, single.URL)
	}
	return status, nil
}

// List checks a single repo (if repo is non-empty) or every local repo, and
// returns their replication status.
func List(c *jfclient.Client, repo string) ([]Status, error) {
	var repos []string
	if repo != "" {
		repos = []string{repo}
	} else {
		var err error
		repos, err = ListLocalRepos(c)
		if err != nil {
			return nil, err
		}
	}

	statuses := make([]Status, 0, len(repos))
	for _, r := range repos {
		st, err := Check(c, r)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, st)
	}
	return statuses, nil
}
