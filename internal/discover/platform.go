package discover

import (
	"encoding/json"
	"fmt"
	"net/http"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetPlatformConfig fetches instance-wide platform/security configuration
// that isn't repo- or user-shaped: global roles/environments, scoped
// tokens, certificates, property sets, proxies, backups, and a grab-bag of
// singleton config endpoints (mail server, trash, password/lock policy,
// repo layouts, reverse proxy, archive cleanup policies, license, workers,
// webhooks). Every field is fetched independently and soft-fails into
// warnings - one broken/unlicensed endpoint never blanks out the rest.
func GetPlatformConfig(c *jfclient.Client) (instance.PlatformConfig, []string) {
	var cfg instance.PlatformConfig
	var warnings []string
	warn := func(label string, err error) {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", label, err))
		}
	}

	warn("global roles", fetchJSON(c, "/access/api/v1/roles", &cfg.GlobalRoles))
	warn("global environments", fetchJSON(c, "/access/api/v1/environments", &cfg.GlobalEnvironments))
	warn("scoped tokens", fetchWrapped(c, "/access/api/v1/tokens", "tokens", &cfg.ScopedTokens))
	warn("certificates", fetchJSON(c, "/artifactory/api/system/security/certificates", &cfg.Certificates))
	warn("property sets", fetchJSON(c, "/artifactory/api/propertysets", &cfg.PropertySets))
	warn("proxies", fetchJSON(c, "/artifactory/api/proxies", &cfg.Proxies))
	warn("backups", fetchWrapped(c, "/artifactory/api/backup", "backups", &cfg.Backups))
	warn("mail server", fetchJSON(c, "/artifactory/api/system/mailserver", &cfg.MailServer))
	warn("trash config", fetchJSON(c, "/artifactory/api/trash/config", &cfg.Trash))
	warn("password expiration policy", fetchJSON(c, "/artifactory/api/security/configuration/passwordExpirationPolicy", &cfg.PasswordPolicy))
	warn("user lock policy", fetchJSON(c, "/artifactory/api/security/userLockPolicy", &cfg.UserLockPolicy))
	warn("repo layouts", fetchJSON(c, "/artifactory/api/repolayouts", &cfg.RepoLayouts))
	warn("reverse proxy config", fetchJSON(c, "/artifactory/api/system/reverse-proxy", &cfg.ReverseProxy))
	warn("archive cleanup policies", fetchWrapped(c, "/artifactory/api/cleanup/archive/policies", "policies", &cfg.ArchiveCleanupPolicies))
	warn("license", fetchJSON(c, "/access/api/v1/license", &cfg.License))
	warn("workers", fetchWrapped(c, "/access/api/v1/workers", "workers", &cfg.Workers))
	warn("webhooks", getWebhooks(c, &cfg.Webhooks))

	return cfg, warnings
}

// isSoftSkip reports whether status means "this feature isn't
// available/licensed/permitted on this instance" rather than a real
// failure worth surfacing loudly - mirrors the tolerance already used for
// optional component versions (Xray/Access/Distribution).
func isSoftSkip(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	}
	return status >= 500
}

// fetchJSON GETs path and decodes the response body directly into out
// (a pointer to a struct or slice whose json tags already match the
// endpoint's wire format). Soft-skippable statuses leave *out untouched.
func fetchJSON(c *jfclient.Client, path string, out interface{}) error {
	resp, err := c.Get(path)
	if err != nil {
		return err
	}
	if isSoftSkip(resp.StatusCode) {
		return nil
	}
	if err := resp.Error("fetch " + path); err != nil {
		return err
	}
	return resp.Unmarshal(out)
}

// fetchWrapped GETs path and decodes the array under wrapperKey (e.g.
// {"tokens": [...]}) into out.
func fetchWrapped(c *jfclient.Client, path, wrapperKey string, out interface{}) error {
	resp, err := c.Get(path)
	if err != nil {
		return err
	}
	if isSoftSkip(resp.StatusCode) {
		return nil
	}
	if err := resp.Error("fetch " + path); err != nil {
		return err
	}
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(resp.Body, &wrapper); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	raw, ok := wrapper[wrapperKey]
	if !ok {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// getWebhooks fetches GET /event/api/v1/subscriptions and flattens each
// webhook's nested event_filter/handlers down to the report-friendly
// instance.Webhook shape. The handler's signing secret is never captured,
// even though the API returns one.
func getWebhooks(c *jfclient.Client, out *[]instance.Webhook) error {
	resp, err := c.Get("/event/api/v1/subscriptions")
	if err != nil {
		return err
	}
	if isSoftSkip(resp.StatusCode) {
		return nil
	}
	if err := resp.Error("fetch webhooks"); err != nil {
		return err
	}

	var raw []struct {
		Key         string `json:"key"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
		EventFilter struct {
			Domain     string   `json:"domain"`
			EventTypes []string `json:"event_types"`
		} `json:"event_filter"`
		Handlers []struct {
			URL string `json:"url"`
		} `json:"handlers"`
	}
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		return fmt.Errorf("decode webhooks response: %w", err)
	}

	for _, r := range raw {
		wh := instance.Webhook{
			Key:         r.Key,
			Description: r.Description,
			Enabled:     r.Enabled,
			Domain:      r.EventFilter.Domain,
			EventTypes:  r.EventFilter.EventTypes,
		}
		for _, h := range r.Handlers {
			wh.HandlerURLs = append(wh.HandlerURLs, h.URL)
		}
		*out = append(*out, wh)
	}
	return nil
}
