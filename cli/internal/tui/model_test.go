package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// key is a key press as Bubble Tea delivers it: a single character, or a
// name such as "enter", "ctrl+c".
func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	if c, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(c[0]), Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// drain runs cmd and every command its messages lead to, feeding each
// message back into m, and reports whether any of them asked to quit.
func drain(t *testing.T, m *Model, cmd tea.Cmd) (quit bool) {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 1000 {
			t.Fatal("drain: commands never settle")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tea.QuitMsg:
			quit = true
		default:
			_, next := m.Update(msg)
			queue = append(queue, next)
		}
	}
	return quit
}

// press sends keys to m one at a time, running the commands each produces.
func press(t *testing.T, m *Model, keys ...string) (quit bool) {
	t.Helper()
	for _, k := range keys {
		_, cmd := m.Update(key(k))
		if drain(t, m, cmd) {
			quit = true
		}
	}
	return quit
}

// typeText types s into whatever has focus.
func typeText(t *testing.T, m *Model, s string) {
	t.Helper()
	for _, r := range s {
		press(t, m, string(r))
	}
}

// view is the screen as text, without styling.
func view(m *Model) string { return ansi.Strip(m.View().Content) }

type fakeSearcher struct {
	queries []string
	filters []Filter
	results map[string]Results
	errs    map[string]error
	mode    string
	notice  string
	checks  int
}

func (f *fakeSearcher) Search(_ context.Context, q string, flt Filter) (Results, error) {
	f.queries = append(f.queries, q)
	f.filters = append(f.filters, flt)
	if err := f.errs[q]; err != nil {
		return Results{}, err
	}
	return f.results[q], nil
}
func (f *fakeSearcher) Mode() string   { return f.mode }
func (f *fakeSearcher) Notice() string { return f.notice }
func (f *fakeSearcher) Recheck()       { f.checks++ }

type fakeCatalog struct {
	details map[string]Detail
	asked   []string
	err     error
}

func (f *fakeCatalog) Detail(id string) (Detail, error) {
	f.asked = append(f.asked, id)
	if f.err != nil {
		return Detail{}, f.err
	}
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return Detail{ID: id, Name: id}, nil
}

type fakePacks struct {
	scope   string
	packs   []Pack
	listErr error
	addErr  error
	calls   []string
}

func (f *fakePacks) Scope() string { return f.scope }
func (f *fakePacks) List() ([]Pack, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]Pack(nil), f.packs...), nil
}
func (f *fakePacks) find(name string) *Pack {
	for i := range f.packs {
		if f.packs[i].Name == name {
			return &f.packs[i]
		}
	}
	return nil
}
func (f *fakePacks) Create(name, description string) error {
	f.calls = append(f.calls, fmt.Sprintf("create %s %q", name, description))
	f.packs = append(f.packs, Pack{Name: name, Description: description})
	return nil
}
func (f *fakePacks) Add(pack string, ids ...string) error {
	f.calls = append(f.calls, "add "+pack+" "+strings.Join(ids, " "))
	if f.addErr != nil {
		return f.addErr
	}
	p := f.find(pack)
	p.Items = append(p.Items, ids...)
	return nil
}
func (f *fakePacks) Remove(pack string, ids ...string) error {
	f.calls = append(f.calls, "remove "+pack+" "+strings.Join(ids, " "))
	p := f.find(pack)
	for _, id := range ids {
		for i, it := range p.Items {
			if it == id {
				p.Items = append(p.Items[:i], p.Items[i+1:]...)
				break
			}
		}
	}
	return nil
}

type fakeInstaller struct {
	lines []string
	err   error
	// gate, when set, holds Install until it is closed.
	gate chan struct{}
}

func (f *fakeInstaller) Install(_ context.Context, out io.Writer) error {
	for _, l := range f.lines {
		fmt.Fprintln(out, l)
	}
	if f.gate != nil {
		<-f.gate
	}
	return f.err
}

func hits(names ...string) []Hit {
	var out []Hit
	for _, n := range names {
		out = append(out, Hit{ID: "skill:o/r:skills/" + n, Type: "skill", Name: n, Repo: "o/r", Score: "50"})
	}
	return out
}

