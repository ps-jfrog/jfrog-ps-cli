package repos

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// ReadKeysFromFile reads newline-delimited repository keys from path,
// skipping blank lines and lines beginning with '#'.
func ReadKeysFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
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
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return keys, nil
}

// DeleteResult is the outcome of attempting to delete one repository.
type DeleteResult struct {
	Key        string
	StatusCode int
	Err        error
}

// DeleteRepos deletes each repo in keys from c via
// DELETE /artifactory/api/repositories/{key}.
//
// If dryRun is true, DeleteRepos performs no HTTP calls whatsoever and
// returns nil immediately.
func DeleteRepos(c *jfclient.Client, keys []string, dryRun bool) []DeleteResult {
	if dryRun {
		return nil
	}
	results := make([]DeleteResult, 0, len(keys))
	for _, key := range keys {
		resp, err := c.Delete("/artifactory/api/repositories/" + key)
		if err != nil {
			results = append(results, DeleteResult{Key: key, Err: err})
			continue
		}
		results = append(results, DeleteResult{
			Key:        key,
			StatusCode: resp.StatusCode,
			Err:        resp.Error(fmt.Sprintf("delete repository %s", key)),
		})
	}
	return results
}
