package discover

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

const listPageLimit = 500

// GetUsers pages through GET /access/api/v2/users (cursor-paginated) and
// returns up to maxUsers users (0 or negative means unlimited). truncated
// is true only when the cap cut the listing short, never when the
// instance genuinely has fewer users than the cap.
func GetUsers(c *jfclient.Client, maxUsers int) (users []instance.User, truncated bool, err error) {
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
