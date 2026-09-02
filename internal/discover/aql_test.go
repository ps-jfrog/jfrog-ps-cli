package discover

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetActivity_ThreeQueriesAndLastActivity(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/artifactory/api/search/aql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(string(body), `"$asc":["created"]`):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"created": "2024-01-01T00:00:00.000Z", "created_by": "alice"}},
			})
		case strings.Contains(string(body), `"$desc":["modified"]`):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"modified": "2024-06-01T00:00:00.000Z", "modified_by": "bob"}},
			})
		case strings.Contains(string(body), "stat.downloaded"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{{"stats": []map[string]interface{}{{"downloaded": "2024-03-01T00:00:00.000Z"}}}},
			})
		default:
			t.Fatalf("unrecognized AQL query: %s", body)
		}
	}))
	defer srv.Close()

	c := testClient(t, srv)
	act, err := GetActivity(c, "my-repo")
	if err != nil {
		t.Fatalf("GetActivity returned error: %v", err)
	}
	if len(bodies) != 3 {
		t.Fatalf("expected 3 AQL calls, got %d", len(bodies))
	}
	if act.Created != "2024-01-01T00:00:00.000Z" {
		t.Errorf("unexpected Created: %q", act.Created)
	}
	if act.CreatedBy != "alice" {
		t.Errorf("unexpected CreatedBy: %q", act.CreatedBy)
	}
	if act.LastPush != "2024-06-01T00:00:00.000Z" {
		t.Errorf("unexpected LastPush: %q", act.LastPush)
	}
	if act.LastPushBy != "bob" {
		t.Errorf("unexpected LastPushBy: %q", act.LastPushBy)
	}
	if act.LastPull != "2024-03-01T00:00:00.000Z" {
		t.Errorf("unexpected LastPull: %q", act.LastPull)
	}
	// LastActivity should be the max of push (June) and pull (March).
	if act.LastActivity != act.LastPush {
		t.Errorf("expected LastActivity to be the later LastPush, got %q", act.LastActivity)
	}
}

func TestGetActivity_EmptyResultsLeaveFieldsBlank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": []map[string]interface{}{}})
	}))
	defer srv.Close()

	act, err := GetActivity(testClient(t, srv), "empty-repo")
	if err != nil {
		t.Fatalf("GetActivity returned error: %v", err)
	}
	if act.Created != "" || act.CreatedBy != "" || act.LastPush != "" || act.LastPushBy != "" || act.LastPull != "" || act.LastActivity != "" {
		t.Fatalf("expected all-blank Activity, got %+v", act)
	}
}

func TestEscapeRepoKey(t *testing.T) {
	if got := escapeRepoKey(`weird"repo`); got != `weird\"repo` {
		t.Fatalf("unexpected escape: %q", got)
	}
}

func TestMaxTimestamp(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", "", ""},
		{"2024-01-01T00:00:00Z", "", "2024-01-01T00:00:00Z"},
		{"", "2024-01-01T00:00:00Z", "2024-01-01T00:00:00Z"},
		{"2024-01-01T00:00:00Z", "2024-06-01T00:00:00Z", "2024-06-01T00:00:00Z"},
	}
	for _, tc := range cases {
		if got := maxTimestamp(tc.a, tc.b); got != tc.want {
			t.Errorf("maxTimestamp(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
