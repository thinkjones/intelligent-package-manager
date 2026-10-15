package tui

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tab is one of the screen's workflow tabs.
type tab int

const (
	tabCatalog tab = iota
	tabPacks
	tabInstalls
	tabMarketplace
	tabCount
)

var tabNames = [tabCount]string{"CATALOG", "PACKS", "INSTALLS", "MARKETPLACE"}

// notLoaded is a tab's count before its data is read.
const notLoaded = -1

// commas formats n with thousands separators: 2314 → 2,314.
func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// tabLabel is a tab's name with its count, "(…)" until it is loaded.
func (m *Model) tabLabel(t tab) string {
	n := "…"
	if c := m.counts[t]; c != notLoaded {
		n = commas(c)
	}
	return tabNames[t] + " (" + n + ")"
}

// tabBar is the tab labels and, under them, a rule with the active tab
// underlined; while the bar has focus the active tab is marked ▶.
func (m *Model) tabBar(w int) []string {
	var labels, rule []string
	for t := range tabCount {
		l := m.tabLabel(t)
		if t == m.tab {
			if m.bar {
				l = "▶ " + l
			}
			labels = append(labels, stSelected.Render(l))
			rule = append(rule, stAmber.Render(strings.Repeat("━", lipgloss.Width(l))))
		} else {
			labels = append(labels, stMuted.Render(l))
			rule = append(rule, stRule.Render(strings.Repeat("─", lipgloss.Width(l))))
		}
	}
	gap := stRule.Render("──")
	return []string{fit(strings.Join(labels, "  "), w), fit(strings.Join(rule, gap), w)}
}

// titleLine is "ipm", the scope, and the search mode on the right.
func (m *Model) titleLine(w int) string {
	mode := stTeal.Render("● hybrid")
	if m.d.Searcher.Mode() != "hybrid" {
		mode = stAmber.Render("○ local")
	}
	head := stTitle.Render("ipm") + "  " + stScope.Render(m.d.Packs.Scope())
	return fit(head, w-lipgloss.Width(mode)-1) + " " + mode
}

// statsStrip is the one line of numbers under the tab bar.
func (m *Model) statsStrip(w int) string {
	switch m.tab {
	case tabCatalog:
		return fit(m.catalogStats(), w)
	case tabPacks:
		return fit(m.packsStats(), w)
	case tabInstalls:
		return fit(m.installsStats(), w)
	case tabMarketplace:
		return fit(m.marketplaceStats(), w)
	}
	return spaces(w)
}

// barHints are the footer's keys while the tab bar has focus.
func (m *Model) barHints() []string {
	find := "/ filter"
	if m.tab == tabCatalog {
		find = "/ search"
	}
	return []string{"←→ tabs", "tab/↓ enter", find}
}

// hints is the footer for the active tab.
func (m *Model) hints() string {
	switch {
	case m.inst != nil && m.inst.done:
		return "esc close"
	case m.inst != nil:
		return "esc close (when finished)"
	case m.form != nil && m.form.step == formConfirm:
		return "y install  esc cancel"
	case m.form != nil:
		return "enter next  esc cancel"
	case m.ext != nil:
		return "↑↓ move  enter jump  esc close"
	case m.help:
		return "esc or ? close"
	case m.picker != nil:
		return "enter choose  n new pack  esc cancel"
	case m.bar:
		return strings.Join(append(m.barHints(), helpQuit), "  ")
	}
	if m.tab == tabCatalog {
		return m.catalogHints()
	}
	if m.tab == tabPacks && m.pk.filter.typing {
		return strings.Join(m.packsHints(), "  ")
	}
	if m.tab == tabInstalls && m.ins.filter.typing {
		return strings.Join(m.installsHints(), "  ")
	}
	if m.tab == tabMarketplace && m.mk.filter.typing {
		return strings.Join(m.marketHints(), "  ")
	}
	return strings.Join(append(m.otherHints(), helpQuit), "  ")
}

// helpQuit ends every footer that can be cut: it is the last thing to go.
const helpQuit = "? help  q quit"

// otherHints are the footer hints of a tab with no keys of its own yet.
func (m *Model) otherHints() []string {
	switch m.tab {
	case tabPacks:
		return m.packsHints()
	case tabInstalls:
		return m.installsHints()
	}
	return m.marketHints()
}

