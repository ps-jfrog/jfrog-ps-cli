package xray

import (
	"fmt"
	"time"

	"jfrogpscli/internal/jfclient"
)

// Report is one entry returned by POST /xray/api/v1/reports. Only the fields
// needed by this CLI are modeled.
type Report struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	ReportType string `json:"report_type"`
	EndTime    string `json:"end_time"`
}

// reportsListResponse models the response body of POST /xray/api/v1/reports.
type reportsListResponse struct {
	Reports      []Report `json:"reports"`
	TotalReports int      `json:"total_reports"`
}

// ReportFilters narrows a report listing. Empty fields are omitted from the
// request filter.
type ReportFilters struct {
	Status     string
	ReportType string
}

// ListReports lists Xray reports matching filters via
// POST /xray/api/v1/reports. Read-only: issues no mutating calls.
//
// NOTE: true pagination is not implemented -- page_number/num_of_rows are
// fixed at 1/100, so listings beyond the first 100 matching reports are not
// returned. A future revision could loop pages until total_reports is
// exhausted.
func ListReports(client *jfclient.Client, filters ReportFilters) ([]Report, error) {
	body := map[string]interface{}{
		"page_number": 1,
		"num_of_rows": 100,
	}
	f := map[string]string{}
	if filters.Status != "" {
		f["status"] = filters.Status
	}
	if filters.ReportType != "" {
		f["type"] = filters.ReportType
	}
	if len(f) > 0 {
		body["filters"] = f
	}

	resp, err := client.Post("/xray/api/v1/reports", body)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list reports"); err != nil {
		return nil, err
	}
	var out reportsListResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("decode reports response: %w", err)
	}
	return out.Reports, nil
}

// ReportsBeforeEndTime splits reports into those whose end_time parses as
// RFC3339 and is strictly before cutoff ("matches"), and those whose
// end_time fails to parse ("skipped", noted rather than causing a crash).
// Reports whose end_time parses but is not before cutoff are silently
// dropped (kept in neither slice).
func ReportsBeforeEndTime(reports []Report, cutoff time.Time) (matches []Report, skipped []Report) {
	for _, r := range reports {
		t, err := time.Parse(time.RFC3339, r.EndTime)
		if err != nil {
			skipped = append(skipped, r)
			continue
		}
		if t.Before(cutoff) {
			matches = append(matches, r)
		}
	}
	return matches, skipped
}

// ReportDeleteResult is the outcome of attempting to delete one report.
type ReportDeleteResult struct {
	ID         int
	Name       string
	EndTime    string
	DryRun     bool
	StatusCode int
	Success    bool
	Error      string
}

// DeleteReports deletes each report via DELETE /xray/api/v1/reports/{id}.
// When dryRun is true, no HTTP DELETE calls are issued at all.
func DeleteReports(client *jfclient.Client, reports []Report, dryRun bool) []ReportDeleteResult {
	results := make([]ReportDeleteResult, 0, len(reports))
	for _, r := range reports {
		res := ReportDeleteResult{ID: r.ID, Name: r.Name, EndTime: r.EndTime, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}
		resp, err := client.Delete(fmt.Sprintf("/xray/api/v1/reports/%d", r.ID))
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.StatusCode = resp.StatusCode
		if resp.OK() {
			res.Success = true
		} else {
			res.Error = resp.Error("delete report").Error()
		}
		results = append(results, res)
	}
	return results
}
