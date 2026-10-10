package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// descLines is how much of a description the detail pane shows; ipm show
// prints it all.
const descLines = 4

// showDetail loads id into the detail pane; push keeps the current artifact
// on the back stack.
func (m *Model) showDetail(id string, push bool) {
	d, err := m.d.Catalog.Detail(id)
	if err != nil {
		m.status = err.Error()
		return
	}
	if push && m.detail.ID != "" {
		m.back = append(m.back, m.detail.ID)
	}
	m.detail, m.nbSel = d, 0
}

// followSelection shows the selected result, starting a fresh back stack.
func (m *Model) followSelection() {
	m.back = nil
	if m.sel < len(m.res.Hits) {
		m.showDetail(m.res.Hits[m.sel].ID, false)
	} else {
		m.detail = Detail{}
	}
}

// neighbours is the detail's relationships in display order: up, then down.
func (m *Model) neighbours() []Neighbour {
	return append(append([]Neighbour(nil), m.detail.Up...), m.detail.Down...)
}

// detailKey handles keys while the detail pane has focus.
func (m *Model) detailKey(k string) {
	nb := m.neighbours()
	switch k {
	case "down", "j":
		m.nbSel = min(len(nb)-1, m.nbSel+1)
	case "up", "k":
		m.nbSel = max(0, m.nbSel-1)
	case "enter":
		if m.nbSel < len(nb) {
			m.showDetail(nb[m.nbSel].ID, true)
		}
	case "backspace":
		if n := len(m.back); n > 0 {
			prev := m.back[n-1]
			m.back = m.back[:n-1]
			m.showDetail(prev, false)
		}
	case "left", "h", "esc":
		m.full = false
		m.setFocus(focusResults)
	}
}

// detailLines is the detail pane: exactly rows lines of width w.
func (m *Model) detailLines(w, rows int) []string {
	d := m.detail
	var lines []string
	if d.ID == "" {
		lines = append(lines, stMuted.Render("select a result to see its detail"))
	} else {
		lines = append(lines, stMuted.Render(d.Type)+"  "+stTitle.Render(d.Name))
		meta := d.Repo
		if d.Stars > 0 {
			meta += " ★ " + stars(d.Stars)
		}
		lines = append(lines, stText.Render(meta), stAmber.Render(d.ID))
		if d.Description != "" {
			wrapped := strings.Split(lipgloss.NewStyle().Width(w).Render(d.Description), "\n")
			if len(wrapped) > descLines {
				wrapped = append(wrapped[:descLines-1], wrapped[descLines-1]+"…")
			}
			for _, l := range wrapped {
				lines = append(lines, stText.Render(l))
			}
		}
		lines = append(lines, "")
		for i, n := range m.neighbours() {
			dir := "down"
			if i < len(d.Up) {
				dir = "up"
			}
			row := fmt.Sprintf("%-5s %-14s %s", dir, n.Type, n.Name)
			switch {
			case m.focus == focusDetail && i == m.nbSel:
				lines = append(lines, stSelected.Render("▶"+row))
			default:
				lines = append(lines, stText.Render(" "+row))
			}
		}
	}
	out := make([]string, 0, rows)
	for _, l := range lines {
		if len(out) == rows {
			break
		}
		out = append(out, fit(l, w))
	}
	for len(out) < rows {
		out = append(out, spaces(w))
	}
	return out
}

// stars formats a star count as ipm search does: 31234 → 31.2k.
func stars(n int) string {
	switch {
	case n >= 100_000:
		return strconv.Itoa(n/1000) + "k"
	case n >= 1000:
		return strconv.FormatFloat(float64(n)/1000, 'f', 1, 64) + "k"
	}
	return strconv.Itoa(n)
}
