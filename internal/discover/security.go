package discover

import (
	"fmt"
	"net/http"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetSecurityInfo reports which authentication methods are configured
// (SAML/OIDC/LDAP) and which repository signing keys exist. A 404 on a
// probe means that method isn't configured, not an error; any other
// failure is returned in warnings. Key *material* (public or private) is
// never captured - only pair name/type/alias.
func GetSecurityInfo(c *jfclient.Client) (instance.SecurityInfo, []string) {
	var sec instance.SecurityInfo
	var warnings []string

	if configured, err := probeConfigured(c, "/access/api/v1/saml/config"); err != nil {
		warnings = append(warnings, fmt.Sprintf("saml config: %v", err))
	} else {
		sec.SAMLConfigured = configured
	}

	if configured, err := probeConfigured(c, "/access/api/v1/ldap"); err != nil {
		warnings = append(warnings, fmt.Sprintf("ldap config: %v", err))
	} else {
		sec.LDAPConfigured = configured
	}

	if configured, err := probeConfigured(c, "/access/api/v1/sso/http"); err != nil {
		warnings = append(warnings, fmt.Sprintf("http sso config: %v", err))
	} else {
		sec.HTTPSSOConfigured = configured
	}

	if configured, err := probeConfigured(c, "/access/api/v1/crowd"); err != nil {
		warnings = append(warnings, fmt.Sprintf("crowd config: %v", err))
	} else {
		sec.CrowdConfigured = configured
	}

	oidcResp, err := c.Get("/access/api/v1/oidc")
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("oidc providers: %v", err))
	} else if oidcResp.StatusCode != http.StatusNotFound {
		if err := oidcResp.Error("fetch oidc providers"); err != nil {
			warnings = append(warnings, err.Error())
		} else {
			var providers []struct {
				Name         string `json:"name"`
				IssuerURL    string `json:"issuer_url"`
				Audience     string `json:"audience"`
				ProviderType string `json:"provider_type"`
			}
			if err := oidcResp.Unmarshal(&providers); err != nil {
				warnings = append(warnings, fmt.Sprintf("decode oidc providers response: %v", err))
			} else {
				for _, p := range providers {
					sec.OIDCProviders = append(sec.OIDCProviders, instance.OIDCProvider{
						Name: p.Name, IssuerURL: p.IssuerURL, Audience: p.Audience, ProviderType: p.ProviderType,
					})
				}
			}
		}
	}

	keysResp, err := c.Get("/artifactory/api/security/keypair")
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("signing keys: %v", err))
	} else if keysResp.StatusCode != http.StatusNotFound {
		if err := keysResp.Error("fetch signing keys"); err != nil {
			warnings = append(warnings, err.Error())
		} else {
			var keys []struct {
				PairName string `json:"pairName"`
				PairType string `json:"pairType"`
				Alias    string `json:"alias"`
			}
			if err := keysResp.Unmarshal(&keys); err != nil {
				warnings = append(warnings, fmt.Sprintf("decode signing keys response: %v", err))
			} else {
				for _, k := range keys {
					sec.SigningKeys = append(sec.SigningKeys, instance.SigningKey{
						PairName: k.PairName, PairType: k.PairType, Alias: k.Alias,
					})
				}
			}
		}
	}

	return sec, warnings
}

// probeConfigured hits a config-presence endpoint and reports true if it
// returns 2xx, false if it returns 404 ("not configured"), and an error for
// anything else (auth failure, 5xx, etc.) so that isn't silently read as
// "not configured."
func probeConfigured(c *jfclient.Client, path string) (bool, error) {
	resp, err := c.Get(path)
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if err := resp.Error("probe " + path); err != nil {
		return false, err
	}
	return true, nil
}
