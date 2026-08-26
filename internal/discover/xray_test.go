package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetXrayConfig_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/xray/api/v1/policies":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "p1", "type": "security", "author": "a", "rules": []map[string]interface{}{{"name": "r1", "criteria": map[string]interface{}{"min_severity": "High"}}}},
			})
		case "/xray/api/v2/watches":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"general_data":      map[string]interface{}{"name": "w1", "description": "d", "active": true, "project_key": "pk"},
					"project_resources": map[string]interface{}{"resources": []map[string]interface{}{{"type": "all-repos"}}},
					"assigned_policies": []map[string]interface{}{{"name": "p1", "type": "security"}},
					"watch_recipients":  []string{"a@example.com"},
				},
			})
		case "/xray/api/v1/ignore_rules":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{{"id": "ir1", "notes": "n", "author": "a"}},
			})
		case "/xray/api/v1/curation/policies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{{"id": "c1", "name": "cp1", "enabled": true, "scope": "specific_repos"}},
			})
		case "/xray/api/v1/configuration/systemParameters":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"maxDiskDataUsage": 80, "monitorSamplingInterval": 300, "jobInterval": 86400, "allowSendingAnalytics": true,
			})
		case "/xray/api/v1/configuration/dbsync/time":
			_ = json.NewEncoder(w).Encode(map[string]string{"db_sync_updates_time": "12:01"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg, warnings := GetXrayConfig(testClient(t, srv))
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if len(cfg.Policies) != 1 || cfg.Policies[0].Name != "p1" || len(cfg.Policies[0].Rules) != 1 {
		t.Errorf("unexpected Policies: %+v", cfg.Policies)
	}
	if len(cfg.Watches) != 1 {
		t.Fatalf("expected 1 watch, got %+v", cfg.Watches)
	}
	w := cfg.Watches[0]
	if w.Name != "w1" || w.ProjectKey != "pk" || !w.Active || len(w.AssignedPolicies) != 1 || w.AssignedPolicies[0] != "p1" || len(w.WatchRecipients) != 1 {
		t.Errorf("unexpected flattened watch: %+v", w)
	}
	if len(cfg.IgnoreRules) != 1 || cfg.IgnoreRules[0].ID != "ir1" {
		t.Errorf("unexpected IgnoreRules: %+v", cfg.IgnoreRules)
	}
	if len(cfg.CurationPolicies) != 1 || cfg.CurationPolicies[0].Name != "cp1" {
		t.Errorf("unexpected CurationPolicies: %+v", cfg.CurationPolicies)
	}
	if cfg.Settings == nil {
		t.Fatal("expected non-nil Settings")
	}
	if cfg.Settings.MaxDiskDataUsage != 80 || cfg.Settings.DBSyncUpdatesTime != "12:01" {
		t.Errorf("unexpected merged Settings: %+v", cfg.Settings)
	}
}

func TestGetXrayConfig_UnlicensedInstanceStaysZeroValued(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg, warnings := GetXrayConfig(testClient(t, srv))
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings on a clean 404, got %v", warnings)
	}
	if cfg.Settings != nil || len(cfg.Policies) != 0 || len(cfg.Watches) != 0 {
		t.Fatalf("expected zero-valued XrayConfig, got %+v", cfg)
	}
}
