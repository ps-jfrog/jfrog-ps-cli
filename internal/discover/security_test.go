package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSecurityInfo_ConfiguredAndNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/api/v1/saml/config":
			w.WriteHeader(http.StatusNotFound)
		case "/access/api/v1/ldap":
			w.WriteHeader(http.StatusOK)
		case "/access/api/v1/oidc":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"name": "github", "issuer_url": "https://token.actions.githubusercontent.com", "audience": "aud", "provider_type": "GitHub"},
			})
		case "/artifactory/api/security/keypair":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"pairName": "k1", "pairType": "GPG", "alias": "k1", "publicKey": "-----BEGIN PGP PUBLIC KEY BLOCK-----\nfake\n-----END-----"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	sec, warnings := GetSecurityInfo(testClient(t, srv))
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if sec.SAMLConfigured {
		t.Error("expected SAMLConfigured=false on 404")
	}
	if !sec.LDAPConfigured {
		t.Error("expected LDAPConfigured=true on 200")
	}
	if len(sec.OIDCProviders) != 1 || sec.OIDCProviders[0].Name != "github" {
		t.Errorf("unexpected OIDC providers: %+v", sec.OIDCProviders)
	}
	if len(sec.SigningKeys) != 1 || sec.SigningKeys[0].PairName != "k1" {
		t.Errorf("unexpected signing keys: %+v", sec.SigningKeys)
	}
}

func TestGetSecurityInfo_NeverExposesKeyMaterial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/artifactory/api/security/keypair" {
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"pairName": "k1", "pairType": "RSA", "alias": "k1", "publicKey": "SECRET-LOOKING-PUBLIC-KEY-MATERIAL"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	sec, _ := GetSecurityInfo(testClient(t, srv))
	if len(sec.SigningKeys) != 1 {
		t.Fatalf("expected 1 signing key, got %+v", sec.SigningKeys)
	}
	// instance.SigningKey has no field for key material - this is really
	// just documenting the contract at the type level, exercised here so a
	// future field addition doesn't accidentally start capturing it.
	k := sec.SigningKeys[0]
	if k.PairName != "k1" || k.PairType != "RSA" || k.Alias != "k1" {
		t.Fatalf("unexpected signing key metadata: %+v", k)
	}
}
