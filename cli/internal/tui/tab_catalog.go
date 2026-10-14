package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// sumCounts is the catalog's item total over every type.
func sumCounts(counts map[string]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

// catalogKey handles a key press on the Catalog tab, for whatever has focus.
func (m *Model) catalogKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
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
		case "e":
			m.openExternal()
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

// catalogBody is the search results table over the tree, h lines in all.
func (m *Model) catalogBody(w, h int) []string {
	resH := max(5, h*40/100)
	m.input.SetWidth(min(w-24, max(lipglossWidth(m.input.Placeholder), lipglossWidth(m.input.Value())+1)))
	search := stMuted.Render("search: ") + m.input.View()
	if f := typeFilters[m.filter]; f != "" {
		search += stMuted.Render("  type: ") + stAmber.Render(f)
	}
	resultsFocused := m.focus == focusSearch || m.focus == focusResults
	mid := panel(search, m.resultLines(w-4, resH-2), w, resultsFocused)
	sel := -1
	if m.focus == focusTree {
		sel = m.treeSel
	}
	title := stMuted.Render("tree")
	if m.cursor != "" {
		title += stMuted.Render(" ── ") + stText.Render(m.cursor)
	}
	return append(mid, panel(title, m.tree.lines(w-4, h-resH-2, sel, m.focus == focusTree), w, m.focus == focusTree)...)
}

// catalogStats is the per-type item counts, in each type's colour.
func (m *Model) catalogStats() string {
	if len(m.catCounts) == 0 {
		return stMuted.Render("catalog empty")
	}
	return strings.Join(typeCounts(m.catCounts), stMuted.Render(" · "))
}

// catalogHints is the Catalog footer: the keys that matter where the focus is.
func (m *Model) catalogHints() string {
	switch m.focus {
	case focusSearch:
		return "type to search  ↓ results  ctrl+c quit"
	case focusTree:
		return "↑↓ siblings  ←→ parent/child  enter follow  a add  d remove  tab/esc back  ? help"
	case focusTray:
		return "←→ items  d remove  esc back"
	}
	return "↑↓ results  ←→ tree  tab tree mode  a add  t tray  p packs  f type  1-4 tabs  ? help  q quit"
}

// countLine is the Catalog's status line: how many results, and the path to
// the one the tree is on.
func (m *Model) countLine() string {
	if len(m.res.Hits) == 0 {
		if strings.TrimSpace(m.input.Value()) == "" {
			return "type to search"
		}
		return "no matches"
	}
	n := fmt.Sprintf("%d of %d", len(m.res.Hits), m.res.Found)
	if m.cursor == "" {
		return stMuted.Render(n)
	}
	return stMuted.Render(n+"  ") + m.breadcrumb(m.width-lipglossWidth(n)-4)
}

// resultLines is the results pane: exactly rows lines of width w, a header
// over the hits, scrolled so the selection is visible.
func (m *Model) resultLines(w, rows int) []string {
	wide := w >= wideWidth-4
	cols := []column{{Title: "TYPE", Min: labelWidth, Max: labelWidth}, {Title: "✓", Min: 1, Max: 1}, {Title: "NAME", Min: 10}}
	if wide {
		cols[2].Max = 32
		cols = append(cols, column{Title: "REPO", Max: 26}, column{Title: "↗", Max: 4, Right: true}, column{Title: "★", Max: 7, Right: true})
	}
	cols = append(cols, column{Title: "SCORE", Min: 4, Max: 5, Right: true})
	if wide {
		cols = append(cols, column{Title: "DESCRIPTION", Min: 1})
	}
	active := m.focus == focusResults || m.focus == focusSearch
	data := make([][]string, len(m.res.Hits))
	styles := make([][]lipgloss.Style, len(m.res.Hits))
	for i, h := range m.res.Hits {
		mark := ""
		if m.inPack(h.ID) {
			mark = "✓"
		}
		name := h.Name
		if h.External {
			name = "↗" + name
		}
		nameStyle := stText
		if i == m.sel && active {
			nameStyle = stSelected
		}
		row := []string{typeLabel(h.Type), mark, name}
		st := []lipgloss.Style{typeStyle(h.Type), stTeal, nameStyle}
		if wide {
			used, star := "", ""
			if h.UsedBy > 0 {
				used = "↗" + strconv.Itoa(h.UsedBy)
			}
			if h.Stars > 0 {
				star = "★" + stars(h.Stars)
			}
			row = append(row, h.Repo, used, star)
			st = append(st, stMuted, stAmber, starStyle(h.Stars))
		}
		row, st = append(row, h.Score), append(st, stText)
		if wide {
			row, st = append(row, firstLine(h.Description)), append(st, stMuted)
		}
		data[i], styles[i] = row, st
	}
	return table(cols, data, styles, m.sel, &m.offset, w, rows, active)
}
