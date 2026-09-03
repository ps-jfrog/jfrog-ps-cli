package cleanup

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jfrogpscli/internal/jfclient"
)

func TestFindArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/artifactory/api/search/aql" {
			t.Fatalf("expected AQL endpoint, got %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "my-repo") {
			t.Fatalf("expected query to contain repo name, got %q", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[
			{"repo":"my-repo","path":"a/b","name":"file1.txt","size":100,"created":"t1","modified":"t2","created_by":"u1","modified_by":"u2"},
			{"repo":"my-repo","path":".","name":"file2.txt","size":200}
		]}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches, err := FindArtifacts(client, `items.find({"repo":"my-repo"})`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Repo != "my-repo" || matches[0].Path != "a/b" || matches[0].Name != "file1.txt" || matches[0].Size != 100 {
		t.Fatalf("unexpected match[0]: %+v", matches[0])
	}
}

func TestFindArtifactsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	if _, err := FindArtifacts(client, `items.find({})`); err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
}

func TestArtifactPath(t *testing.T) {
	cases := []struct {
		name string
		m    ArtifactMatch
		want string
	}{
		{"with path", ArtifactMatch{Repo: "r", Path: "a/b", Name: "n.txt"}, "r/a/b/n.txt"},
		{"empty path", ArtifactMatch{Repo: "r", Path: "", Name: "n.txt"}, "r/n.txt"},
		{"root path dot", ArtifactMatch{Repo: "r", Path: ".", Name: "n.txt"}, "r/n.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ArtifactPath(tc.m); got != tc.want {
				t.Fatalf("ArtifactPath(%+v) = %q, want %q", tc.m, got, tc.want)
			}
		})
	}
}

func TestDeleteArtifactsDryRunNeverCallsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			t.Fatalf("DELETE must never be called when dryRun=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches := []ArtifactMatch{
		{Repo: "r", Path: "a", Name: "n1.txt"},
		{Repo: "r", Path: "", Name: "n2.txt"},
	}
	results := DeleteArtifacts(client, matches, true)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if !res.DryRun {
			t.Fatalf("expected DryRun=true, got %+v", res)
		}
		if res.Success {
			t.Fatalf("expected Success=false in dry-run, got %+v", res)
		}
	}
}

func TestDeleteArtifactsRealDeleteSuccessAndFailure(t *testing.T) {
	jfclient.WritesDisabled = false
	t.Cleanup(func() { jfclient.WritesDisabled = true })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if strings.Contains(r.URL.Path, "missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := jfclient.New(srv.URL, "tok", "", "", false)
	matches := []ArtifactMatch{
		{Repo: "r", Path: "a", Name: "ok.txt"},
		{Repo: "r", Path: "a", Name: "missing.txt"},
	}
	results := DeleteArtifacts(client, matches, false)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Success || results[0].Error != "" {
		t.Fatalf("expected first delete to succeed, got %+v", results[0])
	}
	if results[0].Path != "r/a/ok.txt" {
		t.Fatalf("unexpected path: %q", results[0].Path)
	}
	if results[1].Success || results[1].Error == "" {
		t.Fatalf("expected second delete to fail with error set, got %+v", results[1])
	}
}

func TestWriteArtifactsCSV(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "out.csv")
	matches := []ArtifactMatch{
		{Repo: "r", Path: "a/b", Name: "n.txt", Size: 42, Created: "c1", Modified: "m1", CreatedBy: "u1", ModifiedBy: "u2"},
	}
	if err := WriteArtifactsCSV(csvPath, matches); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "repo,path,name,size") {
		t.Fatalf("expected header row, got %q", content)
	}
	if !strings.Contains(content, "r,a/b,n.txt,42,c1,m1,u1,u2") {
		t.Fatalf("expected data row, got %q", content)
	}
}