type fakes struct {
	s *fakeSearcher
	c *fakeCatalog
	p *fakePacks
	i *fakeInstaller
}

func newTest(t *testing.T) (*Model, fakes) {
	t.Helper()
	return newTestWith(t, nil)
}

// newTestWith lets setup change the fakes before the model is built (packs
// are read once at start).
func newTestWith(t *testing.T, setup func(fakes)) (*Model, fakes) {
	t.Helper()
	f := fakes{
		s: &fakeSearcher{mode: "hybrid", results: map[string]Results{
			"tdd": {Hits: hits("tdd", "tdd-guard"), Found: 100},
		}, errs: map[string]error{}},
		c: &fakeCatalog{details: map[string]Detail{}},
		p: &fakePacks{scope: "project /work"},
		i: &fakeInstaller{},
	}
	if setup != nil {
		setup(f)
	}
	m := New(Deps{Searcher: f.s, Catalog: f.c, Packs: f.p, Installer: f.i})
	m.debounce = 0
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, f
}

func TestTypingSearches(t *testing.T) {
	m, f := newTest(t)
	typeText(t, m, "tdd")
	if len(f.s.queries) == 0 || f.s.queries[len(f.s.queries)-1] != "tdd" {
		t.Fatalf("queries %v", f.s.queries)
	}
	if len(m.res.Hits) != 2 || m.sel != 0 {
		t.Errorf("results %v sel %d", m.res.Hits, m.sel)
	}
	if !strings.Contains(view(m), "2 of 100") {
		t.Errorf("want the count in the view:\n%s", view(m))
	}
}

func TestStaleSearchDropped(t *testing.T) {
	m, _ := newTest(t)
	m.seq = 2
	m.Update(searchDone{seq: 2, res: Results{Hits: hits("new"), Found: 1}})
	m.Update(searchDone{seq: 1, res: Results{Hits: hits("old"), Found: 1}})
	if len(m.res.Hits) != 1 || m.res.Hits[0].Name != "new" {
		t.Errorf("a stale search replaced newer results: %v", m.res.Hits)
	}
}

func TestEmptyQueryClears(t *testing.T) {
	m, f := newTest(t)
	f.s.results["x"] = Results{Hits: hits("x"), Found: 1}
	typeText(t, m, "x")
	n := len(f.s.queries)
	press(t, m, "backspace")
	if len(m.res.Hits) != 0 || len(f.s.queries) != n {
		t.Errorf("results %v, searches %v", m.res.Hits, f.s.queries)
	}
}

func TestSearchErrorKeepsResults(t *testing.T) {
	m, f := newTest(t)
	typeText(t, m, "tdd")
	f.s.errs["tddx"] = errors.New("search server timed out")
	typeText(t, m, "x")
	if !strings.Contains(m.status, "timed out") || len(m.res.Hits) != 2 {
		t.Errorf("status %q results %d", m.status, len(m.res.Hits))
	}
	f.s.errs = map[string]error{}
	press(t, m, "backspace")
	if strings.Contains(m.status, "timed out") {
		t.Errorf("the next keystroke should clear the status: %q", m.status)
	}
}

func TestTypeFilterCycles(t *testing.T) {
	m, f := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down")
	var got []string
	for range 5 {
		press(t, m, "f")
		got = append(got, f.s.filters[len(f.s.filters)-1].Type)
	}
	if strings.Join(got, ",") != "skill,agent,command,plugin," {
		t.Errorf("filter cycle %v", got)
	}
}

func TestNavigation(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down")
	if m.focus != focusResults {
		t.Fatalf("down from search should focus results, focus=%v", m.focus)
	}
	press(t, m, "j", "j", "j")
	if m.sel != 1 {
		t.Errorf("j past the end: sel %d", m.sel)
	}
	press(t, m, "k", "up", "up")
	if m.sel != 0 {
		t.Errorf("k/up past the start: sel %d", m.sel)
	}
	press(t, m, "/")
	if m.focus != focusSearch {
		t.Errorf("/ should focus search")
	}
}

