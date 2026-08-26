// Package federation implements the business logic behind `jfps federation`:
// converting local repos to federated (and back), checking federation
// member health, and resyncing federation config.
package federation

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

type repoSummary struct {
	Key string `json:"key"`
}

// ListLocalRepos returns the keys of every local repository on the server.
func ListLocalRepos(c *jfclient.Client) ([]string, error) {
	return listRepos(c, "local")
}

// ListFederatedRepos returns the keys of every federated repository on the
// server.
func ListFederatedRepos(c *jfclient.Client) ([]string, error) {
	return listRepos(c, "federated")
}

func listRepos(c *jfclient.Client, repoType string) ([]string, error) {
	resp, err := c.Get("/artifactory/api/repositories?type=" + repoType)
	if err != nil {
		return nil, fmt.Errorf("list %s repositories: %w", repoType, err)
	}
	if err := resp.Error("list " + repoType + " repositories"); err != nil {
		return nil, err
	}
	var repos []repoSummary
	if err := resp.Unmarshal(&repos); err != nil {
		return nil, fmt.Errorf("parse %s repositories: %w", repoType, err)
	}
	keys := make([]string, 0, len(repos))
	for _, r := range repos {
		keys = append(keys, r.Key)
	}
	return keys, nil
}
