package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetGroups_BasicPagination(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		call++
		if call == 1 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"groups": []map[string]string{{"group_name": "team-a"}, {"group_name": "team-b"}},
				"cursor": "team-b",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"groups": []map[string]string{}})
	}))
	defer srv.Close()

	groups, truncated, err := GetGroups(testClient(t, srv), 0, "7.49.3")
	if err != nil {
		t.Fatalf("GetGroups returned error: %v", err)
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
	if len(groups) != 2 || groups[1].Name != "team-b" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestGetGroups_LowVersionGoesStraightToLegacy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/api/v2/groups":
			t.Fatalf("should not probe %s on an instance below 7.49.3", r.URL.Path)
		case "/artifactory/api/security/groups":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"name": "team-a"}, {"name": "team-b"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	groups, truncated, err := GetGroups(testClient(t, srv), 0, "7.31.13")
	if err != nil {
		t.Fatalf("GetGroups returned error: %v", err)
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
	if len(groups) != 2 || groups[1].Name != "team-b" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}

func TestGetGroups_FallsBackToLegacyOnUnexpected404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/access/api/v2/groups":
			w.WriteHeader(http.StatusNotFound)
		case "/artifactory/api/security/groups":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]string{
				{"name": "team-a"}, {"name": "team-b"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	groups, truncated, err := GetGroups(testClient(t, srv), 0, "7.49.3")
	if err != nil {
		t.Fatalf("GetGroups returned error: %v", err)
	}
	if truncated {
		t.Fatal("expected truncated=false")
	}
	if len(groups) != 2 || groups[1].Name != "team-b" {
		t.Fatalf("unexpected groups: %+v", groups)
	}
}
