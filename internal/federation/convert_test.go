package federation

import (
	"net/http"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestConvertToFederatedDryRunMakesNoHTTPCalls(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run should not issue any HTTP calls, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()

	results := ConvertToFederated(c, []string{"repo1", "repo2"}, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected dry-run success marker, got %+v", r)
		}
	}
}

func TestConvertToFederatedAppliesPOST(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var gotMethod, gotPath string
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	results := ConvertToFederated(c, []string{"repo1"}, false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected success, got %+v", results)
	}
	if gotMethod != http.MethodPost || gotPath != "/artifactory/api/federation/migrate/repo1" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
}

func TestConvertToFederatedReportsFailure(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	results := ConvertToFederated(c, []string{"repo1"}, false)
	if len(results) != 1 || results[0].Success || results[0].Err == nil {
		t.Fatalf("expected failure result, got %+v", results)
	}
}
