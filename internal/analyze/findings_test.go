package analyze

import (
	"testing"
	"time"
)

func mkRow(fields map[string]string) Row {
	return Row{Fields: fields}
}

func TestComputeFindings_NamingAndSize(t *testing.T) {
	header := []string{"repo_name", "files_count", "usage_gb"}
	rows := []Row{
		mkRow(map[string]string{"repo_name": "Weird_Name", "files_count": "1", "usage_gb": "1"}),
		mkRow(map[string]string{"repo_name": "clean-name", "files_count": "0", "usage_gb": "0"}),
		mkRow(map[string]string{"repo_name": "huge-repo", "files_count": "1", "usage_gb": "1000"}),
		mkRow(map[string]string{"repo_name": "massive-repo", "files_count": "999999", "usage_gb": "1"}),
	}
	opts := DefaultOptions()
	ComputeFindings(header, rows, opts, time.Now())

	if !hasFinding(rows[0], "uppercase") || !hasFinding(rows[0], "underscore") || !hasFinding(rows[0], "naming-nonconformant") {
		t.Fatalf("expected naming findings on row 0, got %v", rows[0].Findings)
	}
	if !hasFinding(rows[1], "empty") {
		t.Fatalf("expected 'empty' finding on row 1, got %v", rows[1].Findings)
	}
	if !hasFinding(rows[2], "huge") {
		t.Fatalf("expected 'huge' finding on row 2, got %v", rows[2].Findings)
	}
	if !hasFinding(rows[3], "massive-files") {
		t.Fatalf("expected 'massive-files' finding on row 3, got %v", rows[3].Findings)
	}
}

func TestComputeFindings_StaleSkippedWithoutActivityColumn(t *testing.T) {
	header := []string{"repo_name", "files_count"} // no last_activity_date column
	rows := []Row{mkRow(map[string]string{"repo_name": "a", "files_count": "1"})}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())
	if hasFinding(rows[0], "stale") {
		t.Fatalf("expected no 'stale' finding when last_activity_date column is absent, got %v", rows[0].Findings)
	}
}

func TestComputeFindings_StaleWhenOldOrMissing(t *testing.T) {
	header := []string{"repo_name", "files_count", "last_activity_date"}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []Row{
		mkRow(map[string]string{"repo_name": "old", "files_count": "1", "last_activity_date": "2020-01-01T00:00:00Z"}),
		mkRow(map[string]string{"repo_name": "recent", "files_count": "1", "last_activity_date": "2025-12-01T00:00:00Z"}),
		mkRow(map[string]string{"repo_name": "no-activity", "files_count": "1", "last_activity_date": ""}),
		mkRow(map[string]string{"repo_name": "empty-repo", "files_count": "0", "last_activity_date": ""}),
	}
	ComputeFindings(header, rows, DefaultOptions(), now)

	if !hasFinding(rows[0], "stale") {
		t.Errorf("expected 'stale' for old activity, got %v", rows[0].Findings)
	}
	if hasFinding(rows[1], "stale") {
		t.Errorf("expected no 'stale' for recent activity, got %v", rows[1].Findings)
	}
	if !hasFinding(rows[2], "stale") {
		t.Errorf("expected 'stale' when files exist but no activity recorded, got %v", rows[2].Findings)
	}
	if hasFinding(rows[3], "stale") {
		t.Errorf("expected empty repos to be flagged 'empty', not 'stale', got %v", rows[3].Findings)
	}
}

func TestComputeFindings_CleanupPolicyFindings(t *testing.T) {
	header := []string{"repo_name", "files_count", "cleanup_policy_keys"}
	rows := []Row{
		mkRow(map[string]string{"repo_name": "no-policy", "files_count": "1", "cleanup_policy_keys": ""}),
		mkRow(map[string]string{"repo_name": "one-policy", "files_count": "1", "cleanup_policy_keys": "p1"}),
		mkRow(map[string]string{"repo_name": "two-policies", "files_count": "1", "cleanup_policy_keys": "p1|p2"}),
	}
	ComputeFindings(header, rows, DefaultOptions(), time.Now())

	if !hasFinding(rows[0], "no-cleanup-policy") {
		t.Errorf("expected 'no-cleanup-policy', got %v", rows[0].Findings)
	}
	if hasFinding(rows[1], "no-cleanup-policy") || hasFinding(rows[1], "multi-policy-match") {
		t.Errorf("expected no cleanup-policy findings for single match, got %v", rows[1].Findings)
	}
	if !hasFinding(rows[2], "multi-policy-match") {
		t.Errorf("expected 'multi-policy-match', got %v", rows[2].Findings)
	}
}

func TestDefs_GatedByColumnPresence(t *testing.T) {
	opts := DefaultOptions()

	withoutExtra := opts.Defs([]string{"repo_name", "files_count"})
	if defsContain(withoutExtra, "stale") || defsContain(withoutExtra, "no-cleanup-policy") {
		t.Fatalf("expected stale/no-cleanup-policy defs to be absent without their columns, got %v", defIDs(withoutExtra))
	}

	withExtra := opts.Defs([]string{"repo_name", "files_count", "last_activity_date", "cleanup_policy_keys"})
	if !defsContain(withExtra, "stale") || !defsContain(withExtra, "no-cleanup-policy") || !defsContain(withExtra, "multi-policy-match") {
		t.Fatalf("expected stale/policy defs to be present with their columns, got %v", defIDs(withExtra))
	}
}

func hasFinding(r Row, id string) bool {
	for _, f := range r.Findings {
		if f == id {
			return true
		}
	}
	return false
}

func defsContain(defs []FindingDef, id string) bool {
	for _, d := range defs {
		if d.ID == id {
			return true
		}
	}
	return false
}

func defIDs(defs []FindingDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.ID
	}
	return out
}

func TestHumanInt(t *testing.T) {
	cases := map[int]string{0: "0", 500: "500", 500000: "500,000", 1000000: "1,000,000", -1234: "-1,234"}
	for in, want := range cases {
		if got := humanInt(in); got != want {
			t.Errorf("humanInt(%d) = %q, want %q", in, got, want)
		}
	}
}
