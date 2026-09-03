package replication

import (
	"encoding/json"
	"net/http"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestDisableDryRunMakesNoHTTPCalls(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run should not issue any HTTP calls, got %s %s", r.Method, r.URL.Path)
	})
	defer srv.Close()

	results := Disable(c, []string{"repo1"}, true)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected dry-run success marker, got %+v", results)
	}
}

func TestDisableSingleObjectShape(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var putBody map[string]interface{}
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"url": "https://t/artifactory/repo1", "enabled": true})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			w.WriteHeader(http.StatusOK)
		}
	})
	defer srv.Close()

	results := Disable(c, []string{"repo1"}, false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected success, got %+v", results)
	}
	if putBody["enabled"] != false {
		t.Fatalf("expected enabled=false in PUT body, got %+v", putBody)
	}
}

func TestDisableArrayShape(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var putBody []map[string]interface{}
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"url": "https://t1/artifactory/repo1", "enabled": true},
				{"url": "https://t2/artifactory/repo1", "enabled": true},
			})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			w.WriteHeader(http.StatusOK)
		}
	})
	defer srv.Close()

	results := Disable(c, []string{"repo1"}, false)
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("expected success, got %+v", results)
	}
	if len(putBody) != 2 {
		t.Fatalf("expected 2 entries in PUT body, got %d", len(putBody))
	}
	for _, entry := range putBody {
		if entry["enabled"] != false {
			t.Fatalf("expected enabled=false on every entry, got %+v", entry)
		}
	}
}

func TestDisableReportsGetFailure(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	results := Disable(c, []string{"repo1"}, false)
	if len(results) != 1 || results[0].Success || results[0].Err == nil {
		t.Fatalf("expected failure result, got %+v", results)
	}
}
