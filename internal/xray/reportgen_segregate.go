package xray

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// ---------------------------------------------------------------------------
// Shared artifact-resolution primitives
// ---------------------------------------------------------------------------

// storageInfo models the fields consumed from
// GET /artifactory/api/storage/{repo}/{path}.
type storageInfo struct {
	CreatedBy  string `json:"createdBy"`
	ModifiedBy string `json:"modifiedBy"`
	Created    string `json:"created"`
	Checksums  struct {
		SHA256 string `json:"sha256"`
		SHA1   string `json:"sha1"`
		MD5    string `json:"md5"`
	} `json:"checksums"`
}

// getStorageInfo fetches storage-item info for repo/path. The returned
// string is a stable lookup_error code ("401_unauthorized",
// "403_forbidden", "404_not_found") on failure, empty on success.
func getStorageInfo(client *jfclient.Client, repo, path string) (*storageInfo, string) {
	resp, err := client.Get("/artifactory/api/storage/" + repo + "/" + path)
	if err != nil {
		return nil, "request_error"
	}
	switch resp.StatusCode {
	case 401:
		return nil, "401_unauthorized"
	case 403:
		return nil, "403_forbidden"
	case 404:
		return nil, "404_not_found"
	}
	if !resp.OK() {
		return nil, "storage_info_error"
	}
	var info storageInfo
	if err := resp.Unmarshal(&info); err != nil {
		return nil, "decode_error"
	}
	return &info, ""
}

func deployerOf(info *storageInfo) string {
	if info == nil {
		return ""
	}
	if info.CreatedBy != "" {
		return info.CreatedBy
	}
	return info.ModifiedBy
}

// searchResult models GET /artifactory/api/search/{checksum,gavc}'s response.
type searchResult struct {
	Results []struct {
		URI string `json:"uri"`
	} `json:"results"`
}

// parseStorageURI extracts (repo, path) from a search result's
// ".../api/storage/<repo>/<path>" URI.
func parseStorageURI(uri string) (repo, path string, ok bool) {
	const marker = "/api/storage/"
	idx := strings.Index(uri, marker)
	if idx < 0 {
		return "", "", false
	}
	rest := uri[idx+len(marker):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// checksumSearch resolves a checksum (kind is "sha256", "sha1", or "md5")
// to a real repo/path via GET /artifactory/api/search/checksum. The first
// match wins if the checksum is shared by multiple artifacts.
func checksumSearch(client *jfclient.Client, kind, value string) (repo, path string, ok bool) {
	resp, err := client.Get(fmt.Sprintf("/artifactory/api/search/checksum?%s=%s", kind, url.QueryEscape(value)))
	if err != nil || !resp.OK() {
		return "", "", false
	}
	var out searchResult
	if err := resp.Unmarshal(&out); err != nil || len(out.Results) == 0 {
		return "", "", false
	}
	return parseStorageURI(out.Results[0].URI)
}

// gavcSearch resolves a Maven group:artifact:version coordinate within a
// known repo via GET /artifactory/api/search/gavc. group may be empty.
func gavcSearch(client *jfclient.Client, group, artifact, version, repo string) (resultRepo, path string, ok bool) {
	q := url.Values{}
	if group != "" {
		q.Set("g", group)
	}
	q.Set("a", artifact)
	q.Set("v", version)
	if repo != "" {
		q.Set("repos", repo)
	}
	resp, err := client.Get("/artifactory/api/search/gavc?" + q.Encode())
	if err != nil || !resp.OK() {
		return "", "", false
	}
	var out searchResult
	if err := resp.Unmarshal(&out); err != nil || len(out.Results) == 0 {
		return "", "", false
	}
	return parseStorageURI(out.Results[0].URI)
}

func stripScheme(v string) string {
	if idx := strings.Index(v, "://"); idx >= 0 {
		return v[idx+3:]
	}
	return v
}

// looksLikeGAV reports whether v looks like a bare "artifact:version" or
// "group:artifact:version" coordinate rather than a repo/path reference.
func looksLikeGAV(v string) bool {
	return strings.Count(v, ":") >= 1 && !strings.Contains(v, "/")
}

func parseGAV(v string) (group, artifact, version string, ok bool) {
	parts := strings.Split(v, ":")
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2], true
	case 2:
		return "", parts[0], parts[1], true
	default:
		return "", "", "", false
	}
}

