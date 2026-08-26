package discover

import (
	"fmt"
	"net/url"
	"sync"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// listPermissionNames pages through GET /access/api/v2/permissions
// (cursor-paginated) and returns every permission target name.
func listPermissionNames(c *jfclient.Client) ([]string, error) {
	var names []string
	cursor := ""
	for {
		path := fmt.Sprintf("/access/api/v2/permissions?limit=%d", listPageLimit)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, err := c.Get(path)
		if err != nil {
			return names, fmt.Errorf("list permission targets: %w", err)
		}
		if err := resp.Error("list permission targets"); err != nil {
			return names, err
		}
		var page struct {
			Permissions []struct {
				Name string `json:"name"`
			} `json:"permissions"`
			Cursor string `json:"cursor"`
		}
		if err := resp.Unmarshal(&page); err != nil {
			return names, fmt.Errorf("decode permission targets response: %w", err)
		}
		if len(page.Permissions) == 0 {
			break
		}
		for _, p := range page.Permissions {
			names = append(names, p.Name)
		}
		if page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
	}
	return names, nil
}

// getPermissionDetail fetches one permission target's repo scope and
// user/group grants from its "artifact" resource (build/release-bundle
// scopes aren't captured).
func getPermissionDetail(c *jfclient.Client, name string) (instance.PermissionTarget, error) {
	target := instance.PermissionTarget{Name: name}

	resp, err := c.Get("/access/api/v2/permissions/" + url.PathEscape(name))
	if err != nil {
		return target, fmt.Errorf("get permission target %s: %w", name, err)
	}
	if err := resp.Error("get permission target " + name); err != nil {
		return target, err
	}

	var detail struct {
		Resources struct {
			Artifact struct {
				Actions struct {
					Users  map[string][]string `json:"users"`
					Groups map[string][]string `json:"groups"`
				} `json:"actions"`
				Targets map[string]interface{} `json:"targets"`
			} `json:"artifact"`
		} `json:"resources"`
	}
	if err := resp.Unmarshal(&detail); err != nil {
		return target, fmt.Errorf("decode permission target %s: %w", name, err)
	}

	for repo := range detail.Resources.Artifact.Targets {
		target.Repos = append(target.Repos, repo)
	}
	target.UserActions = detail.Resources.Artifact.Actions.Users
	target.GroupActions = detail.Resources.Artifact.Actions.Groups
	return target, nil
}

// GetPermissionTargets lists every permission target and fetches up to
// maxTargets of their details concurrently (bounded by concurrency).
// truncated is true only when the cap cut the listing short. A single
// target's detail failing does not abort the rest - it's reported back in
// warnings and omitted from the result. progress, if non-nil, is called
// after each target finishes (done is 1-based, total is len(names) after
// truncation).
func GetPermissionTargets(c *jfclient.Client, maxTargets, concurrency int, progress func(done, total int)) (targets []instance.PermissionTarget, truncated bool, warnings []string, err error) {
	names, err := listPermissionNames(c)
	if err != nil {
		return nil, false, nil, err
	}

	if maxTargets > 0 && len(names) > maxTargets {
		names = names[:maxTargets]
		truncated = true
	}

	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]*instance.PermissionTarget, len(names))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			detail, err := getPermissionDetail(c, name)
			mu.Lock()
			if err != nil {
				warnings = append(warnings, err.Error())
			} else {
				results[i] = &detail
			}
			done++
			if progress != nil {
				progress(done, len(names))
			}
			mu.Unlock()
		}(i, name)
	}
	wg.Wait()

	for _, r := range results {
		if r != nil {
			targets = append(targets, *r)
		}
	}
	return targets, truncated, warnings, nil
}
