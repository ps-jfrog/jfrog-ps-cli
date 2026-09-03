package cleanup

import (
	"fmt"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
	"jfrogpscli/internal/progress"
)

// dockerManifestNames are the file names that mark a Docker/OCI tag
// directory: a single-arch manifest, or the top-level manifest list/index of
// a multi-arch ("fat") manifest.
var dockerManifestNames = []string{"manifest.json", "list.manifest.json"}

// FindStaleDockerTags finds Docker/OCI tags in repo whose manifest has not
// been downloaded (or was downloaded, but not since olderThan). It returns
// one ArtifactMatch per stale manifest file found -- see DockerTagPath to
// turn a match into the tag directory to delete.
//
// Only searching by manifest file name (rather than deleting every stale
// file in the repo) mirrors the documented KB approach: a tag's config JSON
// and layer blobs are shared/deduplicated in Artifactory's Docker storage, so
// download stats and cleanup decisions should be driven by the manifest,
// then the whole tag directory deleted -- see the package doc on
// DeleteDockerTags for why deleting the manifest alone is a common pitfall.
func FindStaleDockerTags(client *jfclient.Client, repo, olderThan string) ([]ArtifactMatch, error) {
	seen := make(map[string]bool)
	var all []ArtifactMatch
	for _, name := range dockerManifestNames {
		query := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: repo, OlderThan: olderThan, NameEquals: name})
		matches, err := FindArtifacts(client, query)
		if err != nil {
			return nil, fmt.Errorf("find stale %s: %w", name, err)
		}
		for _, m := range matches {
			tag := DockerTagPath(m)
			if seen[tag] {
				continue
			}
			seen[tag] = true
			all = append(all, m)
		}
	}
	return all, nil
}

// DockerTagPath returns the full repository-relative path of the Docker tag
// directory containing manifest match m, e.g. repo="docker-local"
// path="my-image/1.0" name="manifest.json" -> "docker-local/my-image/1.0".
//
// This is deliberately the tag's directory, not the manifest file itself:
// running a plain artifact delete against an AQL search for "manifest.json"
// only removes that one file and leaves the tag's config blob and layers
// behind as orphaned storage -- a documented, common cleanup mistake. Always
// delete the directory returned here (with a trailing slash) to remove the
// full tag.
func DockerTagPath(m ArtifactMatch) string {
	parts := []string{m.Repo}
	if m.Path != "" && m.Path != "." {
		parts = append(parts, m.Path)
	}
	return strings.Join(parts, "/")
}

// DeleteDockerTags deletes the full tag directory (not just the manifest)
// for each stale manifest match, via
// DELETE /artifactory/{repo}/{tag-path}/. When dryRun is true, no HTTP DELETE
// calls are issued at all.
func DeleteDockerTags(client *jfclient.Client, matches []ArtifactMatch, dryRun bool, onProgress ...progress.Reporter) []DeleteResult {
	return concurrency.One(matches, 0, func(m ArtifactMatch) DeleteResult {
		tagPath := DockerTagPath(m)
		res := DeleteResult{Match: m, Path: tagPath, DryRun: dryRun}
		if dryRun {
			return res
		}
		resp, err := client.Delete("/artifactory/" + tagPath + "/")
		switch {
		case err != nil:
			res.Error = err.Error()
		case !resp.OK():
			res.Error = resp.Error("delete docker tag").Error()
		default:
			res.Success = true
		}
		return res
	}, onProgress...)
}
