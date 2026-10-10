package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

	cursor    string            // the tree's centre node
	chosen    map[string]string // the parent picked for a node in tree mode
	tree      treeView
	treeSel   int      // the selected row among tree.selectable() in tree mode, or backRow
	backStack []string // external entries jumped from, newest last

	packs    []Pack
	active   string // the active pack's name
	traySel  int
	dirty    bool // a pack changed since the last install
	picker   *picker
	statusOK bool // status is a note, not an error

	inst   *install     // the install or deploy output modal, while open
	deploy *deployModal // the deploy modal before it runs
	help   bool         // the ? overlay
	plain  bool         // NO_COLOR: draw without colour
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
	m.plain = os.Getenv("NO_COLOR") != ""
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
	if m.deploy != nil {
		return m.deployKey(msg)
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
		case "D":
			m.openDeploy()
			return nil
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
		if m.treeSel == backRow {
			m.back()
			return
		}
		m.treeDown()
	case "enter":
		if m.treeSel == backRow {
			m.back()
			return
		}
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

// treeDown moves the tree's cursor to its first child, or, on an external
// entry, into the repo it points at.
func (m *Model) treeDown() {
	switch {
	case m.tree.Node.External != nil:
		m.jump()
		return
	case m.tree.Node.Unresolved != "":
		m.status = m.tree.Node.Unresolved + " isn't one of your sources — ipm source add " + m.cursor
		return
	}
	if len(m.tree.Children) > 0 {
		child := m.tree.Children[0].ID
		m.chosen[child] = m.cursor
		m.setCursor(child)
	}
}

// treeMove steps through the siblings and the 'also in' rows; a sibling
// becomes the cursor, under the same parent. Above the first sibling is the
// back root, when there is one.
func (m *Model) treeMove(by int) {
	switch {
	case m.treeSel == backRow && by > 0:
		m.treeSel = m.tree.Cursor
		return
	case m.treeSel == backRow:
		return
	case m.treeSel == 0 && by < 0 && m.tree.Back != nil:
		m.treeSel = backRow
		return
	}
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

// View draws the screen: a header with the cursor's breadcrumb, the results
// and tree panels (or a modal), the pack tray, the status line and the keys
// for the current mode.
func (m *Model) View() tea.View {
	content := m.render()
	if m.plain {
		content = ansi.Strip(content)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *Model) render() string {
	if m.tooSmall() {
		return fmt.Sprintf("ipm needs at least %d×%d — enlarge the terminal (q quits)", minWidth, minHeight)
	}
	w := m.width
	var top []string
	mode := stTeal.Render("● hybrid")
	if m.d.Searcher.Mode() != "hybrid" {
		mode = stMuted.Render("○ local")
	}
	head := stTitle.Render("ipm") + "  " + m.breadcrumb(w-lipglossWidth(mode)-8)
	top = append(top, fit(head, w-lipglossWidth(mode)-1)+" "+mode)
	if n := m.d.Searcher.Notice(); n != "" {
		top = append(top, fit(stAmber.Render(n), w))
	}
	tray := m.trayLines(w)
	body := max(6, m.height-len(top)-len(tray)-2)

	var mid []string
	switch {
	case m.inst != nil:
		mid = panel(stTitle.Render(m.inst.title), m.installLines(w-4, body-2), w, true)
	case m.deploy != nil:
		mid = panel(stTitle.Render("deploy"), m.deployLines(w-4, body-2), w, true)
	case m.help:
		mid = panel(stTitle.Render("keys"), m.helpLines(w-4, body-2), w, true)
	case m.picker != nil:
		mid = panel(stTitle.Render("packs"), m.pickerLines(w-4, body-2), w, true)
	default:
		resH := max(5, body*40/100)
		m.input.SetWidth(min(w-24, max(lipglossWidth(m.input.Placeholder), lipglossWidth(m.input.Value())+1)))
		search := stMuted.Render("search: ") + m.input.View()
		if f := typeFilters[m.filter]; f != "" {
			search += stMuted.Render("  type: ") + stAmber.Render(f)
		}
		resultsFocused := m.focus == focusSearch || m.focus == focusResults
		mid = panel(search, m.resultLines(w-4, resH-2), w, resultsFocused)
		sel := -1
		if m.focus == focusTree {
			sel = m.treeSel
		}
		title := stMuted.Render("tree")
		if m.cursor != "" {
			title += stMuted.Render(" ── ") + stText.Render(m.cursor)
		}
		mid = append(mid, panel(title, m.tree.lines(w-4, body-resH-2, sel, m.focus == focusTree), w, m.focus == focusTree)...)
	}

	status := stMuted.Render(m.countLine())
	switch {
	case m.status != "" && m.statusOK:
		status = stTeal.Render(m.status)
	case m.status != "":
		status = stError.Render(m.status)
	}
	lines := append(append(append(top, mid...), tray...), fit(status, w), fit(stMuted.Render(m.hints()), w))
	return strings.Join(lines, "\n")
}

// hints is the footer: the keys that matter where the focus is.
func (m *Model) hints() string {
	switch {
	case m.inst != nil:
		return "esc close (when finished)"
	case m.deploy != nil && m.deploy.step == stepConfirm:
		return "y deploy  esc cancel"
	case m.deploy != nil:
		return "enter next  esc cancel"
	case m.help:
		return "esc or ? close"
	case m.picker != nil:
		return "enter choose  n new pack  esc cancel"
	case m.focus == focusSearch:
		return "type to search  ↓ results  ctrl+c quit"
	case m.focus == focusTree:
		return "↑↓ siblings  ←→ parent/child  enter follow  a add  d remove  tab/esc back  ? help"
	case m.focus == focusTray:
		return "←→ items  d remove  esc back"
	}
	return "↑↓ results  ←→ tree  tab tree mode  a add  t tray  p packs  b build  D deploy  ? help  q quit"
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

// followSelection centres the tree on the selected result, forgetting jumps.
func (m *Model) followSelection() {
	m.backStack = nil
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
	if n := len(m.backStack); n > 0 {
		if e, err := m.d.Catalog.Node(m.backStack[n-1]); err == nil {
			back := refOf(e)
			tv.Back = &back
		}
	}
	m.cursor, m.tree, m.treeSel = id, tv, tv.Cursor
}

// maxBack is how many jumps the back stack remembers.
const maxBack = 20

// jump moves from an external entry to what it references, in its own repo,
// remembering the entry so back can return.
func (m *Model) jump() {
	target := m.tree.Node.External.ID
	m.backStack = append(m.backStack, m.cursor)
	if len(m.backStack) > maxBack {
		m.backStack = m.backStack[len(m.backStack)-maxBack:]
	}
	m.setCursor(target)
}

// back returns to the external entry the last jump left.
func (m *Model) back() {
	n := len(m.backStack)
	if n == 0 {
		return
	}
	entry := m.backStack[n-1]
	m.backStack = m.backStack[:n-1]
	m.setCursor(entry)
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
