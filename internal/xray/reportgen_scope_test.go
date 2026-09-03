package xray

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"jfrogpscli/internal/jfclient"
)

// TestResolveReportScopeWatchBuildResource locks in resolveWatchScope's
// build-list branch: every build resource on the watch gets checked
// against the global build-info repo, in the watch's resource order, and
// one whose build-info can't be found there (storageItemExists returns
// false) is silently dropped rather than failing the whole scope
// resolution.
func TestResolveReportScopeWatchBuildResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v2/watches/w1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"build","name":"build-a"},{"type":"build","name":"build-b"},{"type":"build","name":"build-missing"}]}}`))
	})
	mux.HandleFunc("/artifactory/api/storage/artifactory-build-info/build-a", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/artifactory/api/storage/artifactory-build-info/build-b", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/artifactory/api/storage/artifactory-build-info/build-missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	resources, err := ResolveReportScope(client, ReportScope{Watchname: "w1"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resources.Builds == nil {
		t.Fatalf("expected a Builds resource, got %+v", resources)
	}
	want := []string{"build-a", "build-b"}
	if !reflect.DeepEqual(resources.Builds.Names, want) {
		t.Fatalf("expected builds %v in order, got %v", want, resources.Builds.Names)
	}
	for _, bwr := range resources.Builds.BuildsWithRepos {
		if bwr.Repo != "artifactory-build-info" {
			t.Errorf("expected every resolved build to back onto artifactory-build-info, got %+v", bwr)
		}
	}
}

// TestResolveReportScopeAllBuildsDiscovery locks in discoverBuildsWithRepos'
// two-pass behavior: a ground-truth storage-tree scan across the global
// and every per-project build-info repo, deduplicated against, then
// supplemented with the build-listing API only for names the scan missed.
func TestResolveReportScopeAllBuildsDiscovery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/xray/api/v2/watches/w1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"project_resources":{"resources":[{"type":"all-builds","name":""}]}}`))
	})
	mux.HandleFunc("/access/api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"project_key":"proj1"}]`))
	})
	// Ground-truth storage scan: "scanned-build" lives in the global repo.
	mux.HandleFunc("/artifactory/api/storage/artifactory-build-info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"children":[{"uri":"/scanned-build","folder":true}]}`))
	})
	// The per-project build-info repo doesn't exist yet -- 404 degrades to
	// "no builds there", not a failure.
	mux.HandleFunc("/artifactory/api/storage/proj1-build-info", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	// The build-listing API additionally reports "scanned-build" (already
	// found -- must not be resolved/added a second time) and
	// "residual-build" (missed by the scan, must be resolved and added).
	mux.HandleFunc("/artifactory/api/build", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"builds":[{"uri":"/scanned-build"},{"uri":"/residual-build"}]}`))
	})
	mux.HandleFunc("/artifactory/api/storage/artifactory-build-info/residual-build", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// "scanned-build" was already found by the ground-truth storage scan;
	// a dedup bug would re-resolve it via this same existence-check
	// endpoint, so count hits here to catch that even though the result
	// (200 OK) wouldn't otherwise look wrong.
	var scannedBuildRechecked atomic.Int64
	mux.HandleFunc("/artifactory/api/storage/artifactory-build-info/scanned-build", func(w http.ResponseWriter, r *http.Request) {
		scannedBuildRechecked.Add(1)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	resources, err := ResolveReportScope(client, ReportScope{Watchname: "w1"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resources.Builds == nil {
		t.Fatalf("expected a Builds resource, got %+v", resources)
	}

	got := map[string]string{}
	for _, bwr := range resources.Builds.BuildsWithRepos {
		got[bwr.Build] = bwr.Repo
	}
	want := map[string]string{
		"scanned-build":  "artifactory-build-info",
		"residual-build": "artifactory-build-info",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if len(resources.Builds.Names) != 2 {
		t.Fatalf("expected exactly 2 discovered builds (no duplicate), got %v", resources.Builds.Names)
	}
	if scannedBuildRechecked.Load() != 0 {
		t.Fatalf("expected scanned-build to never be re-resolved via the residual API path (dedup should have skipped it), got %d recheck(s)", scannedBuildRechecked.Load())
	}
}
