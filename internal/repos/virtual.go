package repos

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// VirtualRepoSpec is one row of a virtual-repo creation CSV: a virtual
// repository aggregating exactly one local and one remote repository.
type VirtualRepoSpec struct {
	VirtualRepo string
	LocalRepo   string
	RemoteRepo  string
}

// ReadVirtualRepoSpecsCSV reads virtual-repo specs from a CSV file with a
// header row. The header must contain "new-virtual-repo", "local-name", and
// "remote-name" columns (any order; extra columns are ignored).
func ReadVirtualRepoSpecsCSV(path string) ([]VirtualRepoSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header from %s: %w", path, err)
	}

	col := make(map[string]int, len(header))
	for i, name := range header {
		col[strings.TrimSpace(strings.ToLower(name))] = i
	}
	required := []string{"new-virtual-repo", "local-name", "remote-name"}
	var missing []string
	for _, name := range required {
		if _, ok := col[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: missing required column(s): %s", path, strings.Join(missing, ", "))
	}

	var specs []VirtualRepoSpec
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row from %s: %w", path, err)
		}
		specs = append(specs, VirtualRepoSpec{
			VirtualRepo: strings.TrimSpace(row[col["new-virtual-repo"]]),
			LocalRepo:   strings.TrimSpace(row[col["local-name"]]),
			RemoteRepo:  strings.TrimSpace(row[col["remote-name"]]),
		})
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("%s: no data rows found", path)
	}
	return specs, nil
}

// Plan actions for a virtual repo spec.
const (
	VirtualActionCreate   = "create"
	VirtualActionUpdate   = "update"
	VirtualActionNoChange = "no-change"
	VirtualActionError    = "error"
)

// VirtualRepoPlan is the result of validating one VirtualRepoSpec against a
// server: whether both member repos exist with matching package types, and
// whether creating the virtual repo would create it, update it, or find it
// already correctly configured.
type VirtualRepoPlan struct {
	Spec        VirtualRepoSpec
	PackageType string
	Action      string
	Error       string
}

// PlanVirtualRepos validates every spec against c: it fetches the full repo
// list once (so member-existence and package-type checks cost a single
// call, not one per spec), and for any virtual repo that already exists,
// fetches its current member list to detect whether an update is actually
// needed. All calls here are reads -- safe to run in dry-run mode.
func PlanVirtualRepos(c *jfclient.Client, specs []VirtualRepoSpec) ([]VirtualRepoPlan, error) {
	all, err := List(c, "")
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	index := make(map[string]Repo, len(all))
	for _, r := range all {
		index[r.Key] = r
	}

	plans := make([]VirtualRepoPlan, 0, len(specs))
	for _, spec := range specs {
		plan := VirtualRepoPlan{Spec: spec}

		local, localOK := index[spec.LocalRepo]
		remote, remoteOK := index[spec.RemoteRepo]
		var errs []string
		if !localOK {
			errs = append(errs, fmt.Sprintf("local repository %q does not exist", spec.LocalRepo))
		}
		if !remoteOK {
			errs = append(errs, fmt.Sprintf("remote repository %q does not exist", spec.RemoteRepo))
		}
		if localOK && remoteOK && local.PackageType != remote.PackageType {
			errs = append(errs, fmt.Sprintf("package type mismatch: local=%q, remote=%q", local.PackageType, remote.PackageType))
		}
		if len(errs) > 0 {
			plan.Action = VirtualActionError
			plan.Error = strings.Join(errs, "; ")
			plans = append(plans, plan)
			continue
		}
		plan.PackageType = local.PackageType

		if _, exists := index[spec.VirtualRepo]; !exists {
			plan.Action = VirtualActionCreate
			plans = append(plans, plan)
			continue
		}

		currentCfg, err := GetRepoConfig(c, spec.VirtualRepo)
		if err != nil {
			plan.Action = VirtualActionError
			plan.Error = fmt.Sprintf("get existing virtual repo config: %v", err)
			plans = append(plans, plan)
			continue
		}
		if sameMembers(currentCfg["repositories"], []string{spec.LocalRepo, spec.RemoteRepo}) {
			plan.Action = VirtualActionNoChange
		} else {
			plan.Action = VirtualActionUpdate
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// sameMembers reports whether current (decoded from JSON as
// []interface{} of strings) contains exactly the same set of repo keys as
// want, ignoring order.
func sameMembers(current interface{}, want []string) bool {
	list, ok := current.([]interface{})
	if !ok {
		return false
	}
	if len(list) != len(want) {
		return false
	}
	have := make(map[string]bool, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return false
		}
		have[s] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}

// buildVirtualRepoConfig builds the repository-configuration request body
// for creating/updating plan's virtual repo, matching the shape used by the
// original script: a two-member virtual repo with no default deployment
// repo and an auto-generated description.
func buildVirtualRepoConfig(plan VirtualRepoPlan) map[string]interface{} {
	return map[string]interface{}{
		"key":                   plan.Spec.VirtualRepo,
		"rclass":                "virtual",
		"packageType":           plan.PackageType,
		"repositories":          []string{plan.Spec.LocalRepo, plan.Spec.RemoteRepo},
		"description":           fmt.Sprintf("Virtual repository aggregating %s and %s", plan.Spec.LocalRepo, plan.Spec.RemoteRepo),
		"defaultDeploymentRepo": "",
	}
}

// VirtualRepoApplyResult is the outcome of applying one create/update plan.
type VirtualRepoApplyResult struct {
	Spec   VirtualRepoSpec
	Action string
	Err    error
}

// ApplyVirtualRepos issues the create/update calls for every plan whose
// Action is "create" or "update" ("no-change" and "error" plans are
// skipped -- there's nothing to apply). If dryRun is true, ApplyVirtualRepos
// performs no HTTP calls whatsoever and returns nil immediately.
func ApplyVirtualRepos(c *jfclient.Client, plans []VirtualRepoPlan, dryRun bool, onProgress ...progress.Reporter) []VirtualRepoApplyResult {
	if dryRun {
		return nil
	}
	return concurrency.Map(plans, 0, func(plan VirtualRepoPlan) []VirtualRepoApplyResult {
		switch plan.Action {
		case VirtualActionCreate:
			return []VirtualRepoApplyResult{{
				Spec:   plan.Spec,
				Action: plan.Action,
				Err:    createVirtualRepo(c, plan),
			}}
		case VirtualActionUpdate:
			return []VirtualRepoApplyResult{{
				Spec:   plan.Spec,
				Action: plan.Action,
				Err:    updateVirtualRepo(c, plan),
			}}
		default:
			return nil
		}
	}, onProgress...)
}

func createVirtualRepo(c *jfclient.Client, plan VirtualRepoPlan) error {
	resp, err := c.Put("/artifactory/api/repositories/"+plan.Spec.VirtualRepo, buildVirtualRepoConfig(plan))
	if err != nil {
		return fmt.Errorf("create virtual repository %s: %w", plan.Spec.VirtualRepo, err)
	}
	return resp.Error(fmt.Sprintf("create virtual repository %s", plan.Spec.VirtualRepo))
}

func updateVirtualRepo(c *jfclient.Client, plan VirtualRepoPlan) error {
	resp, err := c.Post("/artifactory/api/repositories/"+plan.Spec.VirtualRepo, buildVirtualRepoConfig(plan))
	if err != nil {
		return fmt.Errorf("update virtual repository %s: %w", plan.Spec.VirtualRepo, err)
	}
	return resp.Error(fmt.Sprintf("update virtual repository %s", plan.Spec.VirtualRepo))
}
