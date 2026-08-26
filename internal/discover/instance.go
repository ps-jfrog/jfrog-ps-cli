package discover

import (
	"fmt"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// CollectOptions bounds the size and concurrency of an instance-wide
// collection so it stays reasonable on large instances.
type CollectOptions struct {
	Concurrency    int // for permission-target detail and federated-repo config fetches
	MaxUsers       int // 0 = unlimited
	MaxGroups      int // 0 = unlimited
	MaxPermissions int // 0 = unlimited
}

// DefaultCollectOptions returns sane defaults: modest concurrency, and caps
// high enough to cover most PS-scale instances without silently truncating.
func DefaultCollectOptions() CollectOptions {
	return CollectOptions{Concurrency: 8, MaxUsers: 5000, MaxGroups: 5000, MaxPermissions: 2000}
}

// CollectInstance gathers a full instance.Snapshot: component versions,
// storage summary, authentication/signing-key configuration, users,
// groups, permission-target access, projects, and replication/federation
// config. repos (typically from ListRepoStorage) drives project repo
// counts and which repos to fetch federation config for.
//
// Only the Artifactory version fetch is fatal - every other piece
// soft-fails into Snapshot.Warnings so one broken/unlicensed endpoint
// doesn't blank out the rest of the report.
func CollectInstance(c *jfclient.Client, repos []RepoSummary, opts CollectOptions) (*instance.Snapshot, error) {
	snap := &instance.Snapshot{}

	sysInfo, sysWarnings, err := GetSystemInfo(c)
	if err != nil {
		return nil, fmt.Errorf("system info: %w", err)
	}
	snap.System = sysInfo
	snap.Warnings = append(snap.Warnings, sysWarnings...)

	secInfo, secWarnings := GetSecurityInfo(c)
	snap.Security = secInfo
	snap.Warnings = append(snap.Warnings, secWarnings...)

	if users, truncated, err := GetUsers(c, opts.MaxUsers); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("users: %v", err))
	} else {
		snap.Users, snap.UsersTruncated = users, truncated
	}

	if groups, truncated, err := GetGroups(c, opts.MaxGroups); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("groups: %v", err))
	} else {
		snap.Groups, snap.GroupsTruncated = groups, truncated
	}

	if targets, truncated, warnings, err := GetPermissionTargets(c, opts.MaxPermissions, opts.Concurrency); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("permission targets: %v", err))
	} else {
		snap.Permissions, snap.PermissionsTruncated = targets, truncated
		snap.Warnings = append(snap.Warnings, warnings...)
	}

	if projects, err := GetProjects(c, repos); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("projects: %v", err))
	} else {
		snap.Projects = projects
	}

	if replications, err := GetReplications(c); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("replications: %v", err))
	} else {
		snap.Replications = replications
	}

	federated, fedWarnings := GetFederatedRepos(c, repos, opts.Concurrency)
	snap.FederatedRepos = federated
	snap.Warnings = append(snap.Warnings, fedWarnings...)

	return snap, nil
}
