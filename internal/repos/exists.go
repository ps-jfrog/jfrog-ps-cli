package repos

import (
	"jfrogpscli/internal/jfclient"
)

// ExistenceResult is the outcome of checking one repo key against a server's
// repository list.
type ExistenceResult struct {
	Key    string
	Exists bool
}

// CheckExist reports, for each key in keys, whether it appears in c's
// repository list of the given type ("" or "all" checks every type).
func CheckExist(c *jfclient.Client, keys []string, repoType string) ([]ExistenceResult, error) {
	all, err := List(c, repoType)
	if err != nil {
		return nil, err
	}
	present := make(map[string]bool, len(all))
	for _, r := range all {
		present[r.Key] = true
	}

	results := make([]ExistenceResult, 0, len(keys))
	for _, key := range keys {
		results = append(results, ExistenceResult{Key: key, Exists: present[key]})
	}
	return results, nil
}
