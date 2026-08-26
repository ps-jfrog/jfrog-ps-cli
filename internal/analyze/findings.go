package analyze

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var legalKey = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Options tunes the thresholds findings are flagged against.
type Options struct {
	StaleDays     int
	HugeGB        float64
	MassiveFiles  int
	LongNameChars int
}

func DefaultOptions() Options {
	return Options{StaleDays: 180, HugeGB: 500, MassiveFiles: 500_000, LongNameChars: 40}
}

// FindingDef describes one finding tag for rendering: its label, the
// explanation shown when a viewer clicks its card, and a CSS tone.
type FindingDef struct {
	ID      string
	Label   string
	Explain string
	Tone    string
}

// Defs returns every finding definition this Options would compute,
// annotated with whether its required column was present in the report.
func (o Options) Defs(header []string) []FindingDef {
	hasPolicy := hasColumn(header, "cleanup_policy_keys")
	hasActivity := hasColumn(header, "last_activity_date")

	defs := []FindingDef{
		{"empty", "Empty repos", "Zero files recorded in the report - the safest leave-behind or deletion candidates once an owner confirms no pending need.", "teal"},
		{"naming-nonconformant", "Naming needs normalization", "Key isn't lowercase-hyphen ([a-z][a-z0-9-]*) - legalize before scripting or migrating against it.", "purple"},
		{"uppercase", "Uppercase in key", "Contains uppercase letters, inconsistent with a lowercase-hyphen convention.", "purple"},
		{"underscore", "Underscore in key", "Contains '_', inconsistent with a lowercase-hyphen convention.", "purple"},
		{"very-long-name", fmt.Sprintf("Name over %d chars", o.LongNameChars), "Unusually long repo key - check for accidental duplication or naming-convention drift.", "info"},
		{"huge", fmt.Sprintf("Over %.0f GB used", o.HugeGB), "Needs a dedicated transfer/cleanup window and tuned batching regardless of package type.", "red"},
		{"massive-files", fmt.Sprintf("Over %s files", humanInt(o.MassiveFiles)), "A high file count slows transfers, backups, and UI browsing out of proportion to its GB size.", "red"},
	}
	if hasActivity {
		defs = append(defs, FindingDef{"stale", fmt.Sprintf("No activity in %dd+", o.StaleDays),
			"Non-empty repo with no push or pull inside the window (or no activity recorded despite having files). A candidate for archival or a stricter cleanup policy.", "amber"})
	}
	if hasPolicy {
		defs = append(defs,
			FindingDef{"no-cleanup-policy", "No cleanup policy attached", "No package cleanup policy's repos/excludedRepos/packageTypes/project scope matches this repo - storage here grows unmanaged.", "red"},
			FindingDef{"multi-policy-match", "Matches multiple cleanup policies", "More than one policy's scope overlaps this repo. Verify that's intentional - overlapping policies can produce surprising retention behavior.", "purple"},
		)
	}
	return defs
}

// ComputeFindings tags every row with the generic hygiene/governance signals
// this Options describes, skipping any finding whose source column is
// absent from header rather than mis-flagging it.
func ComputeFindings(header []string, rows []Row, opts Options, now time.Time) {
	hasPolicy := hasColumn(header, "cleanup_policy_keys")
	hasActivity := hasColumn(header, "last_activity_date")

	for i := range rows {
		r := &rows[i]
		name := r.Get("repo_name")
		var findings []string

		if strings.ContainsAny(name, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			findings = append(findings, "uppercase")
		}
		if strings.Contains(name, "_") {
			findings = append(findings, "underscore")
		}
		if !legalKey.MatchString(name) {
			findings = append(findings, "naming-nonconformant")
		}
		if len(name) > opts.LongNameChars {
			findings = append(findings, "very-long-name")
		}

		filesCount := r.Int("files_count")
		if filesCount == 0 {
			findings = append(findings, "empty")
		}
		if r.Float("usage_gb") > opts.HugeGB {
			findings = append(findings, "huge")
		}
		if filesCount > opts.MassiveFiles {
			findings = append(findings, "massive-files")
		}

		if hasActivity && filesCount > 0 && isStale(r.Get("last_activity_date"), opts.StaleDays, now) {
			findings = append(findings, "stale")
		}
		if hasPolicy {
			keys := r.Get("cleanup_policy_keys")
			if keys == "" {
				findings = append(findings, "no-cleanup-policy")
			} else if strings.Contains(keys, "|") {
				findings = append(findings, "multi-policy-match")
			}
		}

		r.Findings = findings
	}
}

// isStale reports whether last is empty (no recorded activity despite
// having files) or older than staleDays.
func isStale(last string, staleDays int, now time.Time) bool {
	if last == "" {
		return true
	}
	t, ok := parseTimestamp(last)
	if !ok {
		return false
	}
	return now.Sub(t) > time.Duration(staleDays)*24*time.Hour
}

func parseTimestamp(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// humanInt renders large counts with thousands separators, e.g. 500000 -> "500,000".
func humanInt(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, d := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, d)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
