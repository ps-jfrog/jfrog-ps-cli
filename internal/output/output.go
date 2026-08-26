// Package output provides small, dependency-free helpers for consistent CLI
// output: tables, JSON dumps, dry-run banners, and confirmation prompts.
package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Table renders rows of equal-length string slices as an aligned, padded
// table to w. header is optional; pass nil to omit it.
func Table(w io.Writer, header []string, rows [][]string) {
	all := rows
	if header != nil {
		all = append([][]string{header}, rows...)
	}
	if len(all) == 0 {
		return
	}
	widths := make([]int, len(all[0]))
	for _, row := range all {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	for rowIdx, row := range all {
		parts := make([]string, len(row))
		for i, cell := range row {
			parts[i] = padRight(cell, widths[i])
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(parts, "  "), " "))
		if rowIdx == 0 && header != nil {
			sep := make([]string, len(row))
			for i := range row {
				sep[i] = strings.Repeat("-", widths[i])
			}
			fmt.Fprintln(w, strings.TrimRight(strings.Join(sep, "  "), " "))
		}
	}
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// JSON pretty-prints v as JSON to w.
func JSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// DryRunBanner prints a standard "[DRY RUN]" notice for an action that would
// otherwise mutate state. Every destructive command in this CLI defaults to
// dry-run mode; callers must pass --dry-run=false to execute for real.
func DryRunBanner(w io.Writer, action string) {
	fmt.Fprintf(w, "[DRY RUN] Would %s. Re-run with --dry-run=false to apply.\n", action)
}

// Confirm prompts the user with a yes/no question on stdin/stdout and returns
// true only if they answer "y" or "yes" (case-insensitive). Used as an extra
// safety gate for the most destructive commands even when --dry-run=false.
func Confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
