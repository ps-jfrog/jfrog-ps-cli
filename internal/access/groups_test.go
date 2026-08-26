package access

import (
	"net/http"
	"testing"
)

func TestFindEmptyGroups(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/artifactory/api/security/groups":
			w.Write([]byte(`[{"name":"g-empty","uri":"/g-empty"},{"name":"g-full","uri":"/g-full"}]`))
		case "/artifactory/api/security/groups/g-empty":
			if r.URL.RawQuery != "includeUsers=true" {
				t.Fatalf("expected includeUsers=true, got %q", r.URL.RawQuery)
			}
			w.Write([]byte(`{"name":"g-empty","userNames":[]}`))
		case "/artifactory/api/security/groups/g-full":
			w.Write([]byte(`{"name":"g-full","userNames":["alice"]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	got, err := FindEmptyGroups(c)
	if err != nil {
		t.Fatalf("FindEmptyGroups: %v", err)
	}
	if len(got) != 1 || got[0] != "g-empty" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestGetGroupDetail_NullUserNames(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"g","userNames":null}`))
	})
	d, err := GetGroupDetail(c, "g")
	if err != nil {
		t.Fatalf("GetGroupDetail: %v", err)
	}
	if len(d.UserNames) != 0 {
		t.Fatalf("expected empty user list, got %+v", d.UserNames)
	}
}
