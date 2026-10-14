package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ipm's palette, shared with docs/assets/brand.
var (
	colText   = lipgloss.Color("#F3EFE6")
	colSoft   = lipgloss.Color("#C9D1D9")
	colMuted  = lipgloss.Color("#8B95A3")
	colBorder = lipgloss.Color("#2C3747")
	colAmber  = lipgloss.Color("#F0A847")
	colTeal   = lipgloss.Color("#5CC8A8")
	colRed    = lipgloss.Color("#F77E7E")

	// Types, the same in results, the tree and the breadcrumb.
	colRepo    = lipgloss.Color("#7AA2F7")
	colMarket  = lipgloss.Color("#BB9AF7")
	colAgent   = lipgloss.Color("#F7768E")
	colCommand = lipgloss.Color("#9ECE6A")

	stTitle    = lipgloss.NewStyle().Foreground(colText).Bold(true)
	stText     = lipgloss.NewStyle().Foreground(colSoft)
	stMuted    = lipgloss.NewStyle().Foreground(colMuted)
	stAmber    = lipgloss.NewStyle().Foreground(colAmber)
	stTeal     = lipgloss.NewStyle().Foreground(colTeal)
	stError    = lipgloss.NewStyle().Foreground(colRed)
	stSelected = lipgloss.NewStyle().Foreground(colAmber).Bold(true)
	stRule     = lipgloss.NewStyle().Foreground(colBorder)
	stScope    = lipgloss.NewStyle().Foreground(colRepo)

	// The selected row of the panel you're not in: a grey caret on a
	// background, so it reads as "selected there", not "you are here".
	colSelBg        = lipgloss.Color("#263142")
	stInactive      = lipgloss.NewStyle().Background(colSelBg).Foreground(colSoft)
	stInactiveCaret = lipgloss.NewStyle().Background(colSelBg).Foreground(colMuted)
)

// caretWidth is the selection column on the far left of every list.
const caretWidth = 2

// selectedRow draws a list row with the caret column: an orange caret when
// its panel is active (the row keeps its colours; its name is drawn orange
// by the caller), a grey caret on a background when not. w includes the
// caret column.
func selectedRow(row string, w int, active bool) string {
	if active {
		return stAmber.Render("▶ ") + fit(row, w-caretWidth)
	}
	return stInactiveCaret.Render("▶ ") + stInactive.Render(fit(ansi.Strip(row), w-caretWidth))
}

// plainRow is an unselected list row, with an empty caret column.
func plainRow(row string, w int) string { return spaces(caretWidth) + fit(row, w-caretWidth) }

// fit cuts s to width w with "…" and pads it to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += spaces(pad)
	}
	return s
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// panel draws lines in a rounded border with title in its top edge: exactly
// len(lines)+2 rows of width w. The focused panel's border is amber.
func panel(title string, lines []string, w int, focused bool) []string {
	border := stRule
	if focused {
		border = stAmber
	}
	inner := max(0, w-4)
	title = ansi.Truncate(title, max(0, w-6), "…")
	top := border.Render("╭─ ") + title + " "
	top += border.Render(strings.Repeat("─", max(0, w-lipgloss.Width(top)-1)) + "╮")
	out := []string{fit(top, w)}
	for _, l := range lines {
		out = append(out, border.Render("│")+" "+fit(l, inner)+" "+border.Render("│"))
	}
	return append(out, border.Render("╰"+strings.Repeat("─", max(0, w-2))+"╯"))
}

// starStyle tints a star count by size: under 100 muted, under 10k soft,
// otherwise bright.
func starStyle(n int) lipgloss.Style {
	switch {
	case n < 100:
		return stMuted
	case n < 10_000:
		return stText
	}
	return lipgloss.NewStyle().Foreground(colText)
}
