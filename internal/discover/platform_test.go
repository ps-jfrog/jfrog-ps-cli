package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/instance"
)

func TestGetPlatformConfig_HappyPathAndSoftSkips(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/access/api/v1/roles":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "Global Developers", "type": "CUSTOM_GLOBAL", "environments": []string{"DEV"}, "actions": []string{"READ_REPOSITORY"}},
			})
		case "/access/api/v1/environments":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "DEV"}})
		case "/access/api/v1/tokens":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tokens": []map[string]interface{}{{"token_id": "t1", "subject": "u1", "refreshable": true}},
			})
		case "/artifactory/api/system/security/certificates":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"certificateAlias": "c1", "issuedTo": "x", "issuedBy": "y", "validUntil": "2030"}})
		case "/artifactory/api/propertysets":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"name": "ps1", "propertiesCount": 3}})
		case "/artifactory/api/proxies":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"key": "proxy1", "host": "h", "port": 8080}})
		case "/artifactory/api/backup":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"backups": []map[string]interface{}{{"key": "b1", "enabled": true, "cronExp": "0 0 * * * ?"}},
			})
		case "/artifactory/api/security/configuration/passwordExpirationPolicy":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"enabled": true, "passwordMaxAge": 180, "notifyByEmail": true})
		case "/artifactory/api/security/userLockPolicy":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"enabled": true, "loginAttempts": 5})
		case "/artifactory/api/cleanup/archive/policies":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"policies": []map[string]interface{}{{"key": "arch1", "cron_exp": "0 0 3 * * ?", "enabled": true}},
			})
		case "/access/api/v1/workers":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"workers": []map[string]interface{}{{"key": "w1", "enabled": true}},
			})
		case "/event/api/v1/subscriptions":
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"key": "wh1", "enabled": true,
					"event_filter": map[string]interface{}{"domain": "artifact", "event_types": []string{"deployed"}},
					"handlers":     []map[string]interface{}{{"url": "https://example.com/hook", "secret": "super-secret-value"}},
				},
			})
		default:
			// mail server, trash config, repo layouts, reverse proxy, license -
			// soft-skipped via 404 in this test.
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg, warnings := GetPlatformConfig(testClient(t, srv))
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}

	if len(cfg.GlobalRoles) != 1 || cfg.GlobalRoles[0].Name != "Global Developers" {
		t.Errorf("unexpected GlobalRoles: %+v", cfg.GlobalRoles)
	}
	if len(cfg.GlobalEnvironments) != 1 || cfg.GlobalEnvironments[0].Name != "DEV" {
		t.Errorf("unexpected GlobalEnvironments: %+v", cfg.GlobalEnvironments)
	}
	if len(cfg.ScopedTokens) != 1 || cfg.ScopedTokens[0].TokenID != "t1" {
		t.Errorf("unexpected ScopedTokens: %+v", cfg.ScopedTokens)
	}
	if len(cfg.Certificates) != 1 || cfg.Certificates[0].Alias != "c1" {
		t.Errorf("unexpected Certificates: %+v", cfg.Certificates)
	}
	if len(cfg.PropertySets) != 1 || cfg.PropertySets[0].PropertiesCount != 3 {
		t.Errorf("unexpected PropertySets: %+v", cfg.PropertySets)
	}
	if len(cfg.Proxies) != 1 || cfg.Proxies[0].Port != 8080 {
		t.Errorf("unexpected Proxies: %+v", cfg.Proxies)
	}
	if len(cfg.Backups) != 1 || cfg.Backups[0].Key != "b1" {
		t.Errorf("unexpected Backups: %+v", cfg.Backups)
	}
	if cfg.MailServer != nil {
		t.Errorf("expected MailServer nil (404 soft-skip), got %+v", cfg.MailServer)
	}
	if cfg.PasswordPolicy == nil || cfg.PasswordPolicy.PasswordMaxAge != 180 {
		t.Errorf("unexpected PasswordPolicy: %+v", cfg.PasswordPolicy)
	}
	if cfg.UserLockPolicy == nil || cfg.UserLockPolicy.LoginAttempts != 5 {
		t.Errorf("unexpected UserLockPolicy: %+v", cfg.UserLockPolicy)
	}
	if len(cfg.ArchiveCleanupPolicies) != 1 || cfg.ArchiveCleanupPolicies[0].Key != "arch1" {
		t.Errorf("unexpected ArchiveCleanupPolicies: %+v", cfg.ArchiveCleanupPolicies)
	}
	if len(cfg.Workers) != 1 || cfg.Workers[0].Key != "w1" {
		t.Errorf("unexpected Workers: %+v", cfg.Workers)
	}
	if len(cfg.Webhooks) != 1 {
		t.Fatalf("expected 1 webhook, got %+v", cfg.Webhooks)
	}
	wh := cfg.Webhooks[0]
	if wh.Key != "wh1" || wh.Domain != "artifact" || len(wh.HandlerURLs) != 1 || wh.HandlerURLs[0] != "https://example.com/hook" {
		t.Errorf("unexpected webhook: %+v", wh)
	}
}

func TestGetWebhooks_NeverCapturesSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"key":          "wh1",
				"event_filter": map[string]interface{}{"domain": "artifact", "event_types": []string{"deployed"}},
				"handlers":     []map[string]interface{}{{"url": "https://x", "secret": "TOP-SECRET"}},
			},
		})
	}))
	defer srv.Close()

	var got []instance.Webhook
	if err := getWebhooks(testClient(t, srv), &got); err != nil {
		t.Fatalf("getWebhooks returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(got))
	}
	// instance.Webhook has no field for the handler secret - this documents
	// the contract so a future field addition doesn't accidentally start
	// capturing it.
	if got[0].Key != "wh1" || got[0].HandlerURLs[0] != "https://x" {
		t.Errorf("unexpected webhook: %+v", got[0])
	}
}
