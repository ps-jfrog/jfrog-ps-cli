package discover

import (
	"net/url"
	"sync"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// EnrichProjectDetail fetches each project's roles and environments (and,
// opportunistically, member users/groups - these 404 on at least some
// JFrog SaaS tiers, so a miss there is expected and silent, not a
// warning) concurrently (bounded by concurrency), mutating projects in
// place. Unlike GetPlatformConfig/GetXrayConfig, per-project soft-skips
// are NOT collected as warnings: at hundreds of projects, a routinely-404
// endpoint would otherwise flood the report with hundreds of identical,
// uninformative lines. progress, if non-nil, is called after each
// project finishes (done is 1-based, total is len(projects)).
func EnrichProjectDetail(c *jfclient.Client, projects []instance.Project, concurrency int, progress func(done, total int)) {
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i := range projects {
		key := projects[i].Key
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if key != "" {
				base := "/access/api/v1/projects/" + url.PathEscape(key)
				_ = fetchJSON(c, base+"/roles", &projects[i].Roles)
				_ = fetchNameList(c, base+"/environments", &projects[i].Environments)
				_ = fetchWrapped(c, base+"/members/users", "members", &projects[i].Users)
				_ = fetchWrapped(c, base+"/members/groups", "members", &projects[i].Groups)
			}

			if progress != nil {
				mu.Lock()
				done++
				progress(done, len(projects))
				mu.Unlock()
			}
		}(i, key)
	}
	wg.Wait()
}

// fetchNameList fetches a bare array of {"name": "..."} objects and
// flattens it to a slice of names.
func fetchNameList(c *jfclient.Client, path string, out *[]string) error {
	var raw []struct {
		Name string `json:"name"`
	}
	if err := fetchJSON(c, path, &raw); err != nil {
		return err
	}
	for _, r := range raw {
		*out = append(*out, r.Name)
	}
	return nil
}
