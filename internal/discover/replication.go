package discover

import (
	"fmt"

	"jfrogpscli/internal/instance"
	"jfrogpscli/internal/jfclient"
)

// GetReplications fetches every configured replication in one call
// (GET /artifactory/api/replications) - there is no per-repo lookup needed.
func GetReplications(c *jfclient.Client) ([]instance.ReplicationConfig, error) {
	resp, err := c.Get("/artifactory/api/replications")
	if err != nil {
		return nil, fmt.Errorf("list replications: %w", err)
	}
	if err := resp.Error("list replications"); err != nil {
		return nil, err
	}

	var raw []struct {
		RepoKey         string `json:"repoKey"`
		ReplicationType string `json:"replicationType"`
		Enabled         bool   `json:"enabled"`
		URL             string `json:"url"`
		CronExp         string `json:"cronExp"`
		SyncDeletes     bool   `json:"syncDeletes"`
	}
	if err := resp.Unmarshal(&raw); err != nil {
		return nil, fmt.Errorf("decode replications response: %w", err)
	}

	out := make([]instance.ReplicationConfig, 0, len(raw))
	for _, r := range raw {
		out = append(out, instance.ReplicationConfig{
			RepoKey:         r.RepoKey,
			ReplicationType: r.ReplicationType,
			Enabled:         r.Enabled,
			URL:             r.URL,
			CronExp:         r.CronExp,
			SyncDeletes:     r.SyncDeletes,
		})
	}
	return out, nil
}
