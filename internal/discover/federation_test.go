package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetFederatedRepos_OnlyFetchesFederatedType(t *testing.T) {
	var fetched []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetched = append(fetched, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []map[string]interface{}{{"url": "https://other/artifactory/repo", "enabled": true}},
		})
	}))
	defer srv.Close()

	repos := []RepoSummary{
		{RepoKey: "fed-repo", RepoType: "FEDERATED"},
		{RepoKey: "local-repo", RepoType: "LOCAL"},
		{RepoKey: "remote-repo", RepoType: "REMOTE"},
	}
	federated, warnings := GetFederatedRepos(testClient(t, srv), repos, 2)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(fetched) != 1 || fetched[0] != "/artifactory/api/repositories/fed-repo" {
		t.Fatalf("expected exactly one fetch for the federated repo, got %v", fetched)
	}
	if len(federated) != 1 || federated[0].RepoKey != "fed-repo" || len(federated[0].Members) != 1 {
		t.Fatalf("unexpected federated result: %+v", federated)
	}
}

func TestGetFederatedRepos_NoFederatedReposReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("no HTTP call should be made when there are no federated repos")
	}))
	defer srv.Close()

	federated, warnings := GetFederatedRepos(testClient(t, srv), []RepoSummary{{RepoKey: "local-repo", RepoType: "LOCAL"}}, 2)
	if federated != nil || warnings != nil {
		t.Fatalf("expected nil/nil, got %+v / %+v", federated, warnings)
	}
}

func TestGetFederatedRepos_OneFailureDoesNotAbortOthers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/artifactory/api/repositories/broken" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"members": []map[string]interface{}{}})
	}))
	defer srv.Close()

	repos := []RepoSummary{
		{RepoKey: "ok", RepoType: "FEDERATED"},
		{RepoKey: "broken", RepoType: "FEDERATED"},
	}
	federated, warnings := GetFederatedRepos(testClient(t, srv), repos, 2)
	if len(federated) != 1 || federated[0].RepoKey != "ok" {
		t.Fatalf("expected only the successful repo, got %+v", federated)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", warnings)
	}
}
