package discover

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"jfrogpscli/internal/instance"
)

// reportHeader is the column set written by WriteReportCSV. It's also the
// contract internal/analyze parses against, so a report CSV from this tool
// (or from jf-audit, its predecessor) analyzes the same way.
var reportHeader = []string{
	"repo_name", "package_type", "repo_type", "project_key", "files_count", "usage_gb",
	"created_date", "created_by", "last_push_date", "last_push_by", "last_pull_date", "last_activity_date",
}

var reportPolicyHeader = []string{
	"cleanup_policy_keys", "cleanup_policy_enabled", "cleanup_policy_cron_exps",
	"cleanup_policy_created_before_days", "cleanup_policy_created_before_months",
	"cleanup_policy_last_downloaded_before_days", "cleanup_policy_last_downloaded_before_months",
	"cleanup_policy_keep_last_n_versions",
}

// WriteReportCSV writes rows to w. withPolicies must match whichever
// ReportOptions.WithPolicies value produced rows, so the header lines up
// with populated (or intentionally absent) policy columns.
func WriteReportCSV(w io.Writer, rows []Row, withPolicies bool) error {
	cw := csv.NewWriter(w)

	header := append([]string{}, reportHeader...)
	if withPolicies {
		header = append(header, reportPolicyHeader...)
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, r := range rows {
		record := []string{
			r.RepoKey, r.PackageType, r.RepoType, r.ProjectKey, strconv.Itoa(r.FilesCount),
			strconv.FormatFloat(r.UsageGB(), 'f', 4, 64),
			r.Activity.Created, r.Activity.CreatedBy, r.Activity.LastPush, r.Activity.LastPushBy,
			r.Activity.LastPull, r.Activity.LastActivity,
		}
		if withPolicies {
			record = append(record,
				r.Policy.Keys, r.Policy.Enabled, r.Policy.CronExps,
				r.Policy.CreatedBeforeDays, r.Policy.CreatedBeforeMonths,
				r.Policy.LastDownloadedBeforeDays, r.Policy.LastDownloadedBeforeMonths,
				r.Policy.KeepLastNVersions,
			)
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("write csv row for %s: %w", r.RepoKey, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WritePoliciesCSV writes one row per repo x matched cleanup policy (or a
// single blank-policy row for a repo matching none), for drilling into
// per-policy detail or spotting ambiguous/overlapping coverage.
func WritePoliciesCSV(w io.Writer, repos []RepoSummary, policies []CleanupPolicy) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"repo_name", "package_type", "policy_key", "policy_enabled",
		"created_before_days", "created_before_months",
		"last_downloaded_before_days", "last_downloaded_before_months",
		"keep_last_n_versions", "cron_exp",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, repo := range repos {
		var matched []CleanupPolicy
		for _, p := range policies {
			if p.Applies(repo) {
				matched = append(matched, p)
			}
		}
		if len(matched) == 0 {
			if err := cw.Write([]string{repo.RepoKey, repo.PackageType, "", "", "", "", "", "", "", ""}); err != nil {
				return fmt.Errorf("write csv row for %s: %w", repo.RepoKey, err)
			}
			continue
		}
		for _, p := range matched {
			if err := cw.Write([]string{
				repo.RepoKey, repo.PackageType, p.Key, strconv.FormatBool(p.Enabled),
				FormatIntPtr(p.SearchCriteria.CreatedBeforeInDays),
				FormatIntPtr(p.SearchCriteria.CreatedBeforeInMonths),
				FormatIntPtr(p.SearchCriteria.LastDownloadedBeforeInDays),
				FormatIntPtr(p.SearchCriteria.LastDownloadedBeforeInMonths),
				FormatIntPtr(p.SearchCriteria.KeepLastNVersions),
				p.CronExp,
			}); err != nil {
				return fmt.Errorf("write csv row for %s: %w", repo.RepoKey, err)
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteProjectRolesCSV writes project roles to CSV format (one row per project + role).
func WriteProjectRolesCSV(w io.Writer, roles []ProjectRoleDetail) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"project_key", "project_name", "role_name", "role_type", "environments", "actions",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, role := range roles {
		environments := ""
		if len(role.Environments) > 0 {
			environments = joinStrings(role.Environments)
		}
		actions := ""
		if len(role.Actions) > 0 {
			actions = joinStrings(role.Actions)
		}

		if err := cw.Write([]string{
			role.ProjectKey,
			role.DisplayName,
			role.RoleName,
			role.RoleType,
			environments,
			actions,
		}); err != nil {
			return fmt.Errorf("write csv row for %s/%s: %w", role.ProjectKey, role.RoleName, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteProjectRepositoriesCSV writes project-repository mappings to CSV format.
func WriteProjectRepositoriesCSV(w io.Writer, mappings []instance.ProjectRepository) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"project_key", "repo_name", "package_type", "repo_type",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, mapping := range mappings {
		if err := cw.Write([]string{
			mapping.ProjectKey, mapping.RepoKey, mapping.PackageType, mapping.RepoType,
		}); err != nil {
			return fmt.Errorf("write csv row for %s/%s: %w", mapping.ProjectKey, mapping.RepoKey, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustPoliciesCSV writes AppTrust/Unified Policy Service policies to CSV format.
func WriteAppTrustPoliciesCSV(w io.Writer, policies []instance.AppTrustPolicy) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"id", "name", "description", "enabled", "mode", "scope_type",
		"stage_key", "stage_gate", "waiver_request_config", "created_at", "created_by",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, p := range policies {
		if err := cw.Write([]string{
			p.ID, p.Name, p.Description, strconv.FormatBool(p.Enabled), p.Mode, p.Scope.ScopeType,
			p.StageKey, p.StageGate, p.WaiverRequestConfig, p.CreatedAt, p.CreatedBy,
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", p.Name, err)
		}
	}
	cw.Flush()
	return cw.Error()
}


// joinStrings joins a slice of strings with pipe separator for CSV.
func joinStrings(s []string) string {
	if len(s) == 0 {
		return ""
	}
	result := ""
	for i, str := range s {
		if i > 0 {
			result += "|"
		}
		result += str
	}
	return result
}

// WriteAppTrustApplicationsCSV writes AppTrust applications to CSV format.
func WriteAppTrustApplicationsCSV(w io.Writer, apps []instance.AppTrustApplication) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"application_key", "application_name", "project_key", "description",
		"criticality", "maturity_level", "user_owners", "group_owners",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, app := range apps {
		if err := cw.Write([]string{
			app.ApplicationKey, app.ApplicationName, app.ProjectKey, app.Description,
			app.Criticality, app.MaturityLevel, joinStrings(app.UserOwners), joinStrings(app.GroupOwners),
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", app.ApplicationName, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustRulesCSV writes AppTrust/Unified Policy Service rules to CSV format.
func WriteAppTrustRulesCSV(w io.Writer, rules []instance.AppTrustRule) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"id", "name", "description", "template_id", "is_custom", "created_at", "created_by",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, r := range rules {
		if err := cw.Write([]string{
			r.ID, r.Name, r.Description, r.TemplateID, strconv.FormatBool(r.IsCustom), r.CreatedAt, r.CreatedBy,
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", r.Name, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustTemplatesCSV writes AppTrust/Unified Policy Service templates to CSV format.
func WriteAppTrustTemplatesCSV(w io.Writer, templates []instance.AppTrustTemplate) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"id", "name", "description", "version", "category", "scanners", "data_source_type", "is_custom",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, t := range templates {
		if err := cw.Write([]string{
			t.ID, t.Name, t.Description, t.Version, t.Category, joinStrings(t.Scanners),
			t.DataSourceType, strconv.FormatBool(t.IsCustom),
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", t.Name, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustStagesCSV writes AppTrust lifecycle stages (one row per
// project x stage) to CSV format.
func WriteAppTrustStagesCSV(w io.Writer, stages []instance.AppTrustStage) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"project_key", "category", "stage_name", "scope", "gates",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, s := range stages {
		if err := cw.Write([]string{
			s.ProjectKey, s.Category, s.StageName, s.Scope, joinStrings(s.Gates),
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", s.StageName, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustEvidenceCSV writes AppTrust evidence records to CSV format.
func WriteAppTrustEvidenceCSV(w io.Writer, evidence []instance.AppTrustEvidence) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"id", "repository", "path", "name", "predicate_category", "predicate_type",
		"stage", "verified", "created_at", "created_by",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, e := range evidence {
		if err := cw.Write([]string{
			e.ID, e.Repository, e.Path, e.Name, e.PredicateCategory, e.PredicateType,
			e.Stage, strconv.FormatBool(e.Verified), e.CreatedAt, e.CreatedBy,
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", e.Name, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustReleaseBundlesCSV writes AppTrust/Release Lifecycle Management
// release bundle v2 names to CSV format.
func WriteAppTrustReleaseBundlesCSV(w io.Writer, bundles []instance.AppTrustReleaseBundle) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"repository_key", "release_bundle_name", "project_key", "project_name",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, b := range bundles {
		if err := cw.Write([]string{
			b.RepositoryKey, b.ReleaseBundleName, b.ProjectKey, b.ProjectName,
		}); err != nil {
			return fmt.Errorf("write csv row for %s: %w", b.ReleaseBundleName, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustApplicationPoliciesCSV writes the junction showing which policies
// apply to which applications (scope mechanism).
func WriteAppTrustApplicationPoliciesCSV(w io.Writer, links []instance.AppTrustApplicationPolicy) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"application_key", "application_name", "project_key", "policy_id", "policy_name",
		"scope_type", "scope_mechanism", "policy_mode", "policy_enabled",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, link := range links {
		if err := cw.Write([]string{
			link.ApplicationKey, link.ApplicationName, link.ProjectKey, link.PolicyID, link.PolicyName,
			link.ScopeType, link.ScopeMechanism, link.PolicyMode, strconv.FormatBool(link.PolicyEnabled),
		}); err != nil {
			return fmt.Errorf("write csv row for %s/%s: %w", link.ApplicationName, link.PolicyName, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteAppTrustPolicyRulesCSV writes the junction showing which rules implement
// which policies (with template resolution).
func WriteAppTrustPolicyRulesCSV(w io.Writer, links []instance.AppTrustPolicyRule) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"policy_id", "policy_name", "rule_id", "rule_name", "rule_order", "template_id", "template_name",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, link := range links {
		if err := cw.Write([]string{
			link.PolicyID, link.PolicyName, link.RuleID, link.RuleName, strconv.Itoa(link.RuleOrder),
			link.TemplateID, link.TemplateName,
		}); err != nil {
			return fmt.Errorf("write csv row for %s/%s: %w", link.PolicyName, link.RuleName, err)
		}
	}
	cw.Flush()
	return cw.Error()
}
