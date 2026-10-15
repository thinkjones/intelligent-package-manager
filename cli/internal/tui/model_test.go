package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

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
	searchFocus(t, m)
	for _, r := range s {
		press(t, m, string(r))
	}
}

// searchFocus presses / when the Catalog results have focus, as a person
// does before typing a query; anywhere else it does nothing.
func searchFocus(t *testing.T, m *Model) {
	t.Helper()
	if m.tab == tabCatalog && m.focus == focusResults && m.inst == nil && m.form == nil && m.picker == nil && !m.help {
		press(t, m, "/")
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
	details map[string]Node
	asked   []string
	err     error
	counts  map[string]int
}

func (f *fakeCatalog) Counts() map[string]int { return f.counts }

func (f *fakeCatalog) Node(id string) (Node, error) {
	f.asked = append(f.asked, id)
	if f.err != nil {
		return Node{}, f.err
	}
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return Node{ID: id, Name: id}, nil
}

type fakePacks struct {
	scope   string
	packs   []Pack
	global  []Pack // the user scope's packs, listed when global is set
	listErr error
	addErr  error
	calls   []string
}

func (f *fakePacks) Scope() string { return f.scope }
func (f *fakePacks) List(global bool) ([]Pack, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := append([]Pack(nil), f.packs...)
	if global {
		for _, p := range f.global {
			p.Global = true
			out = append(out, p)
		}
	}
	return out, nil
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

type fakeBuilder struct {
	lines []string
	err   error
	// gate, when set, holds Build until it is closed.
	gate chan struct{}
}

func (f *fakeBuilder) Build(_ context.Context, out io.Writer) error {
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

type fakeInstaller struct {
	def       Target
	cleaned   *Target // what Plan returns as the target; nil returns t unchanged
	plan      []string
	planErr   error
	lines     []string
	err       error
	installed []string
	targets   []Target // every Target Install was given

	installs       []Install // List's records
	globalInstalls []Install // added when List is asked for global
	listErr        error
	listCalls      int
}

func (f *fakeInstaller) Targets() []string                { return []string{"claude", "gascity"} }
func (f *fakeInstaller) Default(pack, kind string) Target { return f.def }
func (f *fakeInstaller) List(global bool) ([]Install, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := append([]Install(nil), f.installs...)
	if global {
		out = append(out, f.globalInstalls...)
	}
	return out, nil
}
func (f *fakeInstaller) Plan(pack string, t Target) (Target, []string, error) {
	if f.cleaned != nil {
		t = *f.cleaned
	}
	return t, f.plan, f.planErr
}
func (f *fakeInstaller) Install(_ context.Context, pack string, t Target, out io.Writer) error {
	f.installed = append(f.installed, fmt.Sprintf("%s %s %s", pack, t.Scope, t.Path))
	f.targets = append(f.targets, t)
	for _, l := range f.lines {
		fmt.Fprintln(out, l)
	}
	return f.err
}

type fakeMarketplace struct {
	repos     []Repo // List's repos; Follow and Unfollow change Followed here
	listErr   error
	listCalls int
	followErr error
	followed  []string // Follow's and Unfollow's calls, "+repo" and "-repo"
	lines     []string
	syncErr   error
	syncs     int
	// gate, when set, holds Sync until it is closed.
	gate chan struct{}
}

func (f *fakeMarketplace) List() ([]Repo, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]Repo(nil), f.repos...), nil
}

func (f *fakeMarketplace) set(repo string, on bool) error {
	sign := "-"
	if on {
		sign = "+"
	}
	f.followed = append(f.followed, sign+repo)
	if f.followErr != nil {
		return f.followErr
	}
	for i := range f.repos {
		if f.repos[i].Repo == repo {
			f.repos[i].Followed = on
		}
	}
	return nil
}

func (f *fakeMarketplace) Follow(repo string) error   { return f.set(repo, true) }
func (f *fakeMarketplace) Unfollow(repo string) error { return f.set(repo, false) }
func (f *fakeMarketplace) Sync(_ context.Context, out io.Writer) error {
	f.syncs++
	for _, l := range f.lines {
		fmt.Fprintln(out, l)
	}
	if f.gate != nil {
		<-f.gate
	}
	return f.syncErr
}

type fakes struct {
	mk *fakeMarketplace
	s  *fakeSearcher
	c  *fakeCatalog
	p  *fakePacks
	b  *fakeBuilder
	in *fakeInstaller
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
		c:  &fakeCatalog{details: map[string]Node{}},
		p:  &fakePacks{scope: "project /work"},
		b:  &fakeBuilder{},
		mk: &fakeMarketplace{},
		in: &fakeInstaller{def: Target{Scope: "project", Path: "/work"},
			plan: []string{"ipm install", "claude plugin marketplace add /work/.ipm --scope project", "claude plugin install kit@work-ipm --scope project"}},
	}
	if setup != nil {
		setup(f)
	}
	m := New(Deps{Searcher: f.s, Catalog: f.c, Packs: f.p, Builder: f.b, Installer: f.in, Marketplace: f.mk})
	m.debounce = 0
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, f
}

