package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTableAlignsAndTruncates(t *testing.T) {
	cols := []column{{Title: "TYPE", Min: 4, Max: 6}, {Title: "NAME", Min: 5}, {Title: "N", Max: 4, Right: true}}
	rows := [][]string{
		{"skill", strings.Repeat("x", 40), "12"},
		{"agent", "short", "3"},
	}
	off := 0
	lines := table(cols, rows, nil, 0, &off, 30, 5, true)
	if len(lines) != 5 {
		t.Fatalf("%d lines, want 5", len(lines))
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w != 30 {
			t.Errorf("width %d: %q", w, ansi.Strip(l))
		}
	}
	if !strings.Contains(ansi.Strip(lines[0]), "TYPE") || !strings.Contains(ansi.Strip(lines[0]), "NAME") {
		t.Errorf("the header is the first line: %q", ansi.Strip(lines[0]))
	}
	if !strings.Contains(ansi.Strip(lines[1]), "…") {
		t.Errorf("the long cell should end in …: %q", ansi.Strip(lines[1]))
	}
	if !strings.HasPrefix(ansi.Strip(lines[1]), "▶ ") {
		t.Errorf("the selected row carries the caret: %q", ansi.Strip(lines[1]))
	}
	if !strings.HasSuffix(strings.TrimRight(ansi.Strip(lines[2]), " "), "   3") {
		t.Errorf("numbers align right: %q", ansi.Strip(lines[2]))
	}
}

func TestTableScrollsToSelection(t *testing.T) {
	var rows [][]string
	for i := range 50 {
		rows = append(rows, []string{"row" + commas(i)})
	}
	off := 0
	lines := table([]column{{Title: "NAME"}}, rows, nil, 30, &off, 40, 10, true)
	if off == 0 {
		t.Error("offset should move")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "row30") {
		t.Errorf("the selected row is visible:\n%s", strings.Join(lines, "\n"))
	}
	table([]column{{Title: "NAME"}}, rows, nil, 0, &off, 40, 10, true)
	if off != 0 {
		t.Errorf("scrolling back up: offset %d", off)
	}
}

func TestTableOpenColumnTakesSlack(t *testing.T) {
	cols := []column{{Title: "A", Min: 1, Max: 3}, {Title: "LIST", Min: 6}, {Title: "N", Min: 2, Max: 4}}
	rows := [][]string{{"x", "a fairly long list name", "1"}}
	got := columnWidths(cols, rows, 60)
	if got[1] < len(rows[0][1]) {
		t.Errorf("the open column should take the slack, not be cut: %v", got)
	}
}

func TestTableSpanRowKeepsItsEnd(t *testing.T) {
	cols := []column{{Title: "A", Min: 1, Max: 1}, {Title: "B", Min: 4, Max: 6}, {Title: "C", Min: 4, Max: 6}}
	off := 0
	lines := table(cols, [][]string{{"── a long divider text (12)"}, {"x", "b", "c"}}, nil, 1, &off, 40, 3, true)
	if !strings.Contains(ansi.Strip(lines[1]), "(12)") {
		t.Errorf("a row of one cell spans the table: %q", ansi.Strip(lines[1]))
	}
}
