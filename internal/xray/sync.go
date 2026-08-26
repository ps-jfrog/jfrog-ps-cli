package xray

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/jfclient"
)

// SyncResult is the outcome of copying one policy/watch/ignore-rule from a
// source server to a target server.
type SyncResult struct {
	Name       string
	ProjectKey string
	DryRun     bool
	StatusCode int
	Success    bool
	Error      string
}

// postToTarget issues the POST that copies one entity to target and records
// the outcome on res. It never runs when res.DryRun is true -- callers must
// check DryRun before calling this.
func postToTarget(target *jfclient.Client, path string, body interface{}, res *SyncResult) {
	resp, err := target.Post(path, body)
	if err != nil {
		res.Error = err.Error()
		return
	}
	res.StatusCode = resp.StatusCode
	if resp.OK() {
		res.Success = true
	} else {
		res.Error = resp.Error("sync").Error()
	}
}

// withoutKey returns a shallow copy of m with key removed.
func withoutKey(m map[string]interface{}, key string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == key {
			continue
		}
		out[k] = v
	}
	return out
}

// SyncPolicies copies every Xray policy from source to target via
// GET /xray/api/v2/policies (source) and POST /xray/api/v2/policies
// (target). Policies are modeled minimally as map[string]interface{} since
// the full schema isn't needed here.
//
// If a policy carries a non-empty top-level "project_key" field, that field
// is stripped from the copied body and the target POST is made with
// ?projectKey=<value> instead (project-scoped policies aren't created by
// embedding the key in the body). Global policies are posted as-is.
//
// When dryRun is true (the default from the CLI), no POST calls are issued
// against target at all -- the source GET still happens so the caller can
// report what *would* be copied.
func SyncPolicies(source, target *jfclient.Client, dryRun bool) ([]SyncResult, error) {
	resp, err := source.Get("/xray/api/v2/policies")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list source policies"); err != nil {
		return nil, err
	}
	var policies []map[string]interface{}
	if err := resp.Unmarshal(&policies); err != nil {
		return nil, fmt.Errorf("decode policies response: %w", err)
	}

	results := make([]SyncResult, 0, len(policies))
	for _, p := range policies {
		name, _ := p["name"].(string)
		projectKey, _ := p["project_key"].(string)
		res := SyncResult{Name: name, ProjectKey: projectKey, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}

		path := "/xray/api/v2/policies"
		body := interface{}(p)
		if projectKey != "" {
			path += "?projectKey=" + url.QueryEscape(projectKey)
			body = withoutKey(p, "project_key")
		}
		postToTarget(target, path, body, &res)
		results = append(results, res)
	}
	return results, nil
}

// SyncWatches copies every Xray watch from source to target via
// GET /xray/api/v2/watches (source) and POST /xray/api/v2/watches (target).
//
// The project-key field for a watch, when present, lives nested at
// general_data.project_key rather than at the top level. When non-empty,
// that nested field is stripped and the target POST is made with
// ?projectKey=<value>; global watches (no general_data.project_key) are
// posted as-is.
func SyncWatches(source, target *jfclient.Client, dryRun bool) ([]SyncResult, error) {
	resp, err := source.Get("/xray/api/v2/watches")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list source watches"); err != nil {
		return nil, err
	}
	var watches []map[string]interface{}
	if err := resp.Unmarshal(&watches); err != nil {
		return nil, fmt.Errorf("decode watches response: %w", err)
	}

	results := make([]SyncResult, 0, len(watches))
	for _, w := range watches {
		name, _ := w["name"].(string)
		var projectKey string
		if gd, ok := w["general_data"].(map[string]interface{}); ok {
			projectKey, _ = gd["project_key"].(string)
		}
		res := SyncResult{Name: name, ProjectKey: projectKey, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}

		path := "/xray/api/v2/watches"
		body := interface{}(w)
		if projectKey != "" {
			path += "?projectKey=" + url.QueryEscape(projectKey)
			gd, _ := w["general_data"].(map[string]interface{})
			newBody := withoutKey(w, "general_data")
			newBody["general_data"] = withoutKey(gd, "project_key")
			body = newBody
		}
		postToTarget(target, path, body, &res)
		results = append(results, res)
	}
	return results, nil
}

// SyncIgnoreRules copies every Xray ignore rule from source to target via
// GET /xray/api/v1/ignore_rules (source) and POST /xray/api/v1/ignore_rules
// (target).
//
// ASSUMPTION: the ignore-rule schema isn't fully documented here, so the
// project-key field is treated the same way as for policies -- a top-level
// "project_key" field, stripped from the body and passed as
// ?projectKey=<value> when non-empty.
//
// The target client passed in is always used for every POST, whether the
// entity is global or project-scoped -- unlike the original bash script,
// there is no hardcoded server ID/alias anywhere in this path.
func SyncIgnoreRules(source, target *jfclient.Client, dryRun bool) ([]SyncResult, error) {
	resp, err := source.Get("/xray/api/v1/ignore_rules")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list source ignore rules"); err != nil {
		return nil, err
	}
	var rules []map[string]interface{}
	if err := resp.Unmarshal(&rules); err != nil {
		return nil, fmt.Errorf("decode ignore rules response: %w", err)
	}

	results := make([]SyncResult, 0, len(rules))
	for _, r := range rules {
		name, _ := r["name"].(string)
		if name == "" {
			// Ignore rules may be identified by "id" rather than "name" --
			// fall back so the report still identifies the entity.
			if id, ok := r["id"].(string); ok {
				name = id
			}
		}
		projectKey, _ := r["project_key"].(string)
		res := SyncResult{Name: name, ProjectKey: projectKey, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}

		path := "/xray/api/v1/ignore_rules"
		body := interface{}(r)
		if projectKey != "" {
			path += "?projectKey=" + url.QueryEscape(projectKey)
			body = withoutKey(r, "project_key")
		}
		postToTarget(target, path, body, &res)
		results = append(results, res)
	}
	return results, nil
}
