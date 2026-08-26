package cleanup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestResolveCacheRepo(t *testing.T) {
	cases := map[string]string{
		"npm-remote":          "npm-remote-cache",
		"docker-remote":       "docker-remote-cache",
		"already-cache-cache": "already-cache-cache",
	}
	for in, want := range cases {
		if got := ResolveCacheRepo(in); got != want {
			t.Fatalf("ResolveCacheRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRepoPackageType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/repositories/docker-remote" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"packageType":"Docker","type":"remote"}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	pkg, err := RepoPackageType(client, "docker-remote")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg != "docker" {
		t.Fatalf("expected lowercase 'docker', got %q", pkg)
	}
}

func TestNormalizePackageType(t *testing.T) {
	cases := []struct {
		requested, raw, want string
	}{
		{"auto", "Docker", "docker"},
		{"auto", "oci", "docker"},
		{"auto", "npm", "other"},
		{"", "docker", "docker"},
		{"other", "docker", "other"}, // explicit override wins
		{"docker", "npm", "docker"},  // explicit override wins
	}
	for _, tc := range cases {
		if got := NormalizePackageType(tc.requested, tc.raw); got != tc.want {
			t.Fatalf("NormalizePackageType(%q, %q) = %q, want %q", tc.requested, tc.raw, got, tc.want)
		}
	}
}

func TestFindRemoteCacheCandidatesOtherPackageType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "npm-remote-cache") {
			t.Fatalf("expected query against cache repo, got %q", body)
		}
		if !strings.Contains(string(body), `$nmatch`) {
			t.Fatalf("expected managed-root exclusion clause, got %q", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"repo":"npm-remote-cache","path":"pkg","name":"pkg-1.0.tgz","size":1000}]}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches, err := FindRemoteCacheCandidates(client, "npm-remote-cache", "6mo", "other")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFindRemoteCacheCandidatesDockerDedupes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		if strings.Contains(string(body), "list.manifest.json") {
			_, _ = w.Write([]byte(`{"results":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"repo":"docker-remote-cache","path":"img/1.0","name":"manifest.json","size":300}]}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches, err := FindRemoteCacheCandidates(client, "docker-remote-cache", "6mo", "docker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 deduped match, got %d: %+v", len(matches), matches)
	}
}

func TestXrayArtifactSummaryIndexedAndMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/xray/api/v1/summary/artifact" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req artifactSummaryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Paths) != 2 {
			t.Fatalf("expected 2 requested paths, got %d", len(req.Paths))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"artifacts": [{"general": {"path": "repo/a/indexed.txt"}}],
			"errors": [{"identifier": "repo/a/expired.txt", "error": "Artifact doesn't exist or not indexed/cached in Xray"}]
		}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	indexed, err := XrayArtifactSummary(client, []string{"repo/a/indexed.txt", "repo/a/expired.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !indexed["repo/a/indexed.txt"] {
		t.Fatalf("expected indexed.txt to be marked indexed, got %+v", indexed)
	}
	if indexed["repo/a/expired.txt"] {
		t.Fatalf("expected expired.txt to be marked not indexed, got %+v", indexed)
	}
}

func TestXrayArtifactSummaryEmptyPaths(t *testing.T) {
	client := jfclient.New("http://example.invalid", "tok", "", "", false)
	indexed, err := XrayArtifactSummary(client, nil)
	if err != nil {
		t.Fatalf("unexpected error for empty paths: %v", err)
	}
	if len(indexed) != 0 {
		t.Fatalf("expected empty map, got %+v", indexed)
	}
}

func TestClassifyByXrayIndexBatchesAndFiltersIndexed(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req artifactSummaryRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusOK)
		// Mark every path ending in "keep.txt" as indexed; everything else
		// is omitted from artifacts[] and so counts as not-indexed/expired.
		artifacts := []map[string]interface{}{}
		for _, p := range req.Paths {
			if strings.HasSuffix(p, "keep.txt") {
				artifacts = append(artifacts, map[string]interface{}{"general": map[string]string{"path": p}})
			}
		}
		resp := map[string]interface{}{"artifacts": artifacts, "errors": []interface{}{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	candidates := []ArtifactMatch{
		{Repo: "r", Path: "a", Name: "keep.txt"},
		{Repo: "r", Path: "a", Name: "drop1.txt"},
		{Repo: "r", Path: "a", Name: "drop2.txt"},
	}
	expired, err := ClassifyByXrayIndex(client, candidates, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 batched calls for batchSize=2 over 3 items, got %d", callCount)
	}
	if len(expired) != 2 {
		t.Fatalf("expected 2 expired candidates, got %d: %+v", len(expired), expired)
	}
	for _, m := range expired {
		if m.Name == "keep.txt" {
			t.Fatalf("indexed artifact must not be classified as expired: %+v", m)
		}
	}
}

func TestRemoteCacheDeleteTarget(t *testing.T) {
	dockerManifest := ArtifactMatch{Repo: "docker-remote-cache", Path: "img/1.0", Name: "manifest.json"}
	if got := RemoteCacheDeleteTarget(dockerManifest, "other"); got != "docker-remote-cache/img/1.0" {
		t.Fatalf("expected manifest.json to always resolve to tag dir, got %q", got)
	}

	genericFile := ArtifactMatch{Repo: "npm-remote-cache", Path: "pkg", Name: "pkg-1.0.tgz"}
	if got := RemoteCacheDeleteTarget(genericFile, "other"); got != "npm-remote-cache/pkg/pkg-1.0.tgz" {
		t.Fatalf("expected full artifact path for non-manifest file, got %q", got)
	}

	forcedDocker := ArtifactMatch{Repo: "r", Path: "img/1.0", Name: "list.manifest.json"}
	if got := RemoteCacheDeleteTarget(forcedDocker, "docker"); got != "r/img/1.0" {
		t.Fatalf("expected docker packageType to force tag-dir deletion, got %q", got)
	}
}

func TestDeleteRemoteCacheCandidatesDryRunNeverCallsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	candidates := []ArtifactMatch{{Repo: "r", Path: "a", Name: "f.txt"}}
	results := DeleteRemoteCacheCandidates(client, candidates, "other", true)
	if len(results) != 1 || !results[0].DryRun || results[0].Success {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
}

func TestDeleteRemoteCacheCandidatesDedupesDockerTagDirectory(t *testing.T) {
	var deleteCount int
	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCount++
			gotPaths = append(gotPaths, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	// Two manifest names for the same tag directory should collapse to one delete.
	candidates := []ArtifactMatch{
		{Repo: "docker-remote-cache", Path: "img/1.0", Name: "manifest.json"},
		{Repo: "docker-remote-cache", Path: "img/1.0", Name: "list.manifest.json"},
	}
	results := DeleteRemoteCacheCandidates(client, candidates, "docker", false)
	if len(results) != 1 {
		t.Fatalf("expected 1 deduped delete result, got %d: %+v", len(results), results)
	}
	if deleteCount != 1 {
		t.Fatalf("expected exactly 1 DELETE call, got %d (%v)", deleteCount, gotPaths)
	}
	if gotPaths[0] != "/artifactory/docker-remote-cache/img/1.0/" {
		t.Fatalf("expected delete of tag directory, got %q", gotPaths[0])
	}
}

func TestDeleteRemoteCacheCandidatesGenericArtifact(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	candidates := []ArtifactMatch{{Repo: "npm-remote-cache", Path: "pkg", Name: "pkg-1.0.tgz"}}
	results := DeleteRemoteCacheCandidates(client, candidates, "other", false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected successful delete, got %+v", results)
	}
	if gotPath != "/artifactory/npm-remote-cache/pkg/pkg-1.0.tgz" {
		t.Fatalf("expected delete of the single file (no trailing slash), got %q", gotPath)
	}
}
