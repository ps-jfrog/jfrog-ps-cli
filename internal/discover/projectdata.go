package discover

import (
	"fmt"
	"sort"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetProjectRoles fetches all roles (both predefined and custom) for a specific project.
func GetProjectRoles(client *jfclient.Client, projectKey string) ([]instance.ProjectRole, error) {
	endpoint := fmt.Sprintf("/access/api/v1/projects/%s/roles", projectKey)
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("fetch project roles for %s: %w", projectKey, err)
	}
	if err := resp.Error("fetch project roles"); err != nil {
		return nil, err
	}
	var roles []instance.ProjectRole
	if err := resp.Unmarshal(&roles); err != nil {
		return nil, fmt.Errorf("parse project roles: %w", err)
	}
	return roles, nil
}

// BuildProjectRepositoryMapping creates a flat list of project-to-repo mappings
// from the projects and repos already collected.
func BuildProjectRepositoryMapping(projects []instance.Project, repos []RepoSummary) []instance.ProjectRepository {
	// Create a map of repo details for quick lookup
	repoDetails := make(map[string]RepoSummary)
	for _, repo := range repos {
		repoDetails[repo.RepoKey] = repo
	}

	var mappings []instance.ProjectRepository
	seen := make(map[string]bool)

	// Add repos explicitly assigned to projects
	for _, project := range projects {
		for _, repoKey := range project.Repos {
			key := project.Key + "/" + repoKey
			if seen[key] {
				continue
			}
			seen[key] = true

			mapping := instance.ProjectRepository{
				ProjectKey: project.Key,
				RepoKey:    repoKey,
			}
			// Fill in optional repo details if available
			if repoInfo, ok := repoDetails[repoKey]; ok {
				mapping.PackageType = repoInfo.PackageType
				mapping.RepoType = repoInfo.RepoType
			}
			mappings = append(mappings, mapping)
		}
	}

	// Sort by project key, then repo key for consistency
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].ProjectKey != mappings[j].ProjectKey {
			return mappings[i].ProjectKey < mappings[j].ProjectKey
		}
		return mappings[i].RepoKey < mappings[j].RepoKey
	})

	return mappings
}

// FlattenProjectRoles extracts all roles from all projects into a flat list
// for reporting purposes (one row per project + role combination).
type ProjectRoleDetail struct {
	ProjectKey   string
	DisplayName  string
	RoleName     string
	RoleType     string
	Environments []string
	Actions      []string
}

func FlattenProjectRoles(projects []instance.Project) []ProjectRoleDetail {
	var roles []ProjectRoleDetail
	for _, project := range projects {
		for _, role := range project.Roles {
			roles = append(roles, ProjectRoleDetail{
				ProjectKey:   project.Key,
				DisplayName:  project.DisplayName,
				RoleName:     role.Name,
				RoleType:     role.Type,
				Environments: role.Environments,
				Actions:      role.Actions,
			})
		}
	}
	// Sort by project key then role name
	sort.Slice(roles, func(i, j int) bool {
		if roles[i].ProjectKey != roles[j].ProjectKey {
			return roles[i].ProjectKey < roles[j].ProjectKey
		}
		return roles[i].RoleName < roles[j].RoleName
	})
	return roles
}
