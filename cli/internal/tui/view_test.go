package tui

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

// golden compares the screen with testdata/<name>.golden.
func golden(t *testing.T, name string, m *Model) {
	t.Helper()
	got := view(m)
	path := filepath.Join("testdata", name+".golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run Golden -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from %s:\n%s", name, path, got)
	}
}

func TestGoldenViews(t *testing.T) {
	long := strings.Repeat("very-long-name-", 8)
	setup := func(f fakes) {
		withDetails(f)
		f.c.counts = map[string]int{"skill": 1204, "agent": 312, "command": 98, "plugin": 410}
		f.p.packs = []Pack{{Name: "csharp-sdlc", Description: "C# lifecycle", Items: []string{tddID, "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md", "plugin:anthropics/claude-plugins-official#stripe"}}, {Name: "docs", Description: "Writing"}}
		tdd := f.s.results["tdd"]
		tdd.Hits[0].Stars, tdd.Hits[0].UsedBy = 31234, 3
		tdd.Hits[0].Description = "Test-driven development: write the failing test first, then the code that makes it pass, then tidy up."
		tdd.Hits[1].Stars = 842
		f.s.results["long"] = Results{Hits: []Hit{{ID: "skill:o/r:skills/" + long, Type: "copilot-plugin", Name: long, Score: "99", Description: strings.Repeat("words ", 40)}}, Found: 1}
	}
	cases := map[string]func(t *testing.T) *Model{
		"catalog-search": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			typeText(t, m, "tdd")
			press(t, m, "down")
			return m
		},
		"catalog-tree": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			typeText(t, m, "tdd")
			press(t, m, "down", "left", "tab")
			return m
		},
		"deep-tree-scroll": func(t *testing.T) *Model {
			m, f := newTestWith(t, setup)
			p := f.c.details["plugin:o/r:p"]
			for i := range 40 {
				id := fmt.Sprintf("agent:o/r:agents/a%02d.md", i)
				p.Children = append(p.Children, Ref{ID: id, Type: "agent", Name: fmt.Sprintf("agent-%02d", i), Description: "An agent"})
				f.c.details[id] = Node{ID: id, Type: "agent", Name: fmt.Sprintf("agent-%02d", i), Description: "An agent", Parents: []Ref{{ID: "plugin:o/r:p", Type: "plugin", Name: "p"}}}
			}
			f.c.details["plugin:o/r:p"] = p
			typeText(t, m, "tdd")
			press(t, m, "down", "tab")
			for range 25 {
				press(t, m, "down")
			}
			return m
		},
		"also-in": func(t *testing.T) *Model {
			m, f := newTestWith(t, setup)
			tdd := f.c.details["skill:o/r:skills/tdd"]
			tdd.Parents = append(tdd.Parents, Ref{ID: "plugin:o/r:p2", Type: "plugin", Name: "p2"}, Ref{ID: "plugin:x/y:q", Type: "plugin", Name: "q"})
			f.c.details["skill:o/r:skills/tdd"] = tdd
			typeText(t, m, "tdd")
			press(t, m, "down", "tab")
			return m
		},
		"catalog-no-color": func(t *testing.T) *Model {
			t.Setenv("NO_COLOR", "1")
			m, _ := newTestWith(t, setup)
			typeText(t, m, "tdd")
			press(t, m, "down")
			return m
		},
		"catalog-narrow": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
			typeText(t, m, "long")
			press(t, m, "down")
			return m
		},
		"catalog-wide": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
			typeText(t, m, "tdd")
			press(t, m, "down")
			return m
		},
		"install-form-target": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			press(t, m, "down", "2", "i")
			return m
		},
		"install-form-gascity": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			press(t, m, "down", "2", "i", "down", "enter", "enter")
			return m
		},
		"install-form-confirm": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			press(t, m, "down", "2", "i", "enter", "enter", "enter")
			return m
		},
		"installs": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withInstalls)
			press(t, m, "down", "3")
			return m
		},
		"installs-global": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withInstalls)
			press(t, m, "down", "3", "g")
			return m
		},
		"installs-empty": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			press(t, m, "down", "3")
			return m
		},
		"marketplace": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withMarket)
			press(t, m, "down", "4", "down")
			return m
		},
		"marketplace-filtered": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withMarket)
			press(t, m, "down", "4", "/")
			typeText(t, m, "review")
			press(t, m, "enter")
			return m
		},
		"marketplace-long-list": func(t *testing.T) *Model {
			m, _ := newTestWith(t, func(f fakes) {
				f.mk.repos = []Repo{
					{Repo: "o/one", List: "AI · Agentic Engineering Methods and Practices for Teams", Stars: 900, Items: 4},
					{Repo: "o/two", List: "AI · Agentic Engineering Methods and Practices for Teams", Stars: 12, Items: 2},
				}
			})
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
			press(t, m, "down", "4")
			return m
		},
		"marketplace-loading": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withMarket)
			press(t, m, "down")
			m.Update(key("4"))
			return m
		},
		"marketplace-error": func(t *testing.T) *Model {
			m, _ := newTestWith(t, func(f fakes) {
				f.mk.listErr = errors.New("fetch https://example.test/lists.toml: 503 Service Unavailable")
			})
			press(t, m, "down", "4")
			return m
		},
		"marketplace-narrow": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withMarket)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
			press(t, m, "down", "4")
			return m
		},
		"external-modal": func(t *testing.T) *Model {
			m, _ := newTestWith(t, withRefs)
			typeText(t, m, "cf")
			press(t, m, "down", "e")
			return m
		},
		"picker": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			typeText(t, m, "tdd")
			press(t, m, "down", "p")
			return m
		},
		"install": func(t *testing.T) *Model {
			m, f := newTestWith(t, setup)
			f.b.lines = []string{"✓ csharp-sdlc: 1 item(s), 0 plugin dependency(ies)", "validate: ok"}
			typeText(t, m, "tdd")
			press(t, m, "down")
			drain(t, m, m.startBuild())
			return m
		},
		"local-banner": func(t *testing.T) *Model {
			m, f := newTestWith(t, setup)
			f.s.mode, f.s.notice = "local", "server off: keyword search only — run `ipm catalog serve` and `ipm catalog rebuild` for semantic search"
			typeText(t, m, "tdd")
			return m
		},
		"help-catalog": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			press(t, m, "down", "?")
			return m
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) { golden(t, name, build(t)) })
	}
}

