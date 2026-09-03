package federation

import (
	"fmt"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// Member is one member of a federated repository, as embedded in the
// repository's full configuration under the "members" key.
type Member struct {
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

type repoConfig struct {
	Members []Member `json:"members"`
}

// MemberReport summarizes the federation member health of one repo.
type MemberReport struct {
	Repo        string
	MemberCount int
	Healthy     bool
	Issues      []string
}

// CheckMembers fetches the full repository config for repo and evaluates its
// federation members against expectedMembers, flagging any disabled member
// and any mismatch in member count.
func CheckMembers(c *jfclient.Client, repo string, expectedMembers int) (MemberReport, error) {
	report := MemberReport{Repo: repo}

	resp, err := c.Get("/artifactory/api/repositories/" + repo)
	if err != nil {
		return report, fmt.Errorf("get repository config for %s: %w", repo, err)
	}
	if err := resp.Error("get repository config for " + repo); err != nil {
		return report, err
	}

	var cfg repoConfig
	if err := resp.Unmarshal(&cfg); err != nil {
		return report, fmt.Errorf("parse repository config for %s: %w", repo, err)
	}

	report.MemberCount = len(cfg.Members)
	healthy := report.MemberCount == expectedMembers

	var issues []string
	for _, m := range cfg.Members {
		if !m.Enabled {
			issues = append(issues, fmt.Sprintf("member %s is disabled", m.URL))
			healthy = false
		}
	}
	if report.MemberCount != expectedMembers {
		issues = append(issues, fmt.Sprintf("expected %d member(s), found %d", expectedMembers, report.MemberCount))
	}

	report.Healthy = healthy
	report.Issues = issues
	return report, nil
}

// CheckAllMembers checks a single repo (if repo is non-empty) or every
// federated repo on the server.
func CheckAllMembers(c *jfclient.Client, repo string, expectedMembers int, onProgress ...progress.Reporter) ([]MemberReport, error) {
	var repos []string
	if repo != "" {
		repos = []string{repo}
	} else {
		var err error
		repos, err = ListFederatedRepos(c)
		if err != nil {
			return nil, err
		}
	}

	return concurrency.MapOrErr(repos, 0, func(r string) (MemberReport, error) {
		return CheckMembers(c, r, expectedMembers)
	}, onProgress...)
}
