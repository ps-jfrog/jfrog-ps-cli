package discover

import (
	"fmt"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// CollectOptions bounds the size and concurrency of an instance-wide
// collection so it stays reasonable on large instances.
type CollectOptions struct {
	Concurrency    int // for permission-target/federated-repo/project-detail fetches
	MaxUsers       int // 0 = unlimited
	MaxGroups      int // 0 = unlimited
	MaxPermissions int // 0 = unlimited
	// ProjectDetail additionally fetches each project's roles/environments
	// (and, opportunistically, member users/groups) - 2-4 extra calls per
	// project, bounded by Concurrency. Defaults to true; set false to skip
	// on an instance with a very large number of projects.
	ProjectDetail bool
	// Progress, if non-nil, is called as each concurrent-bounded stage
	// (permission targets, project detail, federated repos) makes
	// progress: stage names it, done/total drive a progress bar. Stages
	// with nothing to do (e.g. no federated repos) are never called.
	Progress func(stage string, done, total int)
}

// DefaultCollectOptions returns sane defaults: modest concurrency, and caps
// high enough to cover most PS-scale instances without silently truncating.
func DefaultCollectOptions() CollectOptions {
	return CollectOptions{Concurrency: 8, MaxUsers: 5000, MaxGroups: 5000, MaxPermissions: 2000, ProjectDetail: true}
}

// CollectInstance gathers a full instance.Snapshot: component versions,
// storage summary, authentication/signing-key configuration, platform
// configuration (roles, tokens, certificates, proxies, backups, and other
// singleton config), users, groups, permission-target access, projects
// (optionally with role/membership detail), replication/federation
// config, and Xray policy/watch/ignore-rule/curation configuration. repos
// (typically from ListRepoStorage) drives project repo counts and which
// repos to fetch federation config for.
//
// Only the Artifactory version fetch is fatal - every other piece
// soft-fails into Snapshot.Warnings so one broken/unlicensed endpoint
// doesn't blank out the rest of the report.
func CollectInstance(c *jfclient.Client, repos []RepoSummary, opts CollectOptions) (*instance.Snapshot, error) {
	snap := &instance.Snapshot{URL: c.BaseURL, Name: instance.NameFromURL(c.BaseURL)}

	sysInfo, sysWarnings, err := GetSystemInfo(c)
	if err != nil {
		return nil, fmt.Errorf("system info: %w", err)
	}
	snap.System = sysInfo
	snap.Warnings = append(snap.Warnings, sysWarnings...)

	secInfo, secWarnings := GetSecurityInfo(c)
	snap.Security = secInfo
	snap.Warnings = append(snap.Warnings, secWarnings...)

	if users, truncated, err := GetUsers(c, opts.MaxUsers, sysInfo.ArtifactoryVersion); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("users: %v", err))
	} else {
		snap.Users, snap.UsersTruncated = users, truncated
	}

	if groups, truncated, err := GetGroups(c, opts.MaxGroups, sysInfo.ArtifactoryVersion); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("groups: %v", err))
	} else {
		snap.Groups, snap.GroupsTruncated = groups, truncated
	}

	stageProgress := func(stage string) func(done, total int) {
		if opts.Progress == nil {
			return nil
		}
		return func(done, total int) { opts.Progress(stage, done, total) }
	}

	if targets, truncated, warnings, err := GetPermissionTargets(c, opts.MaxPermissions, opts.Concurrency, sysInfo.ArtifactoryVersion, stageProgress("Permissions")); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("permission targets: %v", err))
	} else {
		snap.Permissions, snap.PermissionsTruncated = targets, truncated
		snap.Warnings = append(snap.Warnings, warnings...)
	}

	if projects, err := GetProjects(c, repos); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("projects: %v", err))
	} else {
		if opts.ProjectDetail {
			EnrichProjectDetail(c, projects, opts.Concurrency, stageProgress("Project detail"))
		}
		snap.Projects = projects
	}

	if replications, err := GetReplications(c); err != nil {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("replications: %v", err))
	} else {
		snap.Replications = replications
	}

	federated, fedWarnings := GetFederatedRepos(c, repos, opts.Concurrency, stageProgress("Federated repos"))
	snap.FederatedRepos = federated
	snap.Warnings = append(snap.Warnings, fedWarnings...)

	platformCfg, platformWarnings := GetPlatformConfig(c)
	snap.Platform = platformCfg
	snap.Warnings = append(snap.Warnings, platformWarnings...)

	xrayCfg, xrayWarnings := GetXrayConfig(c)
	snap.Xray = xrayCfg
	snap.Warnings = append(snap.Warnings, xrayWarnings...)

	return snap, nil
}
