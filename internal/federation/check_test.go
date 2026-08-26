package federation

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCheckMembersHealthy(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []map[string]interface{}{
				{"url": "https://a.example.com", "enabled": true},
				{"url": "https://b.example.com", "enabled": true},
			},
		})
	})
	defer srv.Close()

	report, err := CheckMembers(c, "fed1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Healthy || report.MemberCount != 2 || len(report.Issues) != 0 {
		t.Fatalf("expected healthy report, got %+v", report)
	}
}

func TestCheckMembersDetectsDisabledMember(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []map[string]interface{}{
				{"url": "https://a.example.com", "enabled": true},
				{"url": "https://b.example.com", "enabled": false},
			},
		})
	})
	defer srv.Close()

	report, err := CheckMembers(c, "fed1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy {
		t.Fatalf("expected unhealthy report, got %+v", report)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %v", report.Issues)
	}
}

func TestCheckMembersDetectsCountMismatch(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []map[string]interface{}{
				{"url": "https://a.example.com", "enabled": true},
			},
		})
	})
	defer srv.Close()

	report, err := CheckMembers(c, "fed1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Healthy || report.MemberCount != 1 {
		t.Fatalf("expected unhealthy report with count 1, got %+v", report)
	}
}

func TestCheckAllMembersDefaultsToAllFederatedRepos(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/artifactory/api/repositories":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"key": "fed1"}, {"key": "fed2"}})
		case "/artifactory/api/repositories/fed1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"members": []map[string]interface{}{
				{"url": "a", "enabled": true}, {"url": "b", "enabled": true},
			}})
		case "/artifactory/api/repositories/fed2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"members": []map[string]interface{}{
				{"url": "a", "enabled": true},
			}})
		}
	})
	defer srv.Close()

	reports, err := CheckAllMembers(c, "", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	if !reports[0].Healthy || reports[1].Healthy {
		t.Fatalf("unexpected reports: %+v", reports)
	}
}
