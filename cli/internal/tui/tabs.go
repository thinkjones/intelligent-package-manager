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
// underlined.
func (m *Model) tabBar(w int) []string {
	var labels, rule []string
	for t := range tabCount {
		l := m.tabLabel(t)
		if t == m.tab {
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
	}
	return spaces(w)
}

// hints is the footer for the active tab.
func (m *Model) hints() string {
	switch {
	case m.inst != nil:
		return "esc close (when finished)"
	case m.deploy != nil && m.deploy.step == stepConfirm:
		return "y deploy  esc cancel"
	case m.deploy != nil:
		return "enter next  esc cancel"
	case m.ext != nil:
		return "↑↓ move  enter jump  esc close"
	case m.help:
		return "esc or ? close"
	case m.picker != nil:
		return "enter choose  n new pack  esc cancel"
	}
	if m.tab == tabCatalog {
		return m.catalogHints()
	}
	return strings.Join(append(m.otherHints(), helpQuit), "  ")
}

// helpQuit ends every footer that can be cut: it is the last thing to go.
const helpQuit = "? help  q quit"

// otherHints are the footer hints of a tab with no keys of its own yet.
func (m *Model) otherHints() []string { return []string{"1-4 tabs", "[ ] switch"} }

// footer is the hints cut to w: the middle hints drop from the end first, so
// "? help  q quit" stays visible.
func (m *Model) footer(w int) string {
	var parts []string
	switch {
	case m.inst != nil, m.deploy != nil, m.ext != nil, m.help, m.picker != nil:
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

// tabKey switches tabs on 1–4 and [ ]; it reports whether k was one of them.
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
	return true
}

// otherTabKey handles keys on a tab that has no keys of its own yet.
func (m *Model) otherTabKey(k string) tea.Cmd {
	switch k {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
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
