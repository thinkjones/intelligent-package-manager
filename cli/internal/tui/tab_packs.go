package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// readOnlyGlobal is what x, b and i say on a global pack shown from a project.
const readOnlyGlobal = "global pack — open ipm tui -g to change it"

// packsTab is the Packs tab's state: its own rows, selection, focus and filter,
// kept apart from the Catalog's tray.
type packsTab struct {
	rows   []Pack
	err    error // the last load's error
	loaded bool
	global bool // include the user scope's packs
	filter filterBox

	sel, offset int  // the selected pack among the visible rows
	detail      bool // the item list has focus
	itemSel     int
	itemOffset  int
}

var packCols = []column{
	{Title: "NAME", Min: 8, Max: 28},
	{Title: "SCOPE", Min: 5, Max: 7},
	{Title: "ITEMS", Min: 5, Max: 5, Right: true},
	{Title: "BUILT", Min: 5, Max: 5},
	{Title: "INSTALLED TO", Min: 12},
}

// packScope is the SCOPE cell.
func packScope(p Pack) string {
	if p.Global {
		return "global"
	}
	return "project"
}

// installedTo joins a pack's installs as "claude project, gascity kit".
func installedTo(p Pack) string {
	var parts []string
	for _, in := range p.Installs {
		what := in.Scope
		if in.Kind == "gascity" {
			what = in.Name
		}
		parts = append(parts, strings.TrimSpace(in.Kind+" "+what))
	}
	return strings.Join(parts, ", ")
}

// cells are a pack's visible column text.
func (p Pack) cells() []string {
	return []string{p.Name, packScope(p), fmt.Sprint(len(p.Items)), p.Built, installedTo(p)}
}

// loadPacksTab reads the tab's packs, project ones first, keeping the
// selection on the same pack when it is still there.
func (m *Model) loadPacksTab() {
	pk := &m.pk
	keep := ""
	if p := m.selectedPack(); p != nil {
		keep = p.Name
	}
	rows, err := m.d.Packs.List(pk.global)
	pk.loaded, pk.err = true, err
	if err != nil {
		pk.rows = nil
		m.counts[tabPacks] = notLoaded
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Global != rows[j].Global {
			return !rows[i].Global
		}
		return rows[i].Name < rows[j].Name
	})
	pk.rows = rows
	m.counts[tabPacks] = len(rows)
	pk.sel = 0
	for i, p := range m.visiblePacks() {
		if p.Name == keep {
			pk.sel = i
		}
	}
	m.clampPacks()
}

// visiblePacks are the rows the filter keeps.
func (m *Model) visiblePacks() []Pack {
	var out []Pack
	for _, p := range m.pk.rows {
		if m.pk.filter.match(p.cells()) {
			out = append(out, p)
		}
	}
	return out
}

// selectedPack is the pack under the cursor, nil when there is none.
func (m *Model) selectedPack() *Pack {
	vis := m.visiblePacks()
	if m.pk.sel < 0 || m.pk.sel >= len(vis) {
		return nil
	}
	return &vis[m.pk.sel]
}

// clampPacks keeps both selections valid after the rows or the filter change.
func (m *Model) clampPacks() {
	pk := &m.pk
	pk.sel = max(0, min(pk.sel, len(m.visiblePacks())-1))
	p := m.selectedPack()
	if p == nil || len(p.Items) == 0 {
		pk.detail = false
		pk.itemSel = 0
		return
	}
	pk.itemSel = max(0, min(pk.itemSel, len(p.Items)-1))
}

// enterTab loads what the tab shows when it is switched to: its first visit,
// and a retry after a failed load.
func (m *Model) enterTab() {
	switch {
	case m.tab == tabPacks && (!m.pk.loaded || m.pk.err != nil):
		m.loadPacksTab()
	case m.tab == tabInstalls && (!m.ins.loaded || m.ins.err != nil):
		m.loadInstallsTab()
	}
}

