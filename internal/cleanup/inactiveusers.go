package cleanup

import (
	"fmt"
	"time"

	"jfrogpscli/internal/jfclient"
)

// UserActivity represents one Artifactory user's login activity.
//
// NOTE: this is modeled on GET /artifactory/api/security/users as used by an
// internal PS shell script, NOT verified against official API docs or a live
// JPD. In particular, the exact field name and unit for LastLoggedIn (unix
// milliseconds? seconds? an RFC3339 string that would need different
// handling?) should be double-checked against the target JPD version before
// relying on this in production. As modeled here, LastLoggedIn is assumed to
// be unix milliseconds, and 0 means "never logged in".
type UserActivity struct {
	Name         string `json:"name"`
	Realm        string `json:"realm"`
	LastLoggedIn int64  `json:"lastLoggedIn"`
}

// FetchUsers retrieves all users and their last-login activity from
// GET /artifactory/api/security/users.
func FetchUsers(client *jfclient.Client) ([]UserActivity, error) {
	resp, err := client.Get("/artifactory/api/security/users")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	if err := resp.Error("list users"); err != nil {
		return nil, err
	}
	var users []UserActivity
	if err := resp.Unmarshal(&users); err != nil {
		return nil, fmt.Errorf("parse users response: %w", err)
	}
	return users, nil
}

// InactiveCandidates returns the users in all who are candidates for
// deletion: users whose LastLoggedIn is before cutoff (now minus months
// months), excluding users in excludeRealm. A LastLoggedIn of 0 ("never
// logged in") always makes a user a candidate, regardless of cutoff.
//
// now is accepted as an explicit parameter (rather than calling time.Now()
// internally) so callers can test this deterministically.
func InactiveCandidates(all []UserActivity, months int, excludeRealm string, now time.Time) []UserActivity {
	cutoff := now.AddDate(0, -months, 0)
	cutoffMillis := cutoff.UnixMilli()

	var candidates []UserActivity
	for _, u := range all {
		if u.Realm == excludeRealm {
			continue
		}
		if u.LastLoggedIn == 0 || u.LastLoggedIn < cutoffMillis {
			candidates = append(candidates, u)
		}
	}
	return candidates
}

// UserDeleteResult is the outcome of attempting to delete one inactive user.
type UserDeleteResult struct {
	Name    string
	DryRun  bool
	Success bool
	Error   string
}

// DeleteUsers deletes each user via DELETE /access/api/v2/users/{name}. When
// dryRun is true, no HTTP DELETE calls are issued at all.
func DeleteUsers(client *jfclient.Client, users []UserActivity, dryRun bool) []UserDeleteResult {
	results := make([]UserDeleteResult, 0, len(users))
	for _, u := range users {
		res := UserDeleteResult{Name: u.Name, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}
		resp, err := client.Delete("/access/api/v2/users/" + u.Name)
		switch {
		case err != nil:
			res.Error = err.Error()
		case !resp.OK():
			res.Error = resp.Error("delete user").Error()
		default:
			res.Success = true
		}
		results = append(results, res)
	}
	return results
}
