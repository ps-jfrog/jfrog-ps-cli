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

	groups, truncated, err := GetGroups(testClient(t, srv), 0)
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
