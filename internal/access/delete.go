package access

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// DeleteKind identifies which kind of Access API v2 object delete-users,
// delete-groups, and delete-permissions operate on.
type DeleteKind string

const (
	DeleteUsers       DeleteKind = "users"
	DeleteGroups      DeleteKind = "groups"
	DeletePermissions DeleteKind = "permissions"
)

// protectedNames are always excluded from bulk deletion, regardless of what
// the caller passes via --exclude.
var protectedNames = map[DeleteKind]map[string]bool{
	DeleteUsers:       {"admin": true, "anonymous": true},
	DeleteGroups:      {"readers": true, "Anyremote": true, "admin": true},
	DeletePermissions: {"Any Remote": true, "Anything": true, "INTERNAL": true},
}

func endpointFor(kind DeleteKind, name string) string {
	return fmt.Sprintf("/access/api/v2/%s/%s", kind, url.PathEscape(name))
}

// ParseNamesFile reads newline-delimited names from path, skipping blank
// lines and lines starting with '#'.
func ParseNamesFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read names file %s: %w", path, err)
	}
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

// FilterDeletable removes protected names for kind (see protectedNames) and
// any name in exclude, plus de-duplicates, returning the remaining names in
// their original relative order.
func FilterDeletable(kind DeleteKind, names []string, exclude []string) []string {
	skip := map[string]bool{}
	for n, v := range protectedNames[kind] {
		if v {
			skip[n] = true
		}
	}
	for _, e := range exclude {
		e = strings.TrimSpace(e)
		if e != "" {
			skip[e] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		if skip[n] || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// DeleteResult reports the outcome of deleting one item.
type DeleteResult struct {
	Name    string
	Success bool
	Error   string
}

// DeleteItems issues DELETE /access/api/v2/{kind}/{name} for each name. When
// dryRun is true, no HTTP call is made at all -- every name is reported as a
// would-be success so callers can preview the plan.
func DeleteItems(c *jfclient.Client, kind DeleteKind, names []string, dryRun bool) []DeleteResult {
	results := make([]DeleteResult, 0, len(names))
	for _, n := range names {
		if dryRun {
			results = append(results, DeleteResult{Name: n, Success: true})
			continue
		}
		resp, err := c.Delete(endpointFor(kind, n))
		if err != nil {
			results = append(results, DeleteResult{Name: n, Success: false, Error: err.Error()})
			continue
		}
		if err := resp.Error(fmt.Sprintf("delete %s %s", strings.TrimSuffix(string(kind), "s"), n)); err != nil {
			results = append(results, DeleteResult{Name: n, Success: false, Error: err.Error()})
			continue
		}
		results = append(results, DeleteResult{Name: n, Success: true})
	}
	return results
}
