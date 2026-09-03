package compare

import (
	"fmt"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// buildsListResponse is the shape of GET /artifactory/api/build.
type buildsListResponse struct {
	Builds []struct {
		URI         string `json:"uri"`
		LastStarted string `json:"lastStarted"`
	} `json:"builds"`
}

// buildNumbersResponse is the shape of GET /artifactory/api/build/{name}.
type buildNumbersResponse struct {
	BuildsNumbers []struct {
		URI     string `json:"uri"`
		Started string `json:"started"`
	} `json:"buildsNumbers"`
}

// buildInfoEnvelope is the shape of GET /artifactory/api/build/{name}/{number}
// and the body expected by PUT /artifactory/api/build.
type buildInfoEnvelope struct {
	BuildInfo map[string]interface{} `json:"buildInfo"`
}

// stripLeadingSlash removes a single leading "/" from an AQL/Artifactory
// "uri" field, e.g. "/my-build" -> "my-build".
func stripLeadingSlash(uri string) string {
	return strings.TrimPrefix(uri, "/")
}

// ListBuildNames returns every build name known to client, via
// GET /artifactory/api/build.
func ListBuildNames(client *jfclient.Client) ([]string, error) {
	resp, err := client.Get("/artifactory/api/build")
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	if resp.StatusCode == 404 {
		// Some Artifactory versions return 404 when there are no builds at all.
		return nil, nil
	}
	if err := resp.Error("list builds"); err != nil {
		return nil, err
	}
	var out buildsListResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("parse build list: %w", err)
	}
	names := make([]string, 0, len(out.Builds))
	for _, b := range out.Builds {
		names = append(names, stripLeadingSlash(b.URI))
	}
	return names, nil
}

// ListBuildNumbers returns every build number recorded for buildName on
// client, via GET /artifactory/api/build/{name}.
func ListBuildNumbers(client *jfclient.Client, buildName string) ([]string, error) {
	resp, err := client.Get("/artifactory/api/build/" + buildName)
	if err != nil {
		return nil, fmt.Errorf("list build numbers for %s: %w", buildName, err)
	}
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if err := resp.Error("list build numbers for " + buildName); err != nil {
		return nil, err
	}
	var out buildNumbersResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("parse build numbers for %s: %w", buildName, err)
	}
	numbers := make([]string, 0, len(out.BuildsNumbers))
	for _, n := range out.BuildsNumbers {
		numbers = append(numbers, stripLeadingSlash(n.URI))
	}
	return numbers, nil
}

// BuildDiff is the outcome of comparing which builds/build-numbers exist on
// source vs target. Comparison is one-directional: source-only entries are
// "missing" on target; target-only entries are never reported.
type BuildDiff struct {
	// MissingBuildNames are build names present on source but not target.
	MissingBuildNames []string
	// SharedBuildNames are build names present on both sides.
	SharedBuildNames []string
	// MissingNumbers maps a shared build name to the build numbers present
	// on source but missing on target.
	MissingNumbers map[string][]string
}

// CompareBuilds diffs the set of build names and, for build names present on
// both sides, the set of build numbers within each.
func CompareBuilds(source, target *jfclient.Client) (BuildDiff, error) {
	diff := BuildDiff{MissingNumbers: map[string][]string{}}

	sourceNames, err := ListBuildNames(source)
	if err != nil {
		return diff, fmt.Errorf("list source builds: %w", err)
	}
	targetNames, err := ListBuildNames(target)
	if err != nil {
		return diff, fmt.Errorf("list target builds: %w", err)
	}
	targetNameSet := toSet(targetNames)

	for _, name := range sourceNames {
		if _, ok := targetNameSet[name]; !ok {
			diff.MissingBuildNames = append(diff.MissingBuildNames, name)
			continue
		}
		diff.SharedBuildNames = append(diff.SharedBuildNames, name)
	}

	for _, name := range diff.SharedBuildNames {
		sourceNumbers, err := ListBuildNumbers(source, name)
		if err != nil {
			return diff, fmt.Errorf("list source build numbers for %s: %w", name, err)
		}
		targetNumbers, err := ListBuildNumbers(target, name)
		if err != nil {
			return diff, fmt.Errorf("list target build numbers for %s: %w", name, err)
		}
		targetNumberSet := toSet(targetNumbers)

		var missing []string
		for _, num := range sourceNumbers {
			if _, ok := targetNumberSet[num]; !ok {
				missing = append(missing, num)
			}
		}
		if len(missing) > 0 {
			diff.MissingNumbers[name] = missing
		}
	}

	return diff, nil
}

func toSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, it := range items {
		set[it] = struct{}{}
	}
	return set
}

// BuildPair identifies one (name, number) build-info to migrate.
type BuildPair struct {
	Name   string
	Number string
}

// MissingPairs flattens diff into the full list of (name, number) pairs that
// need to be migrated from source to target: every build number under a
// build name entirely missing on target (fetched from source, since diff
// itself only records missing names, not their numbers), plus every number
// recorded in diff.MissingNumbers for build names shared by both sides.
func MissingPairs(source *jfclient.Client, diff BuildDiff) ([]BuildPair, error) {
	var pairs []BuildPair

	for _, name := range diff.MissingBuildNames {
		numbers, err := ListBuildNumbers(source, name)
		if err != nil {
			return nil, fmt.Errorf("list source build numbers for %s: %w", name, err)
		}
		for _, number := range numbers {
			pairs = append(pairs, BuildPair{Name: name, Number: number})
		}
	}
	for name, numbers := range diff.MissingNumbers {
		for _, number := range numbers {
			pairs = append(pairs, BuildPair{Name: name, Number: number})
		}
	}
	return pairs, nil
}

// BuildMigrationResult is the outcome of migrating (or dry-running the
// migration of) one missing build number from source to target.
type BuildMigrationResult struct {
	Name    string
	Number  string
	DryRun  bool
	Success bool
	Error   string
}

// MigrateBuildPairs fetches each pair's build-info from source and, unless
// dryRun is true, PUTs it to target. In dry-run mode (the default), no PUT
// calls are issued at all -- every result is returned with DryRun=true and
// Success=false, purely describing what would happen.
func MigrateBuildPairs(source, target *jfclient.Client, pairs []BuildPair, dryRun bool, onProgress ...progress.Reporter) []BuildMigrationResult {
	return concurrency.One(pairs, 0, func(p BuildPair) BuildMigrationResult {
		return migrateOneBuild(source, target, p.Name, p.Number, dryRun)
	}, onProgress...)
}

func migrateOneBuild(source, target *jfclient.Client, name, number string, dryRun bool) BuildMigrationResult {
	res := BuildMigrationResult{Name: name, Number: number, DryRun: dryRun}
	if dryRun {
		return res
	}

	resp, err := source.Get(fmt.Sprintf("/artifactory/api/build/%s/%s", name, number))
	if err != nil {
		res.Error = fmt.Sprintf("fetch build-info: %v", err)
		return res
	}
	if err := resp.Error("fetch build-info " + name + "/" + number); err != nil {
		res.Error = err.Error()
		return res
	}
	var envelope buildInfoEnvelope
	if err := resp.Unmarshal(&envelope); err != nil {
		res.Error = fmt.Sprintf("parse build-info: %v", err)
		return res
	}

	delete(envelope.BuildInfo, "uri")

	putResp, err := target.Put("/artifactory/api/build", envelope)
	if err != nil {
		res.Error = fmt.Sprintf("upload build-info: %v", err)
		return res
	}
	if err := putResp.Error("upload build-info " + name + "/" + number); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Success = true
	return res
}
