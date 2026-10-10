package tui

import (
	"strconv"

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
	wideRows := w >= wideWidth-4
	nameW := max(10, w-6-labelWidth-4)
	if wideRows {
		nameW = max(10, min(32, w-6-labelWidth-4-34))
	}
	for i := m.offset; i < len(m.res.Hits) && len(lines) < rows; i++ {
		h := m.res.Hits[i]
		mark := " "
		if m.inPack(h.ID) {
			mark = stTeal.Render("✓")
		}
		row := fit(h.Score, 4) + " " + typeStyle(h.Type).Render(fit(typeLabel(h.Type), labelWidth)) + " " + mark + " "
		name := fit(h.Name, nameW)
		switch {
		case i == m.sel && m.focus == focusResults:
			name = stSelected.Render(name)
			row = stAmber.Render("▶") + row
		case i == m.sel:
			name = stTitle.Render(name)
			row = stMuted.Render("›") + row
		default:
			name = stText.Render(name)
			row = " " + row
		}
		row += name
		if wideRows {
			star := ""
			if h.Stars > 0 {
				star = "★" + stars(h.Stars)
			}
			row += " " + stMuted.Render(fit(h.Repo, 26)) + " " + starStyle(h.Stars).Render(fit(star, 7))
		}
		lines = append(lines, fit(row, w))
	}
	for len(lines) < rows {
		lines = append(lines, spaces(w))
	}
	return lines
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
