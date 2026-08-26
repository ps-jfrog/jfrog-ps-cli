package discover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetUsers_PagesUntilEmpty(t *testing.T) {
	pages := [][]map[string]string{
		{{"username": "alice", "realm": "internal"}, {"username": "bob", "realm": "internal"}},
		{},
	}
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := pages[call]
		call++
		cursor := ""
		if len(page) > 0 {
			cursor = "next-cursor"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": page, "cursor": cursor})
	}))
	defer srv.Close()

	users, truncated, err := GetUsers(testClient(t, srv), 0)
	if err != nil {
		t.Fatalf("GetUsers returned error: %v", err)
	}
	if truncated {
		t.Fatal("expected truncated=false when no cap is hit")
	}
	if len(users) != 2 || users[0].Username != "alice" {
		t.Fatalf("unexpected users: %+v", users)
	}
	if call != 2 {
		t.Fatalf("expected 2 page fetches (data then empty), got %d", call)
	}
}

func TestGetUsers_RespectsMaxAndReportsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"users":  []map[string]string{{"username": "a"}, {"username": "b"}, {"username": "c"}},
			"cursor": "c",
		})
	}))
	defer srv.Close()

	users, truncated, err := GetUsers(testClient(t, srv), 2)
	if err != nil {
		t.Fatalf("GetUsers returned error: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncated=true when cap is hit")
	}
	if len(users) != 2 {
		t.Fatalf("expected exactly 2 users (the cap), got %d", len(users))
	}
}

func TestGetUsers_StopsOnRepeatingCursor(t *testing.T) {
	// A misbehaving server that returns the same cursor forever must not
	// cause an infinite loop.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"users":  []map[string]string{{"username": "a"}},
			"cursor": "same-cursor",
		})
	}))
	defer srv.Close()

	users, _, err := GetUsers(testClient(t, srv), 0)
	if err != nil {
		t.Fatalf("GetUsers returned error: %v", err)
	}
	// First page returns cursor "same-cursor" (!= initial "" so it advances
	// once), second page also returns "same-cursor" which now matches the
	// previous cursor, so the loop must stop there.
	if len(users) != 2 {
		t.Fatalf("expected exactly 2 users before the repeating-cursor guard stops the loop, got %d", len(users))
	}
}