func TestQuit(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down")
	if !press(t, m, "q") {
		t.Error("q in results should quit")
	}
	m, _ = newTest(t)
	if !press(t, m, "ctrl+c") {
		t.Error("ctrl+c should quit")
	}
	m, _ = newTest(t)
	if press(t, m, "q") || m.input.Value() != "q" {
		t.Error("q in the search box is a letter")
	}
}

func withDetails(f fakes) {
	f.c.details["skill:o/r:skills/tdd"] = Detail{ID: "skill:o/r:skills/tdd", Type: "skill", Name: "tdd", Repo: "o/r", Stars: 31234,
		Description: "Test first.",
		Up:          []Neighbour{{ID: "plugin:o/r:p", Type: "plugin", Name: "p"}, {ID: "repo:o/r", Type: "repo", Name: "o/r"}},
		Down:        []Neighbour{{ID: "agent:o/r:agents/helper.md", Type: "agent", Name: "helper"}}}
	f.c.details["skill:o/r:skills/tdd-guard"] = Detail{ID: "skill:o/r:skills/tdd-guard", Type: "skill", Name: "tdd-guard", Repo: "o/r"}
}

func TestDetailFollowsSelection(t *testing.T) {
	m, f := newTest(t)
	withDetails(f)
	typeText(t, m, "tdd")
	if last := f.c.asked[len(f.c.asked)-1]; last != "skill:o/r:skills/tdd" {
		t.Fatalf("detail asked for %q", last)
	}
	v := view(m)
	for _, want := range []string{"skill:o/r:skills/tdd", "31.2k", "Test first.", "plugin", "p", "helper"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail lacks %q:\n%s", want, v)
		}
	}
	press(t, m, "down", "j")
	if last := f.c.asked[len(f.c.asked)-1]; last != "skill:o/r:skills/tdd-guard" {
		t.Errorf("moving should show the new selection, asked %q", last)
	}
}

func TestWalkNeighbours(t *testing.T) {
	m, f := newTest(t)
	withDetails(f)
	typeText(t, m, "tdd")
	press(t, m, "down", "right")
	if m.focus != focusDetail {
		t.Fatalf("right should focus the detail pane")
	}
	press(t, m, "down", "enter")
	if last := f.c.asked[len(f.c.asked)-1]; last != "repo:o/r" {
		t.Fatalf("enter should open the second neighbour, asked %q", last)
	}
	press(t, m, "backspace")
	if last := f.c.asked[len(f.c.asked)-1]; last != "skill:o/r:skills/tdd" || m.detail.ID != "skill:o/r:skills/tdd" {
		t.Errorf("backspace should go back, asked %q", last)
	}
	press(t, m, "left")
	if m.focus != focusResults {
		t.Errorf("left should return to the results")
	}
}

func TestNarrowHidesDetail(t *testing.T) {
	m, f := newTest(t)
	withDetails(f)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	typeText(t, m, "tdd")
	if strings.Contains(view(m), "Test first.") {
		t.Fatalf("a 90-column terminal should not show the detail pane:\n%s", view(m))
	}
	press(t, m, "down", "enter")
	if !strings.Contains(view(m), "Test first.") || strings.Contains(view(m), "tdd-guard") {
		t.Errorf("enter should show the detail full-width:\n%s", view(m))
	}
	press(t, m, "esc")
	if strings.Contains(view(m), "Test first.") || m.focus != focusResults {
		t.Errorf("esc should return to the results:\n%s", view(m))
	}
}

func TestTooSmall(t *testing.T) {
	m, _ := newTest(t)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	if !strings.Contains(view(m), "enlarge") {
		t.Errorf("want a request to enlarge the terminal:\n%s", view(m))
	}
	press(t, m, "x")
	if m.input.Value() != "" {
		t.Errorf("keys other than q and ctrl+c should do nothing")
	}
	if !press(t, m, "q") {
		t.Error("q should still quit")
	}
}

func TestDetailError(t *testing.T) {
	m, f := newTest(t)
	f.c.err = errors.New("catalog record broken")
	typeText(t, m, "tdd")
	if !strings.Contains(m.status, "catalog record broken") {
		t.Errorf("status %q", m.status)
	}
}

func withPacks(packs ...Pack) func(fakes) {
	return func(f fakes) { f.p.packs = packs }
}

