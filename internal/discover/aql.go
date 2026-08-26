package discover

import (
	"fmt"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// Activity holds derived created/last-push/last-pull timestamps for a repo.
// Artifactory has no repo "created" timestamp of its own, so Created is a
// proxy: the created time of the repo's oldest item.
type Activity struct {
	Created      string // proxy: created time of the oldest item in the repo
	LastPush     string // proxy: modified time of the most recently modified item
	LastPull     string // most recent stat.downloaded across the repo's items
	LastActivity string // max(LastPush, LastPull)
}

type aqlItem struct {
	Created  string `json:"created"`
	Modified string `json:"modified"`
	Stats    []struct {
		Downloaded string `json:"downloaded"`
	} `json:"stats"`
}

type aqlResponse struct {
	Results []aqlItem `json:"results"`
}

// GetActivity derives created/last-push/last-pull for a repo via three AQL
// queries. Callers should skip repos with zero files: AQL will simply
// return no results for them, so the calls are wasted.
func GetActivity(c *jfclient.Client, repoKey string) (Activity, error) {
	key := escapeRepoKey(repoKey)
	var act Activity

	created, err := runAQL(c, fmt.Sprintf(
		`items.find({"repo":"%s"}).sort({"$asc":["created"]}).limit(1)`, key))
	if err != nil {
		return act, fmt.Errorf("created query: %w", err)
	}
	if len(created.Results) > 0 {
		act.Created = created.Results[0].Created
	}

	modified, err := runAQL(c, fmt.Sprintf(
		`items.find({"repo":"%s"}).sort({"$desc":["modified"]}).limit(1)`, key))
	if err != nil {
		return act, fmt.Errorf("modified query: %w", err)
	}
	if len(modified.Results) > 0 {
		act.LastPush = modified.Results[0].Modified
	}

	downloaded, err := runAQL(c, fmt.Sprintf(
		`items.find({"repo":"%s"}).include("stat.downloaded").sort({"$desc":["stat.downloaded"]}).limit(1)`, key))
	if err != nil {
		return act, fmt.Errorf("downloaded query: %w", err)
	}
	if len(downloaded.Results) > 0 && len(downloaded.Results[0].Stats) > 0 {
		act.LastPull = downloaded.Results[0].Stats[0].Downloaded
	}

	act.LastActivity = maxTimestamp(act.LastPush, act.LastPull)
	return act, nil
}

func runAQL(c *jfclient.Client, query string) (aqlResponse, error) {
	var resp aqlResponse
	r, err := c.AQL(query)
	if err != nil {
		return resp, err
	}
	if err := r.Error("run AQL query"); err != nil {
		return resp, err
	}
	if err := r.Unmarshal(&resp); err != nil {
		return resp, fmt.Errorf("decode AQL response: %w", err)
	}
	return resp, nil
}

// escapeRepoKey guards against a repo key breaking out of the AQL query's
// quoted string.
func escapeRepoKey(repoKey string) string {
	return strings.ReplaceAll(repoKey, `"`, `\"`)
}

// maxTimestamp returns the lexicographically greatest of two ISO-8601
// timestamps (an empty value loses). Artifactory's AQL timestamps share a
// fixed-width format and timezone, so string comparison orders correctly.
func maxTimestamp(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if a > b {
		return a
	}
	return b
}
