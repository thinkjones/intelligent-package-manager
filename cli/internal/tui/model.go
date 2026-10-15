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
	tab      tab
	counts   [tabCount]int // the tab bar's counts, notLoaded until a tab reads its data
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

	catCounts map[string]int    // items per type, read once at start
	cursor    string            // the tree's centre node
	chosen    map[string]string // the parent picked for a node in tree mode
	tree      treeView
	treeSel   int      // the selected row among tree.selectable() in tree mode, or backRow
	backStack []string // external entries jumped from, newest last

	pk       packsTab    // the Packs tab
	ins      installsTab // the Installs tab
	mk       marketTab   // the Marketplace tab
	packs    []Pack
	active   string // the active pack's name
	traySel  int
	dirty    bool // a pack changed since the last install
	picker   *picker
	statusOK bool // status is a note, not an error

	inst  *install     // the build or install output modal, while open
	form  *installForm // the install form before it runs
	ext   *extModal    // the e modal: external references
	help  bool         // the ? overlay
	plain bool         // NO_COLOR: draw without colour
}

type keyMsg = tea.KeyPressMsg

// Layout limits from the spec.
const (
	wideWidth = 100 // below this the detail pane hides
	minWidth  = 60
	minHeight = 15
)

func (m *Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

// New is the TUI over d.
func New(d Deps) *Model {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	in.Prompt = ""
	in.Placeholder = "press / to search"
	m := &Model{d: d, input: in, width: 120, height: 30, debounce: 150 * time.Millisecond, chosen: map[string]string{}, focus: focusResults}
	m.pk.filter = newFilterBox()
	m.ins.filter = newFilterBox()
	m.mk.filter = newFilterBox()
	m.catCounts = d.Catalog.Counts()
	for i := range m.counts {
		m.counts[i] = notLoaded
	}
	m.counts[tabCatalog] = sumCounts(m.catCounts)
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
			m.status, m.statusOK = "", false
			if msg.err == nil && m.inst.kind != runSync {
				m.dirty = false
			}
			m.reloadPacks()
			if m.inst.kind == runInstall {
				m.loadInstallsTab()
			}
			if m.inst.kind == runSync && msg.err == nil {
				m.note(syncedMsg)
				return m, m.loadMarket()
			}
		}
	case marketLoaded:
		m.marketReady(msg)
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

// key handles a key press for whatever has focus.
func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if m.inst != nil {
		// A run's own status, set when it finished, outlives the key that closes it.
		return m.installKey(k)
	}
	m.status, m.statusOK = "", false
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
	if m.form != nil {
		return m.formKey(msg)
	}
	if m.ext != nil {
		m.extKey(k)
		return nil
	}
	if m.picker != nil {
		m.pickerKey(msg)
		return nil
	}
	if !m.textFocused() && m.tabKey(k) {
		return m.enterTab()
	}
	switch m.tab {
	case tabCatalog:
		return m.catalogKey(msg)
	case tabPacks:
		return m.packsKey(msg)
	case tabInstalls:
		return m.installsKey(msg)
	}
	return m.marketKey(msg)
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	if f == focusSearch {
		m.input.Focus()
	} else {
		m.input.Blur()
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

// View draws the screen: the title line, tab bar and stats strip over the
// active tab's body (or a modal), then the status line and the tab's keys.
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
	top := []string{m.titleLine(w)}
	if n := m.d.Searcher.Notice(); n != "" {
		top = append(top, fit(stAmber.Render(n), w))
	}
	top = append(top, m.tabBar(w)...)
	top = append(top, m.statsStrip(w))
	var tray []string
	if m.tab == tabCatalog {
		tray = m.trayLines(w)
	}
	body := max(6, m.height-len(top)-len(tray)-2)

	var mid []string
	switch {
	case m.inst != nil:
		mid = panel(stTitle.Render(m.inst.title), m.installLines(w-4, body-2), w, true)
	case m.form != nil:
		mid = panel(stTitle.Render("install"), m.formLines(w-4, body-2), w, true)
	case m.ext != nil:
		mid = panel(stTitle.Render("external references"), m.extLines(w-4, body-2), w, true)
	case m.help:
		mid = panel(stTitle.Render("keys"), m.helpLines(w-4, body-2), w, true)
	case m.picker != nil:
		mid = panel(stTitle.Render("packs"), m.pickerLines(w-4, body-2), w, true)
	case m.tab == tabCatalog:
		mid = m.catalogBody(w, body)
	case m.tab == tabPacks:
		mid = m.packsBody(w, body)
	case m.tab == tabInstalls:
		mid = m.installsBody(w, body)
	case m.tab == tabMarketplace:
		mid = m.marketBody(w, body)
	default:
		for range body {
			mid = append(mid, spaces(w))
		}
	}

	status := m.statusText()
	switch {
	case m.status != "" && m.statusOK:
		status = stTeal.Render(m.status)
	case m.status != "":
		status = stError.Render(m.status)
	}
	lines := append(append(append(top, mid...), tray...), fit(status, w), fit(stMuted.Render(m.footer(w)), w))
	return strings.Join(lines, "\n")
}

// statusText is the status line when nothing has been said.
func (m *Model) statusText() string {
	if m.tab == tabCatalog {
		return m.countLine()
	}
	return ""
}
