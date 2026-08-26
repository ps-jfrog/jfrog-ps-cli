package discover

import (
	"fmt"
	"sort"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetProjects fetches every JFrog Project (GET /access/api/v1/projects) and
// fills each one's Repos (and RepoCount, len(Repos)) by matching repos'
// ProjectKey - no extra API call needed, storageinfo already carries it.
func GetProjects(c *jfclient.Client, repos []RepoSummary) ([]instance.Project, error) {
	resp, err := c.Get("/access/api/v1/projects")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	if err := resp.Error("list projects"); err != nil {
		return nil, err
	}

	var raw []struct {
		DisplayName       string `json:"display_name"`
		Description       string `json:"description"`
		ProjectKey        string `json:"project_key"`
		StorageQuotaBytes int64  `json:"storage_quota_bytes"`
	}
	if err := resp.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("decode projects response: %w", err)
	}

	repoKeys := map[string][]string{}
	for _, r := range repos {
		if r.ProjectKey != "" {
			repoKeys[r.ProjectKey] = append(repoKeys[r.ProjectKey], r.RepoKey)
		}
	}
	for key := range repoKeys {
		sort.Strings(repoKeys[key])
	}

	projects := make([]instance.Project, 0, len(raw))
	for _, p := range raw {
		projectRepos := repoKeys[p.ProjectKey]
		projects = append(projects, instance.Project{
			Key:               p.ProjectKey,
			DisplayName:       p.DisplayName,
			Description:       p.Description,
			StorageQuotaBytes: p.StorageQuotaBytes,
			RepoCount:         len(projectRepos),
			Repos:             projectRepos,
		})
	}
	return projects, nil
}
