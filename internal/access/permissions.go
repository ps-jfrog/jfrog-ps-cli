// Package access implements the business logic behind "jfps access": users,
// groups, and permission target management against the (deprecated but
// still-working) Artifactory Security API and the newer Access API v2.
//
// Functions in this package are pure: they take an already-resolved
// *jfclient.Client (or two, for source/target operations) plus plain Go
// parameters, and return (result, error). They never parse flags or read
// environment variables -- that belongs entirely to internal/cli/access.
package access

import (
	"fmt"
	"net/url"
	"sort"

	"jfrogpscli/internal/jfclient"
)

// PermissionRef is one entry from the permissions list endpoint.
type PermissionRef struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Permission is the full detail of a permission target, as returned by the
// deprecated Artifactory Security API
// (GET /artifactory/api/security/permissions/{name}).
type Permission struct {
	Name         string   `json:"name"`
	Repositories []string `json:"repositories"`
	Principals   struct {
		Users  map[string][]string `json:"users"`
		Groups map[string][]string `json:"groups"`
	} `json:"principals"`
}

// ListPermissions returns all permission targets known to Artifactory via
// GET /artifactory/api/security/permissions.
func ListPermissions(c *jfclient.Client) ([]PermissionRef, error) {
	resp, err := c.Get("/artifactory/api/security/permissions")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list permissions"); err != nil {
		return nil, err
	}
	var refs []PermissionRef
	if err := resp.Unmarshal(&refs); err != nil {
		return nil, fmt.Errorf("parse permissions list: %w", err)
	}
	return refs, nil
}

// ShowPermission fetches the full detail of one permission target via
// GET /artifactory/api/security/permissions/{name}.
func ShowPermission(c *jfclient.Client, name string) (*Permission, error) {
	resp, err := c.Get("/artifactory/api/security/permissions/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	if err := resp.Error("show permission " + name); err != nil {
		return nil, err
	}
	var p Permission
	if err := resp.Unmarshal(&p); err != nil {
		return nil, fmt.Errorf("parse permission %s: %w", name, err)
	}
	return &p, nil
}

// FindEmptyPermissions lists every permission target and reports the names
// of any (other than the built-in "INTERNAL" one) with no repositories
// assigned.
func FindEmptyPermissions(c *jfclient.Client) ([]string, error) {
	refs, err := ListPermissions(c)
	if err != nil {
		return nil, err
	}
	var empty []string
	for _, ref := range refs {
		if ref.Name == "INTERNAL" {
			continue
		}
		p, err := ShowPermission(c, ref.Name)
		if err != nil {
			return nil, err
		}
		if len(p.Repositories) == 0 {
			empty = append(empty, p.Name)
		}
	}
	return empty, nil
}

// ListUsersInPermission returns the usernames assigned (in any role) to a
// permission target, sorted alphabetically.
func ListUsersInPermission(c *jfclient.Client, name string) ([]string, error) {
	p, err := ShowPermission(c, name)
	if err != nil {
		return nil, err
	}
	users := make([]string, 0, len(p.Principals.Users))
	for u := range p.Principals.Users {
		users = append(users, u)
	}
	sort.Strings(users)
	return users, nil
}
