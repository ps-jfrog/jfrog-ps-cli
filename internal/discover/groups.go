package discover

import (
	"fmt"
	"net/http"
	"net/url"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetGroups pages through GET /access/api/v2/groups (cursor-paginated) and
// returns up to maxGroups groups (0 or negative means unlimited). truncated
// is true only when the cap cut the listing short.
//
// artifactoryVersion (from GetSystemInfo) decides which API to use: below
// 7.49.3 the Access API v2 endpoint doesn't exist, so this goes straight
// to the deprecated Artifactory Security API (GET /artifactory/api/
// security/groups) instead of probing v2 first. An unexpected 404 on v2
// despite a high-enough version still falls back the same way, as a
// safety net.
func GetGroups(c *jfclient.Client, maxGroups int, artifactoryVersion string) (groups []instance.Group, truncated bool, err error) {
	if !versionAtLeast(artifactoryVersion, usersGroupsV2MinVersion) {
		return getGroupsLegacy(c, maxGroups)
	}
	cursor := ""
	for {
		path := fmt.Sprintf("/access/api/v2/groups?limit=%d", listPageLimit)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, err := c.Get(path)
		if err != nil {
			return groups, false, fmt.Errorf("list groups: %w", err)
		}
		if cursor == "" && resp.StatusCode == http.StatusNotFound {
			return getGroupsLegacy(c, maxGroups)
		}
		if err := resp.Error("list groups"); err != nil {
			return groups, false, err
		}
		var page struct {
			Groups []struct {
				GroupName string `json:"group_name"`
			} `json:"groups"`
			Cursor string `json:"cursor"`
		}
		if err := resp.Unmarshal(&page); err != nil {
			return groups, false, fmt.Errorf("decode groups response: %w", err)
		}
		if len(page.Groups) == 0 {
			break
		}
		for _, g := range page.Groups {
			groups = append(groups, instance.Group{Name: g.GroupName})
			if maxGroups > 0 && len(groups) >= maxGroups {
				return groups, true, nil
			}
		}
		if page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
	}
	return groups, false, nil
}

// getGroupsLegacy fetches every group in a single call via the deprecated
// Artifactory Security API (GET /artifactory/api/security/groups) -
// available on Artifactory 6.23.21+, well before the Access API v2
// endpoint existed.
func getGroupsLegacy(c *jfclient.Client, maxGroups int) (groups []instance.Group, truncated bool, err error) {
	resp, err := c.Get("/artifactory/api/security/groups")
	if err != nil {
		return nil, false, fmt.Errorf("list groups (legacy): %w", err)
	}
	if err := resp.Error("list groups (legacy)"); err != nil {
		return nil, false, err
	}
	var raw []struct {
		Name string `json:"name"`
	}
	if err := resp.Unmarshal(&raw); err != nil {
		return nil, false, fmt.Errorf("decode groups response (legacy): %w", err)
	}
	for _, g := range raw {
		groups = append(groups, instance.Group{Name: g.Name})
		if maxGroups > 0 && len(groups) >= maxGroups {
			return groups, true, nil
		}
	}
	return groups, false, nil
}
