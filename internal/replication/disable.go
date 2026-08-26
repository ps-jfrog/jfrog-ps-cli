package replication

import (
	"bytes"
	"encoding/json"
	"fmt"

	"jfrogpscli/internal/jfclient"
)

// DisableResult is the outcome of attempting to disable replication for one
// repo.
type DisableResult struct {
	Repo    string
	Success bool
	Err     error
}

// disablePayload takes the raw GET response body for a replication config
// (which may be a single object or an array of objects, depending on server
// version) and returns an equivalent body with enabled=false set throughout.
func disablePayload(raw []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []map[string]interface{}
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, err
		}
		for i := range arr {
			arr[i]["enabled"] = false
		}
		return json.Marshal(arr)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	obj["enabled"] = false
	return json.Marshal(obj)
}

// Disable fetches the existing replication config for each repo, sets
// enabled=false (on every element if the config is an array), and writes it
// back. In dry-run mode no GET/PUT calls are made.
func Disable(c *jfclient.Client, repos []string, dryRun bool) []DisableResult {
	results := make([]DisableResult, 0, len(repos))
	for _, repo := range repos {
		if dryRun {
			results = append(results, DisableResult{Repo: repo, Success: true})
			continue
		}

		getResp, err := c.Get("/artifactory/api/replications/" + repo)
		if err != nil {
			results = append(results, DisableResult{Repo: repo, Err: err})
			continue
		}
		if err := getResp.Error("get replication config for " + repo); err != nil {
			results = append(results, DisableResult{Repo: repo, Err: err})
			continue
		}

		newBody, err := disablePayload(getResp.Body)
		if err != nil {
			results = append(results, DisableResult{Repo: repo, Err: fmt.Errorf("parse replication config for %s: %w", repo, err)})
			continue
		}

		putResp, err := c.Put("/artifactory/api/replications/"+repo, json.RawMessage(newBody))
		if err != nil {
			results = append(results, DisableResult{Repo: repo, Err: err})
			continue
		}
		if err := putResp.Error("disable replication for " + repo); err != nil {
			results = append(results, DisableResult{Repo: repo, Err: err})
			continue
		}
		results = append(results, DisableResult{Repo: repo, Success: true})
	}
	return results
}