const tddID = "skill:o/r:skills/tdd"

func TestActivePackAtStart(t *testing.T) {
	m, _ := newTestWith(t, withPacks(Pack{Name: "b"}, Pack{Name: "a", Items: []string{tddID}}))
	if m.active != "a" {
		t.Errorf("active %q, want the first by name", m.active)
	}
	if v := view(m); !strings.Contains(v, "project /work · a (1)") || !strings.Contains(v, "skill tdd") {
		t.Errorf("tray:\n%s", v)
	}
}

func TestAddAndMark(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	typeText(t, m, "tdd")
	press(t, m, "down", "a")
	if strings.Join(f.p.calls, ";") != "add a "+tddID {
		t.Fatalf("calls %v", f.p.calls)
	}
	v := view(m)
	if !strings.Contains(v, "✓ tdd") || !strings.Contains(v, "a (1) • not built") {
		t.Errorf("want the ✓ mark and the not-built tray:\n%s", v)
	}
}

func TestAddFromDetail(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	withDetails(f)
	typeText(t, m, "tdd")
	press(t, m, "down", "right", "a")
	if strings.Join(f.p.calls, ";") != "add a plugin:o/r:p" {
		t.Errorf("a in the detail pane adds the selected neighbour: %v", f.p.calls)
	}
}

func TestAddExistingItem(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a", Items: []string{tddID}}))
	typeText(t, m, "tdd")
	press(t, m, "down", "a")
	if len(f.p.calls) != 0 || !strings.Contains(m.status, "already in a") {
		t.Errorf("calls %v status %q", f.p.calls, m.status)
	}
}

func TestAddRefused(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.p.addErr = errors.New("hook:o/r:h comes only with its plugin: `ipm pack add a plugin:o/r:p`")
	typeText(t, m, "tdd")
	press(t, m, "down", "a")
	if !strings.Contains(m.status, "comes only with its plugin") || m.dirty {
		t.Errorf("status %q dirty %v", m.status, m.dirty)
	}
}

func TestRemove(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a", Items: []string{tddID, "agent:o/r:agents/x.md"}}))
	typeText(t, m, "tdd")
	press(t, m, "down", "d")
	if f.p.calls[0] != "remove a "+tddID {
		t.Fatalf("calls %v", f.p.calls)
	}
	press(t, m, "tab")
	if m.focus != focusTray {
		t.Fatalf("tab from results should focus the tray")
	}
	press(t, m, "d")
	if f.p.calls[1] != "remove a agent:o/r:agents/x.md" {
		t.Errorf("d in the tray removes its selected item: %v", f.p.calls)
	}
	press(t, m, "tab")
	if m.focus != focusSearch {
		t.Errorf("tab from the tray should return to search")
	}
}

func TestPickerSwitchAndCreate(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}, Pack{Name: "b"}))
	typeText(t, m, "tdd")
	press(t, m, "down", "p", "down", "enter")
	if m.active != "b" || m.picker != nil {
		t.Fatalf("active %q picker %v", m.active, m.picker)
	}
	press(t, m, "p", "n")
	typeText(t, m, "kit")
	press(t, m, "enter")
	typeText(t, m, "My kit")
	press(t, m, "enter")
	if f.p.calls[len(f.p.calls)-1] != `create kit "My kit"` || m.active != "kit" {
		t.Errorf("calls %v active %q", f.p.calls, m.active)
	}
	press(t, m, "p", "esc")
	if m.picker != nil || m.active != "kit" {
		t.Errorf("esc should close the picker without changes")
	}
}

func TestNoPacksAddOpensNew(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down", "a")
	if m.picker == nil || !m.picker.creating {
		t.Errorf("with no packs, a should open the picker on a new pack")
	}
}

func TestPacksErrorShown(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	typeText(t, m, "tdd")
	f.p.listErr = errors.New("ipm.toml: unknown key(s) packs.a.oops")
	press(t, m, "down", "a")
	if !strings.Contains(m.status, "unknown key") {
		t.Errorf("status %q", m.status)
	}
	if strings.Contains(view(m), "panic") {
		t.Error("the TUI must keep running")
	}
}
