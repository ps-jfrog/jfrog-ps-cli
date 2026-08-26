package compare

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
)

// JSONChange describes one key that differs between two JSON documents.
type JSONChange struct {
	Key      string
	OldValue interface{} // present for "removed" and "changed"
	NewValue interface{} // present for "added" and "changed"
}

// JSONDiff is the result of comparing two JSON objects: keys only in the
// first document ("removed" relative to the second), keys only in the
// second ("added"), and keys in both with different values ("changed").
type JSONDiff struct {
	Added   []JSONChange
	Removed []JSONChange
	Changed []JSONChange
}

// Empty reports whether the two documents had no differences at all.
func (d JSONDiff) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// CompareJSONObjects compares two decoded top-level JSON objects and reports
// added/removed/changed keys. Value comparison uses reflect.DeepEqual, which
// correctly handles the nested maps/slices produced by json.Unmarshal into
// map[string]interface{}.
func CompareJSONObjects(first, second map[string]interface{}) JSONDiff {
	var diff JSONDiff

	for key, oldVal := range first {
		newVal, ok := second[key]
		if !ok {
			diff.Removed = append(diff.Removed, JSONChange{Key: key, OldValue: oldVal})
			continue
		}
		if !reflect.DeepEqual(oldVal, newVal) {
			diff.Changed = append(diff.Changed, JSONChange{Key: key, OldValue: oldVal, NewValue: newVal})
		}
	}
	for key, newVal := range second {
		if _, ok := first[key]; !ok {
			diff.Added = append(diff.Added, JSONChange{Key: key, NewValue: newVal})
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].Key < diff.Added[j].Key })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].Key < diff.Removed[j].Key })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].Key < diff.Changed[j].Key })

	return diff
}

// LoadJSONObject reads and unmarshals the JSON object at path into a
// map[string]interface{}.
func LoadJSONObject(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse %s as JSON object: %w", path, err)
	}
	return out, nil
}

// CompareJSONFiles reads and compares two JSON object files. file1 plays the
// role of "first" (its unique keys are "removed") and file2 the role of
// "second" (its unique keys are "added"), matching CompareJSONObjects.
func CompareJSONFiles(file1, file2 string) (JSONDiff, error) {
	first, err := LoadJSONObject(file1)
	if err != nil {
		return JSONDiff{}, err
	}
	second, err := LoadJSONObject(file2)
	if err != nil {
		return JSONDiff{}, err
	}
	return CompareJSONObjects(first, second), nil
}
