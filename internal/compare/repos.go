// Package compare provides pure, testable business logic for verifying
// migrations between two JFrog Platform Deployments (JPDs): repository
// contents, build-info, and generic JSON/config diffs. See
// internal/cli/compare for the CLI wiring.
package compare

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// skipFilenames lists exact filenames that are Artifactory-generated
// package-index files and must never be part of a repo content comparison
// (they are regenerated independently on each side and routinely differ).
var skipFilenames = []string{
	"maven-metadata.xml",
	"Packages",
	"repomd.xml",
}

// skipPathContains lists substrings that, if present anywhere in an item's
// full path+name, mark it as a generated index file to skip.
var skipPathContains = []string{
	".npm",
	".pypi",
	".composer",
}

// skipPathSuffixes lists suffixes that, if present on an item's full
// path+name, mark it as a generated index file to skip.
var skipPathSuffixes = []string{
	"index.yaml",
}

// shouldSkipItem reports whether an item (identified by its AQL path and
// name fields) is a known Artifactory-generated/package-index file that
// should be excluded from repo content comparisons on both sides.
func shouldSkipItem(path, name string) bool {
	for _, f := range skipFilenames {
		if name == f {
			return true
		}
	}
	full := path + "/" + name
	for _, sub := range skipPathContains {
		if strings.Contains(full, sub) {
			return true
		}
	}
	for _, suf := range skipPathSuffixes {
		if strings.HasSuffix(full, suf) {
			return true
		}
	}
	return false
}

// RepoItem is one artifact returned by an AQL items.find query, restricted
// to the fields needed for content comparison.
type RepoItem struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Key returns the map key used to identify an item within a repo,
// independent of which side (source/target) it came from. AQL represents an
// item at the root of a repo with path ".", which is normalized away here.
func (i RepoItem) Key() string {
	if i.Path == "" || i.Path == "." {
		return i.Name
	}
	return i.Path + "/" + i.Name
}

// aqlItemsResponse is the shape of a response from
// /artifactory/api/search/aql for an items.find query.
type aqlItemsResponse struct {
	Results []RepoItem `json:"results"`
}

// FetchRepoItems runs an AQL items.find query against repo on client and
// returns every item, with known Artifactory-generated/package-index files
// already filtered out.
func FetchRepoItems(client *jfclient.Client, repo string) ([]RepoItem, error) {
	query := fmt.Sprintf(`items.find({"repo":%q}).include("path","name","sha256")`, repo)
	resp, err := client.AQL(query)
	if err != nil {
		return nil, fmt.Errorf("run AQL query for repo %s: %w", repo, err)
	}
	if err := resp.Error("run AQL query for repo " + repo); err != nil {
		return nil, err
	}
	var out aqlItemsResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("parse AQL response for repo %s: %w", repo, err)
	}
	items := make([]RepoItem, 0, len(out.Results))
	for _, it := range out.Results {
		if shouldSkipItem(it.Path, it.Name) {
			continue
		}
		items = append(items, it)
	}
	return items, nil
}

// ItemDiffStatus describes why an item needs to be synced from source to
// target.
type ItemDiffStatus string

const (
	// StatusMissing means the item exists on source but not on target.
	StatusMissing ItemDiffStatus = "MISSING"
	// StatusDifferent means the item exists on both sides but its sha256
	// checksum differs.
	StatusDifferent ItemDiffStatus = "DIFFERENT"
)

// ItemDiff is one item that needs to be synced from source to target.
type ItemDiff struct {
	Repo         string
	Path         string
	Name         string
	Status       ItemDiffStatus
	SourceSHA256 string
	TargetSHA256 string // empty when Status == StatusMissing
}

// RepoDiffResult is the outcome of comparing one repo between source and
// target. Comparison is one-directional: source is the ground truth, so
// target-only items are never reported.
type RepoDiffResult struct {
	Repo   string
	Diffs  []ItemDiff
	Source int // total items considered on source (after skip filtering)
	Target int // total items considered on target (after skip filtering)
}

