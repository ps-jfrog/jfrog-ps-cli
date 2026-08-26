package cleanup

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// ReadRepoKeys reads newline-delimited repository keys from path, skipping
// blank lines and lines starting with '#' (comments).
func ReadRepoKeys(path string) ([]string, error) {
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

// RepoDeleteResult is the outcome of attempting to delete one repository.
type RepoDeleteResult struct {
	Repo       string
	StatusCode int
	DryRun     bool
	Success    bool
	Error      string
}

// DeleteRepos deletes each repo key via
// DELETE /artifactory/api/repositories/{repo}. When dryRun is true, no HTTP
// DELETE calls are issued at all.
func DeleteRepos(client *jfclient.Client, repos []string, dryRun bool) []RepoDeleteResult {
	results := make([]RepoDeleteResult, 0, len(repos))
	for _, repo := range repos {
		res := RepoDeleteResult{Repo: repo, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}
		resp, err := client.Delete("/artifactory/api/repositories/" + repo)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.StatusCode = resp.StatusCode
		if resp.OK() {
			res.Success = true
		} else {
			res.Error = resp.Error("delete repo").Error()
		}
		results = append(results, res)
	}
	return results
}
