package tui

import (
	"github.com/charmbracelet/x/ansi"
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
	inner := w - caretWidth
	wideRows := w >= wideWidth-4
	nameW := max(10, inner-6-labelWidth-4)
	if wideRows {
		nameW = max(10, min(32, inner-6-labelWidth-4-39))
	}
	active := m.focus == focusResults || m.focus == focusSearch
	for i := m.offset; i < len(m.res.Hits) && len(lines) < rows; i++ {
		h := m.res.Hits[i]
		mark := " "
		if m.inPack(h.ID) {
			mark = stTeal.Render("✓")
		}
		row := fit(h.Score, 4) + " " + typeStyle(h.Type).Render(fit(typeLabel(h.Type), labelWidth)) + " " + mark + " "
		label := h.Name
		if h.External {
			label = "↗" + label
		}
		name := fit(label, nameW)
		if i == m.sel && active {
			row += stSelected.Render(name)
		} else {
			row += stText.Render(name)
		}
		if wideRows {
			star := ""
			if h.Stars > 0 {
				star = "★" + stars(h.Stars)
			}
			used := ""
			if h.UsedBy > 0 {
				used = "↗" + strconv.Itoa(h.UsedBy)
			}
			row += " " + stMuted.Render(fit(h.Repo, 26)) + " " + stAmber.Render(fit(used, 4)) + " " + starStyle(h.Stars).Render(fit(star, 7))
			if left := inner - lipgloss.Width(row) - 1; left > 0 {
				row += " " + stMuted.Render(ansi.Truncate(firstLine(h.Description), left, "…"))
			}
		}
		if i == m.sel {
			lines = append(lines, selectedRow(row, w, active))
		} else {
			lines = append(lines, plainRow(row, w))
		}
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