// resolveArtifactRef resolves one impacted-artifact CSV value to a real
// (repo, path), for the repo-deployer segregation path (no build
// attribution). repoHint, when non-empty (e.g. an exposures CSV's explicit
// "repository" column), is preferred over parsing a repo out of value.
func resolveArtifactRef(client *jfclient.Client, repoHint, value string) (repo, path, lookupErr string) {
	v := stripScheme(value)
	switch {
	case strings.HasPrefix(v, "sha256:"):
		if r, p, ok := checksumSearch(client, "sha256", strings.TrimPrefix(v, "sha256:")); ok {
			return r, p, ""
		}
		return "", "", "checksum_unresolved"
	case strings.HasPrefix(v, "sha1:"):
		if r, p, ok := checksumSearch(client, "sha1", strings.TrimPrefix(v, "sha1:")); ok {
			return r, p, ""
		}
		return "", "", "checksum_unresolved"
	case strings.HasPrefix(v, "md5:"):
		if r, p, ok := checksumSearch(client, "md5", strings.TrimPrefix(v, "md5:")); ok {
			return r, p, ""
		}
		return "", "", "checksum_unresolved"
	case repoHint != "" && looksLikeGAV(v):
		if g, a, ver, ok := parseGAV(v); ok {
			if r, p, ok2 := gavcSearch(client, g, a, ver, repoHint); ok2 {
				return r, p, ""
			}
		}
		return "", "", "gav_unresolved"
	case repoHint != "":
		return repoHint, v, ""
	case strings.Contains(v, "/"):
		parts := strings.SplitN(v, "/", 2)
		return parts[0], parts[1], ""
	default:
		return "", "", "unresolved_reference"
	}
}

// ---------------------------------------------------------------------------
// Build attribution (git-repo segregation only)
// ---------------------------------------------------------------------------

type buildRun struct {
	Number  string
	Started string
}

// getBuildNumbers lists a build's runs, newest first, via
// GET /artifactory/api/build/{name}, capped at limit.
func getBuildNumbers(client *jfclient.Client, buildName string, limit int) ([]buildRun, error) {
	resp, err := client.Get("/artifactory/api/build/" + url.PathEscape(buildName))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if err := resp.Error("get build runs " + buildName); err != nil {
		return nil, err
	}
	var out struct {
		BuildsNumbers []struct {
			URI     string `json:"uri"`
			Started string `json:"started"`
		} `json:"buildsNumbers"`
	}
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("decode build runs for %s: %w", buildName, err)
	}
	runs := make([]buildRun, 0, len(out.BuildsNumbers))
	for _, b := range out.BuildsNumbers {
		runs = append(runs, buildRun{Number: strings.TrimPrefix(b.URI, "/"), Started: b.Started})
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Started > runs[j].Started })
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

type buildInfoArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	SHA1   string `json:"sha1"`
	MD5    string `json:"md5"`
}

type buildInfoModule struct {
	ID           string              `json:"id"`
	Artifacts    []buildInfoArtifact `json:"artifacts"`
	Dependencies []buildInfoArtifact `json:"dependencies"`
}

type buildVCS struct {
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	Revision string `json:"revision"`
}

type buildInfoDetail struct {
	Modules []buildInfoModule `json:"modules"`
	VCS     []buildVCS        `json:"vcs"`
	// Legacy top-level fallback used when vcs[] is empty.
	VcsURL      string `json:"vcsUrl"`
	VcsRevision string `json:"vcsRevision"`
	VcsBranch   string `json:"vcsBranch"`
}

// getBuildInfo fetches one build run's Build Info via
// GET /artifactory/api/build/{name}/{number}, unwrapping the
// {"buildInfo": {...}} envelope some Artifactory versions use.
func getBuildInfo(client *jfclient.Client, buildName, buildNumber, projectKey string) (*buildInfoDetail, error) {
	path := fmt.Sprintf("/artifactory/api/build/%s/%s", url.PathEscape(buildName), url.PathEscape(buildNumber))
	if projectKey != "" {
		path += "?project=" + url.QueryEscape(projectKey)
	}
	resp, err := client.Get(path)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if err := resp.Error("get build info " + buildName + "/" + buildNumber); err != nil {
		return nil, err
	}
	var envelope struct {
		BuildInfo *buildInfoDetail `json:"buildInfo"`
	}
	if err := resp.Unmarshal(&envelope); err == nil && envelope.BuildInfo != nil {
		return envelope.BuildInfo, nil
	}
	var direct buildInfoDetail
	if err := resp.Unmarshal(&direct); err != nil {
		return nil, fmt.Errorf("decode build info %s/%s: %w", buildName, buildNumber, err)
	}
	return &direct, nil
}

