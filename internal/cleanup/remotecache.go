package cleanup

import (
	"fmt"
	"strings"

	"jfrogpscli/internal/jfclient"
)

// ResolveCacheRepo maps a remote repository key to its cache repository key,
// e.g. "npm-remote" -> "npm-remote-cache". A key already ending in "-cache"
// is returned unchanged.
func ResolveCacheRepo(remoteKey string) string {
	if strings.HasSuffix(remoteKey, "-cache") {
		return remoteKey
	}
	return remoteKey + "-cache"
}

// repoConfig models the fields of GET /artifactory/api/repositories/{repo}
// this package needs.
type repoConfig struct {
	PackageType string `json:"packageType"`
	Type        string `json:"type"`
}

// RepoPackageType returns the lowercase packageType of repo (e.g. "docker",
// "npm", "generic") via GET /artifactory/api/repositories/{repo}.
func RepoPackageType(client *jfclient.Client, repo string) (string, error) {
	resp, err := client.Get("/artifactory/api/repositories/" + repo)
	if err != nil {
		return "", fmt.Errorf("get repository config for %s: %w", repo, err)
	}
	if err := resp.Error("get repository config"); err != nil {
		return "", err
	}
	var cfg repoConfig
	if err := resp.Unmarshal(&cfg); err != nil {
		return "", fmt.Errorf("parse repository config for %s: %w", repo, err)
	}
	return strings.ToLower(cfg.PackageType), nil
}

// NormalizePackageType maps a repo's raw packageType to the coarse
// docker/other split used for cleanup decisions (Docker/OCI tags must be
// deleted as a whole directory; everything else is deleted as a single
// artifact). requested overrides auto-detection unless it is "auto".
func NormalizePackageType(requested, rawPackageType string) string {
	if requested != "" && requested != "auto" {
		return requested
	}
	switch strings.ToLower(rawPackageType) {
	case "docker", "oci":
		return "docker"
	default:
		return "other"
	}
}

// FindRemoteCacheCandidates finds download-stale artifacts in cacheRepo
// (typically a "<remote>-cache" repository), excluding Artifactory-managed
// root folders (.jfrog, .npm, etc). When packageType is "docker", only
// manifest files are searched, mirroring FindStaleDockerTags -- the download
// stats that matter for a Docker tag live on its manifest.
func FindRemoteCacheCandidates(client *jfclient.Client, cacheRepo, olderThan, packageType string) ([]ArtifactMatch, error) {
	if packageType == "docker" {
		seen := make(map[string]bool)
		var all []ArtifactMatch
		for _, name := range dockerManifestNames {
			query := BuildDownloadStaleAQL(DownloadStaleQuery{
				Repo: cacheRepo, OlderThan: olderThan, NameEquals: name, ExcludeManagedRootPaths: true,
			})
			matches, err := FindArtifacts(client, query)
			if err != nil {
				return nil, fmt.Errorf("find stale %s: %w", name, err)
			}
			for _, m := range matches {
				key := m.Repo + "/" + m.Path + "/" + m.Name
				if seen[key] {
					continue
				}
				seen[key] = true
				all = append(all, m)
			}
		}
		return all, nil
	}

	query := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: cacheRepo, OlderThan: olderThan, ExcludeManagedRootPaths: true})
	return FindArtifacts(client, query)
}

// artifactSummaryRequest is the body of POST /xray/api/v1/summary/artifact.
type artifactSummaryRequest struct {
	Paths []string `json:"paths"`
}

// artifactSummaryResponse is the shape of a successful Artifact Summary
// response: artifacts Xray has indexed (and their info) appear in Artifacts;
// paths Xray could not summarize (not indexed, or the retention period on a
// remote-repository cache entry has expired) appear in Errors, keyed by the
// full artifact path via Identifier.
type artifactSummaryResponse struct {
	Artifacts []struct {
		General struct {
			Path string `json:"path"`
		} `json:"general"`
	} `json:"artifacts"`
	Errors []struct {
		Identifier string `json:"identifier"`
		Error      string `json:"error"`
	} `json:"errors"`
}

