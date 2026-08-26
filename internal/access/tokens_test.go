package access

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestTokenExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	soon := now.Add(5 * 24 * time.Hour).Unix()
	far := now.Add(90 * 24 * time.Hour).Unix()
	past := now.Add(-24 * time.Hour).Unix()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/access/api/v1/tokens" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tokens":[
			{"token_id":"t-soon","subject":"u1","expiry":` + strconv.FormatInt(soon, 10) + `,"scope":"applied-permissions/user"},
			{"token_id":"t-far","subject":"u2","expiry":` + strconv.FormatInt(far, 10) + `,"scope":"applied-permissions/user"},
			{"token_id":"t-nonexpiring","subject":"u3","expiry":0,"scope":"applied-permissions/admin"},
			{"token_id":"t-past","subject":"u4","expiry":` + strconv.FormatInt(past, 10) + `,"scope":"applied-permissions/user"}
		]}`))
	})

	got, err := TokenExpiry(c, 30, now)
	if err != nil {
		t.Fatalf("TokenExpiry: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 expiring tokens (soon + past), got %+v", got)
	}
	ids := map[string]bool{}
	for _, tok := range got {
		ids[tok.TokenID] = true
	}
	if !ids["t-soon"] || !ids["t-past"] {
		t.Fatalf("expected t-soon and t-past, got %+v", got)
	}
	if ids["t-far"] || ids["t-nonexpiring"] {
		t.Fatalf("t-far and t-nonexpiring should not be reported, got %+v", got)
	}
}
