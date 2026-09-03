package compare

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func buildsListHandler(names []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := buildsListResponse{}
		for _, n := range names {
			resp.Builds = append(resp.Builds, struct {
				URI         string `json:"uri"`
				LastStarted string `json:"lastStarted"`
			}{URI: "/" + n, LastStarted: "2024-01-01T00:00:00.000Z"})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func buildNumbersHandler(numbers []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := buildNumbersResponse{}
		for _, n := range numbers {
			resp.BuildsNumbers = append(resp.BuildsNumbers, struct {
				URI     string `json:"uri"`
				Started string `json:"started"`
			}{URI: "/" + n, Started: "2024-01-01T00:00:00.000Z"})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TestListBuildNames(t *testing.T) {
	srv := httptest.NewServer(buildsListHandler([]string{"build-a", "build-b"}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	names, err := ListBuildNames(client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 || names[0] != "build-a" || names[1] != "build-b" {
		t.Fatalf("unexpected names: %v", names)
	}
}

func TestListBuildNumbers(t *testing.T) {
	srv := httptest.NewServer(buildNumbersHandler([]string{"1", "2", "3"}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	numbers, err := ListBuildNumbers(client, "build-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(numbers) != 3 {
		t.Fatalf("unexpected numbers: %v", numbers)
	}
}

// buildsMockServer serves both /artifactory/api/build and
// /artifactory/api/build/{name} from in-memory fixtures, and fails the test
// if any other path (in particular PUT /artifactory/api/build) is hit,
// unless allowPut is set.
type buildsMockServer struct {
	names         []string
	numbersByName map[string][]string
	buildInfo     map[string]map[string]interface{} // "name/number" -> buildInfo
	allowPut      bool
	putBodies     []buildInfoEnvelope
	t             *testing.T
}

func (m *buildsMockServer) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/artifactory/api/build":
		buildsListHandler(m.names)(w, r)
	case r.Method == http.MethodGet && len(r.URL.Path) > len("/artifactory/api/build/"):
		rest := r.URL.Path[len("/artifactory/api/build/"):]
		if numbers, ok := m.numbersByName[rest]; ok {
			buildNumbersHandler(numbers)(w, r)
			return
		}
		// Might be a /{name}/{number} buildinfo fetch.
		if info, ok := m.buildInfo[rest]; ok {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(buildInfoEnvelope{BuildInfo: info})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodPut && r.URL.Path == "/artifactory/api/build":
		if !m.allowPut {
			m.t.Fatalf("unexpected PUT to /artifactory/api/build -- migration must not mutate in dry-run")
			return
		}
		var envelope buildInfoEnvelope
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			m.t.Fatalf("decode PUT body: %v", err)
		}
		m.putBodies = append(m.putBodies, envelope)
		w.WriteHeader(http.StatusOK)
	default:
		m.t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}
}

func TestCompareBuildsMissingNamesAndNumbers(t *testing.T) {
	source := &buildsMockServer{
		t:     t,
		names: []string{"build-a", "build-b"},
		numbersByName: map[string][]string{
			"build-a": {"1", "2", "3"},
			"build-b": {"1"},
		},
	}
	target := &buildsMockServer{
		t:     t,
		names: []string{"build-a"},
		numbersByName: map[string][]string{
			"build-a": {"1"},
		},
	}
	sourceSrv := httptest.NewServer(http.HandlerFunc(source.handler))
	defer sourceSrv.Close()
	targetSrv := httptest.NewServer(http.HandlerFunc(target.handler))
	defer targetSrv.Close()

	sourceClient := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	targetClient := jfclient.New(targetSrv.URL, "tok", "", "", false)

	diff, err := CompareBuilds(sourceClient, targetClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diff.MissingBuildNames) != 1 || diff.MissingBuildNames[0] != "build-b" {
		t.Fatalf("expected build-b missing, got %v", diff.MissingBuildNames)
	}
	if len(diff.SharedBuildNames) != 1 || diff.SharedBuildNames[0] != "build-a" {
		t.Fatalf("expected build-a shared, got %v", diff.SharedBuildNames)
	}
	missing := diff.MissingNumbers["build-a"]
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing numbers for build-a, got %v", missing)
	}
}

func TestMigrateBuildPairsDryRunNeverPuts(t *testing.T) {
	source := &buildsMockServer{
		t: t,
		buildInfo: map[string]map[string]interface{}{
			"build-a/2": {"name": "build-a", "number": "2", "uri": "/build-a/2"},
		},
	}
	target := &buildsMockServer{t: t, allowPut: false}
	sourceSrv := httptest.NewServer(http.HandlerFunc(source.handler))
	defer sourceSrv.Close()
	targetSrv := httptest.NewServer(http.HandlerFunc(target.handler))
	defer targetSrv.Close()

	sourceClient := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	targetClient := jfclient.New(targetSrv.URL, "tok", "", "", false)

	pairs := []BuildPair{{Name: "build-a", Number: "2"}}
	results := MigrateBuildPairs(sourceClient, targetClient, pairs, true)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].DryRun || results[0].Success {
		t.Fatalf("expected dry-run result with Success=false, got %+v", results[0])
	}
}

func TestMigrateBuildPairsRealRunStripsURIAndPuts(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	source := &buildsMockServer{
		t: t,
		buildInfo: map[string]map[string]interface{}{
			"build-a/2": {"name": "build-a", "number": "2", "uri": "/build-a/2"},
		},
	}
	target := &buildsMockServer{t: t, allowPut: true}
	sourceSrv := httptest.NewServer(http.HandlerFunc(source.handler))
	defer sourceSrv.Close()
	targetSrv := httptest.NewServer(http.HandlerFunc(target.handler))
	defer targetSrv.Close()

	sourceClient := jfclient.New(sourceSrv.URL, "tok", "", "", false)
	targetClient := jfclient.New(targetSrv.URL, "tok", "", "", false)

	pairs := []BuildPair{{Name: "build-a", Number: "2"}}
	results := MigrateBuildPairs(sourceClient, targetClient, pairs, false)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].DryRun {
		t.Fatalf("expected DryRun=false, got %+v", results[0])
	}
	if !results[0].Success {
		t.Fatalf("expected Success=true, got %+v", results[0])
	}
	if len(target.putBodies) != 1 {
		t.Fatalf("expected 1 PUT body recorded, got %d", len(target.putBodies))
	}
	if _, ok := target.putBodies[0].BuildInfo["uri"]; ok {
		t.Fatalf("expected uri field stripped from build-info before PUT, got %+v", target.putBodies[0].BuildInfo)
	}
	if target.putBodies[0].BuildInfo["name"] != "build-a" {
		t.Fatalf("expected name field preserved, got %+v", target.putBodies[0].BuildInfo)
	}
}

func TestMissingPairsIncludesFullyMissingBuildNumbers(t *testing.T) {
	source := &buildsMockServer{
		t:     t,
		names: []string{"build-a", "build-b"},
		numbersByName: map[string][]string{
			"build-a": {"1"},
			"build-b": {"1", "2"},
		},
	}
	sourceSrv := httptest.NewServer(http.HandlerFunc(source.handler))
	defer sourceSrv.Close()
	sourceClient := jfclient.New(sourceSrv.URL, "tok", "", "", false)

	diff := BuildDiff{
		MissingBuildNames: []string{"build-b"},
		SharedBuildNames:  []string{"build-a"},
		MissingNumbers:    map[string][]string{"build-a": {"1"}},
	}

	pairs, err := MissingPairs(sourceClient, diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("expected 3 pairs (build-b/1, build-b/2, build-a/1), got %+v", pairs)
	}
}
