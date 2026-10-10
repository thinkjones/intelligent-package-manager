package tui

import (
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
)

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
