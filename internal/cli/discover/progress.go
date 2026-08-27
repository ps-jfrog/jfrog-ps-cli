package discovercmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

const progressBarWidth = 30

// multiBar renders one live-updating text progress bar per label to
// stderr, all stacked in a single block that's erased and redrawn in
// place as any of them progress - so independently-progressing pieces
// (e.g. --repos and --instance running concurrently, or --instance's own
// Permissions/Project detail/Federated repos stages) can each show their
// own bar without garbling one another. Any plain log line emitted while
// bars are on screen goes through Log/Logf, which temporarily erases the
// block, prints the line, then redraws the block below it - so the bars
// stay pinned at the bottom instead of getting overwritten mid-line.
//
// No external dependency (hand-rolled ANSI cursor movement), consistent
// with this CLI's zero-dependency policy. Falls back to plain,
// non-overwriting lines when stderr isn't a terminal (piped/redirected -
// e.g. into a log file), since cursor-movement escapes would otherwise
// corrupt that output.
type multiBar struct {
	mu    sync.Mutex
	tty   bool
	order []string
	bars  map[string]barState
	drawn int // lines currently on screen for the block, so it can be erased before redrawing
}

type barState struct {
	done, total int
}

func newMultiBar() *multiBar {
	return &multiBar{tty: stderrIsTerminal(), bars: make(map[string]barState)}
}

// stderrIsTerminal reports whether stderr looks like an interactive
// terminal rather than a pipe or redirected file, using only the
// standard library (no external dependency).
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Update sets label's progress and redraws the bar block. A total <= 0 is
// a no-op - nothing to show a fraction of, and it lets a stage that turns
// out to have nothing to do (e.g. zero federated repos) simply never grow
// a line. Safe to call concurrently from multiple goroutines/pieces.
func (m *multiBar) Update(label string, done, total int) {
	if total <= 0 {
		return
	}
	if done > total {
		done = total
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.tty {
		if done >= total {
			fmt.Fprintf(os.Stderr, "%s: %d/%d\n", label, done, total)
		}
		return
	}

	if _, ok := m.bars[label]; !ok {
		m.order = append(m.order, label)
	}
	m.bars[label] = barState{done, total}
	m.erase()
	m.drawBlock()
}

// Log prints a plain log line (like fmt.Fprintln(os.Stderr, args...)),
// temporarily erasing the bar block first and redrawing it below the line
// so the two never overlap on screen.
func (m *multiBar) Log(args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.tty {
		fmt.Fprintln(os.Stderr, args...)
		return
	}
	m.erase()
	fmt.Fprintln(os.Stderr, args...)
	m.drawBlock()
}

// Logf prints a plain formatted log line (like fmt.Fprintf(os.Stderr,
// format, args...) - format should include its own trailing "\n"),
// erasing and redrawing the bar block the same way Log does.
func (m *multiBar) Logf(format string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.tty {
		fmt.Fprintf(os.Stderr, format, args...)
		return
	}
	m.erase()
	fmt.Fprintf(os.Stderr, format, args...)
	m.drawBlock()
}

// erase clears the currently-drawn bar block (if any) and leaves the
// cursor where the block used to start, ready for new content. Callers
// must hold m.mu.
func (m *multiBar) erase() {
	if m.drawn == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\x1b[%dA\x1b[J", m.drawn)
	m.drawn = 0
}

// drawBlock renders every known bar, one per line, in first-seen order.
// Callers must hold m.mu.
func (m *multiBar) drawBlock() {
	for _, label := range m.order {
		s := m.bars[label]
		frac := float64(s.done) / float64(s.total)
		filled := int(frac * float64(progressBarWidth))
		bar := strings.Repeat("#", filled) + strings.Repeat("-", progressBarWidth-filled)
		fmt.Fprintf(os.Stderr, "%-16s [%s] %3d%% (%d/%d)\n", label, bar, int(frac*100), s.done, s.total)
	}
	m.drawn = len(m.order)
}
