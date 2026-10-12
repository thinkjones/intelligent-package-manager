package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var ckNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func ckLists() []CuratedList {
	return []CuratedList{
		{Name: "AI · Agentic Engineering Methods", Repos: []CuratedRepo{
			{Repo: "obra/superpowers", Stars: 52100, Language: "Shell", LastPush: ckNow.AddDate(0, 0, -3), Description: "An agentic skills framework and software development methodology", Topics: []string{"skills", "tdd"}},
			{Repo: "nizos/tdd-guard", Stars: 1800, Language: "TypeScript", LastPush: ckNow.AddDate(0, -2, -5), Description: "Automated TDD enforcement for Claude Code"},
		}},
		{Name: "AI · Skills, Plugins & Registries", Repos: []CuratedRepo{
			{Repo: "anthropics/skills", Stars: 41200, Language: "Python", LastPush: ckNow.AddDate(0, 0, -1), Description: "Public repository for Agent Skills"},
			{Repo: "obra/superpowers", Stars: 52100, Language: "Shell", LastPush: ckNow.AddDate(0, 0, -3), Description: "An agentic skills framework and software development methodology", Topics: []string{"skills", "tdd"}},
			{Repo: "someone/awesome-skills", Stars: 8200, Description: "A curated list of Claude skills, resources and tools for building agents"},
		}},
		{Name: "AI · Diagrams", Repos: []CuratedRepo{
			{Repo: "Agents365-ai/drawio-skill", Stars: 420, Language: "Python", LastPush: ckNow.AddDate(-1, -1, 0), Description: "Draw.io diagrams from Claude"},
		}},
	}
}

func newCk(t *testing.T, ticked, locked map[string]bool, w int) *Checklist {
	t.Helper()
	c := NewChecklist(ckLists(), ticked, locked, ckNow)
	c.Update(tea.WindowSizeMsg{Width: w, Height: 20})
	return c
}

// ckPress sends keys and reports whether the last one asked to quit.
func ckPress(t *testing.T, c *Checklist, keys ...string) (quit bool) {
	t.Helper()
	for _, k := range keys {
		_, cmd := c.Update(key(k))
		quit = false
		if cmd != nil {
			_, quit = cmd().(tea.QuitMsg)
		}
	}
	return quit
}

func ckView(c *Checklist) string { return ansi.Strip(c.View().Content) }

// ckLine is the screen line containing s.
func ckLine(t *testing.T, c *Checklist, s string) string {
	t.Helper()
	for _, l := range strings.Split(ckView(c), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	t.Fatalf("no line with %q in:\n%s", s, ckView(c))
	return ""
}

func TestChecklistTickList(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, " ")
	if l := ckLine(t, c, "Agentic Engineering"); !strings.Contains(l, "[x]") {
		t.Errorf("list row = %q, want [x]", l)
	}
	if got := strings.Join(c.Picked(), ","); got != "obra/superpowers,nizos/tdd-guard" {
		t.Errorf("Picked = %s", got)
	}
	ckPress(t, c, " ")
	if l := ckLine(t, c, "Agentic Engineering"); !strings.Contains(l, "[ ]") {
		t.Errorf("list row = %q, want [ ]", l)
	}
	if len(c.Picked()) != 0 {
		t.Errorf("Picked = %v, want none", c.Picked())
	}
}

func TestChecklistPartial(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, "right", "down", "down", " ")
	if got := strings.Join(c.Picked(), ","); got != "nizos/tdd-guard" {
		t.Errorf("Picked = %s, want nizos/tdd-guard", got)
	}
	if l := ckLine(t, c, "Agentic Engineering"); !strings.Contains(l, "[~]") {
		t.Errorf("list row = %q, want [~]", l)
	}
	if l := ckLine(t, c, "nizos/tdd-guard"); !strings.Contains(l, "[x]") {
		t.Errorf("repo row = %q, want [x]", l)
	}
}

