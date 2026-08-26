package discover

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetGroups pages through GET /access/api/v2/groups (cursor-paginated) and
// returns up to maxGroups groups (0 or negative means unlimited). truncated
// is true only when the cap cut the listing short.
func GetGroups(c *jfclient.Client, maxGroups int) (groups []instance.Group, truncated bool, err error) {
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
