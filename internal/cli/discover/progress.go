package discovercmd

import (
	"fmt"
	"os"
	"strings"
)

const progressBarWidth = 30

// progressBar renders a single-line text progress bar to stderr,
// overwriting the previous line via \r - no external dependency, matching
// this whole CLI's zero-dependency policy. label identifies the current
// stage (left-padded so bars for different stages/lengths still line up).
// Call with done==total to finish the bar (emits a trailing newline so
// subsequent output starts on a fresh line); a total <= 0 is a no-op,
// since there's nothing to show a fraction of.
func progressBar(label string, done, total int) {
	if total <= 0 {
		return
	}
	if done > total {
		done = total
	}
	frac := float64(done) / float64(total)
	filled := int(frac * float64(progressBarWidth))
	bar := strings.Repeat("#", filled) + strings.Repeat("-", progressBarWidth-filled)
	fmt.Fprintf(os.Stderr, "\r%-16s [%s] %3d%% (%d/%d)", label, bar, int(frac*100), done, total)
	if done >= total {
		fmt.Fprintln(os.Stderr)
	}
}
