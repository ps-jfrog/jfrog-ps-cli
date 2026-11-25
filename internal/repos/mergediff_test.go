package repos

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"jfrogpscli/internal/jfclient"
)

const conflictsCSV = `Type,SourceName,TargetName,DifferentProperties
Project,Magic(magic),Magic(magic),storage_quota_bytes
Repository,b-local,b-local,xrayIndex
Repository,a-local,a-local,disableProxy; xrayDataTtl
Repository,b-local,b-local,xrayIndex
`

func TestParseConflictsCSV(t *testing.T) {
	keys, err := ParseConflictsCSV(strings.NewReader(conflictsCSV))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "a-local" || keys[1] != "b-local" {
		t.Fatalf("unexpected keys: %v", keys)
	}
	if _, err := ParseConflictsCSV(strings.NewReader("a,b\n1,2\n")); err == nil {
		t.Fatal("expected header error")
	}
}

func TestDiffConfigs(t *testing.T) {
	src := map[string]interface{}{"xrayIndex": true, "url": "x", "description": "a", "only": 1, "same": "s", "list": []interface{}{"a"}}
	tgt := map[string]interface{}{"xrayIndex": false, "url": "y", "description": "b", "same": "s", "list": []interface{}{"b"}}
	src["members"], tgt["members"] = []interface{}{"a"}, []interface{}{"b"}
	diffs := DiffConfigs(src, tgt)
	if len(diffs) != 2 || diffs[0].Property != "list" || diffs[1].Property != "xrayIndex" {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}
}

// fakeArtifactory serves GET/POST /artifactory/api/repositories/{key} from
// an in-memory map, applying POST bodies as partial updates.
func fakeArtifactory(t *testing.T, cfgs map[string]map[string]interface{}, posts *[]string) *httptest.Server {
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/artifactory/api/v2/repositories/"), "/artifactory/api/repositories/")
		cfg, ok := cfgs[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if r.Header.Get("Accept") == "application/json" {
				http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
				return
			}
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			for k, v := range body {
				cfg[k] = v
			}
			*posts = append(*posts, key)
			return
		}
		_ = json.NewEncoder(w).Encode(cfg)
	}))
}

func TestResolveMergeDiff_EndToEnd(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	var srcPosts, tgtPosts []string
	src := fakeArtifactory(t, map[string]map[string]interface{}{
		"a": {"xrayIndex": true, "url": "src"}, "b": {"xrayIndex": true},
	}, &srcPosts)
	defer src.Close()
	tgt := fakeArtifactory(t, map[string]map[string]interface{}{
		"a": {"xrayIndex": false, "url": "tgt"}, "b": {"xrayIndex": true},
	}, &tgtPosts)
	defer tgt.Close()
	sc, tc := testClient(t, src), testClient(t, tgt)

	keys := []string{"a", "b", "missing"}
	before := DiffRepos(sc, tc, keys)
	if len(before[0].Diffs) != 1 || len(before[1].Diffs) != 0 || before[2].Err == nil {
		t.Fatalf("unexpected before: %+v", before)
	}

	if res := ApplyMergeDiffs(tc, before, true); res != nil || len(tgtPosts) != 0 {
		t.Fatal("dry-run must not write")
	}
	res := ApplyMergeDiffs(tc, before, false)
	if len(res) != 1 || res[0].Err != nil || len(tgtPosts) != 1 || tgtPosts[0] != "a" {
		t.Fatalf("unexpected apply: %+v posts=%v", res, tgtPosts)
	}

	after := DiffRepos(sc, tc, []string{"a", "b"})
	if len(Pending(after)) != 0 {
		t.Fatalf("expected all resolved: %+v", after)
	}
	// url (ignored key) must not have been overwritten.
	cfg, _ := GetRepoConfig(tc, "a")
	if cfg["url"] != "tgt" {
		t.Fatalf("target url was modified: %v", cfg["url"])
	}

	var sb strings.Builder
	if err := WriteDiffReport(&sb, before); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, "a,xrayIndex,true,false,differs") || !strings.Contains(out, "b,,,,match") || !strings.Contains(out, "missing,,,,error:") {
		t.Fatalf("unexpected report:\n%s", out)
	}
}

func TestGetRelevantRepoConfig_FallsBackToV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/artifactory/api/v2/") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"key":"k","xrayIndex":true}`))
	}))
	defer srv.Close()
	cfg, err := GetRelevantRepoConfig(testClient(t, srv), "k")
	if err != nil || cfg["xrayIndex"] != true {
		t.Fatalf("unexpected: %v %v", cfg, err)
	}
}

func TestGetRelevantRepoConfig_PrefersV2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/artifactory/api/v2/") {
			_, _ = w.Write([]byte(`{"key":"k","type":"local"}`))
			return
		}
		t.Error("v1 must not be called when v2 succeeds")
	}))
	defer srv.Close()
	cfg, err := GetRelevantRepoConfig(testClient(t, srv), "k")
	if err != nil || cfg["type"] != "local" {
		t.Fatalf("unexpected: %v %v", cfg, err)
	}
}
