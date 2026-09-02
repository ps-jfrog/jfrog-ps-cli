package analyze

import (
	"fmt"
	"strings"

	"jfrogpscli/internal/instance"
)

// ResourceFinding is one flagged resource, of any type, for the combined
// "All Findings" view - the cross-resource counterpart to the Repos tab's
// per-repo findings/badges.
type ResourceFinding struct {
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
	Finding      string `json:"finding"`
	Detail       string `json:"detail"`
}

// repoResourceFindings flattens each repo row's findings into
// ResourceFindings, looking up each finding's explanation text from defs
// so the combined list carries the same detail the Repos tab's finding
// cards show.
func repoResourceFindings(rows []Row, defs []FindingDef) []ResourceFinding {
	explain := make(map[string]string, len(defs))
	for _, d := range defs {
		explain[d.ID] = d.Explain
	}
	var out []ResourceFinding
	for _, r := range rows {
		name := r.Get("repo_name")
		for _, f := range r.Findings {
			out = append(out, ResourceFinding{ResourceType: "repo", ResourceName: name, Finding: f, Detail: explain[f]})
		}
	}
	return out
}

// computeResourceFindings scans every non-repo resource in an instance
// snapshot for hygiene/governance signals worth flagging - the same
// spirit as the Repos tab's findings, extended across access, projects,
// federation/replication, platform config, and Xray. Returns nil for a
// nil snapshot.
func computeResourceFindings(snap *instance.Snapshot) []ResourceFinding {
	if snap == nil {
		return nil
	}
	var out []ResourceFinding

	for _, p := range snap.Permissions {
		if len(p.UserActions) == 0 && len(p.GroupActions) == 0 {
			out = append(out, ResourceFinding{"permission_target", p.Name, "no-principals",
				"No users or groups are granted any action on this permission target - likely orphaned config."})
		}
		for _, r := range p.Repos {
			if r == "**" || r == "*" {
				out = append(out, ResourceFinding{"permission_target", p.Name, "wildcard-scope",
					fmt.Sprintf("Repo scope includes a wildcard pattern (%q) - grants broad, hard-to-audit access.", r)})
				break
			}
		}
		if actions := anonymousGrant(p); len(actions) > 0 {
			scope := strings.Join(p.Repos, ", ")
			if scope == "" {
				scope = "(no repos scoped)"
			}
			out = append(out, ResourceFinding{"permission_target", p.Name, "anonymous-access",
				fmt.Sprintf("Grants the anonymous user %s on %s - confirm unauthenticated access here is intentional.", strings.Join(actions, "/"), scope)})
		}
	}

	for _, proj := range snap.Projects {
		if proj.RepoCount == 0 && len(proj.Roles) == 0 && len(proj.Users) == 0 && len(proj.Groups) == 0 {
			out = append(out, ResourceFinding{"project", proj.Key, "empty-project",
				"No repos, custom roles, or members detected for this project - possibly unused."})
		}
	}

	for _, f := range snap.FederatedRepos {
		enabled := 0
		for _, m := range f.Members {
			if m.Enabled {
				enabled++
			}
		}
		if enabled < 2 {
			out = append(out, ResourceFinding{"federated_repo", f.RepoKey, "single-member",
				fmt.Sprintf("Only %d enabled member(s) - federation's replication benefit needs 2+.", enabled)})
		}
	}

	for _, r := range snap.Replications {
		if !r.Enabled {
			out = append(out, ResourceFinding{"replication", r.RepoKey, "disabled",
				"Replication is configured but disabled - confirm this is intentional, not stale config."})
		}
	}

	for _, t := range snap.Platform.ScopedTokens {
		if t.Expiry == 0 {
			name := t.Subject
			if name == "" {
				name = t.TokenID
			}
			out = append(out, ResourceFinding{"scoped_token", name, "non-expiring",
				"Token has no expiry - long-lived credentials are a common finding in access reviews."})
		}
	}

	for _, w := range snap.Xray.Watches {
		if !w.Active {
			out = append(out, ResourceFinding{"xray_watch", w.Name, "inactive",
				"Watch is configured but not active - it won't fire on any assigned policy."})
		}
	}

	for _, p := range snap.Xray.Policies {
		if len(p.Rules) == 0 {
			out = append(out, ResourceFinding{"xray_policy", p.Name, "no-rules",
				"Policy has no rules configured - it can't take any action even if a watch assigns it."})
		}
	}

	return out
}
