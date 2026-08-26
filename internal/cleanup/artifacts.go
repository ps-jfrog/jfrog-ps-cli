// Package cleanup provides pure, testable business logic for bulk-deleting
// repositories and artifacts, diffing artifacts between two servers, and
// detecting inactive users -- all safety-first (dry-run by default), with no
// flag/env parsing. See internal/cli/cleanup for the CLI wiring.
package cleanup

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// ArtifactMatch is one item found by an AQL query against Artifactory, e.g.
// via items.find({...}). Only the fields used by the cleanup commands are
// modeled here.
type ArtifactMatch struct {
	Repo       string `json:"repo"`
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Created    string `json:"created"`
	Modified   string `json:"modified"`
	CreatedBy  string `json:"created_by"`
	ModifiedBy string `json:"modified_by"`
	SHA256     string `json:"sha256"`
}

// aqlResponse is the shape of a response from /artifactory/api/search/aql.
type aqlResponse struct {
	Results []ArtifactMatch `json:"results"`
}

// FindArtifacts runs an AQL query against client and returns the matching
// artifacts.
func FindArtifacts(client *jfclient.Client, query string) ([]ArtifactMatch, error) {
	resp, err := client.AQL(query)
	if err != nil {
		return nil, fmt.Errorf("run AQL query: %w", err)
	}
	if err := resp.Error("run AQL query"); err != nil {
		return nil, err
	}
	var out aqlResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("parse AQL response: %w", err)
	}
	return out.Results, nil
}

// ArtifactPath builds the repository-relative deployment path for a match,
// e.g. repo="my-repo" path="foo/bar" name="baz.txt" ->
// "my-repo/foo/bar/baz.txt". An empty (or AQL root ".") path is skipped, so
// an item at the root of a repo becomes "my-repo/baz.txt".
func ArtifactPath(m ArtifactMatch) string {
	parts := []string{m.Repo}
	if m.Path != "" && m.Path != "." {
		parts = append(parts, m.Path)
	}
	parts = append(parts, m.Name)
	return strings.Join(parts, "/")
}

// DeleteResult is the outcome of attempting to delete one artifact.
type DeleteResult struct {
	Match   ArtifactMatch
	Path    string
	DryRun  bool
	Success bool
	Error   string
}

// DeleteArtifacts deletes each match via DELETE /artifactory/{repo/path/name}.
// When dryRun is true, no HTTP DELETE calls are issued at all -- every result
// is returned with DryRun=true and Success=false, purely describing what
// would have been deleted.
func DeleteArtifacts(client *jfclient.Client, matches []ArtifactMatch, dryRun bool) []DeleteResult {
	results := make([]DeleteResult, 0, len(matches))
	for _, m := range matches {
		p := ArtifactPath(m)
		res := DeleteResult{Match: m, Path: p, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}
		resp, err := client.Delete("/artifactory/" + p)
		switch {
		case err != nil:
			res.Error = err.Error()
		case !resp.OK():
			res.Error = resp.Error("delete artifact").Error()
		default:
			res.Success = true
		}
		results = append(results, res)
	}
	return results
}

// WriteArtifactsCSV writes matches to path as CSV with a header row.
func WriteArtifactsCSV(path string, matches []ArtifactMatch) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create csv %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"repo", "path", "name", "size", "created", "modified", "created_by", "modified_by"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, m := range matches {
		row := []string{
			m.Repo, m.Path, m.Name, strconv.FormatInt(m.Size, 10),
			m.Created, m.Modified, m.CreatedBy, m.ModifiedBy,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv %s: %w", path, err)
	}
	return nil
}
