package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// column is one column of a table. Min and Max bound its cell width; a Max of
// 0 makes it take the width the other columns leave.
type column struct {
	Title    string
	Min, Max int
	Right    bool // right-align (numbers)
}

// table renders a header row and rows as exactly height lines of width w,
// scrolled so sel is visible, with the caret column and selection styling of
// selectedRow/plainRow; cells are cut with "…". offset is updated in place.
// styles holds one style per cell, matching rows.
func table(cols []column, rows [][]string, styles [][]lipgloss.Style, sel int, offset *int, w, height int, active bool) []string {
	if height <= 0 {
		return nil
	}
	widths := columnWidths(cols, rows, w-caretWidth)
	lines := make([]string, 0, height)
	var head []string
	for i, c := range cols {
		head = append(head, stMuted.Bold(true).Render(cell(c.Title, widths[i], c.Right)))
	}
	lines = append(lines, plainRow(strings.Join(head, " "), w))

	room := height - 1
	if sel < *offset {
		*offset = sel
	}
	if sel >= *offset+room {
		*offset = sel - room + 1
	}
	*offset = max(0, *offset)
	for i := *offset; i < len(rows) && len(lines) < height; i++ {
		var cells []string
		for j, c := range cols {
			text := ""
			if j < len(rows[i]) {
				text = rows[i][j]
			}
			var st lipgloss.Style
			if i < len(styles) && j < len(styles[i]) {
				st = styles[i][j]
			}
			cells = append(cells, st.Render(cell(text, widths[j], c.Right)))
		}
		row := strings.Join(cells, " ")
		if i == sel {
			lines = append(lines, selectedRow(row, w, active))
		} else {
			lines = append(lines, plainRow(row, w))
		}
	}
	for len(lines) < height {
		lines = append(lines, spaces(w))
	}
	return lines
}

// cell cuts s to w cells with "…" and pads it, on the left when right is set.
func cell(s string, w int, right bool) string {
	s = ansi.Truncate(s, w, "…")
	pad := spaces(w - lipgloss.Width(s))
	if right {
		return pad + s
	}
	return s + pad
}

// columnWidths gives every column its widest cell within [Min, Max]; the
// column with no Max takes what is left. When the columns don't fit in w, the
// open one shrinks to its Min first, then the widest ones.
func columnWidths(cols []column, rows [][]string, w int) []int {
	widths := make([]int, len(cols))
	open := -1
	for i, c := range cols {
		n := lipgloss.Width(c.Title)
		for _, r := range rows {
			if i < len(r) {
				n = max(n, lipgloss.Width(r[i]))
			}
		}
		n = max(n, c.Min)
		if c.Max > 0 {
			n = min(n, c.Max)
		} else {
			open = i
		}
		widths[i] = n
	}
	total := func() int {
		t := len(cols) - 1
		for _, n := range widths {
			t += n
		}
		return t
	}
	if open >= 0 {
		widths[open] = max(cols[open].Min, widths[open]+w-total())
	}
	for total() > w {
		widest := -1
		for i := range widths {
			if widths[i] > max(cols[i].Min, 1) && (widest < 0 || i == open || (widest != open && widths[i] > widths[widest])) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		widths[widest]--
	}
	return widths
}