func TestChecklistRepoInTwoLists(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, "right", "down", " ") // obra/superpowers in the first list
	ckPress(t, c, "down", "down", "right")
	if l := ckLine(t, c, "Skills, Plugins"); !strings.Contains(l, "[~]") {
		t.Errorf("second list = %q, want [~] (superpowers ticked there too)", l)
	}
	if got := strings.Join(c.Picked(), ","); got != "obra/superpowers" {
		t.Errorf("Picked = %s, want it once", got)
	}
}

func TestChecklistAllNone(t *testing.T) {
	locked := map[string]bool{"anthropics/skills": true}
	c := newCk(t, nil, locked, 120)
	ckPress(t, c, "down", "a")
	if got := strings.Join(c.Picked(), ","); got != "obra/superpowers,someone/awesome-skills" {
		t.Errorf("after a: Picked = %s", got)
	}
	ckPress(t, c, "n")
	if len(c.Picked()) != 0 {
		t.Errorf("after n: Picked = %v", c.Picked())
	}
	ckPress(t, c, "right")
	if l := ckLine(t, c, "anthropics/skills"); !strings.Contains(l, "✓") {
		t.Errorf("locked row = %q, want ✓", l)
	}
}

func TestChecklistLocked(t *testing.T) {
	c := newCk(t, nil, map[string]bool{"anthropics/skills": true}, 120)
	ckPress(t, c, "down", "right", "down", " ")
	if l := ckLine(t, c, "anthropics/skills"); !strings.Contains(l, "✓") {
		t.Errorf("locked row = %q, want ✓", l)
	}
	if len(c.Picked()) != 0 {
		t.Errorf("Picked = %v, want none (locked rows aren't picks)", c.Picked())
	}
}

func TestChecklistFilter(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, "/", "T", "D", "D", "enter")
	v := ckView(c)
	for _, want := range []string{"nizos/tdd-guard", "obra/superpowers", "Agentic Engineering", "Skills, Plugins"} {
		if !strings.Contains(v, want) {
			t.Errorf("filtered view lacks %q:\n%s", want, v)
		}
	}
	for _, gone := range []string{"anthropics/skills", "AI · Diagrams", "awesome-skills"} {
		if strings.Contains(v, gone) {
			t.Errorf("filtered view still shows %q:\n%s", gone, v)
		}
	}
	ckPress(t, c, "esc")
	if v := ckView(c); !strings.Contains(v, "AI · Diagrams") || strings.Contains(v, "nizos/tdd-guard") {
		t.Errorf("after esc, want the collapsed lists back:\n%s", v)
	}
}

func TestChecklistNothingTicked(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	if ckPress(t, c, "enter") || c.Confirmed() {
		t.Fatal("enter with nothing ticked must not continue")
	}
	if !strings.Contains(ckView(c), "tick at least one repo") {
		t.Errorf("want the nothing-ticked message:\n%s", ckView(c))
	}
}

func TestChecklistConfirm(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, " ", "enter")
	if !strings.Contains(ckView(c), "2 repos to follow — y continue · esc back") {
		t.Errorf("want the confirm line:\n%s", ckView(c))
	}
	ckPress(t, c, "esc")
	if strings.Contains(ckView(c), "repos to follow") {
		t.Error("esc should leave the confirm line")
	}
	ckPress(t, c, "enter")
	if !ckPress(t, c, "y") || !c.Confirmed() {
		t.Error("y should confirm and quit")
	}
}

func TestChecklistQuit(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, " ")
	if !ckPress(t, c, "q") || c.Confirmed() {
		t.Error("q should quit without confirming")
	}
}

func TestAge(t *testing.T) {
	for _, tc := range []struct {
		then time.Time
		want string
	}{
		{ckNow.AddDate(0, 0, -3), "3d"},
		{ckNow.AddDate(0, 0, -65), "2mo"},
		{ckNow.AddDate(0, 0, -400), "1y"},
		{time.Time{}, "—"},
	} {
		if got := age(ckNow, tc.then); got != tc.want {
			t.Errorf("age(%v) = %q, want %q", tc.then, got, tc.want)
		}
	}
}

