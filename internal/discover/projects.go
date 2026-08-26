package discover

import (
	"fmt"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetProjects fetches every JFrog Project (GET /access/api/v1/projects) and
// fills each one's RepoCount by counting repos whose ProjectKey matches.
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

	repoCounts := map[string]int{}
	for _, r := range repos {
		if r.ProjectKey != "" {
			repoCounts[r.ProjectKey]++
		}
	}

	projects := make([]instance.Project, 0, len(raw))
	for _, p := range raw {
		projects = append(projects, instance.Project{
			Key:               p.ProjectKey,
			DisplayName:       p.DisplayName,
			Description:       p.Description,
			StorageQuotaBytes: p.StorageQuotaBytes,
			RepoCount:         repoCounts[p.ProjectKey],
		})
	}
	return projects, nil
}