func vcsEntries(info *buildInfoDetail) []buildVCS {
	if info == nil {
		return nil
	}
	if len(info.VCS) > 0 {
		return info.VCS
	}
	if info.VcsURL != "" || info.VcsRevision != "" || info.VcsBranch != "" {
		return []buildVCS{{URL: info.VcsURL, Branch: info.VcsBranch, Revision: info.VcsRevision}}
	}
	return nil
}

func buildChecksumSet(info *buildInfoDetail) map[string]bool {
	set := map[string]bool{}
	if info == nil {
		return set
	}
	add := func(items []buildInfoArtifact) {
		for _, a := range items {
			if a.SHA256 != "" {
				set["sha256:"+a.SHA256] = true
			}
			if a.SHA1 != "" {
				set["sha1:"+a.SHA1] = true
			}
			if a.MD5 != "" {
				set["md5:"+a.MD5] = true
			}
		}
	}
	for _, m := range info.Modules {
		add(m.Artifacts)
		add(m.Dependencies)
	}
	return set
}

func firstChecksum(a buildInfoArtifact) string {
	switch {
	case a.SHA256 != "":
		return "sha256:" + a.SHA256
	case a.SHA1 != "":
		return "sha1:" + a.SHA1
	case a.MD5 != "":
		return "md5:" + a.MD5
	default:
		return ""
	}
}

// findModuleArtifact looks for a module artifact/dependency matching a bare
// "artifact:version" coordinate: an exact module-ID match first (module IDs
// are often "group:artifact:version" or "artifact:version"), then a loose
// filename substring match.
func findModuleArtifact(bi *buildInfoDetail, artifact, version string) (buildInfoArtifact, bool) {
	for _, m := range bi.Modules {
		if strings.Contains(m.ID, artifact) && strings.Contains(m.ID, version) {
			if len(m.Artifacts) > 0 {
				return m.Artifacts[0], true
			}
			if len(m.Dependencies) > 0 {
				return m.Dependencies[0], true
			}
		}
	}
	for _, m := range bi.Modules {
		all := append(append([]buildInfoArtifact{}, m.Artifacts...), m.Dependencies...)
		for _, a := range all {
			if strings.Contains(a.Name, artifact) && strings.Contains(a.Name, version) {
				return a, true
			}
		}
	}
	return buildInfoArtifact{}, false
}

// buildAttributionCache memoizes Build Info fetches (and the checksum
// sets/VCS entries derived from them) per (buildName, buildNumber), so
// build-gitrepo segregation costs scale with the number of distinct builds
// actually hit, not the number of CSV rows.
type buildAttributionCache struct {
	client    *jfclient.Client
	info      map[string]*buildInfoDetail
	checksums map[string]map[string]bool
	vcs       map[string][]buildVCS
}

func newBuildAttributionCache(client *jfclient.Client) *buildAttributionCache {
	return &buildAttributionCache{
		client:    client,
		info:      map[string]*buildInfoDetail{},
		checksums: map[string]map[string]bool{},
		vcs:       map[string][]buildVCS{},
	}
}

func (c *buildAttributionCache) get(name, number string) (*buildInfoDetail, error) {
	key := name + "@" + number
	if info, ok := c.info[key]; ok {
		return info, nil
	}
	info, err := getBuildInfo(c.client, name, number, "")
	if err != nil {
		return nil, err
	}
	c.info[key] = info
	c.checksums[key] = buildChecksumSet(info)
	c.vcs[key] = vcsEntries(info)
	return info, nil
}

