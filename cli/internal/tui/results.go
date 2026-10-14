package tui

import (
	"strconv"

	"charm.land/lipgloss/v2"
)

func lipglossWidth(s string) int { return lipgloss.Width(s) }

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
