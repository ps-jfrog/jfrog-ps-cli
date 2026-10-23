// Package discover's AppTrust/governance collection, backed by the JFrog
// AppTrust REST API reference
// (https://docs.jfrog.com/governance/reference/apptrust-rest-api-reference):
// AppTrust Service API (applications), JFrog Unified Policy Service API
// (policies/rules/templates - also shared with Xray/Curation governance),
// Stages & Lifecycle API (per-project, so Stages are fetched once per
// project key), JFrog Artifactory Evidence API (evidence search), and
// Release Lifecycle Management API (release bundle v2 names).
//
// Every endpoint here 404s cleanly (empty slice, nil error) when AppTrust/
// governance isn't licensed on the instance - callers never need to special-
// case "not available" vs "available but empty".
package discover

import (
	"fmt"
	"net/url"
	"sync"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

const appTrustPageLimit = 1000

// GetAppTrustApplications fetches every application from
// GET /apptrust/api/v1/applications, paginating via offset/limit until a
// page returns fewer than appTrustPageLimit results.
func GetAppTrustApplications(client *jfclient.Client) ([]instance.AppTrustApplication, error) {
	var all []instance.AppTrustApplication
	offset := 0
	for {
		var page struct {
			Total        int                            `json:"total"`
			Offset       int                            `json:"offset"`
			Limit        int                            `json:"limit"`
			Applications []instance.AppTrustApplication `json:"applications"`
		}
		resp, err := client.Get(fmt.Sprintf("/apptrust/api/v1/applications?limit=%d&offset=%d", appTrustPageLimit, offset))
		if err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Error("fetch apptrust applications"); err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Unmarshal(&page); err != nil {
			return all, fmt.Errorf("parse apptrust applications: %w", err)
		}
		all = append(all, page.Applications...)
		if len(page.Applications) < appTrustPageLimit {
			break
		}
		offset += appTrustPageLimit
	}
	return all, nil
}

// GetAppTrustPolicies fetches every policy from
// GET /unifiedpolicy/api/v1/policies, paginating via offset/limit.
func GetAppTrustPolicies(client *jfclient.Client) ([]instance.AppTrustPolicy, error) {
	var all []instance.AppTrustPolicy
	offset := 0
	for {
		var page struct {
			Items      []instance.AppTrustPolicy `json:"items"`
			TotalCount int                       `json:"total_count"`
		}
		resp, err := client.Get(fmt.Sprintf("/unifiedpolicy/api/v1/policies?limit=%d&offset=%d", appTrustPageLimit, offset))
		if err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Error("fetch apptrust policies"); err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Unmarshal(&page); err != nil {
			return all, fmt.Errorf("parse apptrust policies: %w", err)
		}
		all = append(all, page.Items...)
		if len(page.Items) < appTrustPageLimit {
			break
		}
		offset += appTrustPageLimit
	}
	return all, nil
}

// GetAppTrustRules fetches every rule from GET /unifiedpolicy/api/v1/rules,
// paginating via offset/limit.
func GetAppTrustRules(client *jfclient.Client) ([]instance.AppTrustRule, error) {
	var all []instance.AppTrustRule
	offset := 0
	for {
		var page struct {
			Items []instance.AppTrustRule `json:"items"`
		}
		resp, err := client.Get(fmt.Sprintf("/unifiedpolicy/api/v1/rules?limit=%d&offset=%d", appTrustPageLimit, offset))
		if err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Error("fetch apptrust rules"); err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Unmarshal(&page); err != nil {
			return all, fmt.Errorf("parse apptrust rules: %w", err)
		}
		all = append(all, page.Items...)
		if len(page.Items) < appTrustPageLimit {
			break
		}
		offset += appTrustPageLimit
	}
	return all, nil
}

// GetAppTrustTemplates fetches every template from
// GET /unifiedpolicy/api/v1/templates, paginating via offset/limit.
func GetAppTrustTemplates(client *jfclient.Client) ([]instance.AppTrustTemplate, error) {
	var all []instance.AppTrustTemplate
	offset := 0
	for {
		var page struct {
			Items []instance.AppTrustTemplate `json:"items"`
		}
		resp, err := client.Get(fmt.Sprintf("/unifiedpolicy/api/v1/templates?limit=%d&offset=%d", appTrustPageLimit, offset))
		if err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Error("fetch apptrust templates"); err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Unmarshal(&page); err != nil {
			return all, fmt.Errorf("parse apptrust templates: %w", err)
		}
		all = append(all, page.Items...)
		if len(page.Items) < appTrustPageLimit {
			break
		}
		offset += appTrustPageLimit
	}
	return all, nil
}

// GetAppTrustStages fetches the lifecycle stages (with their gates) for one
// project from GET /apptrust/api/v1/projects/{project_key}/lifecycle/stages.
// There is no instance-wide "list all stages" endpoint - stages are always
// scoped to a project - so GetAppTrustStagesForProjects below is the entry
// point callers actually want.
func GetAppTrustStages(client *jfclient.Client, projectKey string) ([]instance.AppTrustStage, error) {
	var categories []struct {
		Category string `json:"category"`
		Stages   []struct {
			StageName string   `json:"stage_name"`
			Scope     string   `json:"scope"`
			Gates     []string `json:"gates"`
			GatesInfo []struct {
				Name          string `json:"name"`
				PoliciesCount int    `json:"policies_count"`
			} `json:"gates_info"`
		} `json:"stages"`
	}

	resp, err := client.Get(fmt.Sprintf("/apptrust/api/v1/projects/%s/lifecycle/stages", projectKey))
	if err != nil {
		return nil, nil
	}
	if err := resp.Error("fetch apptrust stages"); err != nil {
		return nil, nil
	}
	if err := resp.Unmarshal(&categories); err != nil {
		return nil, fmt.Errorf("parse apptrust stages for project %s: %w", projectKey, err)
	}

	var out []instance.AppTrustStage
	for _, cat := range categories {
		for _, s := range cat.Stages {
			stage := instance.AppTrustStage{
				ProjectKey: projectKey,
				Category:   cat.Category,
				StageName:  s.StageName,
				Scope:      s.Scope,
				Gates:      s.Gates,
			}
			for _, gi := range s.GatesInfo {
				stage.GatesInfo = append(stage.GatesInfo, instance.AppTrustGateInfo{
					Name:          gi.Name,
					PoliciesCount: gi.PoliciesCount,
				})
			}
			out = append(out, stage)
		}
	}
	return out, nil
}

// GetAppTrustStagesForProjects fetches lifecycle stages for every given
// project key, concurrently (bounded by concurrency), and flattens them into
// one slice. A project with no stages configured (or a 404 because the
// project doesn't use AppTrust) is silently skipped rather than treated as
// an error. onProgress, if non-nil, is called as each project finishes with
// (done, total) so a caller can drive a live progress bar across
// potentially hundreds of projects.
func GetAppTrustStagesForProjects(client *jfclient.Client, projectKeys []string, concurrency int, onProgress func(done, total int)) []instance.AppTrustStage {
	if len(projectKeys) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([][]instance.AppTrustStage, len(projectKeys))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i, key := range projectKeys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			stages, err := GetAppTrustStages(client, key)
			mu.Lock()
			if err == nil {
				results[i] = stages
			}
			done++
			if onProgress != nil {
				onProgress(done, len(projectKeys))
			}
			mu.Unlock()
		}(i, key)
	}
	wg.Wait()

	var all []instance.AppTrustStage
	seen := map[string]bool{} // de-dup global stages reported identically per-project
	for _, stages := range results {
		for _, s := range stages {
			dedupeKey := s.ProjectKey + "/" + s.StageName
			if s.Scope == "global" {
				dedupeKey = "global/" + s.StageName
			}
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			all = append(all, s)
		}
	}
	return all
}

