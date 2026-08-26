// Package analyze turns a `jf-audit report` CSV into flagged findings and a
// self-contained interactive HTML page.
package analyze

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Row is one parsed report row, keyed by CSV header column name (so the
// analyzer adapts to whichever columns the report was run with - e.g.
// --cleanup-policies=false drops the policy columns entirely), plus findings
// computed by ComputeFindings.
type Row struct {
	Fields   map[string]string
	Findings []string
}

// ParseCSV reads a `jf-audit report` CSV and returns its header (column
// order, for rendering) and parsed rows.
func ParseCSV(r io.Reader) ([]string, []Row, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("reading header: %w", err)
	}

	var rows []Row
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading row: %w", err)
		}
		fields := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				fields[col] = record[i]
			}
		}
		rows = append(rows, Row{Fields: fields})
	}
	return header, rows, nil
}

func (r Row) Get(key string) string { return r.Fields[key] }

func (r Row) Int(key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(r.Fields[key]))
	return v
}

func (r Row) Float(key string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(r.Fields[key]), 64)
	return v
}

func hasColumn(header []string, name string) bool {
	for _, h := range header {
		if h == name {
			return true
		}
	}
	return false
}
