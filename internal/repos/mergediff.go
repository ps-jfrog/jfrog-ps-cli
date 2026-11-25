package repos

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// IgnoredMergeKeys are the repository config keys that 'jf rt
// transfer-config-merge' deliberately excludes when comparing repositories
// (they legitimately differ between servers, or are never returned). The
// comparison here mirrors that so a repo only counts as "different" if the
// merge command would have flagged it.
var IgnoredMergeKeys = map[string]bool{
	"url":                          true,
	"password":                     true,
	"suppresspomconsistencychecks": true,
	"description":                  true,
	"gitregistryurl":               true,
	"cargointernalindex":           true,
	// Server-specific, not part of transfer-config-merge's ignore list:
	// federated "members" holds each JPD's own URL and the federation
	// topology, so copying source's list would re-point the target's
	// federation at the source and drop its own links.
	"members": true,
}

// PropDiff is one repository property whose value differs between source
// and target.
type PropDiff struct {
	Property string
	Source   interface{}
	Target   interface{}
}

// RepoDiff is the set of differing properties for one repository.
type RepoDiff struct {
	Key   string
	Diffs []PropDiff
	Err   error // set if the repo config could not be fetched from either side
}

// ParseConflictsCSV reads the conflicts CSV written by 'jf rt
// transfer-config-merge' (header: Type,SourceName,TargetName,
// DifferentProperties) and returns the sorted, de-duplicated keys of the
// Repository rows. Project rows are ignored.
func ParseConflictsCSV(r io.Reader) ([]string, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse conflicts CSV: %w", err)
	}
	if len(rows) == 0 || len(rows[0]) < 2 || rows[0][0] != "Type" || rows[0][1] != "SourceName" {
		return nil, fmt.Errorf("parse conflicts CSV: unexpected header (want Type,SourceName,TargetName,DifferentProperties)")
	}
	seen := map[string]bool{}
	var keys []string
	for _, row := range rows[1:] {
		if len(row) < 2 || row[0] != "Repository" || seen[row[1]] {
			continue
		}
		seen[row[1]] = true
		keys = append(keys, row[1])
	}
	sort.Strings(keys)
	return keys, nil
}

// DiffConfigs returns the properties that exist on both sides with different
// values, ignoring IgnoredMergeKeys, sorted by property name. Like the merge
// command, a key present on only one side is not a difference.
func DiffConfigs(source, target map[string]interface{}) []PropDiff {
	var diffs []PropDiff
	for k, sv := range source {
		if IgnoredMergeKeys[strings.ToLower(k)] {
			continue
		}
		if tv, ok := target[k]; ok && !reflect.DeepEqual(sv, tv) {
			diffs = append(diffs, PropDiff{Property: k, Source: sv, Target: tv})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Property < diffs[j].Property })
	return diffs
}

// GetRelevantRepoConfig fetches a repo's configuration from
// GET /artifactory/api/v2/repositories/{key}, which returns only the
// properties relevant to the repo's package type (the v1 endpoint returns
// every legacy flag for every package type, e.g. Docker/NuGet/Cargo flags on
// a Maven repo, whose defaults differ between Artifactory versions). If the
// server has no v2 endpoint (404) it falls back to the v1 config.
func GetRelevantRepoConfig(c *jfclient.Client, key string) (map[string]interface{}, error) {
	resp, err := c.Get("/artifactory/api/v2/repositories/" + url.PathEscape(key))
	if err != nil {
		return nil, fmt.Errorf("get repository %s config: %w", key, err)
	}
	if resp.StatusCode == 404 {
		return GetRepoConfig(c, key)
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

// DiffRepos fetches each repo's package-type-relevant config (v2 API) from source and target and
// reports the property differences. A fetch failure is recorded on that
// repo's RepoDiff.Err rather than aborting the others. Results keep the
// order of keys.
func DiffRepos(source, target *jfclient.Client, keys []string, onProgress ...progress.Reporter) []RepoDiff {
	return concurrency.One(keys, 0, func(key string) RepoDiff {
		srcCfg, err := GetRelevantRepoConfig(source, key)
		if err != nil {
			return RepoDiff{Key: key, Err: fmt.Errorf("source: %w", err)}
		}
		tgtCfg, err := GetRelevantRepoConfig(target, key)
		if err != nil {
			return RepoDiff{Key: key, Err: fmt.Errorf("target: %w", err)}
		}
		return RepoDiff{Key: key, Diffs: DiffConfigs(srcCfg, tgtCfg)}
	}, onProgress...)
}

// Pending returns only the repos that still have differences or errors.
func Pending(diffs []RepoDiff) []RepoDiff {
	var out []RepoDiff
	for _, d := range diffs {
		if d.Err != nil || len(d.Diffs) > 0 {
			out = append(out, d)
		}
	}
	return out
}

// ApplyMergeDiffs updates each repo on target so its differing properties
// take the source value. Only the differing properties are POSTed
// (Artifactory treats POST /api/repositories/{key} as a partial update), so
// everything else on the target repo -- URL, credentials, description -- is
// left untouched. If dryRun is true no HTTP calls are made and nil is
// returned.
func ApplyMergeDiffs(target *jfclient.Client, diffs []RepoDiff, dryRun bool, onProgress ...progress.Reporter) []UpdateResult {
	if dryRun {
		return nil
	}
	var todo []RepoDiff
	for _, d := range diffs {
		if d.Err == nil && len(d.Diffs) > 0 {
			todo = append(todo, d)
		}
	}
	return concurrency.One(todo, 0, func(d RepoDiff) UpdateResult {
		body := make(map[string]interface{}, len(d.Diffs))
		for _, p := range d.Diffs {
			body[p.Property] = p.Source
		}
		// Accept */*: the update endpoint answers in text/plain (406 otherwise).
		resp, err := target.DoAccept(http.MethodPost, "/artifactory/api/repositories/"+url.PathEscape(d.Key), body, "*/*")
		if err != nil {
			return UpdateResult{Key: d.Key, Err: fmt.Errorf("update repository %s on target: %w", d.Key, err)}
		}
		return UpdateResult{Key: d.Key, Err: resp.Error(fmt.Sprintf("update repository %s on target", d.Key))}
	}, onProgress...)
}

// WriteDiffReport writes diffs as CSV: Repository,Property,SourceValue,
// TargetValue,Status. Values are JSON-encoded so booleans, numbers, lists
// and null are unambiguous. Repos with no differences get one row with
// Status "match"; repos that could not be fetched get Status "error: ...".
func WriteDiffReport(w io.Writer, diffs []RepoDiff) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"Repository", "Property", "SourceValue", "TargetValue", "Status"}); err != nil {
		return err
	}
	for _, d := range diffs {
		var rows [][]string
		switch {
		case d.Err != nil:
			rows = [][]string{{d.Key, "", "", "", "error: " + d.Err.Error()}}
		case len(d.Diffs) == 0:
			rows = [][]string{{d.Key, "", "", "", "match"}}
		default:
			for _, p := range d.Diffs {
				rows = append(rows, []string{d.Key, p.Property, jsonString(p.Source), jsonString(p.Target), "differs"})
			}
		}
		for _, row := range rows {
			if err := cw.Write(row); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func jsonString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
