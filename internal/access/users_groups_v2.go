package access

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/jfclient"
)

// V2UserRef is one entry from the Access API v2 users list endpoint.
type V2UserRef struct {
	Username string `json:"username"`
}

// V2User is one user's full detail, as returned by
// GET /access/api/v2/users/{username} and accepted by
// POST /access/api/v2/users.
type V2User struct {
	Username                 string   `json:"username"`
	Email                    string   `json:"email,omitempty"`
	Admin                    bool     `json:"admin,omitempty"`
	ProfileUpdatable         bool     `json:"profile_updatable,omitempty"`
	DisableUIAccess          bool     `json:"disable_ui_access,omitempty"`
	InternalPasswordDisabled bool     `json:"internal_password_disabled,omitempty"`
	LastLoggedIn             string   `json:"last_logged_in,omitempty"`
	Realm                    string   `json:"realm,omitempty"`
	Groups                   []string `json:"groups,omitempty"`
}

// ListUsersV2 lists all users via GET /access/api/v2/users.
//
// ASSUMPTION (not verified against a live instance): the list endpoint
// returns {"users":[{"username":"..."}, ...]}.
func ListUsersV2(c *jfclient.Client) ([]V2UserRef, error) {
	resp, err := c.Get("/access/api/v2/users")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list users (v2)"); err != nil {
		return nil, err
	}
	var wrapper struct {
		Users []V2UserRef `json:"users"`
	}
	if err := resp.Unmarshal(&wrapper); err != nil {
		return nil, fmt.Errorf("parse v2 users list: %w", err)
	}
	return wrapper.Users, nil
}

// GetUserV2 fetches one user's full detail via
// GET /access/api/v2/users/{username}. It returns ErrUserNotFound (wrapped)
// if the API responds with HTTP 404.
func GetUserV2(c *jfclient.Client, username string) (*V2User, error) {
	resp, err := c.Get("/access/api/v2/users/" + url.PathEscape(username))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("%w: %s", ErrUserNotFound, username)
	}
	if err := resp.Error("get user " + username); err != nil {
		return nil, err
	}
	var u V2User
	if err := resp.Unmarshal(&u); err != nil {
		return nil, fmt.Errorf("parse v2 user %s: %w", username, err)
	}
	return &u, nil
}

// CreateUserV2 creates a new user via POST /access/api/v2/users.
func CreateUserV2(c *jfclient.Client, u *V2User) error {
	resp, err := c.Post("/access/api/v2/users", u)
	if err != nil {
		return err
	}
	return resp.Error("create user " + u.Username)
}

// V2GroupRef is one entry from the Access API v2 groups list endpoint.
//
// ASSUMPTION (not verified against a live instance): the list endpoint's
// entries key the group name as "group_name" rather than "name" (the
// individual group detail/create shape below uses "name").
type V2GroupRef struct {
	GroupName string `json:"group_name"`
}

// V2Group is one group's full detail, as returned by
// GET /access/api/v2/groups/{name} and accepted by
// POST /access/api/v2/groups.
type V2Group struct {
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	AutoJoin        bool     `json:"auto_join,omitempty"`
	AdminPrivileges bool     `json:"admin_privileges,omitempty"`
	Realm           string   `json:"realm,omitempty"`
	Members         []string `json:"members,omitempty"`
}

// ListGroupsV2 lists all groups via GET /access/api/v2/groups.
func ListGroupsV2(c *jfclient.Client) ([]V2GroupRef, error) {
	resp, err := c.Get("/access/api/v2/groups")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list groups (v2)"); err != nil {
		return nil, err
	}
	var wrapper struct {
		Groups []V2GroupRef `json:"groups"`
	}
	if err := resp.Unmarshal(&wrapper); err != nil {
		return nil, fmt.Errorf("parse v2 groups list: %w", err)
	}
	return wrapper.Groups, nil
}

// GetGroupV2 fetches one group's full detail via
// GET /access/api/v2/groups/{name}. It returns ErrGroupNotFound (wrapped)
// if the API responds with HTTP 404.
func GetGroupV2(c *jfclient.Client, name string) (*V2Group, error) {
	resp, err := c.Get("/access/api/v2/groups/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("%w: %s", ErrGroupNotFound, name)
	}
	if err := resp.Error("get group " + name); err != nil {
		return nil, err
	}
	var g V2Group
	if err := resp.Unmarshal(&g); err != nil {
		return nil, fmt.Errorf("parse v2 group %s: %w", name, err)
	}
	return &g, nil
}

// CreateGroupV2 creates a new group via POST /access/api/v2/groups.
func CreateGroupV2(c *jfclient.Client, g *V2Group) error {
	resp, err := c.Post("/access/api/v2/groups", g)
	if err != nil {
		return err
	}
	return resp.Error("create group " + g.Name)
}
