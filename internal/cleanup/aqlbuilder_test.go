package cleanup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestBuildDownloadStaleAQLIsValidJSONShape(t *testing.T) {
	q := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: "my-repo", OlderThan: "3mo"})

	// items.find(<json>).include(...) -- extract the JSON object passed to
	// items.find and confirm it actually parses as JSON, catching quoting bugs.
	start := strings.Index(q, "items.find(")
	if start == -1 {
		t.Fatalf("expected query to start with items.find(: %q", q)
	}
	body := q[start+len("items.find(") : strings.LastIndex(q, ").include(")]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("items.find() body is not valid JSON: %v\nbody=%s", err, body)
	}
	if parsed["repo"] == nil {
		t.Fatalf("expected repo clause, got %v", parsed)
	}
	if _, ok := parsed["name"]; ok {
		t.Fatalf("expected no name clause when NameEquals is empty, got %v", parsed)
	}
}

func TestBuildDownloadStaleAQLNameEquals(t *testing.T) {
	q := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: "docker-local", OlderThan: "3mo", NameEquals: "manifest.json"})
	if !strings.Contains(q, `"name":{"$eq":"manifest.json"}`) {
		t.Fatalf("expected name clause for manifest.json, got %q", q)
	}
}

func TestBuildDownloadStaleAQLExcludeManagedRootPaths(t *testing.T) {
	withExclude := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: "r", OlderThan: "6mo", ExcludeManagedRootPaths: true})
	withoutExclude := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: "r", OlderThan: "6mo"})
	if !strings.Contains(withExclude, `$nmatch`) {
		t.Fatalf("expected $nmatch root-path exclusion clause, got %q", withExclude)
	}
	if strings.Contains(withoutExclude, `$nmatch`) {
		t.Fatalf("expected no root-path exclusion clause by default, got %q", withoutExclude)
	}
}

// TestBuildDownloadStaleAQLAgainstServer exercises the built query end to end
// through FindArtifacts against a mock AQL endpoint, confirming the generated
// query string is accepted as the literal POST body.
func TestBuildDownloadStaleAQLAgainstServer(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"repo":"my-repo","path":"a","name":"old.txt","size":123}]}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	query := BuildDownloadStaleAQL(DownloadStaleQuery{Repo: "my-repo", OlderThan: "3mo"})
	matches, err := FindArtifacts(client, query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 || matches[0].Name != "old.txt" {
		t.Fatalf("unexpected matches: %+v", matches)
	}
	if !strings.Contains(gotBody, "my-repo") {
		t.Fatalf("expected request body to contain repo name, got %q", gotBody)
	}
}

func TestTotalSize(t *testing.T) {
	matches := []ArtifactMatch{{Size: 100}, {Size: 250}, {Size: 0}}
	if got := TotalSize(matches); got != 350 {
		t.Fatalf("expected 350, got %d", got)
	}
	if got := TotalSize(nil); got != 0 {
		t.Fatalf("expected 0 for nil, got %d", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1024 * 1024, "1.00 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
	}
	for _, tc := range cases {
		if got := FormatBytes(tc.in); got != tc.want {
			t.Fatalf("FormatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
