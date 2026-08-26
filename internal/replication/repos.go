// Package replication implements the business logic behind `jfps replication`:
// listing, creating, deleting, disabling, backing up, and restoring
// Artifactory push replication configuration.
package replication

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

type repoSummary struct {
	Key string `json:"key"`
}

// ListLocalRepos returns the keys of every local repository on the server.
func ListLocalRepos(c *jfclient.Client) ([]string, error) {
	resp, err := c.Get("/artifactory/api/repositories?type=local")
	if err != nil {
		return nil, fmt.Errorf("list local repositories: %w", err)
	}
	if err := resp.Error("list local repositories"); err != nil {
		return nil, err
	}
	var repos []repoSummary
	if err := resp.Unmarshal(&repos); err != nil {
		return nil, fmt.Errorf("parse local repositories: %w", err)
	}
	keys := make([]string, 0, len(repos))
	for _, r := range repos {
		keys = append(keys, r.Key)
	}
	return keys, nil
}

// ReposWithReplication filters candidates down to those that currently have
// push replication configured.
func ReposWithReplication(c *jfclient.Client, candidates []string) ([]string, error) {
	var found []string
	for _, repo := range candidates {
		st, err := Check(c, repo)
		if err != nil {
			return nil, err
		}
		if st.Configured {
			found = append(found, repo)
		}
	}
	return found, nil
}
