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
	itemSel     int  // the selected item in the tree's order (packItems)
	itemOffset  int  // the first tree line shown
	// items caches what the catalog says of each pack item; cleared on load.
	items map[string]packItem
}

// packItem is a pack item as the items tree shows it: its repo, type and
// name from the catalog, or no repo when the catalog cannot read it.
type packItem struct{ id, repo, typ, name string }

// unknownRepo heads the items the catalog cannot place in a repo.
const unknownRepo = "? unknown"

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
	pk.loaded, pk.err, pk.items = true, err, nil
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
func (m *Model) enterTab() tea.Cmd {
	switch {
	case m.tab == tabPacks && (!m.pk.loaded || m.pk.err != nil):
		m.loadPacksTab()
	case m.tab == tabInstalls && (!m.ins.loaded || m.ins.err != nil):
		m.loadInstallsTab()
	case m.tab == tabMarketplace:
		return m.marketVisit()
	}
	return nil
}

// textFocused reports whether a text input has focus, so that every
// printable key types into it.
func (m *Model) textFocused() bool {
	switch m.tab {
	case tabCatalog:
		return !m.bar && m.focus == focusSearch
	case tabPacks:
		return m.pk.filter.typing
	case tabInstalls:
		return m.ins.filter.typing
	case tabMarketplace:
		return m.mk.filter.typing
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
		case k == "esc":
			m.toBar()
		}
	case "x":
		m.removePackItem()
	case "n":
		m.openPicker(true)
	case "c":
		return m.openItemInCatalog()
	case "b":
		if m.writablePack() {
			return m.startBuild()
		}
	case "i":
		if m.writablePack() {
			p := m.selectedPack()
			m.openInstallForm(p.Name, p.Global)
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
	id := m.packItems(*p)[pk.itemSel].id
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
		return []string{"type to filter", "enter/tab keep", "esc clear"}
	case m.pk.detail:
		return []string{"↑↓ items", "x remove", "c catalog", "n new", "esc back"}
	}
	return []string{"↑↓ packs", "enter items", "n new", "b build", "i install", "g global", "/ filter", "esc tab bar"}
}

var buildStyle = map[string]lipgloss.Style{"fresh": stTeal, "stale": stAmber, "never": stMuted}

// packsBody is the pack table over the selected pack's items, as the
// Catalog's results sit over its tree, h lines in all.
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
		return m.packsMessage(stMuted.Render("no packs yet — press n to create one, or run ipm pack new"), w, h)
	}
	// The table takes what its rows need, up to 40% of the body.
	top := max(4, min(h*40/100, len(vis)+3))
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
		pk.filter.in.SetWidth(max(10, w-24))
		title += stMuted.Render(" ── ") + f
	}
	focused := !m.bar && !pk.detail
	list := panel(title, table(packCols, rows, styles, pk.sel, &pk.offset, w-4, top-2, focused), w, focused)
	return append(list, m.packDetail(vis[pk.sel], w, h-top)...)
}

// packsMessage is a one-panel body holding a single line.
func (m *Model) packsMessage(line string, w, h int) []string {
	lines := []string{line}
	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	return panel(stMuted.Render("packs"), lines, w, !m.bar)
}

// joinPanels puts two panels of the same height side by side.
func joinPanels(a, b []string) []string {
	out := make([]string, len(a))
	for i := range a {
		out[i] = a[i] + b[i]
	}
	return out
}

// packItem is what the catalog says of the pack item id, read once until the
// packs load again.
func (m *Model) packItem(id string) packItem {
	if it, ok := m.pk.items[id]; ok {
		return it
	}
	it := packItem{id: id}
	if n, err := m.d.Catalog.Node(id); err == nil && n.Repo != "" {
		it.repo, it.typ, it.name = n.Repo, n.Type, n.Name
	}
	if m.pk.items == nil {
		m.pk.items = map[string]packItem{}
	}
	m.pk.items[id] = it
	return it
}