// GetAppTrustEvidenceForRepo fetches every evidence record attached to one
// repository from GET /evidence/api/v1/evidence-search?repository_key=...,
// paginating via offset/limit. There is no instance-wide "search all repos"
// mode - repository_key is a required param (confirmed live: omitting it
// returns 400 "repoKey is empty") - so GetAppTrustEvidenceForRepos below,
// which calls this once per repo, is the entry point callers actually want.
func GetAppTrustEvidenceForRepo(client *jfclient.Client, repoKey string) ([]instance.AppTrustEvidence, error) {
	var all []instance.AppTrustEvidence
	offset := 0
	for {
		var page struct {
			Evidence []instance.AppTrustEvidence `json:"evidence"`
		}
		resp, err := client.Get(fmt.Sprintf("/evidence/api/v1/evidence-search?repository_key=%s&limit=%d&offset=%d", url.QueryEscape(repoKey), appTrustPageLimit, offset))
		if err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Error("fetch apptrust evidence"); err != nil {
			if offset == 0 {
				return nil, nil
			}
			return all, nil
		}
		if err := resp.Unmarshal(&page); err != nil {
			return all, fmt.Errorf("parse apptrust evidence for repo %s: %w", repoKey, err)
		}
		all = append(all, page.Evidence...)
		if len(page.Evidence) < appTrustPageLimit {
			break
		}
		offset += appTrustPageLimit
	}
	return all, nil
}

