package xray

import (
	"fmt"
	"net/url"
	"strings"

	"jfrogpscli/internal/concurrency"
	"jfrogpscli/internal/jfclient"
)

// projectSummary is one element of GET /access/api/v1/projects.
type projectSummary struct {
	ProjectKey string `json:"project_key"`
}

// listAllProjects lists every JFrog Project key via GET /access/api/v1/projects.
// A 403 (the caller's token lacks project-admin rights) degrades to an
// empty list rather than a fatal error -- instance-wide build discovery
// still works against the global default build-info repo in that case.
func listAllProjects(client *jfclient.Client) ([]string, error) {
	resp, err := client.Get("/access/api/v1/projects")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 403 {
		return nil, nil
	}
	if err := resp.Error("list projects"); err != nil {
		return nil, err
	}
	var projects []projectSummary
	if err := resp.Unmarshal(&projects); err != nil {
		return nil, fmt.Errorf("decode projects response: %w", err)
	}
	keys := make([]string, 0, len(projects))
	for _, p := range projects {
		keys = append(keys, p.ProjectKey)
	}
	return keys, nil
}

// storageChild is one element of GET /artifactory/api/storage/{repo}'s
// "children" array.
type storageChild struct {
	URI    string `json:"uri"`
	Folder bool   `json:"folder"`
}

// storageChildrenFolders lists the top-level folder names in repo via
// GET /artifactory/api/storage/{repo}. A 404 (the repo doesn't exist on
// this instance, e.g. a project with no build-info repo yet) degrades to
// an empty list.
func storageChildrenFolders(client *jfclient.Client, repo string) ([]string, error) {
	resp, err := client.Get("/artifactory/api/storage/" + repo)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if err := resp.Error("list storage children of " + repo); err != nil {
		return nil, err
	}
	var out struct {
		Children []storageChild `json:"children"`
	}
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("decode storage response for %s: %w", repo, err)
	}
	names := make([]string, 0, len(out.Children))
	for _, c := range out.Children {
		if c.Folder {
			names = append(names, strings.TrimPrefix(c.URI, "/"))
		}
	}
	return names, nil
}

// storageItemExists reports whether GET /artifactory/api/storage/{repo}/{path}
// returns 200 -- the generic "does X live in repo Y" existence check used
// throughout build-repo resolution.
func storageItemExists(client *jfclient.Client, repo, path string) bool {
	resp, err := client.Get("/artifactory/api/storage/" + repo + "/" + path)
	return err == nil && resp.OK()
}

// buildListEntry is one element of GET /artifactory/api/build's "builds"
// array.
type buildListEntry struct {
	URI string `json:"uri"`
}

// listAllBuildNames lists build names via GET /artifactory/api/build
// (optionally scoped to a project). Documented as less reliable than a
// storage-tree scan (it can miss real builds), so it is only ever used to
// supplement a ground-truth storage scan, never alone. A 404 (no builds at
// all) degrades to an empty list.
func listAllBuildNames(client *jfclient.Client, projectKey string) ([]string, error) {
	path := "/artifactory/api/build"
	if projectKey != "" {
		path += "?projectKey=" + url.QueryEscape(projectKey)
	}
	resp, err := client.Get(path)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 404 {
		return nil, nil
	}
	if err := resp.Error("list builds"); err != nil {
		return nil, err
	}
	var out struct {
		Builds []buildListEntry `json:"builds"`
	}
	if err := resp.Unmarshal(&out); err != nil {
		return nil, fmt.Errorf("decode build list response: %w", err)
	}
	names := make([]string, 0, len(out.Builds))
	for _, b := range out.Builds {
		name := strings.TrimPrefix(b.URI, "/")
		if unescaped, err := url.QueryUnescape(name); err == nil {
			name = unescaped
		}
		names = append(names, name)
	}
	return names, nil
}

// resolveBuildBackingRepo tries each candidate build-info repo in order and
// returns the first one that actually contains buildName, verified via
// storageItemExists.
func resolveBuildBackingRepo(client *jfclient.Client, buildName string, candidateRepos []string) (string, bool) {
	for _, repo := range candidateRepos {
		if storageItemExists(client, repo, buildName) {
			return repo, true
		}
	}
	return "", false
}

