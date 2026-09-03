// Package repos provides pure, testable business logic for listing,
// diffing, syncing, and deleting Artifactory repositories across one or two
// JFrog Platform Deployments. It has no CLI or flag-parsing concerns -- see
// internal/cli/repos for the command-line wrapper.
package repos

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/jfclient"
)

// Repo is a summary of an Artifactory repository as returned by
// GET /artifactory/api/repositories.
type Repo struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	PackageType string `json:"packageType"`
}

// List returns the repositories of the given type from c. repoType may be
// "local", "remote", "virtual", "federated", or "all"/"" to list every type
// (in which case no "type" query parameter is sent at all).
func List(c *jfclient.Client, repoType string) ([]Repo, error) {
	path := "/artifactory/api/repositories"
	if repoType != "" && repoType != "all" {
		path += "?type=" + url.QueryEscape(repoType)
	}
	resp, err := c.Get(path)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	if err := resp.Error("list repositories"); err != nil {
		return nil, err
	}
	var repos []Repo
	if err := resp.Unmarshal(&repos); err != nil {
		return nil, fmt.Errorf("decode repositories response: %w", err)
	}
	return repos, nil
}
