package discover

import (
	"encoding/json"
	"fmt"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetXrayConfig fetches Xray policy/watch/ignore-rule/curation/settings
// configuration. Every field is fetched independently and soft-fails into
// warnings; if Xray isn't licensed/installed at all, every call 404s and
// the whole struct comes back zero-valued (see Snapshot.System.XrayAvailable,
// which is the authoritative "is Xray even here" signal). Curation policies
// and waivers are only captured when curation itself is enabled/available -
// see CurationAvailable.
func GetXrayConfig(c *jfclient.Client) (instance.XrayConfig, []string) {
	var cfg instance.XrayConfig
	var warnings []string
	warn := func(label string, err error) {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", label, err))
		}
	}

	warn("xray policies", fetchJSON(c, "/xray/api/v1/policies", &cfg.Policies))
	warn("xray watches", getXrayWatches(c, &cfg.Watches))
	warn("xray ignore rules", fetchWrapped(c, "/xray/api/v1/ignore_rules", "data", &cfg.IgnoreRules))

	// Waivers only exist as an idea if curation itself is enabled - a 404 on
	// the policies list means curation isn't licensed/enabled here at all
	// (indistinguishable, without this flag, from "enabled with zero
	// policies configured"), so skip the waivers call entirely rather than
	// firing a second doomed request.
	available, err := getCurationPolicies(c, &cfg.CurationPolicies)
	warn("xray curation policies", err)
	cfg.CurationAvailable = available
	if available {
		warn("xray curation waivers", fetchWrapped(c, "/xray/api/v1/curation/waiver_requests", "data", &cfg.CurationWaivers))
	}

	warn("xray settings", getXraySettings(c, &cfg.Settings))

	return cfg, warnings
}

// getCurationPolicies fetches GET /xray/api/v1/curation/policies and
// reports whether curation is available/enabled on this instance at all -
// distinct from the returned error, since a soft-skipped (404/403/5xx)
// response is not an error but does mean "not available."
func getCurationPolicies(c *jfclient.Client, out *[]instance.XrayCurationPolicy) (bool, error) {
	resp, err := c.Get("/xray/api/v1/curation/policies")
	if err != nil {
		return false, err
	}
	if isSoftSkip(resp.StatusCode) {
		return false, nil
	}
	if err := resp.Error("fetch xray curation policies"); err != nil {
		return false, err
	}
	var wrapper struct {
		Data []instance.XrayCurationPolicy `json:"data"`
	}
	if err := resp.Unmarshal(&wrapper); err != nil {
		return false, fmt.Errorf("decode xray curation policies response: %w", err)
	}
	*out = wrapper.Data
	return true, nil
}

// getXrayWatches fetches GET /xray/api/v2/watches and flattens each
// watch's nested general_data/project_resources down to instance.XrayWatch.
func getXrayWatches(c *jfclient.Client, out *[]instance.XrayWatch) error {
	resp, err := c.Get("/xray/api/v2/watches")
	if err != nil {
		return err
	}
	if isSoftSkip(resp.StatusCode) {
		return nil
	}
	if err := resp.Error("fetch xray watches"); err != nil {
		return err
	}

	var raw []struct {
		GeneralData struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Active      bool   `json:"active"`
			ProjectKey  string `json:"project_key"`
		} `json:"general_data"`
		ProjectResources struct {
			Resources []map[string]interface{} `json:"resources"`
		} `json:"project_resources"`
		AssignedPolicies []struct {
			Name string `json:"name"`
		} `json:"assigned_policies"`
		WatchRecipients []string `json:"watch_recipients"`
	}
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		return fmt.Errorf("decode xray watches response: %w", err)
	}

	for _, r := range raw {
		w := instance.XrayWatch{
			Name:            r.GeneralData.Name,
			Description:     r.GeneralData.Description,
			Active:          r.GeneralData.Active,
			ProjectKey:      r.GeneralData.ProjectKey,
			Resources:       r.ProjectResources.Resources,
			WatchRecipients: r.WatchRecipients,
		}
		for _, p := range r.AssignedPolicies {
			w.AssignedPolicies = append(w.AssignedPolicies, p.Name)
		}
		*out = append(*out, w)
	}
	return nil
}

// getXraySettings merges a few fields of GET
// /xray/api/v1/configuration/systemParameters with GET
// /xray/api/v1/configuration/dbsync/time into one instance.XraySettings.
// out is set to nil if both calls are soft-skipped (Xray unavailable);
// a single failing call still yields the fields the other call recovered.
func getXraySettings(c *jfclient.Client, out **instance.XraySettings) error {
	settings := &instance.XraySettings{}
	found := false

	resp, err := c.Get("/xray/api/v1/configuration/systemParameters")
	if err != nil {
		return err
	}
	if !isSoftSkip(resp.StatusCode) {
		if err := resp.Error("fetch xray system parameters"); err != nil {
			return err
		}
		var raw struct {
			MaxDiskDataUsage        int  `json:"maxDiskDataUsage"`
			MonitorSamplingInterval int  `json:"monitorSamplingInterval"`
			JobInterval             int  `json:"jobInterval"`
			AllowSendingAnalytics   bool `json:"allowSendingAnalytics"`
		}
		if err := json.Unmarshal(resp.Body, &raw); err != nil {
			return fmt.Errorf("decode xray system parameters response: %w", err)
		}
		settings.MaxDiskDataUsage = raw.MaxDiskDataUsage
		settings.MonitorSamplingInterval = raw.MonitorSamplingInterval
		settings.JobInterval = raw.JobInterval
		settings.AllowSendingAnalytics = raw.AllowSendingAnalytics
		found = true
	}

	dbResp, err := c.Get("/xray/api/v1/configuration/dbsync/time")
	if err != nil {
		return err
	}
	if !isSoftSkip(dbResp.StatusCode) {
		if err := dbResp.Error("fetch xray dbsync time"); err != nil {
			return err
		}
		var raw struct {
			DBSyncUpdatesTime string `json:"db_sync_updates_time"`
		}
		if err := json.Unmarshal(dbResp.Body, &raw); err != nil {
			return fmt.Errorf("decode xray dbsync time response: %w", err)
		}
		settings.DBSyncUpdatesTime = raw.DBSyncUpdatesTime
		found = true
	}

	if found {
		*out = settings
	}
	return nil
}
