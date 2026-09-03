package access

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// ErrPermissionNotFound is returned by GetPermissionV2 when the Access API
// v2 responds with HTTP 404 for a permission name.
var ErrPermissionNotFound = errors.New("permission not found")

// ErrUserNotFound is returned by GetUserV2 when the Access API v2 responds
// with HTTP 404 for a username.
var ErrUserNotFound = errors.New("user not found")

// ErrGroupNotFound is returned by GetGroupV2 when the Access API v2
// responds with HTTP 404 for a group name.
var ErrGroupNotFound = errors.New("group not found")

// V2Target is one repository's entry under resources.<resource>.targets in
// the Access API v2 permission shape.
type V2Target struct {
	IncludePatterns []string `json:"include_patterns,omitempty"`
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
}

// V2Actions holds the group/user -> action-list maps under
// resources.<resource>.actions.
type V2Actions struct {
	Users  map[string][]string `json:"users,omitempty"`
	Groups map[string][]string `json:"groups,omitempty"`
}

// V2Resource is one resource entry (e.g. "artifact") under a v2 permission's
// "resources" map.
type V2Resource struct {
	Targets map[string]V2Target `json:"targets"`
	Actions *V2Actions          `json:"actions,omitempty"`
}

// V2Permission is the Access API v2 permission shape used by
// create-permission, grant-path, add-repos, remove-repos, backup, and audit.
type V2Permission struct {
	Name      string                `json:"name"`
	Resources map[string]V2Resource `json:"resources"`
}

// PermissionRefV2 is one entry from the Access API v2 permissions list.
type PermissionRefV2 struct {
	Name string `json:"name"`
}

// ListPermissionsV2 lists all permission targets via
// GET /access/api/v2/permissions.
//
// ASSUMPTION (not verified against a live instance): the list endpoint
// returns {"permissions":[{"name":"..."}, ...]}.
func ListPermissionsV2(c *jfclient.Client) ([]PermissionRefV2, error) {
	resp, err := c.Get("/access/api/v2/permissions")
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list permissions (v2)"); err != nil {
		return nil, err
	}
	var wrapper struct {
		Permissions []PermissionRefV2 `json:"permissions"`
	}
	if err := resp.Unmarshal(&wrapper); err != nil {
		return nil, fmt.Errorf("parse v2 permissions list: %w", err)
	}
	return wrapper.Permissions, nil
}

// GetPermissionV2 fetches one permission target via
// GET /access/api/v2/permissions/{name}. It returns ErrPermissionNotFound
// (wrapped) if the API responds with HTTP 404.
func GetPermissionV2(c *jfclient.Client, name string) (*V2Permission, error) {
	resp, err := c.Get("/access/api/v2/permissions/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("%w: %s", ErrPermissionNotFound, name)
	}
	if err := resp.Error("get permission " + name); err != nil {
		return nil, err
	}
	var p V2Permission
	if err := resp.Unmarshal(&p); err != nil {
		return nil, fmt.Errorf("parse v2 permission %s: %w", name, err)
	}
	return &p, nil
}

// CreatePermission creates a brand-new permission target via
// POST /access/api/v2/permissions, granting access to the given repos under
// the "artifact" resource with no principals attached yet. It fails (surfaces
// the API error) if a permission with that name already exists -- no
// dry-run support is needed here since it is a pure create.
func CreatePermission(c *jfclient.Client, name string, repos []string) error {
	targets := make(map[string]V2Target, len(repos))
	for _, r := range repos {
		targets[r] = V2Target{}
	}
	payload := V2Permission{
		Name: name,
		Resources: map[string]V2Resource{
			"artifact": {Targets: targets},
		},
	}
	resp, err := c.Post("/access/api/v2/permissions", payload)
	if err != nil {
		return err
	}
	return resp.Error("create permission " + name)
}

// GrantPathOptions configures GrantPath.
type GrantPathOptions struct {
	Permission string
	Repos      []string
	Paths      []string // include_patterns, already split on ';'
	Groups     map[string][]string
	Users      map[string][]string
	Resource   string // defaults to "artifact"
}

// GrantPathResult describes what GrantPath did (or, in dry-run mode, would
// do).
type GrantPathResult struct {
	Permission string
	Resource   string
	WillCreate bool
	Targets    []string
	DryRun     bool
}

