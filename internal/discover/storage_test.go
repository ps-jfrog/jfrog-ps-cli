package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func testClient(t *testing.T, srv *httptest.Server) *jfclient.Client {
	t.Helper()
	return jfclient.New(srv.URL, "test-token", "", "", false)
}

func TestListRepoStorage_ExcludesTotalRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/storageinfo" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"repositoriesSummaryList": []map[string]interface{}{
				{"repoKey": "TOTAL", "repoType": "NA", "packageType": "NA", "filesCount": 100, "usedSpaceInBytes": 1024},
				{"repoKey": "libs-release-local", "repoType": "LOCAL", "packageType": "Maven", "filesCount": 5, "usedSpaceInBytes": 1073741824},
			},
		})
	}))
	defer srv.Close()

	repos, err := ListRepoStorage(testClient(t, srv))
	if err != nil {
		t.Fatalf("ListRepoStorage returned error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo after excluding TOTAL, got %d: %+v", len(repos), repos)
	}
	if repos[0].RepoKey != "libs-release-local" {
		t.Fatalf("unexpected repo: %+v", repos[0])
	}
	if got := repos[0].UsageGB(); got != 1.0 {
		t.Fatalf("expected UsageGB() == 1.0, got %v", got)
	}
}

func TestListRepoStorage_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	if _, err := ListRepoStorage(testClient(t, srv)); err == nil {
		t.Fatal("expected error on 403 response, got nil")
	}
}
