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
	focusTree
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

	cursor  string            // the tree's centre node
	chosen  map[string]string // the parent picked for a node in tree mode
	tree    treeView
	treeSel int // the selected row among tree.selectable() in tree mode

	packs    []Pack
	active   string // the active pack's name
	traySel  int
	dirty    bool // a pack changed since the last install
	picker   *picker
	statusOK bool // status is a note, not an error

	inst *install // the install modal, while open
	help bool     // the ? overlay
}

type keyMsg = tea.KeyPressMsg

// Layout limits from the spec.
const (
	wideWidth = 100 // below this the detail pane hides
	minWidth  = 60
	minHeight = 15
)

func (m *Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }
func (m *Model) wide() bool     { return m.width >= wideWidth }

// New is the TUI over d.
func New(d Deps) *Model {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	in.Prompt = ""
	in.Placeholder = "type to search"
	in.Focus()
	m := &Model{d: d, input: in, width: 120, height: 30, debounce: 150 * time.Millisecond, chosen: map[string]string{}}
	m.input.SetWidth(m.width - 30)
	m.reloadPacks()
	return m
}

// Init starts nothing; the first search waits for typing.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles one message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(10, m.width-30))
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
		m.followSelection()
		return m, nil
	case installLine:
		if m.inst != nil {
			m.inst.lines = append(m.inst.lines, msg.text)
			return m, m.inst.next
		}
	case installDone:
		if m.inst != nil {
			m.inst.done, m.inst.err = true, msg.err
			if msg.err == nil {
				m.dirty = false
			}
			m.reloadPacks()
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

// key handles a key press for whatever has focus.
func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	m.status, m.statusOK = "", false
	k := msg.String()
	if m.inst != nil {
		return m.installKey(k)
	}
	if k == "ctrl+c" {
		return tea.Quit
	}
	if k == "ctrl+r" {
		m.d.Searcher.Recheck()
		return m.queryChanged()
	}
	if m.help {
		if k == "esc" || k == "?" {
			m.help = false
		}
		if k == "q" {
			return tea.Quit
		}
		return nil
	}
	if m.tooSmall() {
		if k == "q" {
			return tea.Quit
		}
		return nil
	}
	if m.picker != nil {
		m.pickerKey(msg)
		return nil
	}
	if m.focus != focusSearch {
		switch k {
		case "a":
			m.addSelected()
			return nil
		case "d":
			m.removeSelected()
			return nil
		case "p":
			m.openPicker(false)
			return nil
		case "i", "b":
			return m.startInstall()
		case "t":
			m.setFocus(focusTray)
			return nil
		case "?":
			m.help = true
			return nil
		}
	}
	if m.focus == focusTree {
		if k == "q" {
			return tea.Quit
		}
		m.treeKey(k)
		return nil
	}
	if m.focus == focusTray {
		m.trayKey(k)
		if k == "q" {
			return tea.Quit
		}
		return nil
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
	case "left", "h":
		m.treeUp()
	case "right", "l":
		m.treeDown()
	case "tab", "enter":
		if m.cursor != "" {
			m.setFocus(focusTree)
			m.treeSel = m.tree.Cursor
		}
	}
	return nil
}

// treeKey handles keys in tree mode.
func (m *Model) treeKey(k string) {
	switch k {
	case "down", "j":
		m.treeMove(1)
	case "up", "k":
		m.treeMove(-1)
	case "left", "h":
		m.treeUp()
	case "right", "l":
		m.treeDown()
	case "enter":
		if i := m.treeSel - m.tree.Cursor - 1; i >= 0 && i < len(m.tree.AlsoIn) {
			m.chosen[m.cursor] = m.tree.AlsoIn[i].ID
			m.setCursor(m.cursor)
		}
	case "esc", "tab":
		m.setFocus(focusResults)
		m.followSelection()
	}
}

// treeUp moves the tree's cursor to its parent.
func (m *Model) treeUp() {
	if n := len(m.tree.Path); n > 0 {
		m.setCursor(m.tree.Path[n-1].ID)
	}
}

// treeDown moves the tree's cursor to its first child.
func (m *Model) treeDown() {
	if len(m.tree.Children) > 0 {
		child := m.tree.Children[0].ID
		m.chosen[child] = m.cursor
		m.setCursor(child)
	}
}

// treeMove steps through the siblings and the 'also in' rows; a sibling
// becomes the cursor, under the same parent.
func (m *Model) treeMove(by int) {
	rows := m.tree.selectable()
	sel := max(0, min(len(rows)-1, m.treeSel+by))
	also := sel > m.tree.Cursor && sel <= m.tree.Cursor+len(m.tree.AlsoIn)
	if also || sel == m.treeSel {
		m.treeSel = sel
		return
	}
	if n := len(m.tree.Path); n > 0 {
		m.chosen[rows[sel].ID] = m.tree.Path[n-1].ID
	}
	m.setCursor(rows[sel].ID)
}