// textFocused reports whether a text input has focus, so that every
// printable key types into it.
func (m *Model) textFocused() bool {
	switch m.tab {
	case tabCatalog:
		return m.focus == focusSearch
	case tabPacks:
		return m.pk.filter.typing
	case tabInstalls:
		return m.ins.filter.typing
	}
	return false
}

// packsKey handles a key press on the Packs tab.
func (m *Model) packsKey(msg tea.KeyPressMsg) tea.Cmd {
	pk, k := &m.pk, msg.String()
	if pk.filter.typing {
		if pk.filter.key(msg) {
			pk.sel, pk.offset, pk.itemSel = 0, 0, 0
			m.clampPacks()
		}
		return nil
	}
	switch k {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
	case "/":
		pk.detail = false
		pk.filter.focus()
	case "g":
		m.togglePacksGlobal()
	case "down", "j":
		m.packsMove(1)
	case "up", "k":
		m.packsMove(-1)
	case "enter", "right", "l":
		if p := m.selectedPack(); p != nil && len(p.Items) > 0 {
			pk.detail = true
		}
	case "esc", "left", "h":
		switch {
		case pk.detail:
			pk.detail = false
		case k == "esc" && pk.filter.value() != "":
			pk.filter.clear()
			pk.sel, pk.offset = 0, 0
			m.clampPacks()
		}
	case "x":
		m.removePackItem()
	case "b":
		if m.writablePack() {
			return m.startBuild()
		}
	case "i":
		if m.writablePack() {
			m.openInstallForm(m.selectedPack().Name, false)
		}
	}
	return nil
}

// togglePacksGlobal adds or drops the user scope's packs, from a project.
func (m *Model) togglePacksGlobal() {
	if m.d.Packs.Scope() == "global" {
		m.note("already in global scope")
		return
	}
	m.pk.global = !m.pk.global
	m.loadPacksTab()
}

// writablePack reports whether the selected pack can be changed; otherwise it
// says why in the status.
func (m *Model) writablePack() bool {
	p := m.selectedPack()
	switch {
	case p == nil:
		m.status = "no pack selected"
	case p.Global && m.d.Packs.Scope() != "global":
		m.status = readOnlyGlobal
	default:
		return true
	}
	return false
}

func (m *Model) packsMove(d int) {
	pk := &m.pk
	p := m.selectedPack()
	if pk.detail && p != nil {
		pk.itemSel = max(0, min(pk.itemSel+d, len(p.Items)-1))
		return
	}
	pk.sel = max(0, min(pk.sel+d, len(m.visiblePacks())-1))
	pk.itemSel, pk.itemOffset = 0, 0
}

// removePackItem removes the selected item from the selected pack.
func (m *Model) removePackItem() {
	if !m.writablePack() {
		return
	}
	pk := &m.pk
	p := m.selectedPack()
	if !pk.detail {
		m.status = "press enter to pick an item, then x"
		return
	}
	id := p.Items[pk.itemSel]
	if err := m.d.Packs.Remove(p.Name, id); err != nil {
		m.status = err.Error()
		return
	}
	m.dirty = true
	m.note("removed %s from %s", itemLabel(id), p.Name)
	m.reloadPacks()
}

// packsStats is the Packs tab's strip: totals, then the filter while set.
func (m *Model) packsStats() string {
	pk := &m.pk
	if !pk.loaded || pk.err != nil {
		return ""
	}
	fresh, stale, installs := 0, 0, 0
	for _, p := range pk.rows {
		switch p.Built {
		case "fresh":
			fresh++
		case "stale":
			stale++
		}
		installs += len(p.Installs)
	}
	parts := []string{count(len(pk.rows), "pack"), fmt.Sprintf("%d fresh", fresh), fmt.Sprintf("%d stale", stale), count(installs, "install")}
	s := strings.Join(parts, stMuted.Render(" · "))
	if f := pk.filter.value(); f != "" {
		s += stMuted.Render(" · filter: ") + stAmber.Render(f)
	}
	return s
}

