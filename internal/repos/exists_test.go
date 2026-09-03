package repos

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckExist_ClassifiesPresentAndMissing(t *testing.T) {
	srv := httptest.NewServer(reposHandler([]Repo{
		{Key: "repo-a", Type: "local"},
		{Key: "repo-b", Type: "local"},
	}))
	defer srv.Close()

	results, err := CheckExist(testClient(t, srv), []string{"repo-a", "repo-c"}, "local")
	if err != nil {
		t.Fatalf("CheckExist returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Key != "repo-a" || !results[0].Exists {
		t.Fatalf("expected repo-a to exist, got %+v", results[0])
	}
	if results[1].Key != "repo-c" || results[1].Exists {
		t.Fatalf("expected repo-c to be missing, got %+v", results[1])
	}
}

func TestCheckExist_PropagatesListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := CheckExist(testClient(t, srv), []string{"repo-a"}, "local"); err == nil {
		t.Fatal("expected an error when the repository list call fails")
	}
}
