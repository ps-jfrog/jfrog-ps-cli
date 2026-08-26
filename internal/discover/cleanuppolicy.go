package discover

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// SearchCriteria is the matching/retention config of a package cleanup
// policy (GET /artifactory/api/cleanup/packages/policies). Retention can be
// authored in days or months depending on how the policy was created (UI vs
// API/Terraform), so both units are kept - only one pair is typically
// non-nil on any given policy.
type SearchCriteria struct {
	PackageTypes                 []string `json:"packageTypes"`
	Repos                        []string `json:"repos"`
	ExcludedRepos                []string `json:"excludedRepos"`
	CreatedBeforeInDays          *int     `json:"createdBeforeInDays"`
	CreatedBeforeInMonths        *int     `json:"createdBeforeInMonths"`
	LastDownloadedBeforeInDays   *int     `json:"lastDownloadedBeforeInDays"`
	LastDownloadedBeforeInMonths *int     `json:"lastDownloadedBeforeInMonths"`
	KeepLastNVersions            *int     `json:"keepLastNVersions"`
	IncludeAllProjects           bool     `json:"includeAllProjects"`
	IncludedProjects             []string `json:"includedProjects"`
}

// CleanupPolicy is one entry returned by the Package Cleanup Policies API.
type CleanupPolicy struct {
	Key            string         `json:"key"`
	Description    string         `json:"description"`
	CronExp        string         `json:"cronExp"`
	ProjectKey     string         `json:"projectKey"`
	Enabled        bool           `json:"enabled"`
	SearchCriteria SearchCriteria `json:"searchCriteria"`
}

// ListCleanupPolicies fetches all package cleanup policies visible to the
// caller (global and project-scoped). The endpoint returns a bare JSON array.
func ListCleanupPolicies(c *jfclient.Client) ([]CleanupPolicy, error) {
	resp, err := c.Get("/artifactory/api/cleanup/packages/policies")
	if err != nil {
		return nil, fmt.Errorf("fetch cleanup policies: %w", err)
	}
	if err := resp.Error("fetch cleanup policies"); err != nil {
		return nil, err
	}
	var policies []CleanupPolicy
	if err := resp.Unmarshal(&policies); err != nil {
		return nil, fmt.Errorf("decode cleanup policies response: %w", err)
	}
	return policies, nil
}

// Applies reports whether the policy targets repo. Artifactory has no
// reverse "policy for this repo" lookup: repos/excludedRepos are wildcard
// patterns (prefix*, *suffix, "**", or an exact name - no regex, no
// mid-string wildcards in practice), so matching happens here. filepath.Match
// implements a superset of that syntax, which is safe since these patterns
// never use '?' or '[...]'.
//
// Policies can also be scoped to specific JFrog Projects (includeAllProjects
// / includedProjects); a repo's projectKey (from /api/storageinfo, "default"
// when unassigned) is matched against that scope too.
func (p CleanupPolicy) Applies(repo RepoSummary) bool {
	sc := p.SearchCriteria
	if len(sc.Repos) > 0 && !matchesAny(repo.RepoKey, sc.Repos) {
		return false
	}
	if len(sc.ExcludedRepos) > 0 && matchesAny(repo.RepoKey, sc.ExcludedRepos) {
		return false
	}
	if len(sc.PackageTypes) > 0 && !containsFold(sc.PackageTypes, repo.PackageType) {
		return false
	}
	if !sc.IncludeAllProjects && len(sc.IncludedProjects) > 0 && !containsFold(sc.IncludedProjects, repo.ProjectKey) {
		return false
	}
	return true
}

func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}

func containsFold(list []string, val string) bool {
	for _, v := range list {
		if strings.EqualFold(v, val) {
			return true
		}
	}
	return false
}

// PolicyMatch is the pipe-joined detail of every policy matching a repo, in
// matched order - one CSV cell per field regardless of match count.
type PolicyMatch struct {
	Keys                       string
	CronExps                   string
	Enabled                    string
	CreatedBeforeDays          string
	CreatedBeforeMonths        string
	LastDownloadedBeforeDays   string
	LastDownloadedBeforeMonths string
	KeepLastNVersions          string
}

// MatchPolicies finds every policy applying to repo and joins their fields.
func MatchPolicies(repo RepoSummary, policies []CleanupPolicy) PolicyMatch {
	var keys, crons, enabled, cbd, cbm, ldbd, ldbm, keep []string
	for _, p := range policies {
		if !p.Applies(repo) {
			continue
		}
		sc := p.SearchCriteria
		keys = append(keys, p.Key)
		crons = append(crons, p.CronExp)
		enabled = append(enabled, strconv.FormatBool(p.Enabled))
		cbd = append(cbd, FormatIntPtr(sc.CreatedBeforeInDays))
		cbm = append(cbm, FormatIntPtr(sc.CreatedBeforeInMonths))
		ldbd = append(ldbd, FormatIntPtr(sc.LastDownloadedBeforeInDays))
		ldbm = append(ldbm, FormatIntPtr(sc.LastDownloadedBeforeInMonths))
		keep = append(keep, FormatIntPtr(sc.KeepLastNVersions))
	}
	return PolicyMatch{
		Keys:                       strings.Join(keys, "|"),
		CronExps:                   strings.Join(crons, "|"),
		Enabled:                    strings.Join(enabled, "|"),
		CreatedBeforeDays:          strings.Join(cbd, "|"),
		CreatedBeforeMonths:        strings.Join(cbm, "|"),
		LastDownloadedBeforeDays:   strings.Join(ldbd, "|"),
		LastDownloadedBeforeMonths: strings.Join(ldbm, "|"),
		KeepLastNVersions:          strings.Join(keep, "|"),
	}
}

// FormatIntPtr renders a *int as a string, or "" when nil.
func FormatIntPtr(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
