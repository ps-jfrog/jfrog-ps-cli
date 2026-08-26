package instance

import (
	"encoding/json"
	"fmt"
	"io"
)

// WriteJSON writes s to w as indented JSON.
func WriteJSON(w io.Writer, s *Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode instance snapshot: %w", err)
	}
	return nil
}

// ReadJSON reads a Snapshot previously written by WriteJSON.
func ReadJSON(r io.Reader) (*Snapshot, error) {
	var s Snapshot
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode instance snapshot: %w", err)
	}
	return &s, nil
}
