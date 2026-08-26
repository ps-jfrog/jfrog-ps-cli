package analyze

import (
	"strings"
	"testing"
)

func TestParseCSV_HeaderAndRows(t *testing.T) {
	input := "repo_name,files_count,usage_gb\n" +
		"docker-local,5,1.5\n" +
		"npm-local,0,0\n"

	header, rows, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	if len(header) != 3 {
		t.Fatalf("expected 3 header columns, got %d: %v", len(header), header)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Get("repo_name") != "docker-local" {
		t.Fatalf("unexpected repo_name: %q", rows[0].Get("repo_name"))
	}
	if rows[0].Int("files_count") != 5 {
		t.Fatalf("expected files_count 5, got %d", rows[0].Int("files_count"))
	}
	if rows[0].Float("usage_gb") != 1.5 {
		t.Fatalf("expected usage_gb 1.5, got %v", rows[0].Float("usage_gb"))
	}
}

func TestParseCSV_MissingHeaderErrors(t *testing.T) {
	if _, _, err := ParseCSV(strings.NewReader("")); err == nil {
		t.Fatal("expected error parsing an empty CSV, got nil")
	}
}

func TestRow_MissingOrUnparsableFieldsDefaultToZeroValue(t *testing.T) {
	header, rows, err := ParseCSV(strings.NewReader("repo_name,files_count\nbad-row,notanumber\n"))
	if err != nil {
		t.Fatalf("ParseCSV returned error: %v", err)
	}
	_ = header
	if got := rows[0].Int("files_count"); got != 0 {
		t.Fatalf("expected 0 for unparsable int, got %d", got)
	}
	if got := rows[0].Get("nonexistent_column"); got != "" {
		t.Fatalf("expected empty string for missing column, got %q", got)
	}
}
