package cli

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// progress is ipm onboard's one redrawn line: a bar across the pipeline's
// steps, the step under way, the last thing it said and the time so far. It
// is the steps' Stdout, so their output moves the line instead of scrolling,
// and is kept in case the step fails. A nil progress does nothing.
type progress struct {
	mu     sync.Mutex
	w      io.Writer
	width  int
	now    func() time.Time
	start  time.Time
	stop   chan struct{}
	closed bool

	steps, step int
	name, note  string
	done, total int          // how far the step has got; total 0 is unknown
	buf         bytes.Buffer // the step's output
	part        string       // its last line, while unfinished
}

// newProgress starts the line on sys.Stdout, or returns nil when stdout is
// not a terminal.
func newProgress(sys System, steps int) *progress {
	if sys.TermWidth == nil || sys.TermWidth() == 0 {
		return nil
	}
	p := &progress{w: sys.Stdout, width: sys.TermWidth(), now: sys.Now, steps: steps, stop: make(chan struct{})}
	p.start = p.now()
	// pnpm says nothing while it works; the clock shows ipm hasn't hung.
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-tick.C:
				p.mu.Lock()
				p.draw()
				p.mu.Unlock()
			}
		}
	}()
	return p
}

// begin moves to step n (from 1) and forgets the last step's output.
func (p *progress) begin(n int, name string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.step, p.name, p.note, p.part, p.done, p.total = n, name, "", "", 0, 0
	p.buf.Reset()
	p.draw()
}

// count reports that the step has done done of total, what being the latest.
// It is safe to call from several goroutines.
func (p *progress) count(done, total int, what string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done, p.total = done, total
	p.note = fmt.Sprintf("%d/%d %s", done, total, what)
	p.draw()
}

// Write takes the step's output: each finished line becomes the note.
func (p *progress) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf.Write(b)
	lines := strings.Split(p.part+string(b), "\n")
	p.part = lines[len(lines)-1]
	if l := lastLine(strings.Join(lines[:len(lines)-1], "\n")); l != "" && l != p.note {
		p.note = l
		p.draw()
	}
	return len(b), nil
}

// print writes s above the line.
func (p *progress) print(s string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clear()
	io.WriteString(p.w, s)
	p.draw()
}

// summary is the last line the step wrote, as " · <line>". The steps end
// with their totals, then a dash and where they wrote them; the totals are
// enough here.
func (p *progress) summary() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	l, _, _ := strings.Cut(lastLine(p.buf.String()), " — ")
	if l == "" {
		return ""
	}
	return " · " + l
}

// output is everything the step wrote.
func (p *progress) output() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.String()
}

// close removes the line for good; print still works afterwards.
func (p *progress) close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.clear()
	p.closed = true
	if p.stop != nil {
		close(p.stop)
	}
}

func (p *progress) clear() {
	if !p.closed {
		io.WriteString(p.w, "\r\x1b[K")
	}
}

func (p *progress) draw() {
	if !p.closed {
		io.WriteString(p.w, "\r"+p.line()+"\x1b[K")
	}
}

// line is the bar, the step, its note and the time, cut to fit one row.
func (p *progress) line() string {
	cells := 20
	if p.width < 50 {
		cells = 10
	}
	at := float64(p.step - 1)
	if p.total > 0 {
		at += float64(p.done) / float64(p.total)
	}
	full := max(0, min(cells, int(at/float64(p.steps)*float64(cells))))
	head := fmt.Sprintf("[%s%s] %d/%d %s", strings.Repeat("█", full), strings.Repeat("░", cells-full), p.step, p.steps, p.name)
	spent := p.now().Sub(p.start).Round(time.Second)
	tail := fmt.Sprintf(" · %d:%02d", int(spent.Minutes()), int(spent.Seconds())%60)
	// one column short of the width, so the cursor never wraps
	room := p.width - 1 - ansi.StringWidth(head) - ansi.StringWidth(tail) - len(" · ")
	if p.note == "" || room < 2 {
		return head + tail
	}
	return head + " · " + ansi.Truncate(p.note, room, "…") + tail
}

// lastLine is the last line of s that isn't blank, without its indent.
func lastLine(s string) string {
	for _, l := range slices.Backward(strings.Split(s, "\n")) {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
