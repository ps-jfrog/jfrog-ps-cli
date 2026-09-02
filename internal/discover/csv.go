package discover

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
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
