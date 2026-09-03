package cleanup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestFindStaleDockerTagsDedupesAcrossManifestNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		body := string(bodyBytes)

		w.WriteHeader(http.StatusOK)
		if jsonContains(body, "list.manifest.json") {
			// Multi-arch tag "multi-image/2.0" also has a list.manifest.json
			// at the same tag path as a plain manifest.json below.
			_, _ = w.Write([]byte(`{"results":[{"repo":"docker-local","path":"multi-image/2.0","name":"list.manifest.json","size":500}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[
			{"repo":"docker-local","path":"my-image/1.0","name":"manifest.json","size":300},
			{"repo":"docker-local","path":"multi-image/2.0","name":"manifest.json","size":300}
		]}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches, err := FindStaleDockerTags(client, "docker-local", "3mo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 deduped tags, got %d: %+v", len(matches), matches)
	}
	tags := map[string]bool{}
	for _, m := range matches {
		tags[DockerTagPath(m)] = true
	}
	if !tags["docker-local/my-image/1.0"] || !tags["docker-local/multi-image/2.0"] {
		t.Fatalf("unexpected tag set: %+v", tags)
	}
}

func jsonContains(body, substr string) bool {
	return len(body) > 0 && (func() bool {
		for i := 0; i+len(substr) <= len(body); i++ {
			if body[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

func TestDockerTagPath(t *testing.T) {
	cases := []struct {
		name string
		m    ArtifactMatch
		want string
	}{
		{"nested path", ArtifactMatch{Repo: "docker-local", Path: "my-image/1.0", Name: "manifest.json"}, "docker-local/my-image/1.0"},
		{"root path dot", ArtifactMatch{Repo: "docker-local", Path: ".", Name: "manifest.json"}, "docker-local"},
		{"empty path", ArtifactMatch{Repo: "docker-local", Path: "", Name: "manifest.json"}, "docker-local"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DockerTagPath(tc.m); got != tc.want {
				t.Fatalf("DockerTagPath(%+v) = %q, want %q", tc.m, got, tc.want)
			}
		})
	}
}

func TestDeleteDockerTagsDryRunNeverCallsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches := []ArtifactMatch{{Repo: "docker-local", Path: "my-image/1.0", Name: "manifest.json"}}
	results := DeleteDockerTags(client, matches, true)
	if len(results) != 1 || !results[0].DryRun || results[0].Success {
		t.Fatalf("unexpected dry-run results: %+v", results)
	}
}

func TestDeleteDockerTagsDeletesTagDirectoryNotJustManifest(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches := []ArtifactMatch{{Repo: "docker-local", Path: "my-image/1.0", Name: "manifest.json"}}
	results := DeleteDockerTags(client, matches, false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected successful delete, got %+v", results)
	}
	if gotPath != "/artifactory/docker-local/my-image/1.0/" {
		t.Fatalf("expected delete of the whole tag directory (trailing slash), got %q", gotPath)
	}
	if jsonContains(gotPath, "manifest.json") {
		t.Fatalf("must not delete only the manifest.json file, got %q", gotPath)
	}
}

func TestDeleteDockerTagsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "nope"})
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches := []ArtifactMatch{{Repo: "docker-local", Path: "my-image/1.0", Name: "manifest.json"}}
	results := DeleteDockerTags(client, matches, false)
	if len(results) != 1 || results[0].Success || results[0].Error == "" {
		t.Fatalf("expected failed delete with error set, got %+v", results)
	}
}