// XrayArtifactSummary calls POST /xray/api/v1/summary/artifact for the given
// full artifact paths (repo/path/name form) and returns the subset indexed by
// Xray (present in the response's "artifacts" list). Any path not returned as
// indexed -- whether Xray reports it as an explicit error, or simply omits it
// -- is treated as not indexed: this covers both "never indexed" and
// "retention period expired" (a remote-cache-specific state where Xray drops
// its record for an artifact after the cache entry's retention window
// passes), since the API does not reliably distinguish the two in its error
// text across versions.
func XrayArtifactSummary(client *jfclient.Client, paths []string) (indexed map[string]bool, err error) {
	indexed = make(map[string]bool, len(paths))
	if len(paths) == 0 {
		return indexed, nil
	}
	resp, err := client.Post("/xray/api/v1/summary/artifact", artifactSummaryRequest{Paths: paths})
	if err != nil {
		return nil, fmt.Errorf("xray artifact summary: %w", err)
	}
	if err := resp.Error("xray artifact summary"); err != nil {
		return nil, err
	}
	var out artifactSummaryResponse
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("parse xray artifact summary response: %w", err)
	}
	for _, a := range out.Artifacts {
		if a.General.Path != "" {
			indexed[a.General.Path] = true
		}
	}
	return indexed, nil
}

// ClassifyByXrayIndex splits candidates into those still Xray-indexed
// (skip -- leave alone) and those not indexed / retention-expired (safe to
// clean up), by calling XrayArtifactSummary in batches of batchSize.
func ClassifyByXrayIndex(client *jfclient.Client, candidates []ArtifactMatch, batchSize int) (expired []ArtifactMatch, err error) {
	if batchSize <= 0 {
		batchSize = 50
	}
	for start := 0; start < len(candidates); start += batchSize {
		end := start + batchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]

		paths := make([]string, len(batch))
		for i, m := range batch {
			paths[i] = ArtifactPath(m)
		}

		indexed, err := XrayArtifactSummary(client, paths)
		if err != nil {
			return nil, fmt.Errorf("classify batch %d-%d: %w", start, end, err)
		}
		for _, m := range batch {
			if !indexed[ArtifactPath(m)] {
				expired = append(expired, m)
			}
		}
	}
	return expired, nil
}

// RemoteCacheDeleteTarget returns the path to delete for candidate m: the
// full Docker tag directory when packageType is "docker" (or m is a manifest
// file), otherwise the single artifact path.
func RemoteCacheDeleteTarget(m ArtifactMatch, packageType string) string {
	if packageType == "docker" || m.Name == "manifest.json" || m.Name == "list.manifest.json" {
		return DockerTagPath(m)
	}
	return ArtifactPath(m)
}

// DeleteRemoteCacheCandidates deletes the resolved target for each expired
// candidate (deduplicating repeated Docker tag directories), via
// DELETE /artifactory/{target}[/]. When dryRun is true, no HTTP DELETE calls
// are issued at all.
func DeleteRemoteCacheCandidates(client *jfclient.Client, candidates []ArtifactMatch, packageType string, dryRun bool) []DeleteResult {
	seen := make(map[string]bool)
	results := make([]DeleteResult, 0, len(candidates))
	for _, m := range candidates {
		target := RemoteCacheDeleteTarget(m, packageType)
		isDir := target != ArtifactPath(m)
		if seen[target] {
			continue
		}
		seen[target] = true

		res := DeleteResult{Match: m, Path: target, DryRun: dryRun}
		if dryRun {
			results = append(results, res)
			continue
		}

		reqPath := "/artifactory/" + target
		if isDir {
			reqPath += "/"
		}
		resp, err := client.Delete(reqPath)
		switch {
		case err != nil:
			res.Error = err.Error()
		case !resp.OK():
			res.Error = resp.Error("delete remote cache artifact").Error()
		default:
			res.Success = true
		}
		results = append(results, res)
	}
	return results
}
