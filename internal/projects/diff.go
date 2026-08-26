package projects

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

// DiffResult holds the result of comparing JFrog Projects between a source
// and target JPD.
type DiffResult struct {
	Source          []Project
	Target          []Project
	MissingOnTarget []Project // present on source, not on target
	ExtraOnTarget   []Project // present on target, not on source
}

// Diff lists projects on both source and target and reports which are
// missing on target and which are extra on target, compared by ProjectKey.
func Diff(source, target *jfclient.Client) (*DiffResult, error) {
	srcProjects, err := List(source)
	if err != nil {
		return nil, fmt.Errorf("list source projects: %w", err)
	}
	tgtProjects, err := List(target)
	if err != nil {
		return nil, fmt.Errorf("list target projects: %w", err)
	}
	return &DiffResult{
		Source:          srcProjects,
		Target:          tgtProjects,
		MissingOnTarget: MissingOnTarget(srcProjects, tgtProjects),
		ExtraOnTarget:   ExtraOnTarget(srcProjects, tgtProjects),
	}, nil
}

// MissingOnTarget returns the projects present in source but not in target,
// compared by ProjectKey.
func MissingOnTarget(source, target []Project) []Project {
	return diffByKey(source, target)
}

// ExtraOnTarget returns the projects present in target but not in source,
// compared by ProjectKey.
func ExtraOnTarget(source, target []Project) []Project {
	return diffByKey(target, source)
}

func diffByKey(a, b []Project) []Project {
	bKeys := make(map[string]bool, len(b))
	for _, p := range b {
		bKeys[p.ProjectKey] = true
	}
	var diff []Project
	for _, p := range a {
		if !bKeys[p.ProjectKey] {
			diff = append(diff, p)
		}
	}
	return diff
}

// CreateResult is the outcome of attempting to create one project on target.
type CreateResult struct {
	Key string
	Err error
}

// CreateMissing creates each project in missing on target, by fetching its
// full configuration from source (GET /access/api/v1/projects/{key}) and
// POSTing it to target (POST /access/api/v1/projects).
//
// If dryRun is true, CreateMissing performs no HTTP calls whatsoever and
// returns nil immediately.
func CreateMissing(source, target *jfclient.Client, missing []Project, dryRun bool) []CreateResult {
	if dryRun {
		return nil
	}
	results := make([]CreateResult, 0, len(missing))
	for _, p := range missing {
		results = append(results, CreateResult{Key: p.ProjectKey, Err: createProject(source, target, p.ProjectKey)})
	}
	return results
}

func createProject(source, target *jfclient.Client, key string) error {
	cfg, err := getProjectConfig(source, key)
	if err != nil {
		return err
	}
	resp, err := target.Post("/access/api/v1/projects", cfg)
	if err != nil {
		return fmt.Errorf("create project %s on target: %w", key, err)
	}
	return resp.Error(fmt.Sprintf("create project %s on target", key))
}

func getProjectConfig(c *jfclient.Client, key string) (map[string]interface{}, error) {
	resp, err := c.Get("/access/api/v1/projects/" + key)
	if err != nil {
		return nil, fmt.Errorf("get project %s config: %w", key, err)
	}
	if err := resp.Error(fmt.Sprintf("get project %s config", key)); err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := resp.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode project %s config: %w", key, err)
	}
	return cfg, nil
}

// DeleteResult is the outcome of attempting to delete one project.
type DeleteResult struct {
	Key        string
	StatusCode int
	Err        error
}

// DeleteExtra deletes each project in extra from target via
// DELETE /access/api/v1/projects/{key}.
//
// If dryRun is true, DeleteExtra performs no HTTP calls whatsoever and
// returns nil immediately. Callers are responsible for only invoking this
// with dryRun=false when the user has both explicitly requested
// --delete-extra and confirmed the action.
func DeleteExtra(target *jfclient.Client, extra []Project, dryRun bool) []DeleteResult {
	if dryRun {
		return nil
	}
	results := make([]DeleteResult, 0, len(extra))
	for _, p := range extra {
		results = append(results, deleteProject(target, p.ProjectKey))
	}
	return results
}

func deleteProject(c *jfclient.Client, key string) DeleteResult {
	resp, err := c.Delete("/access/api/v1/projects/" + key)
	if err != nil {
		return DeleteResult{Key: key, Err: err}
	}
	return DeleteResult{
		Key:        key,
		StatusCode: resp.StatusCode,
		Err:        resp.Error(fmt.Sprintf("delete project %s", key)),
	}
}