// GrantPath idempotently creates-or-merges path/repo grants on a permission
// target using the Access API v2:
//
//   - If the permission does not exist (404), a full v2 payload is built and
//     POSTed to /access/api/v2/permissions.
//   - If it exists, new repos are unioned into the existing
//     resources.<resource>.targets (existing keys are left untouched) and new
//     group/user actions are merged into resources.<resource>.actions, then
//     PATCHed to /access/api/v2/permissions/{name}/{resource}.
//
// When dryRun is true, only the read (GetPermissionV2) is performed -- no
// POST/PATCH is ever issued.
func GrantPath(c *jfclient.Client, opts GrantPathOptions, dryRun bool) (*GrantPathResult, error) {
	resource := opts.Resource
	if resource == "" {
		resource = "artifact"
	}

	existing, err := GetPermissionV2(c, opts.Permission)
	notFound := errors.Is(err, ErrPermissionNotFound)
	if err != nil && !notFound {
		return nil, err
	}

	if notFound {
		targets := make(map[string]V2Target, len(opts.Repos))
		for _, r := range opts.Repos {
			targets[r] = V2Target{IncludePatterns: append([]string(nil), opts.Paths...)}
		}
		res := V2Resource{Targets: targets}
		if len(opts.Groups) > 0 || len(opts.Users) > 0 {
			res.Actions = &V2Actions{Groups: cloneActionMap(opts.Groups), Users: cloneActionMap(opts.Users)}
		}
		result := &GrantPathResult{
			Permission: opts.Permission,
			Resource:   resource,
			WillCreate: true,
			Targets:    targetKeys(targets),
			DryRun:     dryRun,
		}
		if dryRun {
			return result, nil
		}
		payload := V2Permission{Name: opts.Permission, Resources: map[string]V2Resource{resource: res}}
		resp, err := c.Post("/access/api/v2/permissions", payload)
		if err != nil {
			return nil, err
		}
		if err := resp.Error("create permission " + opts.Permission); err != nil {
			return nil, err
		}
		return result, nil
	}

	res, ok := existing.Resources[resource]
	if !ok {
		res = V2Resource{}
	}
	if res.Targets == nil {
		res.Targets = map[string]V2Target{}
	}
	for _, r := range opts.Repos {
		if _, exists := res.Targets[r]; !exists {
			res.Targets[r] = V2Target{IncludePatterns: append([]string(nil), opts.Paths...)}
		}
	}
	if len(opts.Groups) > 0 || len(opts.Users) > 0 {
		if res.Actions == nil {
			res.Actions = &V2Actions{}
		}
		if res.Actions.Groups == nil {
			res.Actions.Groups = map[string][]string{}
		}
		for g, acts := range opts.Groups {
			res.Actions.Groups[g] = mergeActions(res.Actions.Groups[g], acts)
		}
		if res.Actions.Users == nil {
			res.Actions.Users = map[string][]string{}
		}
		for u, acts := range opts.Users {
			res.Actions.Users[u] = mergeActions(res.Actions.Users[u], acts)
		}
	}

	result := &GrantPathResult{
		Permission: opts.Permission,
		Resource:   resource,
		WillCreate: false,
		Targets:    targetKeys(res.Targets),
		DryRun:     dryRun,
	}
	if dryRun {
		return result, nil
	}
	resp, err := c.Patch(fmt.Sprintf("/access/api/v2/permissions/%s/%s", url.PathEscape(opts.Permission), url.PathEscape(resource)), res)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("update permission " + opts.Permission); err != nil {
		return nil, err
	}
	return result, nil
}

// ReposChangeResult describes the repo target keys before/after an
// AddRepos/RemoveRepos call.
type ReposChangeResult struct {
	Permission string
	Before     []string
	After      []string
	DryRun     bool
}