func goldenString(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
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

func TestGoldenChecklist(t *testing.T) {
	ticked := map[string]bool{"obra/superpowers": true, "nizos/tdd-guard": true}
	cases := map[string]func() *Checklist{
		"checklist-collapsed": func() *Checklist { return newCk(t, ticked, nil, 120) },
		"checklist-expanded": func() *Checklist {
			c := newCk(t, ticked, map[string]bool{"anthropics/skills": true}, 120)
			ckPress(t, c, "down", "right", "down")
			return c
		},
		"checklist-filtered": func() *Checklist {
			c := newCk(t, nil, nil, 120)
			ckPress(t, c, "/", "s", "k", "i", "l", "l", "s", "enter")
			return c
		},
		"checklist-narrow": func() *Checklist {
			c := newCk(t, ticked, nil, 80)
			ckPress(t, c, "right")
			return c
		},
		"checklist-wide-chars": func() *Checklist {
			lists := ckLists()
			lists[0].Repos[0].Description = "🚀 超能力 スキル — an agentic skills framework 🧪 with emoji and 中文 everywhere in the line"
			c := NewChecklist(lists, nil, nil, ckNow)
			c.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
			ckPress(t, c, "right")
			return c
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			c := mk()
			got := ckView(c)
			w := c.width
			for i, l := range strings.Split(got, "\n") {
				if lw := ansi.StringWidth(l); lw > w {
					t.Errorf("line %d is %d cells wide, more than %d: %q", i, lw, w, l)
				}
			}
			goldenString(t, name, got)
		})
	}
}

func TestChecklistContinueWithOnlyLocked(t *testing.T) {
	c := newCk(t, nil, map[string]bool{"anthropics/skills": true}, 120)
	ckPress(t, c, "enter")
	if !strings.Contains(ckView(c), "0 new repos — y continue with the 1 you follow · esc back") {
		t.Errorf("want the continue line:\n%s", ckView(c))
	}
	if !ckPress(t, c, "y") || !c.Confirmed() || len(c.Picked()) != 0 {
		t.Error("y should confirm with no new picks")
	}
}

func TestChecklistFilterTicksOnlyMatches(t *testing.T) {
	c := newCk(t, nil, nil, 120)
	ckPress(t, c, "/", "g", "u", "a", "r", "d", "enter", " ")
	if got := strings.Join(c.Picked(), ","); got != "nizos/tdd-guard" {
		t.Errorf("Picked = %s, want only the visible match", got)
	}
	ckPress(t, c, " ")
	if len(c.Picked()) != 0 {
		t.Errorf("second space should untick the matches, Picked = %v", c.Picked())
	}
}

func TestChecklistDetailLine(t *testing.T) {
	lists := ckLists()
	lists[0].Repos[0].Homepage = "https://superpowers.example"
	lists[0].Repos[1].Homepage = "https://tdd-guard.example"
	c := NewChecklist(lists, nil, nil, ckNow)
	c.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	if strings.Contains(ckView(c), "topics:") {
		t.Errorf("a list row has no detail:\n%s", ckView(c))
	}
	ckPress(t, c, "right", "down")
	if l := ckLine(t, c, "topics:"); strings.TrimSpace(l) != "topics: skills, tdd · https://superpowers.example" {
		t.Errorf("detail = %q", l)
	}
	ckPress(t, c, "down")
	if v := ckView(c); strings.Contains(v, "topics:") || strings.TrimSpace(ckLine(t, c, "https://tdd-guard.example")) != "https://tdd-guard.example" {
		t.Errorf("a repo without topics shows its homepage alone:\n%s", v)
	}
	// the filter line wins the row while filtering
	ckPress(t, c, "/", "t", "d", "d")
	if v := ckView(c); strings.Contains(v, "https://") {
		t.Errorf("no detail while typing a filter:\n%s", v)
	}
	// and a narrow screen cuts the detail rather than wrapping
	n := NewChecklist(lists, nil, nil, ckNow)
	n.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	ckPress(t, n, "right", "down")
	for _, l := range strings.Split(ckView(n), "\n") {
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line wider than the screen: %q", l)
		}
	}
}