// trayKey handles keys while the tray has focus.
func (m *Model) trayKey(k string) {
	n := 0
	if p := m.activePack(); p != nil {
		n = len(p.Items)
	}
	switch k {
	case "right", "l", "down", "j":
		m.traySel = min(max(0, n-1), m.traySel+1)
	case "left", "h", "up", "k":
		m.traySel = max(0, m.traySel-1)
	case "tab", "/":
		m.setFocus(focusSearch)
	case "esc":
		m.setFocus(focusResults)
	}
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
	sel := max(0, min(len(m.res.Hits)-1, m.sel+by))
	if sel != m.sel {
		m.sel = sel
		m.followSelection()
	}
}

// queryChanged invalidates any search in flight and schedules a new one, or
// clears the results for an empty query.
func (m *Model) queryChanged() tea.Cmd {
	m.seq++
	if strings.TrimSpace(m.input.Value()) == "" {
		m.res, m.sel, m.offset = Results{}, 0, 0
		m.followSelection()
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
	if m.tooSmall() {
		v := tea.NewView(fmt.Sprintf("ipm needs at least %d×%d — enlarge the terminal (q quits)", minWidth, minHeight))
		v.AltScreen = true
		return v
	}
	var b strings.Builder
	w := m.width
	mode := stTeal.Render("● hybrid")
	if m.d.Searcher.Mode() != "hybrid" {
		mode = stMuted.Render("○ local")
	}
	head := stTitle.Render("ipm") + stMuted.Render(" ─ search: ") + m.input.View()
	if crumb := m.breadcrumb(w / 2); crumb != "" {
		head = stTitle.Render("ipm") + "  " + crumb + stMuted.Render("  ─ search: ") + m.input.View()
	}
	b.WriteString(fit(head, w-lipglossWidth(mode)-1) + " " + mode + "\n")
	if n := m.d.Searcher.Notice(); n != "" {
		b.WriteString(fit(stAmber.Render(n), w) + "\n")
	}
	if f := typeFilters[m.filter]; f != "" {
		b.WriteString(fit(stMuted.Render("type: ")+stAmber.Render(f), w) + "\n")
	}
	b.WriteString(stRule.Render(strings.Repeat("─", w)) + "\n")
	rows := max(1, m.height-strings.Count(b.String(), "\n")-4)
	var body []string
	switch {
	case m.inst != nil:
		body = m.installLines(w, rows)
	case m.help:
		body = m.helpLines(w, rows)
	case m.picker != nil:
		body = m.pickerLines(w, rows)
	default:
		body = m.bodyLines(w, rows)
	}
	for _, line := range body {
		b.WriteString(line + "\n")
	}
	for _, line := range m.trayLines(w) {
		b.WriteString(line + "\n")
	}
	status := stMuted.Render(m.countLine())
	switch {
	case m.status != "" && m.statusOK:
		status = stTeal.Render(m.status)
	case m.status != "":
		status = stError.Render(m.status)
	}
	b.WriteString(fit(status, w) + "\n")
	b.WriteString(fit(stMuted.Render("/ search  a add  d remove  p packs  i install  f type  ? help  q quit"), w))
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

// bodyLines is the results and the tree side by side, or the results alone
// in a narrow terminal.
func (m *Model) bodyLines(w, rows int) []string {
	sel := -1
	if m.focus == focusTree {
		sel = m.treeSel
	}
	if !m.wide() {
		if m.focus == focusTree {
			return m.tree.lines(w, rows, sel, true)
		}
		return m.resultLines(w, rows)
	}
	left := w * 45 / 100
	res, det := m.resultLines(left, rows), m.tree.lines(w-left-3, rows, sel, m.focus == focusTree)
	out := make([]string, rows)
	for i := range out {
		out[i] = res[i] + stRule.Render(" │ ") + det[i]
	}
	return out
}

// followSelection centres the tree on the selected result.
func (m *Model) followSelection() {
	if m.sel < len(m.res.Hits) {
		m.setCursor(m.res.Hits[m.sel].ID)
		return
	}
	m.cursor, m.tree = "", treeView{}
}

// setCursor centres the tree on id.
func (m *Model) setCursor(id string) {
	tv, err := buildTree(m.d.Catalog, id, m.chosen)
	if err != nil {
		m.status = err.Error()
	}
	m.cursor, m.tree, m.treeSel = id, tv, tv.Cursor
}

// breadcrumb is the cursor's path, "repo › … › node", cut from the left to w.
func (m *Model) breadcrumb(w int) string {
	if m.cursor == "" {
		return ""
	}
	refs := append(append([]Ref(nil), m.tree.Path...), refOf(m.tree.Node))
	sep := stMuted.Render(" › ")
	parts := make([]string, len(refs))
	for i, r := range refs {
		parts[i] = typeStyle(r.Type).Render(r.Name)
	}
	for len(parts) > 1 && lipglossWidth(strings.Join(parts, sep)) > w {
		parts = parts[1:]
		parts[0] = stMuted.Render("…")
	}
	return strings.Join(parts, sep)
}