// AddRepos adds the given repo keys to a permission target's
// resources.artifact.targets (exact-key match; a no-op for repos already
// present), then PUTs the full updated permission object to
// /access/api/v2/permissions/{name}. When dryRun is true, no PUT is issued.
func AddRepos(c *jfclient.Client, name string, repos []string, dryRun bool) (*ReposChangeResult, error) {
	p, err := GetPermissionV2(c, name)
	if err != nil {
		return nil, err
	}
	res := p.Resources["artifact"]
	before := targetKeys(res.Targets)
	if res.Targets == nil {
		res.Targets = map[string]V2Target{}
	}
	for _, r := range repos {
		if _, exists := res.Targets[r]; !exists {
			res.Targets[r] = V2Target{}
		}
	}
	after := targetKeys(res.Targets)
	result := &ReposChangeResult{Permission: name, Before: before, After: after, DryRun: dryRun}
	if dryRun {
		return result, nil
	}
	if p.Resources == nil {
		p.Resources = map[string]V2Resource{}
	}
	p.Resources["artifact"] = res
	resp, err := c.Put("/access/api/v2/permissions/"+url.PathEscape(name), p)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("update permission " + name); err != nil {
		return nil, err
	}
	return result, nil
}

// RemoveRepos removes the given repo keys from a permission target's
// resources.artifact.targets using an exact-key match (not a substring
// match -- this fixes a real bug found in the original bash script), then
// PUTs the full updated permission object. When dryRun is true, no PUT is
// issued.
func RemoveRepos(c *jfclient.Client, name string, repos []string, dryRun bool) (*ReposChangeResult, error) {
	p, err := GetPermissionV2(c, name)
	if err != nil {
		return nil, err
	}
	res := p.Resources["artifact"]
	before := targetKeys(res.Targets)
	remove := make(map[string]bool, len(repos))
	for _, r := range repos {
		remove[r] = true
	}
	for k := range res.Targets {
		if remove[k] {
			delete(res.Targets, k)
		}
	}
	after := targetKeys(res.Targets)
	result := &ReposChangeResult{Permission: name, Before: before, After: after, DryRun: dryRun}
	if dryRun {
		return result, nil
	}
	if p.Resources == nil {
		p.Resources = map[string]V2Resource{}
	}
	p.Resources["artifact"] = res
	resp, err := c.Put("/access/api/v2/permissions/"+url.PathEscape(name), p)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("update permission " + name); err != nil {
		return nil, err
	}
	return result, nil
}

// AuditMatch is one permission target found to cover an audited repo
// pattern, either via an exact repo-key regex match or a wildcard target.
type AuditMatch struct {
	Permission string
	Matched    string // matching repo key, or wildcard target name
}

// AuditPermissions lists all v2 permission targets and reports every one
// whose resources.artifact.targets include a repo key matching repoPattern
// (a regular expression), OR a wildcard target key ("ANY", "ANY LOCAL", "ANY
// REMOTE") that implicitly covers repos of repoType (pass "" to match any
// wildcard regardless of type).
func AuditPermissions(c *jfclient.Client, repoPattern, repoType string, onProgress ...progress.Reporter) ([]AuditMatch, error) {
	re, err := regexp.Compile(repoPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid repo pattern %q: %w", repoPattern, err)
	}
	refs, err := ListPermissionsV2(c)
	if err != nil {
		return nil, err
	}
	return concurrency.MapSliceOrErr(refs, 0, func(ref PermissionRefV2) ([]AuditMatch, error) {
		p, err := GetPermissionV2(c, ref.Name)
		if err != nil {
			return nil, err
		}
		res, ok := p.Resources["artifact"]
		if !ok {
			return nil, nil
		}
		var matches []AuditMatch
		for _, target := range targetKeys(res.Targets) {
			if isWildcardTarget(target, repoType) {
				matches = append(matches, AuditMatch{Permission: p.Name, Matched: target})
				continue
			}
			if re.MatchString(target) {
				matches = append(matches, AuditMatch{Permission: p.Name, Matched: target})
			}
		}
		return matches, nil
	}, onProgress...)
}

func isWildcardTarget(target, repoType string) bool {
	switch target {
	case "ANY":
		return true
	case "ANY LOCAL":
		return repoType == "" || repoType == "local"
	case "ANY REMOTE":
		return repoType == "" || repoType == "remote"
	}
	return false
}

func targetKeys(targets map[string]V2Target) []string {
	keys := make([]string, 0, len(targets))
	for k := range targets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mergeActions(existing, add []string) []string {
	set := map[string]bool{}
	for _, a := range existing {
		set[a] = true
	}
	for _, a := range add {
		set[a] = true
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

func cloneActionMap(m map[string][]string) map[string][]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = append([]string(nil), v...)
	}
	return out
}
