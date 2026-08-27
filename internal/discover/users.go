package discover

import (
	"fmt"
	"net/http"
	"net/url"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

const listPageLimit = 500

// GetUsers pages through GET /access/api/v2/users (cursor-paginated) and
// returns up to maxUsers users (0 or negative means unlimited). truncated
// is true only when the cap cut the listing short, never when the
// instance genuinely has fewer users than the cap.
//
// artifactoryVersion (from GetSystemInfo) decides which API to use: below
// 7.49.3 the Access API v2 endpoint doesn't exist, so this goes straight
// to the deprecated Artifactory Security API (GET /artifactory/api/
// security/users - one call, no paging, no per-user status) instead of
// probing v2 first and eating a 404. An unexpected 404 on v2 despite a
// high-enough version (a custom build, a version string that didn't
// parse) still falls back the same way, as a safety net.
func GetUsers(c *jfclient.Client, maxUsers int, artifactoryVersion string) (users []instance.User, truncated bool, err error) {
	if !versionAtLeast(artifactoryVersion, usersGroupsV2MinVersion) {
		return getUsersLegacy(c, maxUsers)
	}
	cursor := ""
	for {
		path := fmt.Sprintf("/access/api/v2/users?limit=%d", listPageLimit)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, err := c.Get(path)
		if err != nil {
			return users, false, fmt.Errorf("list users: %w", err)
		}
		if cursor == "" && resp.StatusCode == http.StatusNotFound {
			return getUsersLegacy(c, maxUsers)
		}
		if err := resp.Error("list users"); err != nil {
			return users, false, err
		}
		var page struct {
			Users []struct {
				Username string `json:"username"`
				Realm    string `json:"realm"`
				Status   string `json:"status"`
			} `json:"users"`
			Cursor string `json:"cursor"`
		}
		if err := resp.Unmarshal(&page); err != nil {
			return users, false, fmt.Errorf("decode users response: %w", err)
		}
		if len(page.Users) == 0 {
			break
		}
		for _, u := range page.Users {
			users = append(users, instance.User{Username: u.Username, Realm: u.Realm, Status: u.Status})
			if maxUsers > 0 && len(users) >= maxUsers {
				return users, true, nil
			}
		}
		if page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
	}
	return users, false, nil
}

// getUsersLegacy fetches every user in a single call via the deprecated
// Artifactory Security API (GET /artifactory/api/security/users) - no
// paging and no per-user status, but available on Artifactory 6.23.21+,
// well before the Access API v2 endpoint existed.
func getUsersLegacy(c *jfclient.Client, maxUsers int) (users []instance.User, truncated bool, err error) {
	resp, err := c.Get("/artifactory/api/security/users")
	if err != nil {
		return nil, false, fmt.Errorf("list users (legacy): %w", err)
	}
	if err := resp.Error("list users (legacy)"); err != nil {
		return nil, false, err
	}
	var raw []struct {
		Name  string `json:"name"`
		Realm string `json:"realm"`
	}
	if err := resp.Unmarshal(&raw); err != nil {
		return nil, false, fmt.Errorf("decode users response (legacy): %w", err)
	}
	for _, u := range raw {
		users = append(users, instance.User{Username: u.Name, Realm: u.Realm})
		if maxUsers > 0 && len(users) >= maxUsers {
			return users, true, nil
		}
	}
	return users, false, nil
}