// panels splits a screen into its bordered panels, top to bottom.
func panels(screen string) [][]string {
	var out [][]string
	var cur []string
	for _, l := range strings.Split(screen, "\n") {
		plain := ansi.Strip(l)
		switch {
		case strings.HasPrefix(plain, "╭"):
			cur = []string{l}
		case strings.HasPrefix(plain, "╰") && cur != nil:
			out = append(out, append(cur, l))
			cur = nil
		case cur != nil:
			cur = append(cur, l)
		}
	}
	return out
}

func TestPanelsByWeight(t *testing.T) {
	m, _ := treeTest(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	ps := panels(m.View().Content)
	// body = 40 - title, tabs (2), stats - tray (2) - status - hints = 32; results 40%.
	if len(ps) != 2 || len(ps[0]) != 12 || len(ps[1]) != 20 {
		t.Fatalf("panel heights %d, want 12 and 20", lens(ps))
	}
	for _, p := range ps {
		for _, l := range p {
			if w := lipgloss.Width(l); w != 120 {
				t.Errorf("width %d: %q", w, ansi.Strip(l))
			}
		}
	}
}

func lens(ps [][]string) []int {
	var out []int
	for _, p := range ps {
		out = append(out, len(p))
	}
	return out
}

const amberCode = "38;2;240;168;71"

func TestFocusedPanelBorder(t *testing.T) {
	m, _ := treeTest(t)
	ps := panels(m.View().Content)
	if !strings.Contains(ps[0][0], amberCode) || strings.Contains(ps[1][0], amberCode) {
		t.Errorf("results focused: the results border should be amber, the tree's not")
	}
	press(t, m, "tab")
	ps = panels(m.View().Content)
	if strings.Contains(ps[0][0], amberCode) || !strings.Contains(ps[1][0], amberCode) {
		t.Errorf("tree mode: the tree border should be amber, the results' not")
	}
}

func TestResultRowsNarrow(t *testing.T) {
	m, _ := treeTest(t)
	row := func() string {
		for _, l := range panels(m.View().Content)[0] {
			if p := ansi.Strip(l); strings.Contains(p, "tdd-guard") {
				return p
			}
		}
		return ""
	}
	if r := row(); !strings.Contains(r, "o/r") {
		t.Errorf("at 120 columns a result row shows its repo: %q", r)
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	if r := row(); r == "" || strings.Contains(r, "o/r") {
		t.Errorf("at 90 columns a result row drops repo and stars: %q", r)
	}
}

func TestShortTerminalTreeWithoutChildren(t *testing.T) {
	m, _ := treeTest(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 22})
	if strings.Contains(strings.Join(panels(view(m))[1], "\n"), "helper") {
		t.Errorf("a short terminal's tree shows no children:\n%s", view(m))
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	if !strings.Contains(strings.Join(panels(view(m))[1], "\n"), "helper") {
		t.Errorf("a taller terminal's tree shows children:\n%s", view(m))
	}
}

func TestNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m, _ := treeTest(t)
	content := m.View().Content
	if strings.Contains(content, "\x1b[") {
		t.Errorf("NO_COLOR output has escape codes")
	}
	t.Setenv("NO_COLOR", "")
	c, _ := treeTest(t)
	if content != ansi.Strip(c.View().Content) {
		t.Errorf("NO_COLOR should only remove colour")
	}
}

func TestResultsUsedByCell(t *testing.T) {
	m, f := newTest(t)
	f.s.results["cf"] = Results{Hits: []Hit{
		{ID: "skill:b/s:x", Type: "skill", Name: "agents-sdk", Repo: "b/s", UsedBy: 3, Score: "90"},
		{ID: "plugin:a/m#cf", Type: "plugin", Name: "cloudflare", Repo: "a/m", External: true, Score: "80"},
	}, Found: 2}
	typeText(t, m, "cf")
	res := strings.Join(panels(view(m))[0], "\n")
	if !strings.Contains(res, "↗3") || !strings.Contains(res, "↗cloudflare") {
		t.Errorf("results:\n%s", res)
	}
}

const bgCode = "48;2;" // any background colour

// caretLine is the line of a panel holding its ▶ caret.
func caretLine(t *testing.T, lines []string) string {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "▶") {
			return l
		}
	}
	t.Fatalf("no caret in:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	return ""
}

