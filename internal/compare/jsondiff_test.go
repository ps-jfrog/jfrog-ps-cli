package compare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareJSONObjects(t *testing.T) {
	first := map[string]interface{}{
		"name":    "svc-a",
		"version": float64(1),
		"tags":    []interface{}{"a", "b"},
		"nested":  map[string]interface{}{"x": float64(1)},
		"same":    "unchanged",
	}
	second := map[string]interface{}{
		"name":    "svc-a",
		"version": float64(2),
		"tags":    []interface{}{"a", "b"},
		"nested":  map[string]interface{}{"x": float64(2)},
		"same":    "unchanged",
		"extra":   "new-field",
	}

	diff := CompareJSONObjects(first, second)

	if diff.Empty() {
		t.Fatalf("expected differences, got none")
	}
	if len(diff.Added) != 1 || diff.Added[0].Key != "extra" {
		t.Fatalf("expected 1 added key 'extra', got %+v", diff.Added)
	}
	if len(diff.Removed) != 0 {
		t.Fatalf("expected 0 removed keys, got %+v", diff.Removed)
	}
	if len(diff.Changed) != 2 {
		t.Fatalf("expected 2 changed keys (version, nested), got %+v", diff.Changed)
	}
	changedKeys := map[string]bool{}
	for _, c := range diff.Changed {
		changedKeys[c.Key] = true
	}
	if !changedKeys["version"] || !changedKeys["nested"] {
		t.Fatalf("expected version and nested to be changed, got %+v", diff.Changed)
	}
}

func TestCompareJSONObjectsRemoved(t *testing.T) {
	first := map[string]interface{}{"a": "1", "b": "2"}
	second := map[string]interface{}{"a": "1"}

	diff := CompareJSONObjects(first, second)
	if len(diff.Removed) != 1 || diff.Removed[0].Key != "b" {
		t.Fatalf("expected 'b' removed, got %+v", diff.Removed)
	}
	if len(diff.Added) != 0 || len(diff.Changed) != 0 {
		t.Fatalf("expected no added/changed, got added=%+v changed=%+v", diff.Added, diff.Changed)
	}
}

func TestCompareJSONObjectsIdentical(t *testing.T) {
	first := map[string]interface{}{"a": "1", "nested": map[string]interface{}{"x": float64(1)}}
	second := map[string]interface{}{"a": "1", "nested": map[string]interface{}{"x": float64(1)}}

	diff := CompareJSONObjects(first, second)
	if !diff.Empty() {
		t.Fatalf("expected no differences, got %+v", diff)
	}
}

func TestCompareJSONFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "one.json")
	file2 := filepath.Join(dir, "two.json")

	writeFile(t, file1, `{"a": 1, "b": 2}`)
	writeFile(t, file2, `{"a": 1, "c": 3}`)

	diff, err := CompareJSONFiles(file1, file2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Key != "b" {
		t.Fatalf("expected 'b' removed, got %+v", diff.Removed)
	}
	if len(diff.Added) != 1 || diff.Added[0].Key != "c" {
		t.Fatalf("expected 'c' added, got %+v", diff.Added)
	}
}

func TestCompareJSONFilesMissingFile(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "one.json")
	writeFile(t, file1, `{"a": 1}`)

	if _, err := CompareJSONFiles(file1, filepath.Join(dir, "nope.json")); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestCompareJSONFilesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "one.json")
	file2 := filepath.Join(dir, "two.json")
	writeFile(t, file1, `{"a": 1}`)
	writeFile(t, file2, `not json`)

	if _, err := CompareJSONFiles(file1, file2); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
