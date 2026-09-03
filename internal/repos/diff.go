package repos

import (
	"fmt"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// DiffResult holds the result of comparing repositories of a given type
// between a source and target JPD.
type DiffResult struct {
	Source          []Repo
	Target          []Repo
	MissingOnTarget []Repo
}

// Diff lists repositories of repoType on both source and target and reports
// those present on source but missing on target, compared by Key.
//
// Note: repos diff intentionally never deletes anything -- there is no
// "extra on target" handling here and no delete branch at all, mirroring
// the safety choice made by the original ad hoc scripts this replaces.
func Diff(source, target *jfclient.Client, repoType string) (*DiffResult, error) {
	srcRepos, err := List(source, repoType)
	if err != nil {
		return nil, fmt.Errorf("list source repositories: %w", err)
	}
	tgtRepos, err := List(target, repoType)
	if err != nil {
		return nil, fmt.Errorf("list target repositories: %w", err)
	}
	return &DiffResult{
		Source:          srcRepos,
		Target:          tgtRepos,
		MissingOnTarget: MissingOnTarget(srcRepos, tgtRepos),
	}, nil
}

// MissingOnTarget returns the repos present in source but not in target,
// compared by Key. Pure function, independently testable without HTTP.
func MissingOnTarget(source, target []Repo) []Repo {
	tgtKeys := make(map[string]bool, len(target))
	for _, r := range target {
		tgtKeys[r.Key] = true
	}
	var missing []Repo
	for _, r := range source {
		if !tgtKeys[r.Key] {
			missing = append(missing, r)
		}
	}
	return missing
}

// CreateResult is the outcome of attempting to create one repository on the
// target by copying its configuration from the source.
type CreateResult struct {
	Key string
	Err error
}

// CopyMissing creates each repo in missing on target by fetching its full
// configuration from source (stripping the projectKey field, which is
// server-specific) and PUTting it to target.
//
// If dryRun is true, CopyMissing performs no HTTP calls whatsoever -- not
// even reads -- and returns nil immediately.
func CopyMissing(source, target *jfclient.Client, missing []Repo, dryRun bool, onProgress ...progress.Reporter) []CreateResult {
	if dryRun {
		return nil
	}
	return concurrency.One(missing, 0, func(r Repo) CreateResult {
		return CreateResult{Key: r.Key, Err: copyRepoConfig(source, target, r.Key)}
	}, onProgress...)
}

func copyRepoConfig(source, target *jfclient.Client, key string) error {
	cfg, err := GetRepoConfig(source, key)
	if err != nil {
		return err
	}
	delete(cfg, "projectKey")

	putResp, err := target.Put("/artifactory/api/repositories/"+key, cfg)
	if err != nil {
		return fmt.Errorf("create repository %s on target: %w", key, err)
	}
	return putResp.Error(fmt.Sprintf("create repository %s on target", key))
}
