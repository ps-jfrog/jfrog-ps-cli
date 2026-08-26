package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestTableAlignsColumns(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []string{"NAME", "TYPE"}, [][]string{
		{"libs-release-local", "local"},
		{"x", "remote"},
	})
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 { // header, separator, 2 rows
		t.Fatalf("expected 4 lines, got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "NAME") {
		t.Fatalf("expected header row first, got %q", lines[0])
	}
}

func TestTableNoHeader(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, nil, [][]string{{"a", "b"}})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line with no header, got %d", len(lines))
	}
}

func TestJSONEncoding(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, map[string]string{"a": "b"}); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"a": "b"`) {
		t.Fatalf("expected pretty JSON, got %q", buf.String())
	}
}

func TestDryRunBanner(t *testing.T) {
	var buf bytes.Buffer
	DryRunBanner(&buf, "delete 3 repos")
	if !strings.Contains(buf.String(), "[DRY RUN]") || !strings.Contains(buf.String(), "delete 3 repos") {
		t.Fatalf("unexpected banner: %q", buf.String())
	}
}