func TestCaretActiveAndInactive(t *testing.T) {
	m, _ := treeTest(t)
	check := func(active, inactive string, where string) {
		t.Helper()
		if !strings.Contains(active, amberCode) || strings.Contains(active, bgCode) {
			t.Errorf("%s: the active panel's caret row is orange, without a background:\n%q", where, active)
		}
		if strings.Contains(inactive, amberCode) || !strings.Contains(inactive, bgCode) {
			t.Errorf("%s: the inactive panel's caret row is grey on a background:\n%q", where, inactive)
		}
	}
	ps := panels(m.View().Content)
	check(caretLine(t, ps[0][1:len(ps[0])-1]), caretLine(t, ps[1][1:len(ps[1])-1]), "search mode")
	press(t, m, "tab")
	ps = panels(m.View().Content)
	check(caretLine(t, ps[1][1:len(ps[1])-1]), caretLine(t, ps[0][1:len(ps[0])-1]), "tree mode")
}

func TestCaretOnTheFarLeft(t *testing.T) {
	m, _ := treeTest(t)
	for _, p := range panels(view(m)) {
		for _, l := range p[1 : len(p)-1] {
			plain := strings.TrimPrefix(l, "│ ")
			if i := strings.Index(plain, "▶"); i > 0 {
				t.Errorf("the caret belongs in the first column: %q", l)
			}
		}
	}
	if v := view(m); strings.Contains(v, "─▶") || strings.Contains(v, "▶tdd") {
		t.Errorf("no inline arrows in the tree:\n%s", v)
	}
}

func TestUpDownNeverChangesTreeLevel(t *testing.T) {
	m, _ := treeTest(t)
	press(t, m, "tab")
	depth := len(m.tree.Path)
	press(t, m, "down", "up", "down")
	if len(m.tree.Path) != depth {
		t.Errorf("↑↓ moved between levels: depth %d → %d", depth, len(m.tree.Path))
	}
	press(t, m, "esc")
	sel := m.sel
	press(t, m, "right", "left", "left")
	if m.sel != sel || m.focus != focusResults {
		t.Errorf("←→ moved the result selection or focus: sel %d focus %v", m.sel, m.focus)
	}
}

func TestResultsDescriptionColumn(t *testing.T) {
	m, f := newTest(t)
	f.s.results["d"] = Results{Hits: []Hit{{ID: "skill:o/r:x", Type: "skill", Name: "x", Repo: "o/r", Description: "Test first, always", Score: "9"}}, Found: 1}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	typeText(t, m, "d")
	if !strings.Contains(strings.Join(panels(view(m))[0], "\n"), "Test first, always") {
		t.Errorf("wide results show the description:\n%s", view(m))
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	if strings.Contains(strings.Join(panels(view(m))[0], "\n"), "Test first") {
		t.Errorf("narrow results drop the description first:\n%s", view(m))
	}
}
