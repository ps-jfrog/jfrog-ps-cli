package discover

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"jfrogpscli/internal/jfclient"
)

// Row is one repo's full discovery report: storage summary, derived
// activity, and (optionally) matched cleanup-policy detail.
type Row struct {
	RepoSummary
	Activity Activity
	Policy   PolicyMatch
}

// ReportOptions controls a Report run.
type ReportOptions struct {
	// Concurrency bounds how many repos are processed at once. Values < 1
	// are treated as 1.
	Concurrency int
	// WithPolicies fetches and matches cleanup policies for every repo.
	WithPolicies bool
	// Include, if non-empty, keeps only repos whose key matches at least
	// one of these glob patterns (path.Match syntax).
	Include []string
	// Exclude, if non-empty, drops any repo whose key matches at least one
	// of these glob patterns. Applied after Include.
	Exclude []string
	// Progress, if non-nil, is called after each repo finishes (done is
	// 1-based) so a caller can print progress without this package
	// depending on any output/progress-bar library.
	Progress func(done, total int)
}

// Report builds one Row per repo: storage/usage from
// /artifactory/api/storageinfo, then (for non-empty repos) created/last-push
// /last-pull derived via AQL, and (if requested) matched cleanup-policy
// detail. Rows are processed up to opts.Concurrency at a time and returned
// sorted by repo key. A per-repo AQL failure does not abort the run - it
// leaves that repo's activity fields blank and is reported back in
// warnings, in no particular order, rather than failing the whole report.
func Report(c *jfclient.Client, opts ReportOptions) (rows []Row, warnings []string, err error) {
	concurrency := opts.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	repos, err := ListRepoStorage(c)
	if err != nil {
		return nil, nil, err
	}
	repos = FilterRepos(repos, opts.Include, opts.Exclude)

	var policies []CleanupPolicy
	if opts.WithPolicies {
		policies, err = ListCleanupPolicies(c)
		if err != nil {
			return nil, nil, err
		}
	}

	rows = make([]Row, len(repos))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var done int
	var mu sync.Mutex

	for i, repo := range repos {
		wg.Add(1)
		go func(i int, repo RepoSummary) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			row := Row{RepoSummary: repo}
			if repo.FilesCount > 0 {
				if act, actErr := GetActivity(c, repo.RepoKey); actErr != nil {
					mu.Lock()
					warnings = append(warnings, fmt.Sprintf("%s: %v", repo.RepoKey, actErr))
					mu.Unlock()
				} else {
					row.Activity = act
				}
			}
			if opts.WithPolicies {
				row.Policy = MatchPolicies(repo, policies)
			}
			rows[i] = row

			if opts.Progress != nil {
				mu.Lock()
				done++
				opts.Progress(done, len(repos))
				mu.Unlock()
			}
		}(i, repo)
	}
	wg.Wait()

	sort.Slice(rows, func(i, j int) bool { return rows[i].RepoKey < rows[j].RepoKey })
	return rows, warnings, nil
}

// FilterRepos keeps only repos matching include (if non-empty) and not
// matching exclude.
func FilterRepos(repos []RepoSummary, include, exclude []string) []RepoSummary {
	if len(include) == 0 && len(exclude) == 0 {
		return repos
	}
	out := make([]RepoSummary, 0, len(repos))
	for _, r := range repos {
		if len(include) > 0 && !matchesAnyGlob(r.RepoKey, include) {
			continue
		}
		if len(exclude) > 0 && matchesAnyGlob(r.RepoKey, exclude) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func matchesAnyGlob(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, name); err == nil && ok {
			return true
		}
	}
	return false
}