// discoverBuildsWithRepos performs instance-wide build discovery: a
// ground-truth scan of every known build-info repo's storage tree (the
// global "artifactory-build-info" plus "<project>-build-info" for every
// JFrog Project), supplemented with the build-listing API for any build
// name the storage scan missed.
func discoverBuildsWithRepos(client *jfclient.Client) ([]buildWithRepo, error) {
	candidateRepos := []string{"artifactory-build-info"}
	projects, err := listAllProjects(client)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		candidateRepos = append(candidateRepos, p+"-build-info")
	}

	// Scan every candidate repo's storage tree concurrently (each is an
	// independent, read-only GET); any failure aborts the whole scan, same
	// as the previous sequential version. Deduplication happens afterward,
	// sequentially, so two repos never race to decide who "owns" a build
	// name found in both.
	perRepo, err := concurrency.MapOrErr(candidateRepos, 0, func(repo string) (repoBuildNames, error) {
		names, err := storageChildrenFolders(client, repo)
		if err != nil {
			return repoBuildNames{}, err
		}
		return repoBuildNames{repo: repo, names: names}, nil
	})
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var result []buildWithRepo
	for _, rb := range perRepo {
		for _, n := range rb.names {
			if seen[n] {
				continue
			}
			seen[n] = true
			result = append(result, buildWithRepo{Build: n, Repo: rb.repo})
		}
	}

	// Supplement with the build-listing API for any residual names the
	// storage scan missed. Best-effort: a failure here doesn't invalidate
	// the ground-truth scan that already ran. Resolving each residual
	// name's backing repo is itself a per-repo existence check
	// (storageItemExists), so run the residual names concurrently too;
	// seen is only read here (never written until the sequential merge
	// below), so concurrent lookups never race on it.
	if listed, err := listAllBuildNames(client, ""); err == nil {
		var toResolve []string
		for _, n := range listed {
			if !seen[n] {
				toResolve = append(toResolve, n)
			}
		}
		resolved := concurrency.Map(toResolve, 0, func(n string) []buildWithRepo {
			if repo, ok := resolveBuildBackingRepo(client, n, candidateRepos); ok {
				return []buildWithRepo{{Build: n, Repo: repo}}
			}
			return nil
		})
		for _, bwr := range resolved {
			if seen[bwr.Build] {
				continue
			}
			seen[bwr.Build] = true
			result = append(result, bwr)
		}
	}
	return result, nil
}

// repoBuildNames pairs one candidate build-info repo with the build names
// discoverBuildsWithRepos found in its storage tree.
type repoBuildNames struct {
	repo  string
	names []string
}

// ListRepositoriesForProjectFiltered is ListRepositoriesForProject with an
// optional repo-class filter (local/remote/virtual/federated).
func ListRepositoriesForProjectFiltered(client *jfclient.Client, projectKey, rclass string) ([]string, error) {
	path := "/artifactory/api/repositories?project=" + url.QueryEscape(projectKey)
	if rclass != "" {
		path += "&type=" + url.QueryEscape(rclass)
	}
	resp, err := client.Get(path)
	if err != nil {
		return nil, err
	}
	if err := resp.Error("list repositories for project " + projectKey); err != nil {
		return nil, err
	}
	var repos []repoSummary
	if err := resp.Unmarshal(&repos); err != nil {
		return nil, fmt.Errorf("decode repositories response: %w", err)
	}
	keys := make([]string, 0, len(repos))
	for _, r := range repos {
		keys = append(keys, r.Key)
	}
	return keys, nil
}

func reposToResourceList(names []string) []repositoryResource {
	out := make([]repositoryResource, 0, len(names))
	for _, n := range names {
		out = append(out, repositoryResource{Name: n})
	}
	return out
}

// watchResource is one element of a watch's project_resources.resources.
type watchResource struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// watchResponse models the fields used from GET /xray/api/v2/watches/{name}.
type watchResponse struct {
	ProjectResources struct {
		Resources []watchResource `json:"resources"`
	} `json:"project_resources"`
}

// GetWatch fetches a watch's resource list via GET /xray/api/v2/watches/{name}.
func GetWatch(client *jfclient.Client, name string) ([]watchResource, error) {
	resp, err := client.Get("/xray/api/v2/watches/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	if err := resp.Error("get watch " + name); err != nil {
		return nil, err
	}
	var w watchResponse
	if err := resp.Unmarshal(&w); err != nil {
		return nil, fmt.Errorf("decode watch response: %w", err)
	}
	return w.ProjectResources.Resources, nil
}

