package repos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestIntersectKeys(t *testing.T) {
	source := []Repo{{Key: "b"}, {Key: "a"}, {Key: "c"}}
	target := []Repo{{Key: "a"}, {Key: "c"}, {Key: "d"}}

	common := IntersectKeys(source, target)
	if len(common) != 2 || common[0] != "a" || common[1] != "c" {
		t.Fatalf("expected sorted [a c], got %v", common)
	}
}

func TestConfigsEqual_IgnoresFieldOrder(t *testing.T) {
	a := map[string]interface{}{"key": "x", "rclass": "local", "n": float64(1)}
	// Decoding JSON with fields in a different order produces the same map.
	var b map[string]interface{}
	_ = json.Unmarshal([]byte(`{"n":1,"rclass":"local","key":"x"}`), &b)

	if !ConfigsEqual(a, b) {
		t.Fatalf("expected configs with reordered fields to be equal: %+v vs %+v", a, b)
	}
}

func TestConfigsEqual_DetectsDifference(t *testing.T) {
	a := map[string]interface{}{"key": "x", "rclass": "local"}
	b := map[string]interface{}{"key": "x", "rclass": "remote"}
	if ConfigsEqual(a, b) {
		t.Fatal("expected differing configs to not be equal")
	}
}

func repoConfigHandler(configs map[string]map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path[len("/artifactory/api/repositories/"):]
		cfg, ok := configs[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	}
}

func TestSyncConfigDiff_DetectsDifferingRepos(t *testing.T) {
	// Each server must serve both the repo list and per-key config
	// endpoints, so we use a combined handler.
	source := httptest.NewServer(combinedHandler(
		[]Repo{{Key: "a"}, {Key: "b"}},
		map[string]map[string]interface{}{
			"a": {"key": "a", "rclass": "local"},
			"b": {"key": "b", "rclass": "local", "description": "same"},
		},
	))
	defer source.Close()
	target := httptest.NewServer(combinedHandler(
		[]Repo{{Key: "a"}, {Key: "b"}},
		map[string]map[string]interface{}{
			"a": {"key": "a", "rclass": "remote"}, // differs
			"b": {"key": "b", "rclass": "local", "description": "same"},
		},
	))
	defer target.Close()

	result, err := SyncConfigDiff(testClient(t, source), testClient(t, target), "all")
	if err != nil {
		t.Fatalf("SyncConfigDiff returned error: %v", err)
	}
	if len(result.Common) != 2 {
		t.Fatalf("expected 2 common repos, got %v", result.Common)
	}
	if len(result.Differing) != 1 || result.Differing[0] != "a" {
		t.Fatalf("expected only 'a' to differ, got %v", result.Differing)
	}
}

func combinedHandler(list []Repo, configs map[string]map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/artifactory/api/repositories" {
			reposHandler(list)(w, r)
			return
		}
		repoConfigHandler(configs)(w, r)
	}
}

func TestApplyConfigSync_DryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Fatalf("dry-run must not issue POST, got %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := ApplyConfigSync(c, c, []string{"a", "b"}, true)
	if results != nil {
		t.Fatalf("expected nil results for dry-run, got %+v", results)
	}
}

func TestApplyConfigSync_PushesSourceConfig(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	source := httptest.NewServer(repoConfigHandler(map[string]map[string]interface{}{
		"a": {"key": "a", "rclass": "local"},
	}))
	defer source.Close()

	var postedBody map[string]interface{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&postedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	results := ApplyConfigSync(testClient(t, source), testClient(t, target), []string{"a"}, false)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
	if postedBody["rclass"] != "local" {
		t.Fatalf("expected source config to be posted, got %+v", postedBody)
	}
}
