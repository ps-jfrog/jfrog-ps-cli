package access

import (
	"fmt"
	"net/url"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// PrincipalPermission is one user/group entry from
// GET /artifactory/api/artifactpermissions -- the actions it's been granted
// on the queried repo, plus which permission target(s) grant them. It is an
// in-memory display shape (flattened from rawPrincipalPermission below),
// not itself decoded from or encoded to JSON.
type PrincipalPermission struct {
	Principal         string
	Admin             bool
	Delete            bool
	Deploy            bool
	Annotate          bool
	Read              bool
	Distribute        bool
	ManagedXrayMeta   bool
	PermissionTargets []string
}

// effectivePermissionsResponse mirrors the raw shape of
// GET /artifactory/api/artifactpermissions?repoKey={repo}: each entry's
// action flags live nested under "permission", but callers of this package
// want them flattened onto PrincipalPermission -- see decodeEffective.
type effectivePermissionsResponse struct {
	UserEffectivePermissions  []rawPrincipalPermission `json:"userEffectivePermissions"`
	GroupEffectivePermissions []rawPrincipalPermission `json:"groupEffectivePermissions"`
}

type rawPrincipalPermission struct {
	Principal         string   `json:"principal"`
	Admin             bool     `json:"admin"`
	PermissionTargets []string `json:"permissionTargets"`
	Permission        struct {
		Delete          bool `json:"delete"`
		Deploy          bool `json:"deploy"`
		Annotate        bool `json:"annotate"`
		Read            bool `json:"read"`
		Distribute      bool `json:"distribute"`
		ManagedXrayMeta bool `json:"managedXrayMeta"`
	} `json:"permission"`
}

func (r rawPrincipalPermission) flatten() PrincipalPermission {
	return PrincipalPermission{
		Principal:         r.Principal,
		Admin:             r.Admin,
		Delete:            r.Permission.Delete,
		Deploy:            r.Permission.Deploy,
		Annotate:          r.Permission.Annotate,
		Read:              r.Permission.Read,
		Distribute:        r.Permission.Distribute,
		ManagedXrayMeta:   r.Permission.ManagedXrayMeta,
		PermissionTargets: r.PermissionTargets,
	}
}

// RepoEffectivePermissions is the effective-permissions result for one repo:
// every user and group with any access to it, and at what action level.
type RepoEffectivePermissions struct {
	Repo   string
	Users  []PrincipalPermission
	Groups []PrincipalPermission
}

// GetEffectivePermissions fetches the effective (i.e. already resolved
// across every permission target that applies) user/group permissions for
// one repo via the legacy
// GET /artifactory/api/artifactpermissions?repoKey={repo} endpoint -- this
// answers "who can touch repo X and how", which is the inverse question
// from `access audit` (which starts from a permission target and asks
// "which repos does it cover").
func GetEffectivePermissions(c *jfclient.Client, repo string) (*RepoEffectivePermissions, error) {
	resp, err := c.Get("/artifactory/api/artifactpermissions?repoKey=" + url.QueryEscape(repo))
	if err != nil {
		return nil, err
	}
	if err := resp.Error("get effective permissions for " + repo); err != nil {
		return nil, err
	}
	var raw effectivePermissionsResponse
	if err := resp.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("parse effective permissions for %s: %w", repo, err)
	}
	result := &RepoEffectivePermissions{Repo: repo}
	for _, u := range raw.UserEffectivePermissions {
		result.Users = append(result.Users, u.flatten())
	}
	for _, g := range raw.GroupEffectivePermissions {
		result.Groups = append(result.Groups, g.flatten())
	}
	return result, nil
}

// GetEffectivePermissionsForRepos calls GetEffectivePermissions for each
// repo in repos, in order, stopping at the first error.
func GetEffectivePermissionsForRepos(c *jfclient.Client, repos []string, onProgress ...progress.Reporter) ([]*RepoEffectivePermissions, error) {
	return concurrency.MapOrErr(repos, 0, func(repo string) (*RepoEffectivePermissions, error) {
		return GetEffectivePermissions(c, repo)
	}, onProgress...)
}
