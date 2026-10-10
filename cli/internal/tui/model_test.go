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
	details map[string]Node
	asked   []string
	err     error
}

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

type fakeDeployer struct {
	def      Target
	cleaned  *Target // what Plan returns as the target; nil returns t unchanged
	plan     []string
	planErr  error
	lines    []string
	err      error
	deployed []string
}

func (f *fakeDeployer) Default(pack string) Target { return f.def }
func (f *fakeDeployer) Plan(pack string, t Target) (Target, []string, error) {
	if f.cleaned != nil {
		t = *f.cleaned
	}
	return t, f.plan, f.planErr
}
func (f *fakeDeployer) Deploy(_ context.Context, pack string, t Target, out io.Writer) error {
	f.deployed = append(f.deployed, fmt.Sprintf("%s %s %s", pack, t.Scope, t.Path))
	for _, l := range f.lines {
		fmt.Fprintln(out, l)
	}
	return f.err
}

type fakes struct {
	s *fakeSearcher
	c *fakeCatalog
	p *fakePacks
	i *fakeInstaller
	d *fakeDeployer
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
		c: &fakeCatalog{details: map[string]Node{}},
		p: &fakePacks{scope: "project /work"},
		i: &fakeInstaller{},
		d: &fakeDeployer{def: Target{Scope: "project", Path: "/work"},
			plan: []string{"ipm install", "claude plugin marketplace add /work/.ipm --scope project", "claude plugin install kit@work-ipm --scope project"}},
	}
	if setup != nil {
		setup(f)
	}
	m := New(Deps{Searcher: f.s, Catalog: f.c, Packs: f.p, Installer: f.i, Deployer: f.d})
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
	for _, want := range []string{"o/r", "The p plugin", "▶tdd", "Test first.", "helper"} {
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
	f.i.lines = []string{"✓ kit: 2 item(s)", "validate: ok"}
	typeText(t, m, "tdd")
	press(t, m, "down", "a", "i")
	v := view(m)
	for _, want := range []string{"✓ kit: 2 item(s)", "validate: ok", "✓ installed"} {
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
	f.i.gate = make(chan struct{})
	typeText(t, m, "tdd")
	press(t, m, "down")
	_, cmd := m.Update(key("i"))
	for _, k := range []string{"q", "ctrl+c", "esc"} {
		if _, c := m.Update(key(k)); c != nil {
			t.Errorf("%s during install returned a command", k)
		}
	}
	if m.inst == nil || !strings.Contains(m.status, "install is running") {
		t.Errorf("modal %v status %q", m.inst, m.status)
	}
	close(f.i.gate)
	drain(t, m, cmd)
	if !strings.Contains(view(m), "✓ installed") {
		t.Errorf("install should finish:\n%s", view(m))
	}
}

func TestInstallFailure(t *testing.T) {
	m, f := newTestWith(t, withPacks(Pack{Name: "a"}))
	f.i.lines = []string{"pnpm install …"}
	f.i.err = errors.New("pnpm failed")
	typeText(t, m, "tdd")
	press(t, m, "down", "a", "i")
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
	f.i.lines = []string{strings.Repeat("x", 70*1024), "validate: ok"}
	typeText(t, m, "tdd")
	press(t, m, "down")
	_, cmd := m.Update(key("i"))
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

func deployTest(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withPacks(Pack{Name: "kit", Items: []string{tddID}}))
	typeText(t, m, "tdd")
	press(t, m, "down")
	return m, f
}

func TestBuildKey(t *testing.T) {
	for _, k := range []string{"b", "i"} {
		m, _ := deployTest(t)
		press(t, m, k)
		if m.inst == nil {
			t.Errorf("%s should build", k)
		}
	}
}

func TestDeployFlow(t *testing.T) {
	m, f := deployTest(t)
	f.d.lines = []string{"$ claude plugin install kit@work-ipm --scope project"}
	press(t, m, "D")
	if m.deploy == nil || !strings.Contains(view(m), "▶project") {
		t.Fatalf("D should open the target step on the default:\n%s", view(m))
	}
	press(t, m, "enter")
	if m.deploy.path.Value() != "/work" {
		t.Fatalf("path step prefilled %q", m.deploy.path.Value())
	}
	press(t, m, "enter")
	v := view(m)
	for _, want := range append([]string{"Deploy kit to Claude Code — project scope in /work", "y deploy · esc cancel"}, f.d.plan...) {
		if !strings.Contains(v, want) {
			t.Errorf("confirm lacks %q:\n%s", want, v)
		}
	}
	press(t, m, "y")
	v = view(m)
	if strings.Join(f.d.deployed, ";") != "kit project /work" || !strings.Contains(v, "$ claude plugin install") || !strings.Contains(v, "✓ deployed kit (project /work)") {
		t.Errorf("deployed %v:\n%s", f.d.deployed, v)
	}
	press(t, m, "esc")
	if m.inst != nil || m.deploy != nil {
		t.Error("esc should close the finished deploy")
	}
}

func TestDeployUserSkipsPath(t *testing.T) {
	m, _ := deployTest(t)
	press(t, m, "D", "up", "enter")
	if !strings.Contains(view(m), "Deploy kit to Claude Code — user scope") || strings.Contains(view(m), "user scope in") {
		t.Errorf("user scope should go straight to confirm:\n%s", view(m))
	}
}

func TestDeployBadPath(t *testing.T) {
	m, f := deployTest(t)
	f.d.planErr = errors.New("/nope is not a folder")
	press(t, m, "D", "enter", "enter")
	if m.deploy == nil || m.deploy.step != stepPath || !strings.Contains(m.status, "/nope is not a folder") {
		t.Errorf("step %v status %q", m.deploy, m.status)
	}
}

func TestDeployFailure(t *testing.T) {
	m, f := deployTest(t)
	f.d.err = errors.New("`claude plugin install kit@work-ipm --scope project` failed: exit status 1")
	press(t, m, "D", "enter", "enter", "y")
	if !strings.Contains(view(m), "✗ `claude plugin install") {
		t.Errorf("modal:\n%s", view(m))
	}
	press(t, m, "esc")
	if m.inst != nil {
		t.Error("esc should close")
	}
}

func TestDeployCancel(t *testing.T) {
	for _, keys := range [][]string{{"D", "esc"}, {"D", "enter", "esc"}, {"D", "enter", "enter", "esc"}} {
		m, f := deployTest(t)
		press(t, m, keys...)
		if m.deploy != nil || len(f.d.deployed) != 0 {
			t.Errorf("%v: modal %v deployed %v", keys, m.deploy, f.d.deployed)
		}
	}
}

func TestDeployWithoutPack(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down", "D")
	if m.deploy != nil || !strings.Contains(m.status, "no pack to deploy — press p to create one") {
		t.Errorf("modal %v status %q", m.deploy, m.status)
	}
}

func TestDeployIgnoredWhileRunning(t *testing.T) {
	m, f := deployTest(t)
	f.i.gate = make(chan struct{})
	_, cmd := m.Update(key("b"))
	for _, k := range []string{"D", "b"} {
		if _, c := m.Update(key(k)); c != nil || m.deploy != nil || !strings.Contains(m.status, "running") {
			t.Errorf("%s while building: cmd %v deploy %v status %q", k, c, m.deploy, m.status)
		}
	}
	close(f.i.gate)
	drain(t, m, cmd)
}

func TestDeployShowsCleanedTarget(t *testing.T) {
	m, f := deployTest(t)
	f.d.cleaned = &Target{Scope: "project", Path: "/home/me/work"}
	press(t, m, "D", "enter")
	m.deploy.path.SetValue("~/work/")
	press(t, m, "enter")
	if !strings.Contains(view(m), "project scope in /home/me/work") {
		t.Errorf("confirm should show the folder claude runs in:\n%s", view(m))
	}
	press(t, m, "y")
	if !strings.Contains(view(m), "✓ deployed kit (project /home/me/work)") || f.d.deployed[0] != "kit project /home/me/work" {
		t.Errorf("deployed %v:\n%s", f.d.deployed, view(m))
	}
}
