package projects

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// ReadKeysFromFile reads newline-delimited project keys from path, skipping
// blank lines and lines beginning with '#'.
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

// DeleteProjects deletes each project in keys from c via
// DELETE /access/api/v1/projects/{key}.
//
// Unlike the mutating functions in diff.go, DeleteProjects has no dryRun
// parameter: this mirrors the "projects delete" subcommand, which has no
// --dry-run flag by design (matching the original script's "confirm once,
// then just do it" behavior). Callers (the CLI layer) are responsible for
// obtaining explicit confirmation -- via output.Confirm or --yes -- before
// ever calling this function.
func DeleteProjects(c *jfclient.Client, keys []string, onProgress ...progress.Reporter) []DeleteResult {
	return concurrency.One(keys, 0, func(key string) DeleteResult {
		return deleteProject(c, key)
	}, onProgress...)
}