// footer is the hints cut to w: the middle hints drop from the end first, so
// "? help  q quit" stays visible.
func (m *Model) footer(w int) string {
	var parts []string
	switch {
	case m.inst != nil, m.form != nil, m.ext != nil, m.help, m.picker != nil:
		return m.hints()
	case m.bar:
		parts = m.barHints()
	case m.tab == tabPacks && m.pk.filter.typing, m.tab == tabInstalls && m.ins.filter.typing, m.tab == tabMarketplace && m.mk.filter.typing:
		return m.hints()
	case m.tab != tabCatalog:
		parts = m.otherHints()
	case m.focus == focusResults:
		parts = m.catalogResultHints()
	default:
		return m.hints()
	}
	for len(parts) > 0 && lipgloss.Width(strings.Join(append(parts[:len(parts):len(parts)], helpQuit), "  ")) > w {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(append(parts, helpQuit), "  ")
}

// tabKey switches tabs on 1–4 and [ ], landing on the tab bar; it reports
// whether k was one of them.
func (m *Model) tabKey(k string) bool {
	switch k {
	case "1", "2", "3", "4":
		m.tab = tab(k[0] - '1')
	case "]":
		m.tab = (m.tab + 1) % tabCount
	case "[":
		m.tab = (m.tab + tabCount - 1) % tabCount
	default:
		return false
	}
	m.toBar()
	return true
}

// barKey handles a key while the tab bar has focus: ← → pick a tab, tab ↓
// enter go into its first pane, shift+tab its last, / its search or filter.
func (m *Model) barKey(k string) tea.Cmd {
	switch k {
	case "left", "h":
		m.tab = (m.tab + tabCount - 1) % tabCount
		return m.enterTab()
	case "right", "l":
		m.tab = (m.tab + 1) % tabCount
		return m.enterTab()
	case "tab", "down", "enter":
		m.setPane(0)
	case "shift+tab":
		m.setPane(m.panes() - 1)
	case "/":
		m.bar = false
		switch m.tab {
		case tabCatalog:
			m.setFocus(focusSearch)
		case tabPacks:
			m.pk.filter.focus()
		case tabInstalls:
			m.ins.filter.focus()
		case tabMarketplace:
			m.mk.filter.focus()
		}
	case "q":
		return tea.Quit
	case "?":
		m.help = true
	default:
		if m.tabKey(k) {
			return m.enterTab()
		}
	}
	return nil
}

// toBar gives the tab bar focus; every tab's panes start over from the bar.
func (m *Model) toBar() {
	m.bar = true
	m.setFocus(focusResults)
	m.pk.detail = false
	m.pk.filter.blur()
	m.ins.filter.blur()
	m.mk.filter.blur()
}

// panes is how many panes tab cycles through on the active tab: the Catalog's
// search box, results and tree (when there is one), the Packs table and its
// items (when the pack has some), the one table of the other tabs.
func (m *Model) panes() int {
	switch m.tab {
	case tabCatalog:
		if m.cursor == "" {
			return 2
		}
		return 3
	case tabPacks:
		if p := m.selectedPack(); p != nil && len(p.Items) > 0 {
			return 2
		}
	}
	return 1
}

// pane is the index of the focused pane among panes; the tray counts as the
// results.
func (m *Model) pane() int {
	switch {
	case m.tab == tabCatalog && m.focus == focusSearch:
		return 0
	case m.tab == tabCatalog && m.focus == focusTree:
		return 2
	case m.tab == tabCatalog:
		return 1
	case m.tab == tabPacks && m.pk.detail:
		return 1
	}
	return 0
}

// setPane focuses pane i of the active tab.
func (m *Model) setPane(i int) {
	m.bar = false
	switch m.tab {
	case tabCatalog:
		switch i {
		case 0:
			m.setFocus(focusSearch)
		case 1:
			m.setFocus(focusResults)
		default:
			m.setFocus(focusTree)
			m.treeSel = m.tree.Cursor
		}
	case tabPacks:
		m.pk.detail = i == 1
	}
}

// cyclePane moves by one pane, to the tab bar past either end.
func (m *Model) cyclePane(by int) {
	i := m.pane() + by
	if i < 0 || i >= m.panes() {
		m.toBar()
		return
	}
	m.setPane(i)
}

// typingFilter is the filter box being typed into, nil when there is none.
// The Catalog's search box is a pane, not a filter.
func (m *Model) typingFilter() *filterBox {
	for _, f := range []struct {
		t tab
		f *filterBox
	}{{tabPacks, &m.pk.filter}, {tabInstalls, &m.ins.filter}, {tabMarketplace, &m.mk.filter}} {
		if m.tab == f.t && f.f.typing {
			return f.f
		}
	}
	return nil
}

// typeOrder is how the catalog's types are listed in the stats strip; types
// beyond it follow alphabetically.
var typeOrder = []string{"skill", "agent", "command", "hook", "mcp", "plugin", "marketplace", "repo"}

// typeCounts is the catalog's per-type counts in typeOrder, as "type n".
func typeCounts(counts map[string]int) []string {
	var types []string
	for _, t := range typeOrder {
		if _, ok := counts[t]; ok {
			types = append(types, t)
		}
	}
	var rest []string
	for t := range counts {
		if !slices.Contains(typeOrder, t) {
			rest = append(rest, t)
		}
	}
	sort.Strings(rest)
	var out []string
	for _, t := range append(types, rest...) {
		out = append(out, typeStyle(t).Render(t+" "+commas(counts[t])))
	}
	return out
}
