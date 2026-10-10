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
	pr, pw := io.Pipe()
	in := &install{scanner: bufio.NewScanner(pr), pipe: pr, result: make(chan error, 1)}
	in.scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	m.inst = in
	go func() {
		err := m.d.Installer.Install(context.Background(), pw)
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
		io.Copy(io.Discard, in.pipe)
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
		m.status = "install is running — wait for it to finish"
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
		result = stTeal.Render("✓ installed") + stMuted.Render("  · esc close")
	}
	head := []string{stTitle.Render("ipm install"), ""}
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

// helpLines is the ? overlay in place of the body.
func (m *Model) helpLines(w, rows int) []string {
	lines := []string{stTitle.Render("Keys"), ""}
	for _, kh := range keyHelp {
		lines = append(lines, stAmber.Render(fit(kh[0], 12))+stText.Render(kh[1]))
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