// packGroups are p's items grouped by repo. Repos come in the order of their
// first item in the pack, so the tree reads like the pack; the items the
// catalog cannot place come last, under unknownRepo.
func (m *Model) packGroups(p Pack) [][]packItem {
	var groups [][]packItem
	at := map[string]int{}
	var unknown []packItem
	for _, id := range p.Items {
		it := m.packItem(id)
		if it.repo == "" {
			unknown = append(unknown, it)
			continue
		}
		i, ok := at[it.repo]
		if !ok {
			i = len(groups)
			at[it.repo] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], it)
	}
	if unknown != nil {
		groups = append(groups, unknown)
	}
	return groups
}

// packItems are p's items in the tree's order: what itemSel indexes.
func (m *Model) packItems(p Pack) []packItem {
	var out []packItem
	for _, g := range m.packGroups(p) {
		out = append(out, g...)
	}
	return out
}

// packDetail is the panel of a pack's description and its items as a tree
// of repos, w wide and h lines high.
func (m *Model) packDetail(p Pack, w, h int) []string {
	pk := &m.pk
	rows := h - 2
	var lines []string
	if rows >= 5 { // below that the items need every line
		lines = []string{stText.Render(p.Description), ""}
		if p.Description == "" {
			lines[0] = stMuted.Render("no description")
		}
	}
	room := max(1, rows-len(lines))
	if len(p.Items) == 0 {
		lines = append(lines, stMuted.Render("empty — add items from the Catalog tab"))
	}
	focused := !m.bar && pk.detail
	var tree []string
	// sel is the selected item's line, top the first line to keep visible
	// with it: its repo's, for a repo's first item.
	sel, top, n := -1, 0, 0
	for _, g := range m.packGroups(p) {
		head := stMuted.Render("▾ ") + stText.Render(g[0].repo)
		if g[0].repo == "" {
			head = stMuted.Render("▾ " + unknownRepo)
		}
		tree = append(tree, head)
		for i, it := range g {
			label, plain := stMuted.Render(it.id), it.id
			if it.repo != "" {
				label, plain = typeStyle(it.typ).Render(it.typ)+" "+stText.Render(it.name), it.typ+" "+it.name
			}
			if focused && n == pk.itemSel {
				sel, top = len(tree), len(tree)
				if i == 0 {
					top--
				}
				tree = append(tree, "  "+stSelected.Render("▶ "+plain))
			} else {
				tree = append(tree, "    "+label)
			}
			n++
		}
	}
	switch {
	case sel < 0:
		pk.itemOffset = 0 // unfocused, the tree shows from its first repo
	case top < pk.itemOffset:
		pk.itemOffset = top
	case sel >= pk.itemOffset+room:
		pk.itemOffset = sel - room + 1
	}
	pk.itemOffset = max(0, min(pk.itemOffset, len(tree)-room))
	for i := pk.itemOffset; i < len(tree) && i < pk.itemOffset+room; i++ {
		lines = append(lines, tree[i])
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return panel(stTitle.Render(p.Name)+stMuted.Render(fmt.Sprintf(" (%d)", len(p.Items))), lines[:rows], w, focused)
}

// selectPackNamed puts the table's cursor on the pack called name, clearing
// a filter that hides it, and gives the table focus.
func (m *Model) selectPackNamed(name string) {
	pk := &m.pk
	find := func() bool {
		for i, p := range m.visiblePacks() {
			if p.Name == name {
				pk.sel = i
				return true
			}
		}
		return false
	}
	if !find() && pk.filter.value() != "" {
		pk.filter.clear()
		find()
	}
	pk.detail, pk.itemSel, pk.itemOffset = false, 0, 0
	m.clampPacks()
}

// openItemInCatalog shows the selected item in the Catalog: its repo's
// items as the results (repo:owner/repo), the tree on the item, with focus.
func (m *Model) openItemInCatalog() tea.Cmd {
	p := m.selectedPack()
	if p == nil || !m.pk.detail {
		m.status = "press enter to pick an item, then c"
		return nil
	}
	it := m.packItems(*p)[m.pk.itemSel]
	if it.repo == "" {
		m.note("%s is not in the catalog — ipm catalog sync may bring it back", it.id)
		return nil
	}
	m.tab, m.bar, m.filter = tabCatalog, false, 0
	cmd := m.runQuery("repo:" + it.repo)
	m.reveal = it.id
	return cmd
}

// count is "n word", pluralised.
func count(n int, word string) string { return fmt.Sprintf("%d %s", n, plural(n, word)) }