// matchBuildByChecksum cross-references an artifact's checksums against
// each candidate build's last versionsToSearch runs' own artifact +
// dependency checksums, returning the first (build, run) match.
func matchBuildByChecksum(candidateBuilds []string, versionsToSearch int, info *storageInfo, cache *buildAttributionCache) (buildName, buildNumber string) {
	if info == nil {
		return "", ""
	}
	artifactChecksums := map[string]bool{}
	if info.Checksums.SHA256 != "" {
		artifactChecksums["sha256:"+info.Checksums.SHA256] = true
	}
	if info.Checksums.SHA1 != "" {
		artifactChecksums["sha1:"+info.Checksums.SHA1] = true
	}
	if info.Checksums.MD5 != "" {
		artifactChecksums["md5:"+info.Checksums.MD5] = true
	}
	if len(artifactChecksums) == 0 {
		return "", ""
	}

	for _, build := range candidateBuilds {
		runs, err := getBuildNumbers(cache.client, build, versionsToSearch)
		if err != nil {
			continue
		}
		for _, run := range runs {
			if _, err := cache.get(build, run.Number); err != nil {
				continue
			}
			set := cache.checksums[build+"@"+run.Number]
			for cs := range artifactChecksums {
				if set[cs] {
					return build, run.Number
				}
			}
		}
	}
	return "", ""
}

// resolveArtifactRefForBuild is resolveArtifactRef plus one extra branch:
// a bare "artifact:version" coordinate (no repo hint) is looked up against
// each candidate build's last versionsToSearch runs' modules before giving
// up, since a Build Watch's CSV rows often only carry the GAV coordinate.
func resolveArtifactRefForBuild(cache *buildAttributionCache, repoHint, value string, candidateBuilds []string, versionsToSearch int) (repo, path, matchedBuild, matchedRun, lookupErr string) {
	v := stripScheme(value)
	switch {
	case strings.HasPrefix(v, "sha256:"):
		if r, p, ok := checksumSearch(cache.client, "sha256", strings.TrimPrefix(v, "sha256:")); ok {
			return r, p, "", "", ""
		}
		return "", "", "", "", "checksum_unresolved"
	case strings.HasPrefix(v, "sha1:"):
		if r, p, ok := checksumSearch(cache.client, "sha1", strings.TrimPrefix(v, "sha1:")); ok {
			return r, p, "", "", ""
		}
		return "", "", "", "", "checksum_unresolved"
	case strings.HasPrefix(v, "md5:"):
		if r, p, ok := checksumSearch(cache.client, "md5", strings.TrimPrefix(v, "md5:")); ok {
			return r, p, "", "", ""
		}
		return "", "", "", "", "checksum_unresolved"
	case repoHint == "" && looksLikeGAV(v) && len(candidateBuilds) > 0:
		if _, artifact, version, ok := parseGAV(v); ok {
			for _, build := range candidateBuilds {
				runs, err := getBuildNumbers(cache.client, build, versionsToSearch)
				if err != nil {
					continue
				}
				for _, run := range runs {
					bi, err := cache.get(build, run.Number)
					if err != nil || bi == nil {
						continue
					}
					if art, ok := findModuleArtifact(bi, artifact, version); ok {
						if cs := firstChecksum(art); cs != "" {
							kind, val, _ := strings.Cut(cs, ":")
							if r, p, ok2 := checksumSearch(cache.client, kind, val); ok2 {
								return r, p, build, run.Number, ""
							}
						}
					}
				}
			}
		}
		return "", "", "", "", "build_artifact_unmatched"
	case repoHint != "" && looksLikeGAV(v):
		if g, a, ver, ok := parseGAV(v); ok {
			if r, p, ok2 := gavcSearch(cache.client, g, a, ver, repoHint); ok2 {
				return r, p, "", "", ""
			}
		}
		return "", "", "", "", "gav_unresolved"
	case repoHint != "":
		return repoHint, v, "", "", ""
	case strings.Contains(v, "/"):
		parts := strings.SplitN(v, "/", 2)
		return parts[0], parts[1], "", "", ""
	default:
		return "", "", "", "", "unresolved_reference"
	}
}

// ---------------------------------------------------------------------------
// CSV grouping
// ---------------------------------------------------------------------------

// artifactGroup is one unique (repoHint, impactedArtifact) pair from a
// source report CSV, with its violation count and distinct severities.
type artifactGroup struct {
	RepoHint         string
	ImpactedArtifact string
	Count            int
	Severities       map[string]bool
}

type csvColumns struct {
	index map[string]int
}

func indexColumns(header []string) csvColumns {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return csvColumns{index: m}
}

func (c csvColumns) get(row []string, name string) string {
	if idx, ok := c.index[name]; ok && idx < len(row) {
		return row[idx]
	}
	return ""
}

