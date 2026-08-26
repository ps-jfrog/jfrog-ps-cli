package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetReplications(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/replications" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"repoKey": "libs-release-local", "replicationType": "PUSH", "enabled": true, "cronExp": "0 0 12 * * ?", "url": "https://target/artifactory/libs-release-local", "syncDeletes": true},
		})
	}))
	defer srv.Close()

	replications, err := GetReplications(testClient(t, srv))
	if err != nil {
		t.Fatalf("GetReplications returned error: %v", err)
	}
	if len(replications) != 1 || replications[0].RepoKey != "libs-release-local" || !replications[0].SyncDeletes {
		t.Fatalf("unexpected replications: %+v", replications)
	}
}
