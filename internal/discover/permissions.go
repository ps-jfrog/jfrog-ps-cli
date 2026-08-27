package discover

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// listPermissionNames pages through GET /access/api/v2/permissions
// (cursor-paginated) and returns every permission target name.
// artifactoryVersion (from GetSystemInfo) decides which API to use: below
// 7.72.0 the Access API v2 endpoint doesn't exist, so this goes straight
// to the deprecated Artifactory Security API (GET /artifactory/api/
// security/permissions) instead of probing v2 first. An unexpected 404 on
// v2 despite a high-enough version still falls back the same way, as a
// safety net. Either way, legacy=true is reported back so the caller
// fetches each target's detail from the matching API.
func listPermissionNames(c *jfclient.Client, artifactoryVersion string) (names []string, legacy bool, err error) {
	if !versionAtLeast(artifactoryVersion, permissionsV2MinVersion) {
		legacyNames, err := listPermissionNamesLegacy(c)
		return legacyNames, true, err
	}
	cursor := ""
	for {
		path := fmt.Sprintf("/access/api/v2/permissions?limit=%d", listPageLimit)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, err := c.Get(path)
		if err != nil {
			return names, false, fmt.Errorf("list permission targets: %w", err)
		}
		if cursor == "" && resp.StatusCode == http.StatusNotFound {
			legacyNames, err := listPermissionNamesLegacy(c)
			return legacyNames, true, err
		}
		if err := resp.Error("list permission targets"); err != nil {
			return names, false, err
		}
		var page struct {
			Permissions []struct {
				Name string `json:"name"`
			} `json:"permissions"`
			Cursor string `json:"cursor"`
		}
		if err := resp.Unmarshal(&page); err != nil {
			return names, false, fmt.Errorf("decode permission targets response: %w", err)
		}
		if len(page.Permissions) == 0 {
			break
		}
		for _, p := range page.Permissions {
			names = append(names, p.Name)
		}
		if page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
	}
	return names, false, nil
}

// listPermissionNamesLegacy fetches every permission target name in a
// single call via the deprecated Artifactory Security API
// (GET /artifactory/api/security/permissions).
func listPermissionNamesLegacy(c *jfclient.Client) ([]string, error) {
	resp, err := c.Get("/artifactory/api/security/permissions")
	if err != nil {
		return nil, fmt.Errorf("list permission targets (legacy): %w", err)
	}
	if err := resp.Error("list permission targets (legacy)"); err != nil {
		return nil, err
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := resp.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("decode permission targets response (legacy): %w", err)
	}
	names := make([]string, 0, len(raw))
	for _, p := range raw {
		names = append(names, p.Name)
	}
	return names, nil
}

// getPermissionDetail fetches one permission target's repo scope and
// user/group grants. When legacy is true (the Access API v2 listing 404'd
// for this instance), it reads from the deprecated Artifactory Security
// API instead; otherwise from the Access API v2's "artifact" resource
// (build/release-bundle scopes aren't captured either way).
func getPermissionDetail(c *jfclient.Client, name string, legacy bool) (instance.PermissionTarget, error) {
	if legacy {
		return getPermissionDetailLegacy(c, name)
	}

	target := instance.PermissionTarget{Name: name}

	resp, err := c.Get("/access/api/v2/permissions/" + url.PathEscape(name))
	if err != nil {
		return target, fmt.Errorf("get permission target %s: %w", name, err)
	}
	if err := resp.Error("get permission target " + name); err != nil {
		return target, err
	}

	var detail struct {
		Resources struct {
			Artifact struct {
				Actions struct {
					Users  map[string][]string `json:"users"`
					Groups map[string][]string `json:"groups"`
				} `json:"actions"`
				Targets map[string]interface{} `json:"targets"`
			} `json:"artifact"`
		} `json:"resources"`
	}
	if err := resp.Unmarshal(&detail); err != nil {
		return target, fmt.Errorf("decode permission target %s: %w", name, err)
	}

	for repo := range detail.Resources.Artifact.Targets {
		target.Repos = append(target.Repos, repo)
	}
	target.UserActions = detail.Resources.Artifact.Actions.Users
	target.GroupActions = detail.Resources.Artifact.Actions.Groups
	return target, nil
}

// getPermissionDetailLegacy fetches one permission target's repo scope
// and user/group grants via the deprecated Artifactory Security API
// (GET /artifactory/api/security/permissions/{name}), available on
// Artifactory 6.23.21+.
func getPermissionDetailLegacy(c *jfclient.Client, name string) (instance.PermissionTarget, error) {
	target := instance.PermissionTarget{Name: name}

	resp, err := c.Get("/artifactory/api/security/permissions/" + url.PathEscape(name))
	if err != nil {
		return target, fmt.Errorf("get permission target %s (legacy): %w", name, err)
	}
	if err := resp.Error("get permission target " + name + " (legacy)"); err != nil {
		return target, err
	}

	var detail struct {
		Repositories []string `json:"repositories"`
		Principals   struct {
			Users  map[string][]string `json:"users"`
			Groups map[string][]string `json:"groups"`
		} `json:"principals"`
	}
	if err := resp.Unmarshal(&detail); err != nil {
		return target, fmt.Errorf("decode permission target %s (legacy): %w", name, err)
	}

	target.Repos = detail.Repositories
	target.UserActions = detail.Principals.Users
	target.GroupActions = detail.Principals.Groups
	return target, nil
}

// GetPermissionTargets lists every permission target and fetches up to
// maxTargets of their details concurrently (bounded by concurrency).
// artifactoryVersion (from GetSystemInfo) picks the Access API v2 vs.
// legacy Artifactory Security API, per listPermissionNames. truncated is
// true only when the cap cut the listing short. A single target's detail
// failing does not abort the rest - it's reported back in
// warnings and omitted from the result. progress, if non-nil, is called
// after each target finishes (done is 1-based, total is len(names) after
// truncation).
func GetPermissionTargets(c *jfclient.Client, maxTargets, concurrency int, artifactoryVersion string, progress func(done, total int)) (targets []instance.PermissionTarget, truncated bool, warnings []string, err error) {
	names, legacy, err := listPermissionNames(c, artifactoryVersion)
	if err != nil {
		return nil, false, nil, err
	}

	if maxTargets > 0 && len(names) > maxTargets {
		names = names[:maxTargets]
		truncated = true
	}

	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]*instance.PermissionTarget, len(names))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			detail, err := getPermissionDetail(c, name, legacy)
			mu.Lock()
			if err != nil {
				warnings = append(warnings, err.Error())
			} else {
				results[i] = &detail
			}
			done++
			if progress != nil {
				progress(done, len(names))
			}
			mu.Unlock()
		}(i, name)
	}
	wg.Wait()

	for _, r := range results {
		if r != nil {
			targets = append(targets, *r)
		}
	}
	return targets, truncated, warnings, nil
}
