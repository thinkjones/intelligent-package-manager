package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
		f.p.packs = []Pack{{Name: "csharp-sdlc", Description: "C# lifecycle", Items: []string{tddID, "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md", "plugin:anthropics/claude-plugins-official#stripe"}}, {Name: "docs", Description: "Writing"}}
		f.s.results["long"] = Results{Hits: []Hit{{ID: "skill:o/r:skills/" + long, Type: "copilot-plugin", Name: long, Score: "99", Description: strings.Repeat("words ", 40)}}, Found: 1}
	}
	cases := map[string]func(t *testing.T) *Model{
		"main": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			typeText(t, m, "tdd")
			press(t, m, "down")
			return m
		},
		"narrow-long": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			typeText(t, m, "long")
			press(t, m, "down")
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
			f.i.lines = []string{"✓ csharp-sdlc: 1 item(s), 0 plugin dependency(ies)", "validate: ok"}
			typeText(t, m, "tdd")
			press(t, m, "down", "i")
			return m
		},
		"local-banner": func(t *testing.T) *Model {
			m, f := newTestWith(t, setup)
			f.s.mode, f.s.notice = "local", "server off: keyword search only — run `ipm serve` and `ipm load` for semantic search"
			typeText(t, m, "tdd")
			return m
		},
		"help": func(t *testing.T) *Model {
			m, _ := newTestWith(t, setup)
			press(t, m, "down", "?")
			return m
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) { golden(t, name, build(t)) })
	}
}