// resolveWatchScope derives report resources from a watch's own resource
// list. A watch scoped to a JFrog Project resolves that project's
// repositories directly. A watch scoped to repositories/builds/release
// bundles uses those resources as-is. "all-builds" folds into instance-wide
// build discovery. Any other wildcard type (all-repos, all-releaseBundles*,
// all-projects) -- or a watch with no resources at all -- is rejected: the
// caller should use --reponame/--project/--buildname explicitly instead.
//
// SIMPLIFICATION: a watch whose resources mix kinds (which the Xray API
// itself rejects when creating the watch) is resolved using the first
// supported kind encountered in the resources array, rather than picking
// alphabetically and logging the rest as dropped.
func resolveWatchScope(client *jfclient.Client, watchName string, buildNumberOfVersions int) (reportResources, error) {
	resources, err := GetWatch(client, watchName)
	if err != nil {
		return reportResources{}, err
	}
	if len(resources) == 0 {
		return reportResources{}, fmt.Errorf("watch %q has no resources", watchName)
	}

	for _, r := range resources {
		if r.Type == "project" {
			repos, err := ListRepositoriesForProjectFiltered(client, r.Name, "")
			if err != nil {
				return reportResources{}, err
			}
			return reportResources{Repositories: reposToResourceList(repos)}, nil
		}
	}

	var repoNames, buildNames, bundleNames, unsupported []string
	sawAllBuilds := false
	for _, r := range resources {
		switch r.Type {
		case "repository":
			repoNames = append(repoNames, r.Name)
		case "build":
			buildNames = append(buildNames, r.Name)
		case "releaseBundle", "releaseBundleV2":
			bundleNames = append(bundleNames, r.Name)
		case "all-builds":
			sawAllBuilds = true
		default:
			unsupported = append(unsupported, r.Type)
		}
	}

	switch {
	case len(repoNames) > 0:
		return reportResources{Repositories: reposToResourceList(repoNames)}, nil
	case len(buildNames) > 0:
		bwr := concurrency.Map(buildNames, 0, func(b string) []buildWithRepo {
			if repo, ok := resolveBuildBackingRepo(client, b, []string{"artifactory-build-info"}); ok {
				return []buildWithRepo{{Build: b, Repo: repo}}
			}
			return nil
		})
		if len(bwr) == 0 {
			return reportResources{}, fmt.Errorf("could not resolve a build-info repo for any build in watch %q", watchName)
		}
		names := make([]string, len(bwr))
		for i, b := range bwr {
			names[i] = b.Build
		}
		return reportResources{Builds: &buildsResource{Names: names, NumberOfLatestVersions: buildNumberOfVersions, BuildsWithRepos: bwr}}, nil
	case len(bundleNames) > 0:
		return reportResources{ReleaseBundles: &releaseBundlesResource{Names: bundleNames, NumberOfLatestVersions: buildNumberOfVersions}}, nil
	case sawAllBuilds:
		bwr, err := discoverBuildsWithRepos(client)
		if err != nil {
			return reportResources{}, err
		}
		if len(bwr) == 0 {
			return reportResources{}, fmt.Errorf("no builds discovered instance-wide for watch %q", watchName)
		}
		names := make([]string, len(bwr))
		for i, b := range bwr {
			names[i] = b.Build
		}
		return reportResources{Builds: &buildsResource{Names: names, NumberOfLatestVersions: buildNumberOfVersions, BuildsWithRepos: bwr}}, nil
	default:
		return reportResources{}, fmt.Errorf("watch %q targets an unsupported resource type %v -- use --reponame/--project/--buildname explicitly", watchName, unsupported)
	}
}

// ResolveReportScope turns a ReportScope into the "resources" object of a
// report-generation request. Exactly one of Project, Reponame, Buildname,
// (ResourceType=="builds" alone), or Watchname (alone) must be set --
// checked by the CLI layer before this is called, but re-validated here.
func ResolveReportScope(client *jfclient.Client, scope ReportScope, buildNumberOfVersions int) (reportResources, error) {
	if buildNumberOfVersions <= 0 {
		buildNumberOfVersions = 5
	}
	explicitCount := 0
	for _, s := range []string{scope.Reponame, scope.Project, scope.Buildname} {
		if s != "" {
			explicitCount++
		}
	}
	instanceWideBuilds := explicitCount == 0 && scope.ResourceType == "builds"
	watchOnly := explicitCount == 0 && !instanceWideBuilds && scope.Watchname != ""

	switch {
	case scope.Project != "":
		repos, err := ListRepositoriesForProjectFiltered(client, scope.Project, scope.RepoRClass)
		if err != nil {
			return reportResources{}, err
		}
		return reportResources{Repositories: reposToResourceList(repos)}, nil
	case scope.Reponame != "":
		return reportResources{Repositories: []repositoryResource{{Name: scope.Reponame}}}, nil
	case scope.Buildname != "":
		if scope.BuildRepo == "" {
			return reportResources{}, fmt.Errorf("--build-repo is required with --buildname")
		}
		return reportResources{Builds: &buildsResource{
			Names:                  []string{scope.Buildname},
			NumberOfLatestVersions: buildNumberOfVersions,
			BuildsWithRepos:        []buildWithRepo{{Build: scope.Buildname, Repo: scope.BuildRepo}},
		}}, nil
	case instanceWideBuilds:
		bwr, err := discoverBuildsWithRepos(client)
		if err != nil {
			return reportResources{}, err
		}
		if len(bwr) == 0 {
			return reportResources{}, fmt.Errorf("no builds discovered instance-wide")
		}
		names := make([]string, len(bwr))
		for i, b := range bwr {
			names[i] = b.Build
		}
		return reportResources{Builds: &buildsResource{Names: names, NumberOfLatestVersions: buildNumberOfVersions, BuildsWithRepos: bwr}}, nil
	case watchOnly:
		return resolveWatchScope(client, scope.Watchname, buildNumberOfVersions)
	default:
		return reportResources{}, fmt.Errorf("exactly one of --reponame, --project, --buildname, --resource-type=builds (alone), or --watchname (alone) is required")
	}
}
