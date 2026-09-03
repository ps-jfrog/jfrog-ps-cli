package cleanup

import (
	"fmt"
	"sort"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// storageChild is one entry of a Get Storage Item Info response's "children"
// array (GET /artifactory/api/storage/{repo}[/{path}]).
type storageChild struct {
	URI    string `json:"uri"`
	Folder bool   `json:"folder"`
}

type storageInfo struct {
	Children []storageChild `json:"children"`
}

func getStorageInfo(client *jfclient.Client, repo, path string) (*storageInfo, error) {
	u := "/artifactory/api/storage/" + repo
	if path != "" {
		u += "/" + path
	}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("get storage info for " + repo + "/" + path); err != nil {
		return nil, err
	}
	var info storageInfo
	if err := resp.Unmarshal(&info); err != nil {
		return nil, fmt.Errorf("decode storage info for %s/%s: %w", repo, path, err)
	}
	return &info, nil
}

// FindEmptyFolders returns the minimal set of top-level folder paths in repo
// that are "recursively empty" -- contain no files anywhere in their
// subtree (only, at most, other empty folders). A folder already covered by
// an empty ancestor in the result is not listed separately: deleting the
// ancestor (DeleteEmptyFolders issues a single recursive DELETE per path)
// removes it too. The repository root itself is never returned, even if the
// whole repository is empty.
//
// This differs from the original script's approach (which deletes leaf
// folders one at a time, bottom-up, re-checking emptiness after each
// delete) only in mechanism, not outcome: both leave the same set of paths
// deleted from the repo. Computing the minimal covering set up front lets
// dry-run report the exact same folders that would be deleted without
// issuing any writes, and real deletion needs one DELETE call per top-level
// empty folder instead of one per folder at every depth.
func FindEmptyFolders(client *jfclient.Client, repo string) ([]string, error) {
	isEmpty, candidates, err := emptySubtree(client, repo, "")
	if err != nil {
		return nil, err
	}
	if isEmpty {
		// The whole repo is empty; never delete the repo root itself.
		return nil, nil
	}
	sort.Strings(candidates)
	return candidates, nil
}

// emptySubtree walks path's subtree depth-first. It reports whether path
// itself is recursively empty (no files anywhere below it), and the list of
// top-level empty-folder candidates found so far that are NOT subsumed by
// path being empty (i.e. returned only when path is NOT itself empty, since
// an empty path makes every candidate beneath it redundant).
func emptySubtree(client *jfclient.Client, repo, path string) (isEmpty bool, candidates []string, err error) {
	info, err := getStorageInfo(client, repo, path)
	if err != nil {
		if path == "" {
			// The repo root itself failed to load (typo'd/nonexistent repo,
			// no read access, etc.) -- this is a real failure of the whole
			// operation, not a single unreadable subfolder, so it must not
			// be swallowed into a false "0 empty folders found".
			return false, nil, err
		}
		// An unreadable folder is treated as "cannot verify empty" -- safer
		// to leave it alone than to guess. It is not reported as a
		// candidate, and the surrounding walk continues everywhere else.
		return false, nil, nil
	}

	subtreeHasFile := false
	var collected []string
	for _, child := range info.Children {
		name := strings.TrimPrefix(child.URI, "/")
		childPath := name
		if path != "" {
			childPath = path + "/" + name
		}
		if !child.Folder {
			subtreeHasFile = true
			continue
		}
		childEmpty, childCandidates, err := emptySubtree(client, repo, childPath)
		if err != nil {
			return false, nil, err
		}
		if childEmpty {
			collected = append(collected, childPath)
		} else {
			subtreeHasFile = true
			collected = append(collected, childCandidates...)
		}
	}

	if !subtreeHasFile {
		return true, nil, nil
	}
	return false, collected, nil
}

// EmptyFolderDeleteResult is the outcome of attempting to delete one empty
// folder.
type EmptyFolderDeleteResult struct {
	Repo       string
	Path       string
	DryRun     bool
	StatusCode int
	Success    bool
	Error      string
}

// DeleteEmptyFolders deletes each folder path in repo via
// DELETE /artifactory/{repo}/{path}. When dryRun is true, no HTTP DELETE
// calls are issued at all.
// paths never nest (see FindEmptyFolders), so each delete is independent
// and safe to run concurrently.
func DeleteEmptyFolders(client *jfclient.Client, repo string, paths []string, dryRun bool, onProgress ...progress.Reporter) []EmptyFolderDeleteResult {
	return concurrency.One(paths, 0, func(path string) EmptyFolderDeleteResult {
		res := EmptyFolderDeleteResult{Repo: repo, Path: path, DryRun: dryRun}
		if dryRun {
			return res
		}
		resp, err := client.Delete("/artifactory/" + repo + "/" + path)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.StatusCode = resp.StatusCode
		if resp.OK() {
			res.Success = true
		} else {
			res.Error = resp.Error("delete folder " + path).Error()
		}
		return res
	}, onProgress...)
}
