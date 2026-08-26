// Package projects provides pure, testable business logic for listing,
// diffing, and deleting JFrog Projects across one or two JFrog Platform
// Deployments. It has no CLI or flag-parsing concerns -- see
// internal/cli/projects for the command-line wrapper.
package projects

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

// Project is a summary of a JFrog Project as returned by
// GET /access/api/v1/projects.
type Project struct {
	ProjectKey  string `json:"project_key"`
	DisplayName string `json:"display_name"`
}

// List returns every JFrog Project from c.
//
// Assumption: GET /access/api/v1/projects returns a plain JSON array of
// project objects (e.g. [{"project_key":"...","display_name":"..."}, ...])
// rather than wrapping the array in an envelope object. This matches the
// documented Access API v1 "get all projects" response shape; if a given
// platform version instead wraps it (e.g. {"projects": [...]}), List's
// Unmarshal call would need a small adjustment.
func List(c *jfclient.Client) ([]Project, error) {
	resp, err := c.Get("/access/api/v1/projects")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	if err := resp.Error("list projects"); err != nil {
		return nil, err
	}
	var list []Project
	if err := resp.Unmarshal(&list); err != nil {
		return nil, fmt.Errorf("decode projects response: %w", err)
	}
	return list, nil
}