// GetAppTrustEvidenceForRepos fetches evidence for every given repo key,
// concurrently (bounded by concurrency), and flattens the results into one
// slice. A repo with no evidence (or a 404/400 because evidence isn't
// available for it) is silently skipped rather than treated as an error.
// onProgress, if non-nil, is called as each repo finishes with (done,
// total) so a caller can drive a live progress bar across however many
// repos there are.
func GetAppTrustEvidenceForRepos(client *jfclient.Client, repoKeys []string, concurrency int, onProgress func(done, total int)) []instance.AppTrustEvidence {
	if len(repoKeys) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([][]instance.AppTrustEvidence, len(repoKeys))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i, key := range repoKeys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			evidence, err := GetAppTrustEvidenceForRepo(client, key)
			mu.Lock()
			if err == nil {
				results[i] = evidence
			}
			done++
			if onProgress != nil {
				onProgress(done, len(repoKeys))
			}
			mu.Unlock()
		}(i, key)
	}
	wg.Wait()

	var all []instance.AppTrustEvidence
	for _, evidence := range results {
		all = append(all, evidence...)
	}
	return all
}

// GetAppTrustReleaseBundles fetches every Release Bundle v2 name from
// GET /lifecycle/api/v2/release_bundle/names. This endpoint is not paginated
// (it returns every name/repo pairing in one call).
func GetAppTrustReleaseBundles(client *jfclient.Client) ([]instance.AppTrustReleaseBundle, error) {
	var page struct {
		ReleaseBundles []instance.AppTrustReleaseBundle `json:"release_bundles"`
	}
	resp, err := client.Get("/lifecycle/api/v2/release_bundle/names")
	if err != nil {
		return nil, nil
	}
	if err := resp.Error("fetch apptrust release bundles"); err != nil {
		return nil, nil
	}
	if err := resp.Unmarshal(&page); err != nil {
		return nil, fmt.Errorf("parse apptrust release bundles: %w", err)
	}
	return page.ReleaseBundles, nil
}

// BuildAppTrustApplicationPolicies creates junctions between applications and
// the policies that target them, based on policy scope (global, project, app-key, or label).
func BuildAppTrustApplicationPolicies(apps []instance.AppTrustApplication, policies []instance.AppTrustPolicy) []instance.AppTrustApplicationPolicy {
	var result []instance.AppTrustApplicationPolicy

	for _, app := range apps {
		for _, policy := range policies {
			matches := false
			scopeMechanism := ""

			// Global scope = all applications
			if policy.Scope.ScopeType == "global" {
				matches = true
				scopeMechanism = "global"
			} else if policy.Scope.ScopeType == "project" {
				// Project scope = apps in that project
				for _, pk := range policy.Scope.ProjectKeys {
					if app.ProjectKey == pk {
						matches = true
						scopeMechanism = "project:" + pk
						break
					}
				}
			} else if policy.Scope.ScopeType == "application" {
				// App-key scope
				for _, appKey := range policy.Scope.ApplicationKeys {
					if app.ApplicationKey == appKey {
						matches = true
						scopeMechanism = "app-key:" + appKey
						break
					}
				}
				// Label scope (if no app-key match, try labels)
				if !matches {
					for _, label := range policy.Scope.ApplicationLabels {
						// Labels format: "key:value", app has label key:value pairs
						for _, appLabel := range app.Labels {
							if label == appLabel.Key+":"+appLabel.Value {
								matches = true
								scopeMechanism = "label:" + label
								break
							}
						}
						if matches {
							break
						}
					}
				}
			}

			if matches {
				result = append(result, instance.AppTrustApplicationPolicy{
					ApplicationKey:  app.ApplicationKey,
					ApplicationName: app.ApplicationName,
					ProjectKey:      app.ProjectKey,
					PolicyID:        policy.ID,
					PolicyName:      policy.Name,
					ScopeType:       policy.Scope.ScopeType,
					ScopeMechanism:  scopeMechanism,
					PolicyMode:      policy.Mode,
					PolicyEnabled:   policy.Enabled,
				})
			}
		}
	}

	return result
}

// BuildAppTrustPolicyRules creates junctions between policies and their rules,
// with template information.
func BuildAppTrustPolicyRules(policies []instance.AppTrustPolicy, rules []instance.AppTrustRule, templates []instance.AppTrustTemplate) []instance.AppTrustPolicyRule {
	// Build template lookup
	templateByID := make(map[string]instance.AppTrustTemplate)
	for _, t := range templates {
		templateByID[t.ID] = t
	}

	// Build rule lookup
	ruleByID := make(map[string]instance.AppTrustRule)
	for _, r := range rules {
		ruleByID[r.ID] = r
	}

	var result []instance.AppTrustPolicyRule
	for _, policy := range policies {
		for ruleOrder, ruleID := range policy.RuleIDs {
			rule, ruleOk := ruleByID[ruleID]
			if !ruleOk {
				continue // Skip if rule not found
			}

			templateName := ""
			if rule.TemplateID != "" {
				if tmpl, ok := templateByID[rule.TemplateID]; ok {
					templateName = tmpl.Name
				}
			}

			result = append(result, instance.AppTrustPolicyRule{
				PolicyID:     policy.ID,
				PolicyName:   policy.Name,
				RuleID:       ruleID,
				RuleName:     rule.Name,
				RuleOrder:    ruleOrder,
				TemplateID:   rule.TemplateID,
				TemplateName: templateName,
			})
		}
	}

	return result
}
