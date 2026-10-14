package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
)

// installLine is one line of install output; installDone ends the install.
type (
	installLine struct{ text string }
	installDone struct{ err error }
)

// install is the state of the install modal.
type install struct {
	title   string // "install" or "deploy"
	success string // the line shown when it finishes without error
	lines   []string
	done    bool
	err     error
	scanner *bufio.Scanner
	pipe    *io.PipeReader
	result  chan error
}

// startInstall runs the installer in the background, its output piped into
// the modal line by line.
func (m *Model) startInstall() tea.Cmd {
	return m.stream("install", "✓ built", func(w io.Writer) error {
		return m.d.Builder.Build(context.Background(), w)
	})
}

// stream runs work in the background with its output piped into the modal
// line by line.
func (m *Model) stream(title, success string, work func(io.Writer) error) tea.Cmd {
	pr, pw := io.Pipe()
	in := &install{title: title, success: success, scanner: bufio.NewScanner(pr), pipe: pr, result: make(chan error, 1)}
	in.scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	m.inst = in
	go func() {
		err := work(pw)
		pw.Close()
		in.result <- err
	}()
	return in.next
}

// next reads the next line of output, or, at its end, the install's result.
func (in *install) next() tea.Msg {
	if in.scanner.Scan() {
		return installLine{in.scanner.Text()}
	}
	if err := in.scanner.Err(); err != nil {
		// A line too long to show: keep reading so the install can finish.
		_, _ = io.Copy(io.Discard, in.pipe)
		if res := <-in.result; res != nil {
			return installDone{res}
		}
		return installDone{fmt.Errorf("install output not shown: %v", err)}
	}
	return installDone{<-in.result}
}

// installKey handles keys while the modal is open.
func (m *Model) installKey(k string) tea.Cmd {
	if !m.inst.done {
		m.status = m.inst.title + " is running — wait for it to finish"
		return nil
	}
	switch k {
	case "esc", "enter":
		m.inst = nil
	case "q", "ctrl+c":
		return tea.Quit
	}
	return nil
}

// installLines is the modal in place of the body: the tail of the output
// that fits, then the result.
func (m *Model) installLines(w, rows int) []string {
	in := m.inst
	result := stAmber.Render("running… (esc closes when it finishes)")
	switch {
	case in.done && in.err != nil:
		result = stError.Render("✗ "+in.err.Error()) + stMuted.Render("  · esc close")
	case in.done:
		result = stTeal.Render(in.success) + stMuted.Render("  · esc close")
	}
	head := []string{stTitle.Render("ipm " + in.title), ""}
	room := max(0, rows-len(head)-2)
	shown := in.lines
	if len(shown) > room {
		shown = shown[len(shown)-room:]
	}
	var lines []string
	lines = append(lines, head...)
	for _, l := range shown {
		lines = append(lines, stText.Render(l))
	}
	lines = append(lines, "", result)
	out := make([]string, 0, rows)
	for _, l := range lines {
		if len(out) == rows {
			break
		}
		out = append(out, fit(l, w))
	}
	for len(out) < rows {
		out = append(out, spaces(w))
	}
	return out
}

// helpLines is the ? overlay in place of the body: the two modes' keys side
// by side when there is room, one after the other otherwise.
func (m *Model) helpLines(w, rows int) []string {
	keyTable := func(title string, keys [][2]string, cw int) []string {
		out := []string{fit(stTitle.Render(title), cw), spaces(cw)}
		for _, kh := range keys {
			out = append(out, fit(stAmber.Render(fit(kh[0], 14))+stText.Render(kh[1]), cw))
		}
		return out
	}
	var lines []string
	if m.tab != tabCatalog {
		lines = keyTable("Keys", tabKeys, w)
	} else if w >= 100 {
		half := w / 2
		left, right := keyTable("Search mode", searchKeys, half-2), keyTable("Tree mode", treeKeys, w-half)
		for i := range max(len(left), len(right)) {
			l, r := spaces(half-2), ""
			if i < len(left) {
				l = left[i]
			}
			if i < len(right) {
				r = right[i]
			}
			lines = append(lines, l+"  "+r)
		}
	} else {
		lines = append(keyTable("Search mode", searchKeys, w), "")
		lines = append(lines, keyTable("Tree mode", treeKeys, w)...)
	}
	lines = append(lines, "", stMuted.Render("esc or ? closes this"))
	out := make([]string, 0, rows)
	for _, l := range lines {
		if len(out) == rows {
			break
		}
		out = append(out, fit(l, w))
	}
	for len(out) < rows {
		out = append(out, spaces(w))
	}
	return out
}