// findColumn returns the first header matching one of candidates exactly,
// falling back to a substring match (case-insensitive, since CSV export
// column naming can vary by report type/Xray version).
func findColumn(cols csvColumns, candidates ...string) (string, bool) {
	for _, c := range candidates {
		if _, ok := cols.index[c]; ok {
			return c, true
		}
	}
	// Substring fallback: if more than one header matches, deterministically
	// prefer the leftmost column in the original CSV rather than depending on
	// Go's randomized map iteration order.
	best, bestIdx, found := "", -1, false
	for h, idx := range cols.index {
		for _, c := range candidates {
			if strings.Contains(h, c) && (!found || idx < bestIdx) {
				best, bestIdx, found = h, idx, true
			}
		}
	}
	return best, found
}

func parseCSVRecords(data []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.ReadAll()
}

// groupRows deduplicates a source report's data rows into unique
// (repo-hint, impacted-artifact) groups. ok is false if no impacted-artifact
// column could be found at all.
func groupRows(records [][]string) (groups []artifactGroup, ok bool) {
	if len(records) == 0 {
		return nil, false
	}
	cols := indexColumns(records[0])
	artifactCol, found := findColumn(cols, "impacted_artifact", "impacted artifact", "artifact")
	if !found {
		return nil, false
	}
	repoCol, _ := findColumn(cols, "repository", "repo")
	severityCol, _ := findColumn(cols, "severity", "severities")

	order := make([]string, 0)
	byKey := make(map[string]*artifactGroup)
	for _, row := range records[1:] {
		artifactVal := cols.get(row, artifactCol)
		if artifactVal == "" {
			continue
		}
		repoHint := ""
		if repoCol != "" {
			repoHint = cols.get(row, repoCol)
		}
		key := repoHint + "\x00" + artifactVal
		g, exists := byKey[key]
		if !exists {
			g = &artifactGroup{RepoHint: repoHint, ImpactedArtifact: artifactVal, Severities: map[string]bool{}}
			byKey[key] = g
			order = append(order, key)
		}
		g.Count++
		if severityCol != "" {
			if sev := cols.get(row, severityCol); sev != "" {
				g.Severities[sev] = true
			}
		}
	}
	groups = make([]artifactGroup, 0, len(order))
	for _, k := range order {
		groups = append(groups, *byKey[k])
	}
	return groups, true
}

func joinDistinct(values []string) string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, ";")
}

func joinDistinctSet(set map[string]bool) string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ";")
}

// ---------------------------------------------------------------------------
// Repo-deployer segregation
// ---------------------------------------------------------------------------

// DeployerSegregationRow is one output row of "jfps xray reports
// segregate-by-deployer".
type DeployerSegregationRow struct {
	Deployer         string
	Repo             string
	ArtifactPath     string
	ImpactedArtifact string
	ViolationCount   int
	Severities       string
	WatchName        string
	LookupError      string
}

// DeployerSegregationOptions configures "jfps xray reports
// segregate-by-deployer": generate an Xray report scoped to a repository-
// or project-scoped watch, then group its findings by the Artifactory user
// who deployed each impacted artifact.
type DeployerSegregationOptions struct {
	Watchname                string
	ReportType               string // violations (default) | exposures
	ExposureCategory         string
	ExposureImpactedArtifact string
	Severities               []string
	UpdatedStart             string
	UpdatedEnd               string
	ReportUploadRepo         string
	OutputDir                string
}

// DeployerSegregationResult is the outcome of one
// SegregateByDeployer call.
type DeployerSegregationResult struct {
	Source            GeneratedReport
	Rows              []DeployerSegregationRow
	SegregatedCSVPath string
	UploadedPaths     []string
}

