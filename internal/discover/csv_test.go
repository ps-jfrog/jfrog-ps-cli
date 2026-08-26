package discover

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestWriteReportCSV_WithAndWithoutPolicies(t *testing.T) {
	rows := []Row{
		{
			RepoSummary: RepoSummary{RepoKey: "docker-local", PackageType: "Docker", RepoType: "LOCAL", ProjectKey: "team-a", FilesCount: 3, UsedSpaceInBytes: 1073741824},
			Activity:    Activity{Created: "2024-01-01T00:00:00Z", LastPush: "2024-06-01T00:00:00Z"},
			Policy:      PolicyMatch{Keys: "p1", Enabled: "true"},
		},
	}

	var withPolicies bytes.Buffer
	if err := WriteReportCSV(&withPolicies, rows, true); err != nil {
		t.Fatalf("WriteReportCSV (with policies) returned error: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(withPolicies.String())).ReadAll()
	if err != nil {
		t.Fatalf("failed to parse written CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header + 1 row, got %d records", len(records))
	}
	if got := len(records[0]); got != len(reportHeader)+len(reportPolicyHeader) {
		t.Fatalf("expected %d header columns, got %d", len(reportHeader)+len(reportPolicyHeader), got)
	}
	if records[1][0] != "docker-local" || records[1][3] != "team-a" || records[1][5] != "1.0000" {
		t.Fatalf("unexpected row data: %v", records[1])
	}

	var withoutPolicies bytes.Buffer
	if err := WriteReportCSV(&withoutPolicies, rows, false); err != nil {
		t.Fatalf("WriteReportCSV (without policies) returned error: %v", err)
	}
	records2, err := csv.NewReader(strings.NewReader(withoutPolicies.String())).ReadAll()
	if err != nil {
		t.Fatalf("failed to parse written CSV: %v", err)
	}
	if got := len(records2[0]); got != len(reportHeader) {
		t.Fatalf("expected %d header columns without policies, got %d", len(reportHeader), got)
	}
}

func TestWritePoliciesCSV_BlankRowWhenNoMatch(t *testing.T) {
	repos := []RepoSummary{{RepoKey: "npm-local", PackageType: "Npm"}}
	policies := []CleanupPolicy{{Key: "docker-only", SearchCriteria: SearchCriteria{Repos: []string{"docker-*"}}}}

	var buf bytes.Buffer
	if err := WritePoliciesCSV(&buf, repos, policies); err != nil {
		t.Fatalf("WritePoliciesCSV returned error: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("failed to parse written CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected header + 1 row, got %d", len(records))
	}
	if records[1][0] != "npm-local" || records[1][2] != "" {
		t.Fatalf("expected blank policy_key for unmatched repo, got %v", records[1])
	}
}

func TestWritePoliciesCSV_OneRowPerMatch(t *testing.T) {
	repos := []RepoSummary{{RepoKey: "docker-local", PackageType: "Docker"}}
	policies := []CleanupPolicy{
		{Key: "p1", SearchCriteria: SearchCriteria{Repos: []string{"docker-*"}}},
		{Key: "p2", SearchCriteria: SearchCriteria{Repos: []string{"**"}}},
	}

	var buf bytes.Buffer
	if err := WritePoliciesCSV(&buf, repos, policies); err != nil {
		t.Fatalf("WritePoliciesCSV returned error: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("failed to parse written CSV: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected header + 2 matched rows, got %d: %v", len(records), records)
	}
}
