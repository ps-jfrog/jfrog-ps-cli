// Package xray implements the business logic behind "jfps xray ..." --
// Xray repository indexing, report management, and policy/watch/ignore-rule
// synchronization between two JFrog Platform Deployments. All HTTP access
// goes through jfclient.Client so the whole package is testable against
// net/http/httptest servers.
package xray

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/jfclient"
)

// indexedReposResponse models the response of
// GET /xray/api/v1/binMgr/default/repos.
//
// ASSUMPTION: the exact response shape isn't documented in a machine-readable
// way here, so we model it as the simplest plausible shape -- a JSON object
// with an "indexed_repos" array of repository keys.
type indexedReposResponse struct {
	IndexedRepos []string `json:"indexed_repos"`
}

// repoSummary models one element of the array returned by
// GET /artifactory/api/repositories[?type=...]. Only the fields we need are
// modeled; the real response has many more.
type repoSummary struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}

// RepoIndexStatus is the indexing state of a single repository.
type RepoIndexStatus struct {
	Repo    string
	Indexed bool
}

// ListRepositories returns the keys of all repositories of the given type
// (e.g. "local", "remote", "virtual", "federated") via
// GET /artifactory/api/repositories?type=<type>. An empty repoType lists all
// repositories.
func ListRepositories(client *jfclient.Client, repoType string) ([]string, error) {
	path := "/artifactory/api/repositories"
	if repoType != "" {
		path += "?type=" + url.QueryEscape(repoType)
	}
	resp, err := client.Get(path)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list repositories"); err != nil {
		return nil, err
	}
	var repos []repoSummary
	if err := resp.Unmarshal(&repos); err != nil {
		return nil, fmt.Errorf("decode repositories response: %w", err)
	}
	keys := make([]string, 0, len(repos))
	for _, r := range repos {
		keys = append(keys, r.Key)
	}
	return keys, nil
}

// IndexedRepos returns the set of repository keys already indexed by the
// default binary manager, via GET /xray/api/v1/binMgr/default/repos.
func IndexedRepos(client *jfclient.Client) ([]string, error) {
	resp, err := client.Get("/xray/api/v1/binMgr/default/repos")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list indexed repos"); err != nil {
		return nil, err
	}
	var out indexedReposResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("decode indexed repos response: %w", err)
	}
	return out.IndexedRepos, nil
}

// IndexStatus reports the indexing status of every repository of repoType,
// combining GET /xray/api/v1/binMgr/default/repos with
// GET /artifactory/api/repositories?type=<repoType>. Read-only: issues no
// mutating calls.
func IndexStatus(client *jfclient.Client, repoType string) ([]RepoIndexStatus, error) {
	indexed, err := IndexedRepos(client)
	if err != nil {
		return nil, err
	}
	indexedSet := make(map[string]bool, len(indexed))
	for _, r := range indexed {
		indexedSet[r] = true
	}

	repos, err := ListRepositories(client, repoType)
	if err != nil {
		return nil, err
	}

	out := make([]RepoIndexStatus, 0, len(repos))
	for _, r := range repos {
		out = append(out, RepoIndexStatus{Repo: r, Indexed: indexedSet[r]})
	}
	return out, nil
}

// ToggleResult is the outcome of attempting to enable/disable Xray indexing
// on a single repository.
type ToggleResult struct {
	Repo       string
	Enable     bool
	DryRun     bool
	StatusCode int
	Success    bool
	Error      string
}

// ToggleIndexing enables or disables Xray indexing on each repo via
// POST /artifactory/api/repositories/{repo} with body {"xrayIndex": enable}.
// When dryRun is true, no HTTP POST calls are issued at all.
func ToggleIndexing(client *jfclient.Client, repos []string, enable bool, dryRun bool) []ToggleResult {
	results := make([]ToggleResult, 0, len(repos))
	for _, repo := range repos {
		res := ToggleResult{Repo: repo, Enable: enable, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}
		resp, err := client.Post("/artifactory/api/repositories/"+repo, map[string]bool{"xrayIndex": enable})
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.StatusCode = resp.StatusCode
		if resp.OK() {
			res.Success = true
		} else {
			res.Error = resp.Error("toggle indexing").Error()
		}
		results = append(results, res)
	}
	return results
}

// Reindex modes.
const (
	ReindexModeIndex   = "index"
	ReindexModeForce   = "force"
	ReindexModeScanNow = "scan-now"
)

// ReindexResult is the outcome of a single reindex/rescan request.
type ReindexResult struct {
	Repo       string
	Mode       string
	Endpoint   string
	DryRun     bool
	StatusCode int
	Success    bool
	Error      string
}

// Reindex triggers repository (re)indexing/scanning for repo using one of
// three modes:
//
//   - "index":    POST /xray/api/v1/index/repository/{repo} (no body). The
//     standard way to add a newly-indexed repo to Xray's index.
//   - "force":    POST /xray/api/v1/forceReindex with
//     {"repos":["repo"],"is_non_indexed_repo_allowed":true}. Requires Xray
//     Admin privileges and re-scans every artifact in the repo -- this can be
//     very expensive on large repositories.
//   - "scan-now": POST /xray/api/v2/index with {"repos":["repo"]}. Also
//     requires Xray Admin privileges and can be expensive on large repos.
//
// When dryRun is true (the default from the CLI), no HTTP call is issued;
// the result describes what would have been called.
func Reindex(client *jfclient.Client, repo string, mode string, dryRun bool) (ReindexResult, error) {
	var (
		method = "POST"
		path   string
		body   interface{}
	)
	switch mode {
	case ReindexModeIndex:
		path = "/xray/api/v1/index/repository/" + repo
	case ReindexModeForce:
		path = "/xray/api/v1/forceReindex"
		body = map[string]interface{}{
			"repos":                       []string{repo},
			"is_non_indexed_repo_allowed": true,
		}
	case ReindexModeScanNow:
		path = "/xray/api/v2/index"
		body = map[string]interface{}{"repos": []string{repo}}
	default:
		return ReindexResult{}, fmt.Errorf("unknown reindex mode %q (want %q, %q, or %q)", mode, ReindexModeIndex, ReindexModeForce, ReindexModeScanNow)
	}

	res := ReindexResult{Repo: repo, Mode: mode, Endpoint: path, DryRun: dryRun}
	if dryRun {
		return res, nil
	}

	resp, err := client.Do(method, path, body)
	if err != nil {
		res.Error = err.Error()
		return res, nil
	}
	res.StatusCode = resp.StatusCode
	if resp.OK() {
		res.Success = true
	} else {
		res.Error = resp.Error("reindex").Error()
	}
	return res, nil
}