// SegregateByDeployer generates a watch-scoped report (rejecting any watch
// not scoped to repositories or a project -- deployer attribution requires
// knowing the actual repository an artifact was pushed to) and segregates
// its findings by deployer.
func SegregateByDeployer(client *jfclient.Client, opts DeployerSegregationOptions, onProgress ...progress.Reporter) (DeployerSegregationResult, error) {
	resources, err := resolveWatchScope(client, opts.Watchname, 1)
	if err != nil {
		return DeployerSegregationResult{}, err
	}
	if resources.Builds != nil || resources.ReleaseBundles != nil {
		return DeployerSegregationResult{}, fmt.Errorf("watch %q must be scoped to repositories or a project, not builds/release bundles", opts.Watchname)
	}

	genOpts := ReportGenOptions{
		ReportType:               opts.ReportType,
		Scope:                    ReportScope{Watchname: opts.Watchname},
		Severities:               opts.Severities,
		UpdatedStart:             opts.UpdatedStart,
		UpdatedEnd:               opts.UpdatedEnd,
		ExposureCategory:         opts.ExposureCategory,
		ExposureImpactedArtifact: opts.ExposureImpactedArtifact,
		OutputDir:                opts.OutputDir,
	}
	filters, _ := buildFilters(genOpts, opts.UpdatedStart, opts.UpdatedEnd)
	name := reportName(opts.ReportType, opts.UpdatedStart, opts.UpdatedEnd) + "_deployer_segregated_source"

	report, err := generateOneReport(client, genOpts, resources, filters, name, opts.UpdatedStart, opts.UpdatedEnd)
	result := DeployerSegregationResult{Source: report}
	if err != nil {
		return result, err
	}
	if len(report.CSVData) == 0 {
		return result, nil
	}

	records, err := parseCSVRecords(report.CSVData)
	if err != nil {
		return result, fmt.Errorf("parse exported CSV: %w", err)
	}
	groups, ok := groupRows(records)
	if !ok {
		return result, fmt.Errorf("no impacted-artifact column found in exported CSV")
	}

	rows := concurrency.One(groups, 0, func(g artifactGroup) DeployerSegregationRow {
		row := DeployerSegregationRow{
			ImpactedArtifact: g.ImpactedArtifact,
			ViolationCount:   g.Count,
			Severities:       joinDistinctSet(g.Severities),
			WatchName:        opts.Watchname,
			Deployer:         "UNKNOWN",
		}
		repo, path, lookupErr := resolveArtifactRef(client, g.RepoHint, g.ImpactedArtifact)
		row.LookupError = lookupErr
		if lookupErr == "" {
			row.Repo = repo
			row.ArtifactPath = path
			info, sErr := getStorageInfo(client, repo, path)
			if sErr != "" {
				row.LookupError = sErr
			} else if d := deployerOf(info); d != "" {
				row.Deployer = d
			}
		}
		return row
	}, onProgress...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Deployer != rows[j].Deployer {
			return rows[i].Deployer < rows[j].Deployer
		}
		return rows[i].ArtifactPath < rows[j].ArtifactPath
	})
	result.Rows = rows

	path, err := writeDeployerSegregatedCSV(opts.OutputDir, report.Name, rows)
	if err != nil {
		return result, err
	}
	result.SegregatedCSVPath = path

	if opts.ReportUploadRepo != "" {
		if target, uerr := UploadReportCSV(client, opts.ReportUploadRepo, "", report.Name+".csv", report.CSVData); uerr == nil {
			result.UploadedPaths = append(result.UploadedPaths, target)
		}
		if data, rerr := os.ReadFile(path); rerr == nil {
			if target, uerr := UploadReportCSV(client, opts.ReportUploadRepo, "", filepath.Base(path), data); uerr == nil {
				result.UploadedPaths = append(result.UploadedPaths, target)
			}
		}
	}
	return result, nil
}

func writeDeployerSegregatedCSV(outputDir, reportName string, rows []DeployerSegregationRow) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"deployer", "repo", "artifact_path", "impacted_artifact", "violation_count", "severities", "watch_name", "lookup_error"})
	for _, r := range rows {
		_ = w.Write([]string{r.Deployer, r.Repo, r.ArtifactPath, r.ImpactedArtifact, strconv.Itoa(r.ViolationCount), r.Severities, r.WatchName, r.LookupError})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return writeCSVFile(outputDir, reportName+"_deployer_segregated.csv", buf.Bytes())
}

// ---------------------------------------------------------------------------
// Build-gitrepo segregation
// ---------------------------------------------------------------------------

// GitRepoSegregationRow is one output row of the --build-gitrepo-report
// segregation applied to a build-scoped report.
type GitRepoSegregationRow struct {
	GitURL           string
	BuildName        string
	VCSBranch        string
	VCSRevision      string
	Deployer         string
	Repo             string
	ArtifactPath     string
	ImpactedArtifact string
	ViolationCount   int
	Severities       string
	WatchName        string
	LookupError      string
}

