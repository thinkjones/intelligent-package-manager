package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

func lipglossWidth(s string) int { return lipgloss.Width(s) }

// resultLines is the results pane: exactly rows lines of width w, scrolled so
// the selection is visible.
func (m *Model) resultLines(w, rows int) []string {
	if m.sel < m.offset {
		m.offset = m.sel
	}
	if m.sel >= m.offset+rows {
		m.offset = m.sel - rows + 1
	}
	lines := make([]string, 0, rows)
	for i := m.offset; i < len(m.res.Hits) && len(lines) < rows; i++ {
		h := m.res.Hits[i]
		row := fmt.Sprintf("%-4s %-14s %s", h.Score, h.Type, h.Name)
		switch {
		case i == m.sel && m.focus == focusResults:
			lines = append(lines, stSelected.Render(fit("▶"+row, w)))
		case i == m.sel:
			lines = append(lines, stTitle.Render(fit("›"+row, w)))
		default:
			lines = append(lines, stText.Render(fit(" "+row, w)))
		}
	}
	for len(lines) < rows {
		lines = append(lines, spaces(w))
	}
	return lines
}
