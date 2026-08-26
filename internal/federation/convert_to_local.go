package federation

import (
	"fmt"

	"jfrogpscli/internal/jfclient"
)

// DefaultTempRepoSuffix is appended to a repo's key to name the temporary
// local repo used during ConvertToLocal when --temp-repo is not given.
const DefaultTempRepoSuffix = "-temp"

// TempRepoName returns tempRepo if non-empty, else repo+DefaultTempRepoSuffix.
func TempRepoName(repo, tempRepo string) string {
	if tempRepo != "" {
		return tempRepo
	}
	return repo + DefaultTempRepoSuffix
}

// CopyFailure records one artifact that failed to copy during a
// ConvertToLocal migration.
type CopyFailure struct {
	Item string
	Err  error
}

// ConvertToLocalPlan describes what ConvertToLocal would do, for dry-run
// reporting. Building a plan only issues read-only calls (get config, AQL).
type ConvertToLocalPlan struct {
	Repo     string
	TempRepo string
	Items    []string
}

// PlanConvertToLocal lists the artifacts currently in repo so the caller can
// print a dry-run plan. It makes no mutating calls.
func PlanConvertToLocal(c *jfclient.Client, repo, tempRepo string) (ConvertToLocalPlan, error) {
	items, err := listArtifactItems(c, repo)
	if err != nil {
		return ConvertToLocalPlan{}, err
	}
	return ConvertToLocalPlan{Repo: repo, TempRepo: TempRepoName(repo, tempRepo), Items: items}, nil
}

// ConvertToLocalResult summarizes the outcome of a ConvertToLocal run.
type ConvertToLocalResult struct {
	Repo         string
	TempRepo     string
	ItemCount    int
	CopyFailures []CopyFailure
	Completed    bool
	Message      string
}

// ConvertToLocal converts a federated repo to a local repo without losing
// its content:
//
//  1. Read the federated repo's config.
//  2. Create a temporary local repo (same config minus "members", rclass
//     "local") and copy every artifact from repo into it.
//  3. If (and only if) every copy in step 2 succeeded: delete the federated
//     repo and recreate it as local with the same config.
//  4. Copy every artifact back from the temp repo into the (now local) repo.
//
// The temp repo is never auto-deleted; the caller is told to remove it once
// they've verified the migration. If any copy in step 2 fails, the function
// stops before touching the source repo at all -- data is never stranded
// with a half-done migration.
func ConvertToLocal(c *jfclient.Client, repo, tempRepo string) (ConvertToLocalResult, error) {
	tempRepo = TempRepoName(repo, tempRepo)
	result := ConvertToLocalResult{Repo: repo, TempRepo: tempRepo}

	// a. Read the federated repo's config.
	cfg, err := getRepoConfig(c, repo)
	if err != nil {
		return result, err
	}

	// b. Create the temp local repo.
	tempCfg := buildLocalConfig(cfg, tempRepo)
	if err := putRepoConfig(c, tempRepo, tempCfg); err != nil {
		return result, fmt.Errorf("create temp repo %s: %w", tempRepo, err)
	}

	// c. Copy every artifact from repo into the temp repo.
	items, err := listArtifactItems(c, repo)
	if err != nil {
		return result, fmt.Errorf("list artifacts in %s: %w", repo, err)
	}
	result.ItemCount = len(items)

	failures := copyAll(c, repo, items, tempRepo)
	if len(failures) > 0 {
		result.CopyFailures = failures
		result.Message = fmt.Sprintf(
			"aborted before touching %s: %d of %d artifact copies to temp repo %s failed; fix the issue and retry (temp repo %s was created but is incomplete)",
			repo, len(failures), len(items), tempRepo, tempRepo,
		)
		return result, fmt.Errorf("%d artifact copy failure(s) while copying %s to %s; aborting", len(failures), repo, tempRepo)
	}

	// d. Only now: delete the federated repo and recreate it as local.
	if err := deleteRepo(c, repo); err != nil {
		return result, fmt.Errorf("delete federated repo %s: %w", repo, err)
	}
	localCfg := buildLocalConfig(cfg, repo)
	if err := putRepoConfig(c, repo, localCfg); err != nil {
		return result, fmt.Errorf("recreate %s as local: %w", repo, err)
	}

	// e. Copy artifacts back from the temp repo into the newly-local repo.
	backFailures := copyAll(c, tempRepo, items, repo)
	if len(backFailures) > 0 {
		result.CopyFailures = backFailures
		result.Message = fmt.Sprintf(
			"%s was recreated as local, but %d of %d artifacts failed to copy back from %s; retry the missing items manually before deleting %s",
			repo, len(backFailures), len(items), tempRepo, tempRepo,
		)
		return result, fmt.Errorf("%d artifact copy failure(s) copying back from %s to %s", len(backFailures), tempRepo, repo)
	}

	result.Completed = true
	result.Message = fmt.Sprintf(
		"%s has been converted to a local repo. Temp repo %s still exists with a full copy of the data; delete it manually once you've verified the migration.",
		repo, tempRepo,
	)
	return result, nil
}