// CompareRepo compares the contents of repo between source and target,
// reporting items missing from target and items present on both sides with
// a different sha256. Target-only items are never reported (source is
// ground truth).
func CompareRepo(source, target *jfclient.Client, repo string) (RepoDiffResult, error) {
	result := RepoDiffResult{Repo: repo}

	sourceItems, err := FetchRepoItems(source, repo)
	if err != nil {
		return result, fmt.Errorf("fetch source items for repo %s: %w", repo, err)
	}
	targetItems, err := FetchRepoItems(target, repo)
	if err != nil {
		return result, fmt.Errorf("fetch target items for repo %s: %w", repo, err)
	}
	result.Source = len(sourceItems)
	result.Target = len(targetItems)

	targetBySHA := make(map[string]string, len(targetItems))
	for _, it := range targetItems {
		targetBySHA[it.Key()] = it.SHA256
	}

	for _, src := range sourceItems {
		tgtSHA, ok := targetBySHA[src.Key()]
		switch {
		case !ok:
			result.Diffs = append(result.Diffs, ItemDiff{
				Repo: repo, Path: src.Path, Name: src.Name,
				Status: StatusMissing, SourceSHA256: src.SHA256,
			})
		case tgtSHA != src.SHA256:
			result.Diffs = append(result.Diffs, ItemDiff{
				Repo: repo, Path: src.Path, Name: src.Name,
				Status: StatusDifferent, SourceSHA256: src.SHA256, TargetSHA256: tgtSHA,
			})
		}
	}
	return result, nil
}

// repoListEntry is one entry from GET /artifactory/api/repositories.
type repoListEntry struct {
	Key  string `json:"key"`
	Type string `json:"type"`
}

// ListLocalRepos returns the keys of every local-type repository on client,
// via GET /artifactory/api/repositories?type=local.
func ListLocalRepos(client *jfclient.Client) ([]string, error) {
	resp, err := client.Get("/artifactory/api/repositories?type=local")
	if err != nil {
		return nil, fmt.Errorf("list local repositories: %w", err)
	}
	if err := resp.Error("list local repositories"); err != nil {
		return nil, err
	}
	var entries []repoListEntry
	if err := resp.Unmarshal(&entries); err != nil {
		return nil, fmt.Errorf("parse repository list: %w", err)
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Key)
	}
	return keys, nil
}

// CompareRepos compares every repo in repos between source and target,
// returning one RepoDiffResult per repo, in the same order as repos.
func CompareRepos(source, target *jfclient.Client, repos []string, onProgress ...progress.Reporter) ([]RepoDiffResult, error) {
	return concurrency.MapOrErr(repos, 0, func(repo string) (RepoDiffResult, error) {
		return CompareRepo(source, target, repo)
	}, onProgress...)
}

// WriteRepoReports writes one "<repo>.txt" file per result into dir, listing
// its missing/different items, plus a "transfer-plan.txt" aggregating every
// result into one suggested (non-executed) transfer command per line.
func WriteRepoReports(dir string, results []RepoDiffResult) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create report dir %s: %w", dir, err)
	}

	var plan strings.Builder
	for _, res := range results {
		var buf strings.Builder
		fmt.Fprintf(&buf, "Repo: %s (source items: %d, target items: %d)\n", res.Repo, res.Source, res.Target)
		if len(res.Diffs) == 0 {
			buf.WriteString("No differences found.\n")
		}
		for _, d := range res.Diffs {
			fmt.Fprintf(&buf, "%s\t%s%s\tsource_sha256=%s\ttarget_sha256=%s\n",
				d.Status, dirPath(d.Repo, d.Path), d.Name, d.SourceSHA256, d.TargetSHA256)
			plan.WriteString(transferPlanLine(d))
		}

		path := filepath.Join(dir, res.Repo+".txt")
		if err := os.WriteFile(path, []byte(buf.String()), 0o644); err != nil {
			return fmt.Errorf("write report %s: %w", path, err)
		}
	}

	planPath := filepath.Join(dir, "transfer-plan.txt")
	if err := os.WriteFile(planPath, []byte(plan.String()), 0o644); err != nil {
		return fmt.Errorf("write transfer plan %s: %w", planPath, err)
	}
	return nil
}

// dirPath joins repo and path (repo-relative directory, "." meaning root)
// into a single "/"-terminated Artifactory path.
func dirPath(repo, path string) string {
	if path == "" || path == "." {
		return repo + "/"
	}
	return repo + "/" + path + "/"
}

// transferPlanLine renders a suggested (not executed) jf CLI transfer
// command for one missing/different item.
func transferPlanLine(d ItemDiff) string {
	dir := dirPath(d.Repo, d.Path)
	fullPath := dir + d.Name
	return fmt.Sprintf("jf rt dl %q && jf rt u %q %q\n", fullPath, d.Name, dir)
}
