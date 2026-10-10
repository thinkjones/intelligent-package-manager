package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type focus int

const (
	focusSearch focus = iota
	focusResults
	focusDetail
	focusTray
)

// searchTick fires debounce after a keystroke; only the latest one searches.
type searchTick struct{ seq int }

// searchDone carries a search's results back; stale ones are dropped.
type searchDone struct {
	seq int
	res Results
	err error
}

// Model is the TUI's state. It implements tea.Model.
type Model struct {
	d        Deps
	input    textinput.Model
	focus    focus
	filter   int // index into typeFilters
	seq      int // the latest query's sequence number
	res      Results
	sel      int
	offset   int // first result row shown
	status   string
	width    int
	height   int
	debounce time.Duration
}

// New is the TUI over d.
func New(d Deps) *Model {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	in.Prompt = ""
	in.Placeholder = "type to search"
	in.Focus()
	return &Model{d: d, input: in, width: 120, height: 30, debounce: 150 * time.Millisecond}
}

// Init starts nothing; the first search waits for typing.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles one message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case searchTick:
		if msg.seq != m.seq {
			return m, nil
		}
		return m, m.searchCmd(msg.seq)
	case searchDone:
		if msg.seq != m.seq {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.res, m.sel, m.offset = msg.res, 0, 0
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

// key handles a key press for whatever has focus.
func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	m.status = ""
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}
	if m.focus == focusSearch {
		switch k {
		case "down", "tab":
			m.setFocus(focusResults)
			return nil
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			return tea.Batch(cmd, m.queryChanged())
		}
		return cmd
	}
	switch k {
	case "q":
		return tea.Quit
	case "/":
		m.setFocus(focusSearch)
	case "f":
		m.filter = (m.filter + 1) % len(typeFilters)
		return m.queryChanged()
	case "down", "j":
		m.move(1)
	case "up", "k":
		m.move(-1)
	case "tab":
		m.setFocus(focusSearch)
	}
	return nil
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	if f == focusSearch {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m *Model) move(by int) {
	m.sel = max(0, min(len(m.res.Hits)-1, m.sel+by))
}

// queryChanged invalidates any search in flight and schedules a new one, or
// clears the results for an empty query.
func (m *Model) queryChanged() tea.Cmd {
	m.seq++
	if strings.TrimSpace(m.input.Value()) == "" {
		m.res, m.sel, m.offset = Results{}, 0, 0
		return nil
	}
	seq := m.seq
	return tea.Tick(m.debounce, func(time.Time) tea.Msg { return searchTick{seq} })
}

func (m *Model) searchCmd(seq int) tea.Cmd {
	q, f, s := strings.TrimSpace(m.input.Value()), Filter{Type: typeFilters[m.filter]}, m.d.Searcher
	return func() tea.Msg {
		res, err := s.Search(context.Background(), q, f)
		return searchDone{seq: seq, res: res, err: err}
	}
}

// View draws the screen.
func (m *Model) View() tea.View {
	var b strings.Builder
	w := m.width
	mode := stTeal.Render("● hybrid")
	if m.d.Searcher.Mode() != "hybrid" {
		mode = stMuted.Render("○ local")
	}
	head := stTitle.Render("ipm") + stMuted.Render(" ─ search: ") + m.input.View()
	b.WriteString(fit(head, w-lipglossWidth(mode)-1) + " " + mode + "\n")
	if n := m.d.Searcher.Notice(); n != "" {
		b.WriteString(fit(stAmber.Render(n), w) + "\n")
	}
	if f := typeFilters[m.filter]; f != "" {
		b.WriteString(fit(stMuted.Render("type: ")+stAmber.Render(f), w) + "\n")
	}
	b.WriteString(stRule.Render(strings.Repeat("─", w)) + "\n")
	rows := max(1, m.height-strings.Count(b.String(), "\n")-4)
	for _, line := range m.resultLines(w, rows) {
		b.WriteString(line + "\n")
	}
	b.WriteString(stRule.Render(strings.Repeat("─", w)) + "\n")
	status := stMuted.Render(m.countLine())
	if m.status != "" {
		status = stError.Render(m.status)
	}
	b.WriteString(fit(status, w) + "\n")
	b.WriteString(fit(stMuted.Render("/ search  ↑↓ move  f type  ? help  q quit"), w))
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m *Model) countLine() string {
	if len(m.res.Hits) == 0 {
		if strings.TrimSpace(m.input.Value()) == "" {
			return "type to search"
		}
		return "no matches"
	}
	return fmt.Sprintf("%d of %d", len(m.res.Hits), m.res.Found)
}
