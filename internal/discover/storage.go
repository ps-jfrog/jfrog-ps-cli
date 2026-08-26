// Package discover provides pure, testable business logic for the "jfps
// discover" command group: per-repo storage/activity reporting (derived via
// /api/storageinfo and AQL, since Artifactory tracks neither a repo's
// creation time nor its own "last activity") and mapping repos to the
// package cleanup policies that apply to them (Artifactory has no reverse
// "policy for this repo" lookup, so the matching happens client-side against
// GET /artifactory/api/cleanup/packages/policies). It has no CLI or
// flag-parsing concerns -- see internal/cli/discover for the command-line
// wrapper.
package discover

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

// RepoSummary is one entry of GET /artifactory/api/storageinfo's
// repositoriesSummaryList.
type RepoSummary struct {
	RepoKey          string `json:"repoKey"`
	RepoType         string `json:"repoType"`
	PackageType      string `json:"packageType"`
	FoldersCount     int    `json:"foldersCount"`
	FilesCount       int    `json:"filesCount"`
	ItemsCount       int    `json:"itemsCount"`
	UsedSpaceInBytes int64  `json:"usedSpaceInBytes"`
	ProjectKey       string `json:"projectKey"`
}

// UsageGB returns the repo's used space in GB (base-1024).
func (r RepoSummary) UsageGB() float64 {
	return float64(r.UsedSpaceInBytes) / (1024 * 1024 * 1024)
}

type storageInfoResponse struct {
	RepositoriesSummaryList []RepoSummary `json:"repositoriesSummaryList"`
}

// ListRepoStorage returns per-repo storage stats from
// /artifactory/api/storageinfo, excluding the synthetic "TOTAL" row.
func ListRepoStorage(c *jfclient.Client) ([]RepoSummary, error) {
	resp, err := c.Get("/artifactory/api/storageinfo")
	if err != nil {
		return nil, fmt.Errorf("fetch storageinfo: %w", err)
	}
	if err := resp.Error("fetch storageinfo"); err != nil {
		return nil, err
	}
	var out storageInfoResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("decode storageinfo response: %w", err)
	}

	repos := make([]RepoSummary, 0, len(out.RepositoriesSummaryList))
	for _, r := range out.RepositoriesSummaryList {
		if r.RepoKey == "TOTAL" {
			continue
		}
		repos = append(repos, r)
	}
	return repos, nil
}
