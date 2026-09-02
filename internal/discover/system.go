package discover

import (
	"fmt"
	"net/http"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

type artifactoryVersionResponse struct {
	Version  string   `json:"version"`
	Revision string   `json:"revision"`
	Addons   []string `json:"addons"`
}

// GetSystemInfo fetches the Artifactory version plus Xray/Access/
// Distribution versions and the storage summary. A failure fetching the
// Artifactory version itself is returned as an error (callers should treat
// it as a warning rather than aborting the whole collection - an empty
// ArtifactoryVersion safely falls back to legacy API behavior elsewhere).
// The other three components are optional: a 404 (not installed/licensed)
// is recorded as "unavailable" rather than an error; any other failure is
// returned in warnings so the rest of the collection still completes.
func GetSystemInfo(c *jfclient.Client) (instance.SystemInfo, []string, error) {
	var info instance.SystemInfo
	var warnings []string

	resp, err := c.Get("/artifactory/api/system/version")
	if err != nil {
		return info, warnings, fmt.Errorf("fetch artifactory version: %w", err)
	}
	if err := resp.Error("fetch artifactory version"); err != nil {
		return info, warnings, err
	}
	var av artifactoryVersionResponse
	if err := resp.Unmarshal(&av); err != nil {
		return info, warnings, fmt.Errorf("decode artifactory version response: %w", err)
	}
	info.ArtifactoryVersion = av.Version
	info.ArtifactoryRevision = av.Revision
	info.Addons = av.Addons

	if v, rev, avail, err := getComponentVersion(c, "/xray/api/v1/system/version", "xray_version", "xray_revision"); err != nil {
		warnings = append(warnings, fmt.Sprintf("xray version: %v", err))
	} else {
		info.XrayAvailable, info.XrayVersion = avail, v
		_ = rev
	}

	if v, _, avail, err := getComponentVersion(c, "/access/api/v1/system/version", "name", "revision"); err != nil {
		warnings = append(warnings, fmt.Sprintf("access version: %v", err))
	} else {
		info.AccessAvailable, info.AccessVersion = avail, v
	}

	if v, _, avail, err := getComponentVersion(c, "/distribution/api/v1/system/version", "version", "revision"); err != nil {
		warnings = append(warnings, fmt.Sprintf("distribution version: %v", err))
	} else {
		info.DistributionAvailable, info.DistributionVersion = avail, v
	}

	storageResp, err := c.Get("/artifactory/api/storageinfo")
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("storage summary: %v", err))
		return info, warnings, nil
	}
	if err := storageResp.Error("fetch storageinfo"); err != nil {
		warnings = append(warnings, fmt.Sprintf("storage summary: %s", err.Error()))
		return info, warnings, nil
	}
	var si struct {
		BinariesSummary struct {
			BinariesCount  string `json:"binariesCount"`
			BinariesSize   string `json:"binariesSize"`
			ArtifactsSize  string `json:"artifactsSize"`
			Optimization   string `json:"optimization"`
			ArtifactsCount string `json:"artifactsCount"`
		} `json:"binariesSummary"`
		FileStoreSummary struct {
			StorageType string `json:"storageType"`
		} `json:"fileStoreSummary"`
	}
	if err := storageResp.Unmarshal(&si); err == nil {
		info.BinariesCount = si.BinariesSummary.BinariesCount
		info.BinariesSize = si.BinariesSummary.BinariesSize
		info.ArtifactsSize = si.BinariesSummary.ArtifactsSize
		info.ArtifactsCount = si.BinariesSummary.ArtifactsCount
		info.Optimization = si.BinariesSummary.Optimization
		info.StorageType = si.FileStoreSummary.StorageType
	}

	return info, warnings, nil
}

// getComponentVersion fetches an optional component's version endpoint,
// pulling versionField/revisionField out of the response as raw JSON keys
// (component version responses aren't shaped consistently - Artifactory
// uses "version", Xray uses "xray_version", Access uses "name"). A 404 is
// treated as "component not available," not an error.
func getComponentVersion(c *jfclient.Client, path, versionField, revisionField string) (version, revision string, available bool, err error) {
	resp, err := c.Get(path)
	if err != nil {
		return "", "", false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", "", false, nil
	}
	if err := resp.Error("fetch " + path); err != nil {
		return "", "", false, err
	}
	var raw map[string]interface{}
	if err := resp.Unmarshal(&raw); err != nil {
		return "", "", false, fmt.Errorf("decode %s response: %w", path, err)
	}
	if v, ok := raw[versionField].(string); ok {
		version = v
	}
	if v, ok := raw[revisionField].(string); ok {
		revision = v
	}
	return version, revision, true, nil
}