func getRepoConfig(c *jfclient.Client, repo string) (map[string]interface{}, error) {
	resp, err := c.Get("/artifactory/api/repositories/" + repo)
	if err != nil {
		return nil, fmt.Errorf("get repository config for %s: %w", repo, err)
	}
	if err := resp.Error("get repository config for " + repo); err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := resp.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parse repository config for %s: %w", repo, err)
	}
	return cfg, nil
}

// buildLocalConfig copies cfg, strips the "members" key, forces rclass to
// "local", and sets key to newKey.
func buildLocalConfig(cfg map[string]interface{}, newKey string) map[string]interface{} {
	out := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	delete(out, "members")
	out["rclass"] = "local"
	out["key"] = newKey
	return out
}

func putRepoConfig(c *jfclient.Client, repo string, cfg map[string]interface{}) error {
	resp, err := c.Put("/artifactory/api/repositories/"+repo, cfg)
	if err != nil {
		return err
	}
	return resp.Error("create/update repository config for " + repo)
}

func deleteRepo(c *jfclient.Client, repo string) error {
	resp, err := c.Delete("/artifactory/api/repositories/" + repo)
	if err != nil {
		return err
	}
	return resp.Error("delete repository " + repo)
}

type aqlResponse struct {
	Results []struct {
		Path string `json:"path"`
		Name string `json:"name"`
	} `json:"results"`
}

// listArtifactItems runs an AQL query listing every artifact in repo and
// returns their repo-relative paths (e.g. "some/dir/file.txt").
func listArtifactItems(c *jfclient.Client, repo string) ([]string, error) {
	query := fmt.Sprintf(`items.find({"repo":"%s"}).include("path","name")`, repo)
	resp, err := c.AQL(query)
	if err != nil {
		return nil, fmt.Errorf("aql list artifacts in %s: %w", repo, err)
	}
	if err := resp.Error("aql list artifacts in " + repo); err != nil {
		return nil, err
	}
	var result aqlResponse
	if err := resp.Unmarshal(&result); err != nil {
		return nil, fmt.Errorf("parse aql results for %s: %w", repo, err)
	}
	items := make([]string, 0, len(result.Results))
	for _, r := range result.Results {
		if r.Path == "" || r.Path == "." {
			items = append(items, r.Name)
		} else {
			items = append(items, r.Path+"/"+r.Name)
		}
	}
	return items, nil
}

func copyItem(c *jfclient.Client, srcRepo, item, dstRepo string) error {
	path := fmt.Sprintf("/artifactory/api/copy/%s/%s?to=%s/%s", srcRepo, item, dstRepo, item)
	resp, err := c.Post(path, nil)
	if err != nil {
		return err
	}
	return resp.Error(fmt.Sprintf("copy %s/%s to %s", srcRepo, item, dstRepo))
}

func copyAll(c *jfclient.Client, srcRepo string, items []string, dstRepo string) []CopyFailure {
	var failures []CopyFailure
	for _, item := range items {
		if err := copyItem(c, srcRepo, item, dstRepo); err != nil {
			failures = append(failures, CopyFailure{Item: item, Err: err})
		}
	}
	return failures
}
