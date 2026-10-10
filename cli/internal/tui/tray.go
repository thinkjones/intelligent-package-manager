package tui

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// reloadPacks reads the scope's packs, keeping the active pack when it still
// exists and otherwise taking the first by name.
func (m *Model) reloadPacks() {
	packs, err := m.d.Packs.List()
	if err != nil {
		m.status = err.Error()
		return
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	m.packs = packs
	if m.activePack() == nil {
		m.active = ""
		if len(packs) > 0 {
			m.active = packs[0].Name
		}
	}
	if p := m.activePack(); p != nil {
		m.traySel = min(m.traySel, max(0, len(p.Items)-1))
	}
}

func (m *Model) activePack() *Pack {
	for i := range m.packs {
		if m.packs[i].Name == m.active {
			return &m.packs[i]
		}
	}
	return nil
}

func (m *Model) inPack(id string) bool {
	p := m.activePack()
	return p != nil && slices.Contains(p.Items, id)
}

// selectedID is the item a or d acts on: the selected result, neighbour or
// tray item.
func (m *Model) selectedID() string {
	switch m.focus {
	case focusTray:
		if p := m.activePack(); p != nil && m.traySel < len(p.Items) {
			return p.Items[m.traySel]
		}
	default:
		if m.sel < len(m.res.Hits) {
			return m.res.Hits[m.sel].ID
		}
	}
	return ""
}

// addSelected adds the selected item to the active pack.
func (m *Model) addSelected() {
	id := m.selectedID()
	if id == "" {
		return
	}
	if m.activePack() == nil {
		m.openPicker(true)
		return
	}
	if m.inPack(id) {
		m.note("%s is already in %s", itemLabel(id), m.active)
		return
	}
	if err := m.d.Packs.Add(m.active, id); err != nil {
		m.status = err.Error()
		return
	}
	m.dirty = true
	m.note("added %s to %s", itemLabel(id), m.active)
	m.reloadPacks()
}

// removeSelected removes the selected item from the active pack.
func (m *Model) removeSelected() {
	id := m.selectedID()
	if id == "" || m.activePack() == nil {
		return
	}
	if !m.inPack(id) {
		m.note("%s is not in %s", itemLabel(id), m.active)
		return
	}
	if err := m.d.Packs.Remove(m.active, id); err != nil {
		m.status = err.Error()
		return
	}
	m.dirty = true
	m.note("removed %s from %s", itemLabel(id), m.active)
	m.reloadPacks()
}

// note shows an informational status.
func (m *Model) note(format string, a ...any) {
	m.status, m.statusOK = fmt.Sprintf(format, a...), true
}

// itemLabel is an item ID as "type name": skill:o/r:skills/tdd → "skill tdd".
func itemLabel(id string) string {
	typ, rest, _ := strings.Cut(id, ":")
	name := rest
	if _, after, ok := strings.Cut(rest, "#"); ok {
		name = after
	} else if _, p, ok := strings.Cut(rest, ":"); ok {
		name = strings.TrimSuffix(path.Base(p), ".md")
	}
	return typ + " " + name
}

// trayLines is the pack tray: a header and one line of items.
func (m *Model) trayLines(w int) []string {
	p := m.activePack()
	if p == nil {
		head := stMuted.Render("─ "+m.d.Packs.Scope()+" · ") + stAmber.Render("no pack — press p to create one")
		return []string{fit(head, w), spaces(w)}
	}
	// The pack comes first so a long project path is what gets cut.
	head := stMuted.Render("─ ") + stTitle.Render(fmt.Sprintf("%s (%d)", p.Name, len(p.Items)))
	if m.dirty {
		head += stAmber.Render(" • not built")
	}
	head += stMuted.Render(" · " + m.d.Packs.Scope())
	var items []string
	for i, id := range p.Items {
		label := itemLabel(id)
		switch {
		case m.focus == focusTray && i == m.traySel:
			items = append(items, stSelected.Render("▶"+label))
		default:
			items = append(items, stText.Render(label))
		}
	}
	// Start the line late enough that the selected item fits on it.
	sep := stMuted.Render(" · ")
	start := 0
	if m.focus == focusTray {
		for start < m.traySel && lipgloss.Width(strings.Join(items[start:m.traySel+1], sep))+4 > w {
			start++
		}
	}
	line := strings.Join(items[start:], sep)
	if start > 0 {
		line = stMuted.Render("… · ") + line
	}
	if len(items) == 0 {
		line = stMuted.Render("empty — select a result and press a")
	}
	rule := stRule.Render(strings.Repeat("─", max(0, w-lipgloss.Width(head)-1)))
	return []string{fit(head+" "+rule, w), fit(line, w)}
}