// SegregateByBuildGitRepo groups a build-scoped report's findings by the
// Git repository/revision of the build run that produced the impacted
// artifact. ran is false (no error) when the report isn't build-scoped, or
// its CSV has no impacted-artifact column -- the caller should skip writing
// an output file in that case, leaving the source report untouched.
func SegregateByBuildGitRepo(client *jfclient.Client, resources reportResources, watchName string, records [][]string, onProgress ...progress.Reporter) (rows []GitRepoSegregationRow, ran bool, err error) {
	if resources.Builds == nil {
		return nil, false, nil
	}
	groups, ok := groupRows(records)
	if !ok {
		return nil, false, nil
	}

	candidateBuilds := resources.Builds.Names
	versionsToSearch := resources.Builds.NumberOfLatestVersions
	if versionsToSearch <= 0 {
		versionsToSearch = 5
	}
	cache := newBuildAttributionCache(client)

	rows = make([]GitRepoSegregationRow, 0, len(groups))
	for i, g := range groups {
		row := GitRepoSegregationRow{
			ImpactedArtifact: g.ImpactedArtifact,
			ViolationCount:   g.Count,
			Severities:       joinDistinctSet(g.Severities),
			WatchName:        watchName,
			BuildName:        "UNKNOWN",
			Deployer:         "UNKNOWN",
		}

		repo, path, matchedBuild, matchedRun, lookupErr := resolveArtifactRefForBuild(cache, g.RepoHint, g.ImpactedArtifact, candidateBuilds, versionsToSearch)
		if lookupErr != "" {
			row.LookupError = lookupErr
			rows = append(rows, row)
			progress.Report(onProgress, i+1, len(groups))
			continue
		}
		row.Repo = repo
		row.ArtifactPath = path

		info, sErr := getStorageInfo(client, repo, path)
		if sErr != "" {
			row.LookupError = sErr
			rows = append(rows, row)
			progress.Report(onProgress, i+1, len(groups))
			continue
		}
		if d := deployerOf(info); d != "" {
			row.Deployer = d
		}

		buildName, buildNumber := matchedBuild, matchedRun
		if buildName == "" {
			buildName, buildNumber = matchBuildByChecksum(candidateBuilds, versionsToSearch, info, cache)
		}
		if buildName == "" {
			row.LookupError = "build_artifact_unmatched"
			rows = append(rows, row)
			progress.Report(onProgress, i+1, len(groups))
			continue
		}
		row.BuildName = buildName

		if _, err := cache.get(buildName, buildNumber); err != nil {
			row.LookupError = "build_vcs_missing"
			rows = append(rows, row)
			progress.Report(onProgress, i+1, len(groups))
			continue
		}
		vcs := cache.vcs[buildName+"@"+buildNumber]
		if len(vcs) == 0 {
			row.LookupError = "build_vcs_missing"
		} else {
			urls := make([]string, 0, len(vcs))
			branches := make([]string, 0, len(vcs))
			revs := make([]string, 0, len(vcs))
			for _, v := range vcs {
				urls = append(urls, v.URL)
				branches = append(branches, v.Branch)
				revs = append(revs, v.Revision)
			}
			row.GitURL = joinDistinct(urls)
			row.VCSBranch = joinDistinct(branches)
			row.VCSRevision = joinDistinct(revs)
		}
		rows = append(rows, row)
		progress.Report(onProgress, i+1, len(groups))
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].GitURL != rows[j].GitURL {
			return rows[i].GitURL < rows[j].GitURL
		}
		if rows[i].BuildName != rows[j].BuildName {
			return rows[i].BuildName < rows[j].BuildName
		}
		return rows[i].ArtifactPath < rows[j].ArtifactPath
	})
	return rows, true, nil
}

func writeGitRepoSegregatedCSV(outputDir, reportName string, rows []GitRepoSegregationRow) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"git_url", "build_name", "vcs_branch", "vcs_revision", "deployer", "repo", "artifact_path", "impacted_artifact", "violation_count", "severities", "watch_name", "lookup_error"})
	for _, r := range rows {
		_ = w.Write([]string{r.GitURL, r.BuildName, r.VCSBranch, r.VCSRevision, r.Deployer, r.Repo, r.ArtifactPath, r.ImpactedArtifact, strconv.Itoa(r.ViolationCount), r.Severities, r.WatchName, r.LookupError})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return writeCSVFile(outputDir, reportName+"_build_gitrepo_segregated.csv", buf.Bytes())
}
