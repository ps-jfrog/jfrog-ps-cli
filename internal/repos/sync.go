package repos

import (
	"fmt"
	"reflect"
	"sort"

	"jfrogpscli/internal/jfclient"
)

// ConfigDiffResult summarizes, among repos present on both source and
// target, which have differing full configurations.
type ConfigDiffResult struct {
	Common    []string // keys present on both sides, sorted
	Differing []string // subset of Common whose config differs, in Common order
}

// SyncConfigDiff lists repos of repoType on both source and target, and for
// every repo present on both sides (the intersection by Key) fetches and
// compares the full repository configuration, reporting which differ.
func SyncConfigDiff(source, target *jfclient.Client, repoType string) (*ConfigDiffResult, error) {
	srcRepos, err := List(source, repoType)
	if err != nil {
		return nil, fmt.Errorf("list source repositories: %w", err)
	}
	tgtRepos, err := List(target, repoType)
	if err != nil {
		return nil, fmt.Errorf("list target repositories: %w", err)
	}

	result := &ConfigDiffResult{Common: IntersectKeys(srcRepos, tgtRepos)}
	for _, key := range result.Common {
		differs, err := configDiffers(source, target, key)
		if err != nil {
			return nil, err
		}
		if differs {
			result.Differing = append(result.Differing, key)
		}
	}
	return result, nil
}

func configDiffers(source, target *jfclient.Client, key string) (bool, error) {
	srcCfg, err := GetRepoConfig(source, key)
	if err != nil {
		return false, err
	}
	tgtCfg, err := GetRepoConfig(target, key)
	if err != nil {
		return false, err
	}
	return !ConfigsEqual(srcCfg, tgtCfg), nil
}

// IntersectKeys returns the sorted keys present in both source and target.
func IntersectKeys(source, target []Repo) []string {
	tgtKeys := make(map[string]bool, len(target))
	for _, r := range target {
		tgtKeys[r.Key] = true
	}
	var common []string
	for _, r := range source {
		if tgtKeys[r.Key] {
			common = append(common, r.Key)
		}
	}
	sort.Strings(common)
	return common
}

// GetRepoConfig fetches a repository's full configuration as a generic map,
// decoded from GET /artifactory/api/repositories/{key}.
func GetRepoConfig(c *jfclient.Client, key string) (map[string]interface{}, error) {
	resp, err := c.Get("/artifactory/api/repositories/" + key)
	if err != nil {
		return nil, fmt.Errorf("get repository %s config: %w", key, err)
	}
	if err := resp.Error(fmt.Sprintf("get repository %s config", key)); err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := resp.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decode repository %s config: %w", key, err)
	}
	return cfg, nil
}

// ConfigsEqual reports whether two decoded JSON config maps are equal. Both
// maps are decoded from JSON into map[string]interface{}, so field order
// never affects the comparison -- reflect.DeepEqual compares the decoded
// key/value structure recursively, which is exactly the semantics we want
// for "same config, possibly serialized with fields in a different order".
func ConfigsEqual(a, b map[string]interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// UpdateResult is the outcome of pushing one repo's source configuration to
// the target.
type UpdateResult struct {
	Key string
	Err error
}

// ApplyConfigSync updates each differing repo's configuration on target to
// match source, via POST /artifactory/api/repositories/{key}.
//
// If dryRun is true, ApplyConfigSync performs no HTTP calls whatsoever and
// returns nil immediately.
func ApplyConfigSync(source, target *jfclient.Client, differing []string, dryRun bool) []UpdateResult {
	if dryRun {
		return nil
	}
	results := make([]UpdateResult, 0, len(differing))
	for _, key := range differing {
		results = append(results, UpdateResult{Key: key, Err: pushConfig(source, target, key)})
	}
	return results
}

func pushConfig(source, target *jfclient.Client, key string) error {
	cfg, err := GetRepoConfig(source, key)
	if err != nil {
		return err
	}
	resp, err := target.Post("/artifactory/api/repositories/"+key, cfg)
	if err != nil {
		return fmt.Errorf("update repository %s on target: %w", key, err)
	}
	return resp.Error(fmt.Sprintf("update repository %s on target", key))
}