func TestTypingSearches(t *testing.T) {
	m, f := newTest(t)
	press(t, m, "/")
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

// TestOpensOnResults: ipm tui opens with the results focused, so the tab
// keys work at once and / focuses the search box.
func TestOpensOnResults(t *testing.T) {
	m, f := newTest(t)
	press(t, m, "2")
	if m.tab != tabPacks {
		t.Fatalf("2 on launch: tab %v, want Packs", m.tab)
	}
	press(t, m, "1")
	press(t, m, "/")
	typeText(t, m, "tdd")
	if m.focus != focusSearch || len(f.s.queries) == 0 || f.s.queries[len(f.s.queries)-1] != "tdd" {
		t.Errorf("/ then typing: focus %v queries %v", m.focus, f.s.queries)
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
	press(t, m, "/")
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
	press(t, m, "/")
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
	press(t, m, "/")
	if press(t, m, "q") || m.input.Value() != "q" {
		t.Error("q in the search box is a letter")
	}
}

// withDetails gives the tdd result a plugin and repo above it and a child.
func withDetails(f fakes) {
	f.c.details["skill:o/r:skills/tdd"] = Node{ID: "skill:o/r:skills/tdd", Type: "skill", Name: "tdd", Repo: "o/r", Stars: 31234,
		Description: "Test first.",
		Parents:     []Ref{{ID: "plugin:o/r:p", Type: "plugin", Name: "p", Description: "The p plugin"}},
		Children:    []Ref{{ID: "agent:o/r:agents/helper.md", Type: "agent", Name: "helper", Description: "Helps"}}}
	f.c.details["plugin:o/r:p"] = Node{ID: "plugin:o/r:p", Type: "plugin", Name: "p", Description: "The p plugin",
		Parents:  []Ref{{ID: "repo:o/r", Type: "repo", Name: "o/r", Description: "The repo"}},
		Children: []Ref{{ID: "skill:o/r:skills/tdd", Type: "skill", Name: "tdd", Description: "Test first."}, {ID: "skill:o/r:skills/tdd-guard", Type: "skill", Name: "tdd-guard"}}}
	f.c.details["repo:o/r"] = Node{ID: "repo:o/r", Type: "repo", Name: "o/r", Description: "The repo",
		Children: []Ref{{ID: "plugin:o/r:p", Type: "plugin", Name: "p", Description: "The p plugin"}}}
	f.c.details["agent:o/r:agents/helper.md"] = Node{ID: "agent:o/r:agents/helper.md", Type: "agent", Name: "helper", Description: "Helps",
		Parents: []Ref{{ID: "skill:o/r:skills/tdd", Type: "skill", Name: "tdd"}}}
	f.c.details["skill:o/r:skills/tdd-guard"] = Node{ID: "skill:o/r:skills/tdd-guard", Type: "skill", Name: "tdd-guard", Repo: "o/r",
		Parents: []Ref{{ID: "plugin:o/r:p", Type: "plugin", Name: "p", Description: "The p plugin"}}}
}

func TestTreeFollowsSelection(t *testing.T) {
	m, f := newTest(t)
	withDetails(f)
	typeText(t, m, "tdd")
	if m.cursor != "skill:o/r:skills/tdd" {
		t.Fatalf("cursor %q", m.cursor)
	}
	v := view(m)
	if row := caretRow(panels(m.View().Content)[1]); !strings.Contains(row, "tdd") {
		// panels keep the border; strip it before looking for the caret
		found := false
		for _, l := range panels(view(m))[1] {
			if strings.HasPrefix(strings.TrimPrefix(l, "│ "), "▶ ") && strings.Contains(l, "tdd") {
				found = true
			}
		}
		if !found {
			t.Errorf("the tree's caret row should be tdd:\n%s", view(m))
		}
	}
	for _, want := range []string{"o/r", "The p plugin", "tdd", "Test first.", "helper"} {
		if !strings.Contains(v, want) {
			t.Errorf("tree lacks %q:\n%s", want, v)
		}
	}
	press(t, m, "down", "j")
	if m.cursor != "skill:o/r:skills/tdd-guard" {
		t.Errorf("moving should move the tree to the new result, cursor %q", m.cursor)
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

func TestTreeError(t *testing.T) {
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
	if v := view(m); !strings.Contains(v, "a (1) · project /work") || !strings.Contains(v, "skill tdd") {
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
	press(t, m, "t")
	if m.focus != focusTray {
		t.Fatalf("t should focus the tray")
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

func TestInstallStreams(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.b.lines = []string{"✓ kit: 2 item(s)", "validate: ok"}
	typeText(t, m, "tdd")
	press(t, m, "down", "a")
	drain(t, m, m.startBuild())
	v := view(m)
	for _, want := range []string{"✓ kit: 2 item(s)", "validate: ok", "✓ built"} {
		if !strings.Contains(v, want) {
			t.Errorf("install modal lacks %q:\n%s", want, v)
		}
	}
	if m.dirty {
		t.Error("a successful install clears • not built")
	}
	press(t, m, "esc")
	if m.inst != nil {
		t.Error("esc should close the finished install")
	}
}

func TestQuitIgnoredDuringInstall(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.b.gate = make(chan struct{})
	typeText(t, m, "tdd")
	press(t, m, "down")
	cmd := m.startBuild()
	for _, k := range []string{"q", "esc"} {
		if _, c := m.Update(key(k)); c != nil {
			t.Errorf("%s during install returned a command", k)
		}
	}
	if m.inst == nil || m.status != busyMsg {
		t.Errorf("modal %v status %q", m.inst, m.status)
	}
	if _, c := m.Update(key("ctrl+c")); c != nil || m.status != abandonMsg {
		t.Errorf("first ctrl+c: cmd %v status %q", c, m.status)
	}
	if !strings.Contains(view(m), abandonMsg) {
		t.Errorf("the warning should show:\n%s", view(m))
	}
	close(f.b.gate)
	drain(t, m, cmd)
	if !strings.Contains(view(m), "✓ built") {
		t.Errorf("install should finish:\n%s", view(m))
	}
}

func TestCtrlCTwiceAbandonsARun(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.b.gate = make(chan struct{})
	cmd := m.startBuild()
	m.Update(key("ctrl+c"))
	if _, c := m.Update(key("ctrl+c")); c == nil {
		t.Fatal("the second ctrl+c should quit")
	} else if _, ok := c().(tea.QuitMsg); !ok {
		t.Errorf("second ctrl+c returned %T", c())
	}
	close(f.b.gate)
	drain(t, m, cmd)
}

func TestInstallFailure(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.b.lines = []string{"pnpm install …"}
	f.b.err = errors.New("pnpm failed")
	typeText(t, m, "tdd")
	press(t, m, "down", "a")
	drain(t, m, m.startBuild())
	if v := view(m); !strings.Contains(v, "pnpm install …") || !strings.Contains(v, "✗ pnpm failed") {
		t.Errorf("modal:\n%s", v)
	}
	if !m.dirty {
		t.Error("a failed install keeps • not built")
	}
	press(t, m, "esc")
	if !press(t, m, "q") {
		t.Error("q should quit once install has finished")
	}
}

func TestHelpOverlay(t *testing.T) {
	m, _ := newTest(t)
	press(t, m, "down", "?")
	v := view(m)
	for _, kh := range append(append([][2]string{}, searchKeys...), treeKeys...) {
		if !strings.Contains(v, kh[0]) || !strings.Contains(v, kh[1]) {
			t.Errorf("help lacks %q %q", kh[0], kh[1])
		}
	}
	press(t, m, "esc")
	if m.help {
		t.Error("esc should hide the help")
	}
}

func TestRecheck(t *testing.T) {
	m, f := newTest(t)
	typeText(t, m, "tdd")
	n := len(f.s.queries)
	press(t, m, "ctrl+r")
	if f.s.checks != 1 || len(f.s.queries) != n+1 {
		t.Errorf("checks %d, searches %d → %d", f.s.checks, n, len(f.s.queries))
	}
}

func TestTrayKeepsPackNameWithLongScope(t *testing.T) {
	m, _ := newTestWith(t, func(f fakes) {
		f.p.scope = "project /" + strings.Repeat("very/long/path/", 12)
		f.p.packs = []Pack{{Name: "kit", Items: []string{tddID}}}
	})
	m.dirty = true
	if v := view(m); !strings.Contains(v, "kit (1) • not built") {
		t.Errorf("a long scope must not hide the pack name:\n%s", v)
	}
}

func TestPickerWithNoPacks(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down", "p", "down", "j", "enter")
	if m.active != "" {
		t.Errorf("no pack to switch to, active %q", m.active)
	}
}

func TestTraySelectionStaysVisible(t *testing.T) {
	var items []string
	for i := range 15 {
		items = append(items, fmt.Sprintf("skill:o/r:skills/item-number-%c-with-a-long-name", 'a'+i))
	}
	m, _ := newTestWith(t, withPacks(Pack{Name: "a", Items: items}))
	press(t, m, "down", "t")
	if m.focus != focusTray {
		t.Fatalf("focus %v", m.focus)
	}
	for range 14 {
		press(t, m, "right")
	}
	if v := view(m); !strings.Contains(v, "▶skill item-number-o-with-a-long-name") {
		t.Errorf("the selected tray item must be visible:\n%s", v)
	}
}

func TestInstallLongLineDoesNotHang(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.b.lines = []string{strings.Repeat("x", 70*1024), "validate: ok"}
	typeText(t, m, "tdd")
	press(t, m, "down")
	cmd := m.startBuild()
	done := make(chan struct{})
	go func() { drain(t, m, cmd); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("install never finished")
	}
	if !m.inst.done {
		t.Error("install should be done")
	}
}

// treeTest is a model whose "tdd" results sit in the withDetails tree, with
// the results focused.
func treeTest(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, func(f fakes) {
		withDetails(f)
		f.p.packs = []Pack{{Name: "a"}}
	})
	typeText(t, m, "tdd")
	press(t, m, "down")
	return m, f
}

func TestSearchModeArrowsMoveTree(t *testing.T) {
	m, _ := treeTest(t)
	press(t, m, "right")
	if m.cursor != "agent:o/r:agents/helper.md" || m.sel != 0 || m.focus != focusResults {
		t.Fatalf("right: cursor %q sel %d focus %v", m.cursor, m.sel, m.focus)
	}
	press(t, m, "left", "left")
	if m.cursor != "plugin:o/r:p" {
		t.Errorf("left twice: cursor %q", m.cursor)
	}
	press(t, m, "down")
	if m.cursor != "skill:o/r:skills/tdd-guard" {
		t.Errorf("moving the result should reset the tree: %q", m.cursor)
	}
}

func TestLeftAtRootRightAtLeaf(t *testing.T) {
	m, _ := treeTest(t)
	press(t, m, "left", "left", "left")
	if m.cursor != "repo:o/r" {
		t.Errorf("left at the root: %q", m.cursor)
	}
	m, _ = treeTest(t)
	press(t, m, "right", "right")
	if m.cursor != "agent:o/r:agents/helper.md" {
		t.Errorf("right at a leaf: %q", m.cursor)
	}
}

func TestTreeModeSiblings(t *testing.T) {
	m, _ := treeTest(t)
	press(t, m, "tab")
	if m.focus != focusTree {
		t.Fatalf("tab from results should enter tree mode")
	}
	press(t, m, "down")
	if m.cursor != "skill:o/r:skills/tdd-guard" {
		t.Errorf("down: %q", m.cursor)
	}
	press(t, m, "k")
	if m.cursor != "skill:o/r:skills/tdd" {
		t.Errorf("k: %q", m.cursor)
	}
	press(t, m, "j")
	if m.cursor != "skill:o/r:skills/tdd-guard" || m.sel != 0 {
		t.Errorf("j: %q, result selection %d", m.cursor, m.sel)
	}
}

func TestTreeModeAlsoIn(t *testing.T) {
	m, f := treeTest(t)
	tdd := f.c.details["skill:o/r:skills/tdd"]
	tdd.Parents = append(tdd.Parents, Ref{ID: "plugin:o/r:p2", Type: "plugin", Name: "p2"})
	f.c.details["skill:o/r:skills/tdd"] = tdd
	f.c.details["plugin:o/r:p2"] = Node{ID: "plugin:o/r:p2", Type: "plugin", Name: "p2",
		Children: []Ref{{ID: "skill:o/r:skills/tdd", Type: "skill", Name: "tdd"}}}
	m.followSelection()
	press(t, m, "tab", "down")
	if m.cursor != "skill:o/r:skills/tdd" || !strings.Contains(view(m), "also in: plugin p2") {
		t.Fatalf("down onto also-in should not move the cursor: %q\n%s", m.cursor, view(m))
	}
	press(t, m, "enter")
	if m.chosen["skill:o/r:skills/tdd"] != "plugin:o/r:p2" || len(m.tree.Path) == 0 || m.tree.Path[len(m.tree.Path)-1].ID != "plugin:o/r:p2" {
		t.Errorf("enter should follow p2: chosen %v path %v", m.chosen, m.tree.Path)
	}
}

func TestTreeModeExit(t *testing.T) {
	for _, k := range []string{"esc", "tab"} {
		m, _ := treeTest(t)
		press(t, m, "tab", "down", k)
		if m.focus != focusResults || m.cursor != "skill:o/r:skills/tdd" {
			t.Errorf("%s: focus %v cursor %q", k, m.focus, m.cursor)
		}
	}
}

func TestTreeModeAdd(t *testing.T) {
	m, f := treeTest(t)
	press(t, m, "left", "tab", "a")
	if strings.Join(f.p.calls, ";") != "add a plugin:o/r:p" {
		t.Errorf("calls %v", f.p.calls)
	}
}

func TestEnterFromResultsOpensTreeMode(t *testing.T) {
	m, _ := treeTest(t)
	press(t, m, "enter")
	if m.focus != focusTree {
		t.Errorf("focus %v", m.focus)
	}
}

func TestHelpListsBothModes(t *testing.T) {
	m, _ := treeTest(t)
	press(t, m, "?")
	v := view(m)
	for _, kh := range append(append([][2]string{}, searchKeys...), treeKeys...) {
		if !strings.Contains(v, kh[1]) {
			t.Errorf("help lacks %q", kh[1])
		}
	}
}

func TestBreadcrumb(t *testing.T) {
	m, _ := treeTest(t)
	if !strings.Contains(view(m), "o/r › p › tdd") {
		t.Errorf("breadcrumb:\n%s", view(m))
	}
}

func catalogWithPack(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withPacks(Pack{Name: "kit", Items: []string{tddID}}))
	typeText(t, m, "tdd")
	press(t, m, "down")
	return m, f
}

func TestCatalogHasNoBuildOrInstallKeys(t *testing.T) {
	for _, k := range []string{"b", "i", "D"} {
		m, _ := catalogWithPack(t)
		press(t, m, k)
		if m.inst != nil || m.form != nil || m.status != "" {
			t.Errorf("%s should do nothing on Catalog: modal %v %v status %q", k, m.inst, m.form, m.status)
		}
		if h := m.hints(); strings.Contains(h, " b ") || strings.Contains(h, "deploy") {
			t.Errorf("footer lists %s: %q", k, h)
		}
	}
}

// withExternal gives the fake catalog repo a/m whose marketplace lists a local
// plugin and an external entry for b/s's plugin cf, and b/s itself.
func withExternal(f fakes) {
	ref := func(id, typ, name, repo string) Ref { return Ref{ID: id, Type: typ, Name: name, Repo: repo} }
	d := f.c.details
	d["repo:a/m"] = Node{ID: "repo:a/m", Type: "repo", Name: "a/m", Repo: "a/m", Children: []Ref{ref("marketplace:a/m:.", "marketplace", "m", "a/m")}}
	entry := ref("plugin:a/m#cf", "plugin", "cf", "a/m")
	entry.External = true
	d["marketplace:a/m:."] = Node{ID: "marketplace:a/m:.", Type: "marketplace", Name: "m", Repo: "a/m",
		Parents: []Ref{ref("repo:a/m", "repo", "a/m", "a/m")}, Children: []Ref{entry, ref("plugin:a/m:local", "plugin", "local", "a/m")}}
	target := ref("plugin:b/s:cf", "plugin", "cf", "b/s")
	d["plugin:a/m#cf"] = Node{ID: "plugin:a/m#cf", Type: "plugin", Name: "cf", Repo: "a/m", External: &target,
		Parents: []Ref{ref("marketplace:a/m:.", "marketplace", "m", "a/m")}}
	d["plugin:a/m:local"] = Node{ID: "plugin:a/m:local", Type: "plugin", Name: "local", Repo: "a/m", Parents: []Ref{ref("marketplace:a/m:.", "marketplace", "m", "a/m")}}
	d["repo:b/s"] = Node{ID: "repo:b/s", Type: "repo", Name: "b/s", Repo: "b/s", Children: []Ref{ref("marketplace:b/s:.", "marketplace", "m", "b/s")}}
	d["marketplace:b/s:."] = Node{ID: "marketplace:b/s:.", Type: "marketplace", Name: "m", Repo: "b/s",
		Parents: []Ref{ref("repo:b/s", "repo", "b/s", "b/s")}, Children: []Ref{target}}
	d["plugin:b/s:cf"] = Node{ID: "plugin:b/s:cf", Type: "plugin", Name: "cf", Repo: "b/s", Parents: []Ref{ref("marketplace:b/s:.", "marketplace", "m", "b/s")}}
	f.s.results["cf"] = Results{Hits: []Hit{{ID: "plugin:a/m#cf", Type: "plugin", Name: "cf", Repo: "a/m", External: true}}, Found: 1}
}

func externalTest(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withExternal)
	typeText(t, m, "cf")
	press(t, m, "down")
	return m, f
}

func TestJumpIntoExternal(t *testing.T) {
	m, _ := externalTest(t)
	press(t, m, "tab", "right")
	if m.cursor != "plugin:b/s:cf" || len(m.tree.Path) == 0 || m.tree.Path[0].ID != "repo:b/s" {
		t.Fatalf("cursor %q path %v", m.cursor, m.tree.Path)
	}
	if len(m.backStack) != 1 || m.backStack[0] != "plugin:a/m#cf" {
		t.Errorf("back stack %v", m.backStack)
	}
	tree := panels(view(m))[1]
	if !strings.Contains(ansiStrip(tree[1]), "↩ a/m — back to cf") {
		t.Errorf("the first tree row should be the back root:\n%s", strings.Join(tree, "\n"))
	}
}

func TestBackRootReturns(t *testing.T) {
	m, _ := externalTest(t)
	press(t, m, "tab", "right", "up")
	if m.treeSel != backRow {
		t.Fatalf("up from the first sibling selects the back root, treeSel %d", m.treeSel)
	}
	press(t, m, "enter")
	if m.cursor != "plugin:a/m#cf" || len(m.backStack) != 0 || strings.Contains(view(m), "↩") {
		t.Errorf("cursor %q stack %v", m.cursor, m.backStack)
	}
}

func TestJumpUnresolved(t *testing.T) {
	m, f := newTestWith(t, withExternal)
	e := f.c.details["plugin:a/m#cf"]
	e.External, e.Unresolved = nil, "c/missing"
	f.c.details["plugin:a/m#cf"] = e
	typeText(t, m, "cf")
	press(t, m, "down", "right")
	if m.cursor != "plugin:a/m#cf" || !strings.Contains(m.status, "ipm marketplace add plugin:a/m#cf") {
		t.Errorf("cursor %q status %q", m.cursor, m.status)
	}
}

func TestJumpCycleBackStack(t *testing.T) {
	m, f := newTest(t)
	d := f.c.details
	toB := Ref{ID: "plugin:b:x", Type: "plugin", Name: "x", Repo: "b"}
	toA := Ref{ID: "plugin:a:y", Type: "plugin", Name: "y", Repo: "a"}
	d["plugin:a#tob"] = Node{ID: "plugin:a#tob", Type: "plugin", Name: "tob", Repo: "a", External: &toB}
	d["plugin:b#toa"] = Node{ID: "plugin:b#toa", Type: "plugin", Name: "toa", Repo: "b", External: &toA}
	m.setCursor("plugin:a#tob")
	m.treeDown()
	m.setCursor("plugin:b#toa")
	m.treeDown()
	if m.cursor != "plugin:a:y" || len(m.backStack) != 2 {
		t.Fatalf("cursor %q stack %v", m.cursor, m.backStack)
	}
	m.back()
	m.back()
	if m.cursor != "plugin:a#tob" || len(m.backStack) != 0 {
		t.Errorf("two backs: cursor %q stack %v", m.cursor, m.backStack)
	}
	for range 21 {
		m.setCursor("plugin:a#tob")
		m.treeDown()
	}
	if len(m.backStack) != 20 {
		t.Errorf("the back stack holds 20, has %d", len(m.backStack))
	}
}

func TestMovingResultClearsBackStack(t *testing.T) {
	m, _ := newTestWith(t, func(f fakes) {
		withExternal(f)
		r := f.s.results["cf"]
		r.Hits, r.Found = append(r.Hits, Hit{ID: "plugin:a/m:local", Type: "plugin", Name: "local", Repo: "a/m"}), 2
		f.s.results["cf"] = r
	})
	typeText(t, m, "cf")
	press(t, m, "down", "right")
	if len(m.backStack) != 1 {
		t.Fatalf("search-mode → on an external entry jumps too: %v", m.backStack)
	}
	press(t, m, "down")
	if len(m.backStack) != 0 {
		t.Errorf("moving the result clears the back stack: %v", m.backStack)
	}
}

func ansiStrip(s string) string { return ansi.Strip(s) }

// withRefs gives b/s's repo node references from a/m (twice) and c/n.
func withRefs(f fakes) {
	withExternal(f)
	d := f.c.details
	b := d["repo:b/s"]
	b.UsedByRepos = 2
	b.UsedBy = []Ref{
		{ID: "plugin:a/m#cf", Type: "plugin", Name: "cf", Repo: "a/m", External: true},
		{ID: "plugin:c/n#cloud", Type: "plugin", Name: "cloud", Repo: "c/n", External: true},
		{ID: "plugin:a/m#cf2", Type: "plugin", Name: "cf2", Repo: "a/m", External: true},
	}
	d["repo:b/s"] = b
	a := d["repo:a/m"]
	a.Stars = 37600
	d["repo:a/m"] = a
	d["repo:c/n"] = Node{ID: "repo:c/n", Type: "repo", Name: "c/n", Repo: "c/n", Stars: 120}
	d["plugin:c/n#cloud"] = Node{ID: "plugin:c/n#cloud", Type: "plugin", Name: "cloud", Repo: "c/n",
		Parents: []Ref{{ID: "marketplace:c/n:.", Type: "marketplace", Name: "plugins", Repo: "c/n"}}}
	d["plugin:a/m#cf2"] = Node{ID: "plugin:a/m#cf2", Type: "plugin", Name: "cf2", Repo: "a/m",
		Parents: []Ref{{ID: "marketplace:a/m:.", Type: "marketplace", Name: "m", Repo: "a/m"}}}
	f.s.results["cf"] = Results{Hits: []Hit{{ID: "plugin:b/s:cf", Type: "plugin", Name: "cf", Repo: "b/s"}}, Found: 1}
}

func TestExternalModal(t *testing.T) {
	m, _ := newTestWith(t, withRefs)
	typeText(t, m, "cf")
	press(t, m, "down", "e")
	v := view(m)
	for _, want := range []string{"External references to b/s — used by 2 repos", "a/m", "★37.6k", "cf", "c/n", "cloud", "enter jump · esc close"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal lacks %q:\n%s", want, v)
		}
	}
}

func TestExternalModalGroups(t *testing.T) {
	m, _ := newTestWith(t, withRefs)
	typeText(t, m, "cf")
	press(t, m, "down", "e")
	var order []string
	for _, r := range m.ext.rows {
		order = append(order, r.ref.Repo+" "+r.ref.Name)
	}
	if strings.Join(order, ";") != "a/m cf;a/m cf2;c/n cloud" {
		t.Errorf("rows grouped by repo: %v", order)
	}
}

func TestExternalModalJump(t *testing.T) {
	m, _ := newTestWith(t, withRefs)
	typeText(t, m, "cf")
	press(t, m, "down", "e", "down", "enter")
	if m.ext != nil || m.cursor != "plugin:a/m#cf2" || len(m.backStack) != 1 || m.backStack[0] != "plugin:b/s:cf" {
		t.Errorf("modal %v cursor %q stack %v", m.ext, m.cursor, m.backStack)
	}
}

func TestExternalModalEmpty(t *testing.T) {
	m, _ := newTestWith(t, withExternal)
	f := m.d.Searcher.(*fakeSearcher)
	f.results["cf"] = Results{Hits: []Hit{{ID: "plugin:b/s:cf", Type: "plugin", Name: "cf", Repo: "b/s"}}, Found: 1}
	typeText(t, m, "cf")
	press(t, m, "down", "e")
	if !strings.Contains(view(m), "no other repo references b/s") {
		t.Errorf("empty modal:\n%s", view(m))
	}
	press(t, m, "esc")
	if m.ext != nil {
		t.Error("esc should close")
	}
}

func TestExternalModalSingular(t *testing.T) {
	m, f := newTestWith(t, withRefs)
	b := f.c.details["repo:b/s"]
	b.UsedBy = b.UsedBy[:1]
	f.c.details["repo:b/s"] = b
	typeText(t, m, "cf")
	press(t, m, "down", "e")
	if !strings.Contains(view(m), "used by 1 repo\n") && !strings.Contains(view(m), "used by 1 repo ") {
		t.Errorf("one repo, singular:\n%s", view(m))
	}
}

func TestExternalModalEmptyKeys(t *testing.T) {
	m, f := newTestWith(t, withExternal)
	f.s.results["cf"] = Results{Hits: []Hit{{ID: "plugin:b/s:cf", Type: "plugin", Name: "cf", Repo: "b/s"}}, Found: 1}
	typeText(t, m, "cf")
	press(t, m, "down", "e", "down", "enter")
	if m.cursor != "plugin:b/s:cf" {
		t.Errorf("enter in an empty modal does nothing: cursor %q", m.cursor)
	}
}

func TestExternalModalScrolls(t *testing.T) {
	m, f := newTestWith(t, withRefs)
	b := f.c.details["repo:b/s"]
	b.UsedBy = nil
	for i := range 30 {
		b.UsedBy = append(b.UsedBy, Ref{ID: fmt.Sprintf("plugin:r%02d/m#x", i), Type: "plugin", Name: fmt.Sprintf("entry-%02d", i), Repo: fmt.Sprintf("r%02d/m", i), External: true})
	}
	f.c.details["repo:b/s"] = b
	typeText(t, m, "cf")
	press(t, m, "down", "e")
	for range 25 {
		press(t, m, "down")
	}
	if v := view(m); !strings.Contains(v, "▶ r25/m") || !strings.Contains(v, "enter jump · esc close") {
		t.Errorf("the selected reference and the hint must stay visible:\n%s", v)
	}
}

func TestSyncDoesNotClearDirty(t *testing.T) {
	m, _ := newTestWith(t, withPacks(Pack{Name: "a"}))
	m.dirty = true
	press(t, m, "down", "4")
	drain(t, m, m.startSync())
	if !m.dirty {
		t.Error("a sync built nothing: the packs are still unbuilt")
	}
	drain(t, m, m.startBuild())
	if m.dirty {
		t.Error("a successful build clears dirty")
	}
}
