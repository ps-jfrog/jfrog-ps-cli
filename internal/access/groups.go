package access

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/jfclient"
)

// GroupRef is one entry from the groups list endpoint.
type GroupRef struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// GroupDetail is the detail of a group, as returned by
// GET /artifactory/api/security/groups/{name}?includeUsers=true.
type GroupDetail struct {
	Name      string   `json:"name"`
	UserNames []string `json:"userNames"`
}

// ListGroups returns all groups via GET /artifactory/api/security/groups.
func ListGroups(c *jfclient.Client) ([]GroupRef, error) {
	resp, err := c.Get("/artifactory/api/security/groups")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list groups"); err != nil {
		return nil, err
	}
	var refs []GroupRef
	if err := resp.Unmarshal(&refs); err != nil {
		return nil, fmt.Errorf("parse groups list: %w", err)
	}
	return refs, nil
}

// GetGroupDetail fetches one group's detail, including its member usernames,
// via GET /artifactory/api/security/groups/{name}?includeUsers=true.
func GetGroupDetail(c *jfclient.Client, name string) (*GroupDetail, error) {
	resp, err := c.Get("/artifactory/api/security/groups/" + url.PathEscape(name) + "?includeUsers=true")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("get group " + name); err != nil {
		return nil, err
	}
	var d GroupDetail
	if err := resp.Unmarshal(&d); err != nil {
		return nil, fmt.Errorf("parse group %s: %w", name, err)
	}
	return &d, nil
}

// FindEmptyGroups lists every group and reports the names of any with no
// members.
func FindEmptyGroups(c *jfclient.Client) ([]string, error) {
	groups, err := ListGroups(c)
	if err != nil {
		return nil, err
	}
	var empty []string
	for _, g := range groups {
		d, err := GetGroupDetail(c, g.Name)
		if err != nil {
			return nil, err
		}
		if len(d.UserNames) == 0 {
			empty = append(empty, d.Name)
		}
	}
	return empty, nil
}
