package xray

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jfrogpscli/internal/jfclient"
)

func TestListReportsSendsFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/xray/api/v1/reports" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["page_number"] != float64(1) || body["num_of_rows"] != float64(100) {
			t.Fatalf("unexpected paging: %v", body)
		}
		filters, ok := body["filters"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected filters object, got %v", body["filters"])
		}
		if filters["status"] != "Completed" || filters["type"] != "license" {
			t.Fatalf("unexpected filters: %v", filters)
		}
		_, _ = w.Write([]byte(`{"reports":[{"id":1,"name":"r1","status":"Completed","report_type":"license","end_time":"2024-01-01T00:00:00Z"}],"total_reports":1}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	reports, err := ListReports(client, ReportFilters{Status: "Completed", ReportType: "license"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 1 || reports[0].ID != 1 || reports[0].Name != "r1" {
		t.Fatalf("unexpected reports: %+v", reports)
	}
}

func TestListReportsOmitsEmptyFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if _, ok := body["filters"]; ok {
			t.Fatalf("expected no filters key when no filters passed, got %v", body["filters"])
		}
		_, _ = w.Write([]byte(`{"reports":[],"total_reports":0}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	if _, err := ListReports(client, ReportFilters{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportsBeforeEndTime(t *testing.T) {
	cutoff := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	reports := []Report{
		{ID: 1, EndTime: "2024-01-01T00:00:00Z"},  // before cutoff
		{ID: 2, EndTime: "2024-12-01T00:00:00Z"},  // after cutoff
		{ID: 3, EndTime: "not-a-valid-timestamp"}, // unparsable
	}
	matches, skipped := ReportsBeforeEndTime(reports, cutoff)
	if len(matches) != 1 || matches[0].ID != 1 {
		t.Fatalf("unexpected matches: %+v", matches)
	}
	if len(skipped) != 1 || skipped[0].ID != 3 {
		t.Fatalf("unexpected skipped: %+v", skipped)
	}
}

func TestDeleteReportsDryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("dry-run must not issue DELETE calls")
		}
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := DeleteReports(client, []Report{{ID: 1, Name: "r1"}}, true)
	if len(results) != 1 || !results[0].DryRun || results[0].Success {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
}

func TestDeleteReportsAppliesDeletes(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	results := DeleteReports(client, []Report{{ID: 42, Name: "r1"}}, false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected success, got %+v", results)
	}
	if len(calls) != 1 || calls[0] != "/xray/api/v1/reports/42" {
		t.Fatalf("unexpected calls: %v", calls)
	}
}
