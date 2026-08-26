package cleanup

import "fmt"

// DownloadStaleQuery holds the parameters for the single most-reused cleanup
// AQL pattern across the JFrog PS scripts this CLI is based on: find items in
// a repo that were either never downloaded (and haven't been touched
// recently) or were downloaded, but not since a cutoff duration.
type DownloadStaleQuery struct {
	// Repo is the repository key to search (required).
	Repo string
	// OlderThan is an AQL relative-time duration, e.g. "3mo", "6mo", "1y",
	// "90d" (required). See "Relative Time Operators in AQL".
	OlderThan string
	// NameEquals, if set, restricts matches to items with this exact name
	// (e.g. "manifest.json" for Docker tag cleanup).
	NameEquals string
	// ExcludeManagedRootPaths, if true, excludes items under
	// Artifactory-managed root folders (path segments starting with '.',
	// e.g. ".jfrog", ".npm") -- relevant for remote-repository caches, which
	// contain such housekeeping folders alongside real cached artifacts.
	ExcludeManagedRootPaths bool
}

// BuildDownloadStaleAQL builds the AQL query for q: items with type "file" in
// Repo that are either never downloaded and last updated before OlderThan, or
// downloaded but not since OlderThan.
func BuildDownloadStaleAQL(q DownloadStaleQuery) string {
	nameClause := ""
	if q.NameEquals != "" {
		nameClause = fmt.Sprintf(`,"name":{"$eq":%q}`, q.NameEquals)
	}

	staleClause := `{"$or":[
		{"$and":[{"stat.downloads":{"$eq":null}},{"updated":{"$before":"` + q.OlderThan + `"}}]},
		{"$and":[{"stat.downloads":{"$eq":0}},{"updated":{"$before":"` + q.OlderThan + `"}}]},
		{"$and":[{"stat.downloads":{"$gt":0}},{"stat.downloaded":{"$before":"` + q.OlderThan + `"}}]}
	]}`

	andClauses := staleClause
	if q.ExcludeManagedRootPaths {
		rootClause := `{"$or":[{"path":{"$eq":"."}},{"path":{"$nmatch":".*"}}]}`
		andClauses = rootClause + "," + staleClause
	}

	return fmt.Sprintf(
		`items.find({"repo":{"$eq":%q},"type":"file"%s,"$and":[%s]}).include("repo","path","name","size","created","modified","updated","stat.downloaded","stat.downloads")`,
		q.Repo, nameClause, andClauses,
	)
}

// TotalSize sums the Size field across matches, for reporting expected
// storage savings before a cleanup runs (e.g. in a dry-run summary).
func TotalSize(matches []ArtifactMatch) int64 {
	var total int64
	for _, m := range matches {
		total += m.Size
	}
	return total
}

// FormatBytes renders n bytes as a human-readable string (B/KB/MB/GB/TB),
// using 1024-based units.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), units[exp])
}
