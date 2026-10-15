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
		case "down", "tab", "esc":
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
		return "type to search  ↓/esc results  ctrl+c quit"
	case focusTree:
		return "↑↓ siblings  ←→ parent/child  enter follow  a add  d remove  tab/esc back  ? help"
	case focusTray:
		return "←→ items  d remove  esc back"
	}
	return strings.Join(append(m.catalogResultHints(), helpQuit), "  ")
}

// catalogResultHints are the Catalog footer's middle hints with the results
// focused.
func (m *Model) catalogResultHints() []string {
	return []string{"/ search", "1-4 tabs", "↑↓ results", "←→ tree", "tab tree mode", "a add", "t tray", "p packs", "f type"}
}

// countLine is the Catalog's status line: how many results, and the path to
// the one the tree is on.
func (m *Model) countLine() string {
	if len(m.res.Hits) == 0 {
		if strings.TrimSpace(m.input.Value()) == "" {
			if m.focus == focusSearch {
				return "type to search"
			}
			return "press / to search"
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
	// w is the panel's inner width: the screen is wideWidth columns or more.
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
		m.status = m.tree.Node.Unresolved + " isn't one of your sources — ipm marketplace add " + m.cursor
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

func (m *Model) move(by int) {
	sel := max(0, min(len(m.res.Hits)-1, m.sel+by))
	if sel != m.sel {
		m.sel = sel
		m.followSelection()
	}
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
