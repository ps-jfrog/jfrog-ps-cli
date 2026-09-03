package xray

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// RepoJASConfig is a snapshot of the fields this CLI cares about within one
// repository's Xray repo-config: Contextual Analysis and the four Exposures
// scanner categories, plus scan-result retention. Pointers distinguish
// "field absent from the API response" (nil) from "field present and
// false" -- mirroring the source script's defensive `jq has(...)` checks,
// since not every repo type/Xray version exposes every field.
type RepoJASConfig struct {
	VulnContextualAnalysis *bool
	RetentionInDays        *int
	SecretsScan            *bool
	ServicesScan           *bool
	ApplicationsScan       *bool
	IacScan                *bool
}

// JASToggleOptions selects which fields to change on each repo. A nil field
// leaves that setting untouched. EnableExposures/EnableContextualAnalysis
// apply uniformly to every scanner category that is present in the repo's
// current config (matching the source script's "map every present scanner
// to the same value" behavior).
type JASToggleOptions struct {
	EnableExposures          *bool
	EnableContextualAnalysis *bool
	RetentionInDays          *int
}

// RepoJASToggleResult is the outcome of inspecting (and optionally updating)
// one repository's JAS/Contextual-Analysis configuration.
type RepoJASToggleResult struct {
	Repo       string
	Before     RepoJASConfig
	After      RepoJASConfig
	Changed    bool
	DryRun     bool
	StatusCode int
	Success    bool
	Error      string
}

// extractJASConfig reads the fields we report on out of a raw
// GET /xray/api/v1/repos_config/{repo} response.
func extractJASConfig(raw map[string]interface{}) RepoJASConfig {
	var cfg RepoJASConfig
	repoConfig, _ := raw["repo_config"].(map[string]interface{})
	if repoConfig == nil {
		return cfg
	}
	if v, ok := repoConfig["vuln_contextual_analysis"].(bool); ok {
		cfg.VulnContextualAnalysis = &v
	}
	if v, ok := repoConfig["retention_in_days"].(float64); ok {
		days := int(v)
		cfg.RetentionInDays = &days
	}
	exposures, _ := repoConfig["exposures"].(map[string]interface{})
	if exposures == nil {
		return cfg
	}
	scanners, _ := exposures["scanners_category"].(map[string]interface{})
	if scanners == nil {
		return cfg
	}
	if v, ok := scanners["secrets_scan"].(bool); ok {
		cfg.SecretsScan = &v
	}
	if v, ok := scanners["services_scan"].(bool); ok {
		cfg.ServicesScan = &v
	}
	if v, ok := scanners["applications_scan"].(bool); ok {
		cfg.ApplicationsScan = &v
	}
	if v, ok := scanners["iac_scan"].(bool); ok {
		cfg.IacScan = &v
	}
	return cfg
}

// applyJASToggle mutates raw's nested repo_config fields per opts, in place,
// only touching fields that already exist in the response (a field absent
// from a given repo/Xray version is left alone), and reports whether
// anything actually changed.
func applyJASToggle(raw map[string]interface{}, opts JASToggleOptions) bool {
	repoConfig, _ := raw["repo_config"].(map[string]interface{})
	if repoConfig == nil {
		return false
	}
	changed := false

	if opts.EnableContextualAnalysis != nil {
		if cur, ok := repoConfig["vuln_contextual_analysis"].(bool); ok && cur != *opts.EnableContextualAnalysis {
			repoConfig["vuln_contextual_analysis"] = *opts.EnableContextualAnalysis
			changed = true
		}
	}
	if opts.RetentionInDays != nil {
		if cur, ok := repoConfig["retention_in_days"].(float64); ok && int(cur) != *opts.RetentionInDays {
			repoConfig["retention_in_days"] = *opts.RetentionInDays
			changed = true
		}
	}
	if opts.EnableExposures != nil {
		if exposures, ok := repoConfig["exposures"].(map[string]interface{}); ok {
			if scanners, ok := exposures["scanners_category"].(map[string]interface{}); ok {
				for key, val := range scanners {
					if b, ok := val.(bool); ok && b != *opts.EnableExposures {
						scanners[key] = *opts.EnableExposures
						changed = true
					}
				}
			}
		}
	}
	return changed
}

// ToggleJAS inspects (and, unless dryRun, updates) the JAS/Contextual
// Analysis configuration of each repo via
// GET /xray/api/v1/repos_config/{repo} then
// PUT /xray/api/v1/repos_config (whole-object update: {repo_name,
// repo_config, repo_paths_config}, matching the source script's payload
// shape). When dryRun is true, only the GET is issued, for every repo.
func ToggleJAS(client *jfclient.Client, repos []string, opts JASToggleOptions, dryRun bool, onProgress ...progress.Reporter) []RepoJASToggleResult {
	return concurrency.One(repos, 0, func(repo string) RepoJASToggleResult {
		res := RepoJASToggleResult{Repo: repo, DryRun: dryRun}

		getResp, err := client.Get("/xray/api/v1/repos_config/" + repo)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		if err := getResp.Error("get repo config " + repo); err != nil {
			res.Error = err.Error()
			return res
		}
		var raw map[string]interface{}
		if err := getResp.Unmarshal(&raw); err != nil {
			res.Error = fmt.Sprintf("decode repo config: %v", err)
			return res
		}
		res.Before = extractJASConfig(raw)
		res.After = res.Before

		if dryRun {
			return res
		}

		if !applyJASToggle(raw, opts) {
			res.Success = true
			return res
		}
		res.Changed = true
		res.After = extractJASConfig(raw)

		payload := map[string]interface{}{
			"repo_name":         raw["repo_name"],
			"repo_config":       raw["repo_config"],
			"repo_paths_config": raw["repo_paths_config"],
		}
		putResp, err := client.Put("/xray/api/v1/repos_config", payload)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.StatusCode = putResp.StatusCode
		if putResp.OK() {
			res.Success = true
		} else {
			res.Error = putResp.Error("update repo config").Error()
		}
		return res
	}, onProgress...)
}

// ListRepositoriesForProject lists repository keys assigned to a JFrog
// Project via GET /artifactory/api/repositories?project=<key>.
func ListRepositoriesForProject(client *jfclient.Client, projectKey string) ([]string, error) {
	resp, err := client.Get("/artifactory/api/repositories?project=" + url.QueryEscape(projectKey))
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list repositories for project " + projectKey); err != nil {
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

// ReadRepoNamesFile reads newline-delimited repo keys from path, skipping
// blank lines and "#"-prefixed comments.
func ReadRepoNamesFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open repo list %s: %w", path, err)
	}
	defer f.Close()

	var keys []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keys = append(keys, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read repo list %s: %w", path, err)
	}
	return keys, nil
}
