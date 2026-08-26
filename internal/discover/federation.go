package discover

import (
	"fmt"
	"net/url"
	"sync"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetFederatedRepoConfig fetches one repo's member list from its full
// configuration (GET /artifactory/api/repositories/{key}) - there is no
// bulk federation-config endpoint the way there is for replication.
func GetFederatedRepoConfig(c *jfclient.Client, repoKey string) (instance.FederatedRepo, error) {
	fed := instance.FederatedRepo{RepoKey: repoKey}

	resp, err := c.Get("/artifactory/api/repositories/" + url.PathEscape(repoKey))
	if err != nil {
		return fed, fmt.Errorf("get repo config for %s: %w", repoKey, err)
	}
	if err := resp.Error("get repo config for " + repoKey); err != nil {
		return fed, err
	}

	var cfg struct {
		Members []struct {
			URL     string `json:"url"`
			Enabled bool   `json:"enabled"`
		} `json:"members"`
	}
	if err := resp.Unmarshal(&cfg); err != nil {
		return fed, fmt.Errorf("decode repo config for %s: %w", repoKey, err)
	}
	for _, m := range cfg.Members {
		fed.Members = append(fed.Members, instance.FederationMember{URL: m.URL, Enabled: m.Enabled})
	}
	return fed, nil
}

// GetFederatedRepos fetches member config for every repo in repos whose
// RepoType is "FEDERATED", concurrently (bounded by concurrency). A single
// repo's fetch failing is reported in warnings rather than aborting the rest.
func GetFederatedRepos(c *jfclient.Client, repos []RepoSummary, concurrency int) (federated []instance.FederatedRepo, warnings []string) {
	var keys []string
	for _, r := range repos {
		if r.RepoType == "FEDERATED" {
			keys = append(keys, r.RepoKey)
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}

	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]*instance.FederatedRepo, len(keys))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fed, err := GetFederatedRepoConfig(c, key)
			if err != nil {
				mu.Lock()
				warnings = append(warnings, err.Error())
				mu.Unlock()
				return
			}
			results[i] = &fed
		}(i, key)
	}
	wg.Wait()

	for _, r := range results {
		if r != nil {
			federated = append(federated, *r)
		}
	}
	return federated, warnings
}