// packsHints are the Packs footer's keys, before help and quit.
func (m *Model) packsHints() []string {
	switch {
	case m.pk.filter.typing:
		return []string{"type to filter", "enter keep", "esc clear"}
	case m.pk.detail:
		return []string{"↑↓ items", "x remove", "esc back"}
	}
	return []string{"↑↓ packs", "enter items", "b build", "i install", "g global", "/ filter", "1-4 tabs"}
}

var buildStyle = map[string]lipgloss.Style{"fresh": stTeal, "stale": stAmber, "never": stMuted}

// packsBody is the pack table, with the selected pack's detail beside it when
// the screen is wide, h lines in all.
func (m *Model) packsBody(w, h int) []string {
	pk := &m.pk
	if pk.err != nil {
		return m.packsMessage(stError.Render("could not read packs: "+pk.err.Error()), w, h)
	}
	vis := m.visiblePacks()
	switch {
	case len(vis) == 0 && pk.filter.value() != "":
		return m.packsMessage(stMuted.Render("no pack matches the filter — esc clears it"), w, h)
	case len(vis) == 0:
		return m.packsMessage(stMuted.Render("no packs yet — create one from the Catalog tab (p, then n), or with ipm pack new"), w, h)
	}
	wide := w >= wideWidth
	if pk.detail && !wide {
		// No room beside the table: the item list takes the whole body.
		return m.packDetail(vis[pk.sel], w, h)
	}
	tw := w
	if wide {
		tw = w * 58 / 100
	}
	rows := make([][]string, len(vis))
	styles := make([][]lipgloss.Style, len(vis))
	for i, p := range vis {
		rows[i] = p.cells()
		scope := stText
		if p.Global {
			scope = stMuted
		}
		styles[i] = []lipgloss.Style{stText, scope, stText, buildStyle[p.Built], stMuted}
	}
	title := stMuted.Render("packs")
	if f := pk.filter.line(); f != "" {
		pk.filter.in.SetWidth(max(10, tw-24))
		title += stMuted.Render(" ── ") + f
	}
	list := panel(title, table(packCols, rows, styles, pk.sel, &pk.offset, tw-4, h-2, !pk.detail), tw, !pk.detail)
	if !wide {
		return list
	}
	return joinPanels(list, m.packDetail(vis[pk.sel], w-tw, h))
}

// packsMessage is a one-panel body holding a single line.
func (m *Model) packsMessage(line string, w, h int) []string {
	lines := []string{line}
	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	return panel(stMuted.Render("packs"), lines, w, true)
}

// joinPanels puts two panels of the same height side by side.
func joinPanels(a, b []string) []string {
	out := make([]string, len(a))
	for i := range a {
		out[i] = a[i] + b[i]
	}
	return out
}

// packDetail is the panel of a pack's description and items, w wide and h
// lines high.
func (m *Model) packDetail(p Pack, w, h int) []string {
	pk := &m.pk
	rows := h - 2
	lines := []string{stText.Render(p.Description)}
	if p.Description == "" {
		lines = []string{stMuted.Render("no description")}
	}
	lines = append(lines, "")
	room := max(1, rows-len(lines))
	if len(p.Items) == 0 {
		lines = append(lines, stMuted.Render("empty — add items from the Catalog tab"))
	}
	if pk.itemSel < pk.itemOffset {
		pk.itemOffset = pk.itemSel
	}
	if pk.itemSel >= pk.itemOffset+room {
		pk.itemOffset = pk.itemSel - room + 1
	}
	for i := pk.itemOffset; i < len(p.Items) && i < pk.itemOffset+room; i++ {
		label := itemLabel(p.Items[i])
		typ, name, _ := strings.Cut(label, " ")
		item := typeStyle(typ).Render(typ) + " " + stText.Render(name)
		if pk.detail && i == pk.itemSel {
			item = stSelected.Render("▶ " + label)
		} else {
			item = "  " + item
		}
		lines = append(lines, item)
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return panel(stTitle.Render(p.Name)+stMuted.Render(fmt.Sprintf(" (%d)", len(p.Items))), lines[:rows], w, pk.detail)
}

// count is "n word", pluralised.
func count(n int, word string) string { return fmt.Sprintf("%d %s", n, plural(n, word)) }
