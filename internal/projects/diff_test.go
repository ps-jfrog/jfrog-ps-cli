package projects

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMissingOnTarget(t *testing.T) {
	source := []Project{{ProjectKey: "a"}, {ProjectKey: "b"}, {ProjectKey: "c"}}
	target := []Project{{ProjectKey: "b"}}

	missing := MissingOnTarget(source, target)
	if len(missing) != 2 || missing[0].ProjectKey != "a" || missing[1].ProjectKey != "c" {
		t.Fatalf("unexpected missing set: %+v", missing)
	}
}

func TestExtraOnTarget(t *testing.T) {
	source := []Project{{ProjectKey: "a"}}
	target := []Project{{ProjectKey: "a"}, {ProjectKey: "x"}}

	extra := ExtraOnTarget(source, target)
	if len(extra) != 1 || extra[0].ProjectKey != "x" {
		t.Fatalf("unexpected extra set: %+v", extra)
	}
}

func projectsHandler(list []Project) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	}
}

func TestDiff_EndToEnd(t *testing.T) {
	source := httptest.NewServer(projectsHandler([]Project{{ProjectKey: "a"}, {ProjectKey: "b"}}))
	defer source.Close()
	target := httptest.NewServer(projectsHandler([]Project{{ProjectKey: "a"}, {ProjectKey: "x"}}))
	defer target.Close()

	result, err := Diff(testClient(t, source), testClient(t, target))
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if len(result.MissingOnTarget) != 1 || result.MissingOnTarget[0].ProjectKey != "b" {
		t.Fatalf("unexpected missing: %+v", result.MissingOnTarget)
	}
	if len(result.ExtraOnTarget) != 1 || result.ExtraOnTarget[0].ProjectKey != "x" {
		t.Fatalf("unexpected extra: %+v", result.ExtraOnTarget)
	}
}

func TestCreateMissing_DryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not issue any HTTP call, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := CreateMissing(c, c, []Project{{ProjectKey: "a"}}, true)
	if results != nil {
		t.Fatalf("expected nil results for dry-run, got %+v", results)
	}
}

func TestCreateMissing_CreatesOnTarget(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/access/api/v1/projects/newproj" {
			t.Fatalf("unexpected path on source: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"project_key":  "newproj",
			"display_name": "New Project",
		})
	}))
	defer source.Close()

	var postedBody map[string]interface{}
	var postedPath string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		postedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&postedBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()

	results := CreateMissing(testClient(t, source), testClient(t, target), []Project{{ProjectKey: "newproj"}}, false)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
	if postedPath != "/access/api/v1/projects" {
		t.Fatalf("expected POST to /access/api/v1/projects, got %s", postedPath)
	}
	if postedBody["project_key"] != "newproj" {
		t.Fatalf("expected source config to be posted, got %+v", postedBody)
	}
}

func TestDeleteExtra_DryRunMakesNoCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dry-run must not issue any HTTP call, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	results := DeleteExtra(c, []Project{{ProjectKey: "x"}}, true)
	if results != nil {
		t.Fatalf("expected nil results for dry-run, got %+v", results)
	}
}

func TestDeleteExtra_DeletesFromTarget(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/access/api/v1/projects/x" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	results := DeleteExtra(testClient(t, target), []Project{{ProjectKey: "x"}}, false)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
}
