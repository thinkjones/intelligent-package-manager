package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func packsSetup(f fakes) {
	f.p.packs = []Pack{
		{Name: "csharp-sdlc", Description: "C# lifecycle", Items: []string{tddID, "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md", "plugin:anthropics/claude-plugins-official#stripe"}, Built: "fresh",
			Installs: []Install{{Pack: "csharp-sdlc", Target: Target{Kind: "claude", Scope: "project"}}, {Pack: "csharp-sdlc", Target: Target{Kind: "gascity", Name: "kit"}}}},
		{Name: "docs", Description: "Writing", Items: []string{"skill:o/r:skills/write"}, Built: "stale", Installs: []Install{{Pack: "docs", Target: Target{Kind: "claude", Scope: "user"}}}},
		{Name: "empty", Built: "never"},
	}
	f.p.global = []Pack{{Name: "mine", Description: "Everywhere", Items: []string{"skill:o/r:skills/mine"}, Built: "fresh"}}
}

func packsModel(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, packsSetup)
	press(t, m, "down", "2")
	return m, f
}

func TestPacksTable(t *testing.T) {
	m, _ := packsModel(t)
	v := view(m)
	for _, want := range []string{"NAME", "SCOPE", "ITEMS", "BUILT", "INSTALLED TO", "csharp-sdlc", "project", "claude project, gascity kit", "claude user"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	raw := m.View().Content
	for built, st := range map[string]string{"fresh": stTeal.Render("fresh"), "stale": stAmber.Render("stale"), "never": stMuted.Render("never")} {
		if !strings.Contains(raw, st) {
			t.Errorf("BUILT %s is not styled", built)
		}
	}
	if !strings.Contains(v, "3 packs · 1 fresh · 1 stale · 3 installs") {
		t.Errorf("stats strip:\n%s", v)
	}
}

func TestPacksStatsStrip(t *testing.T) {
	m, _ := newTestWith(t, func(f fakes) {
		f.p.packs = []Pack{{Name: "a", Built: "fresh", Installs: []Install{{}, {}}}, {Name: "b", Built: "fresh", Installs: []Install{{}, {}}}, {Name: "c", Built: "stale"}}
	})
	press(t, m, "down", "2")
	if !strings.Contains(view(m), "3 packs · 2 fresh · 1 stale · 4 installs") {
		t.Errorf("stats strip:\n%s", view(m))
	}
}

func TestPacksDetailAndRemoveItem(t *testing.T) {
	m, f := packsModel(t)
	v := view(m)
	if !strings.Contains(v, "C# lifecycle") || !strings.Contains(v, "agent csharp-pro") {
		t.Errorf("detail pane:\n%s", v)
	}
	press(t, m, "x")
	if len(f.p.calls) != 0 || !strings.Contains(m.status, "enter") {
		t.Errorf("x needs the item list: calls %v status %q", f.p.calls, m.status)
	}
	press(t, m, "enter", "down", "x")
	if got := f.p.calls; len(got) != 1 || !strings.HasPrefix(got[0], "remove csharp-sdlc agent:") {
		t.Fatalf("calls %v", got)
	}
	if len(m.selectedPack().Items) != 2 || m.pk.itemSel != 1 {
		t.Errorf("items %v sel %d", m.selectedPack().Items, m.pk.itemSel)
	}
	press(t, m, "x", "x")
	if len(m.selectedPack().Items) != 0 || m.pk.itemSel != 0 || m.pk.detail {
		t.Errorf("removing the last item: items %v sel %d detail %v", m.selectedPack().Items, m.pk.itemSel, m.pk.detail)
	}
	if m.counts[tabPacks] != 3 {
		t.Errorf("count %d", m.counts[tabPacks])
	}
	press(t, m, "esc", "right")
	if m.pk.detail {
		t.Error("an empty pack has no items to focus")
	}
}

func TestPacksDetailFocusKeys(t *testing.T) {
	m, _ := packsModel(t)
	press(t, m, "right")
	if !m.pk.detail {
		t.Fatal("→ focuses the items")
	}
	press(t, m, "down", "down", "down")
	if m.pk.itemSel != 2 || m.pk.sel != 0 {
		t.Errorf("item %d pack %d", m.pk.itemSel, m.pk.sel)
	}
	press(t, m, "left", "down")
	if m.pk.detail || m.pk.sel != 1 || m.pk.itemSel != 0 {
		t.Errorf("detail %v pack %d item %d", m.pk.detail, m.pk.sel, m.pk.itemSel)
	}
}

func TestPacksGlobalToggleAndReadOnly(t *testing.T) {
	m, f := packsModel(t)
	press(t, m, "g")
	if m.counts[tabPacks] != 4 || !strings.Contains(view(m), "global") || !strings.Contains(view(m), "PACKS (4)") {
		t.Errorf("global rows:\n%s", view(m))
	}
	press(t, m, "down", "down", "down")
	if p := m.selectedPack(); p == nil || p.Name != "mine" {
		t.Fatalf("selected %+v", p)
	}
	for _, k := range []string{"x", "b", "i"} {
		press(t, m, k)
		if m.status != readOnlyGlobal {
			t.Errorf("%s: status %q", k, m.status)
		}
		if m.inst != nil || m.deploy != nil {
			t.Errorf("%s opened a modal", k)
		}
	}
	press(t, m, "enter", "x")
	if m.status != readOnlyGlobal || len(f.p.calls) != 0 {
		t.Errorf("x in the item list: status %q calls %v", m.status, f.p.calls)
	}
	press(t, m, "esc", "g")
	if m.counts[tabPacks] != 3 || len(m.pk.rows) != 3 {
		t.Errorf("g again drops the global rows: %d", len(m.pk.rows))
	}
}

func TestPacksGlobalInGlobalScope(t *testing.T) {
	m, _ := newTestWith(t, func(f fakes) {
		packsSetup(f)
		f.p.scope = "global"
	})
	press(t, m, "down", "2", "g")
	if m.pk.global || len(m.pk.rows) != 3 || !m.statusOK {
		t.Errorf("g does nothing in global scope: global %v rows %d status %q", m.pk.global, len(m.pk.rows), m.status)
	}
	press(t, m, "b")
	if m.inst == nil {
		t.Error("packs of the global scope are editable there")
	}
}

func TestPacksBuild(t *testing.T) {
	m, f := packsModel(t)
	f.b.lines = []string{"built csharp-sdlc"}
	_, cmd := m.Update(key("b"))
	f.p.packs[1].Built = "fresh"
	drain(t, m, cmd)
	if m.inst == nil || !m.inst.done || m.inst.success != "✓ built" || !strings.Contains(view(m), "✓ built") {
		t.Fatalf("build modal:\n%s", view(m))
	}
	press(t, m, "esc")
	if m.pk.rows[1].Built != "fresh" || !strings.Contains(view(m), "3 packs · 2 fresh") {
		t.Errorf("the packs reload after a build:\n%s", view(m))
	}
}

func TestPacksInstallKey(t *testing.T) {
	m, f := packsModel(t)
	press(t, m, "down", "i")
	if m.deploy == nil || m.deploy.pack != "docs" {
		t.Fatalf("i opens the install for the selected pack: %+v", m.deploy)
	}
	press(t, m, "enter", "enter", "y")
	if len(f.in.installed) != 1 || !strings.HasPrefix(f.in.installed[0], "docs ") {
		t.Errorf("installed %v", f.in.installed)
	}
}

func TestPacksFilter(t *testing.T) {
	m, f := packsModel(t)
	press(t, m, "/")
	typeText(t, m, "doc")
	v := view(m)
	if !strings.Contains(v, "docs") || strings.Contains(v, "csharp-sdlc") || !strings.Contains(v, "filter: doc") {
		t.Errorf("filtered:\n%s", v)
	}
	press(t, m, "esc")
	if !strings.Contains(view(m), "csharp-sdlc") || m.pk.filter.value() != "" || m.pk.filter.typing {
		t.Errorf("esc clears:\n%s", view(m))
	}
	// Keys with a meaning elsewhere type into the filter.
	press(t, m, "/")
	typeText(t, m, "x1]qbigdw")
	if m.pk.filter.value() != "x1]qbigdw" || m.tab != tabPacks || m.inst != nil || m.deploy != nil || len(f.p.calls) != 0 || m.help {
		t.Errorf("filter %q tab %d", m.pk.filter.value(), m.tab)
	}
	if !strings.Contains(view(m), "no pack matches") {
		t.Errorf("no match:\n%s", view(m))
	}
	// enter keeps the filter and frees the keys.
	press(t, m, "esc", "/")
	typeText(t, m, "STALE")
	press(t, m, "enter")
	if len(m.visiblePacks()) != 1 || m.pk.filter.typing {
		t.Errorf("visible %d typing %v", len(m.visiblePacks()), m.pk.filter.typing)
	}
	press(t, m, "1")
	if m.tab != tabCatalog {
		t.Error("1 switches tabs once the filter is left")
	}
	press(t, m, "2", "esc")
	if m.pk.filter.value() != "" {
		t.Error("esc clears a kept filter")
	}
}

func TestFilterBoxMatch(t *testing.T) {
	f := newFilterBox()
	if !f.match([]string{"anything"}) {
		t.Error("an empty filter matches")
	}
	f.in.SetValue("  Doc ")
	if !f.match([]string{"a", "my-DOCS"}) || f.match([]string{"a", "b"}) {
		t.Error("match is a case-insensitive substring over the cells")
	}
}

func TestPacksLoadErrorShowsAndRetries(t *testing.T) {
	m, f := newTestWith(t, packsSetup)
	f.p.listErr = errors.New("ipm.toml is broken")
	press(t, m, "down", "2")
	if !strings.Contains(view(m), "ipm.toml is broken") {
		t.Errorf("the error shows in the body:\n%s", view(m))
	}
	press(t, m, "1")
	if m.tab != tabCatalog {
		t.Fatal("1 goes back to the Catalog")
	}
	f.p.listErr = nil
	press(t, m, "2")
	if m.pk.err != nil || !strings.Contains(view(m), "csharp-sdlc") {
		t.Errorf("returning retries:\n%s", view(m))
	}
}

func TestLeavingCatalogDoesNotLeakFocus(t *testing.T) {
	m, _ := newTestWith(t, packsSetup)
	typeText(t, m, "tdd")
	press(t, m, "down", "2")
	if m.textFocused() {
		t.Error("the Catalog's focus leaked into Packs")
	}
	press(t, m, "q")
}

func TestPacksFirstVisitLoads(t *testing.T) {
	m, f := newTestWith(t, packsSetup)
	if m.pk.loaded {
		t.Fatal("packs load on first visit")
	}
	press(t, m, "down", "2")
	if !m.pk.loaded || len(m.pk.rows) != 3 {
		t.Errorf("rows %d", len(m.pk.rows))
	}
	f.p.packs = append(f.p.packs, Pack{Name: "zzz"})
	press(t, m, "1", "2")
	if len(m.pk.rows) != 3 {
		t.Error("a loaded tab is not re-read on every visit")
	}
}

func TestPacksNarrowHidesDetail(t *testing.T) {
	m, _ := packsModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if strings.Contains(view(m), "C# lifecycle") {
		t.Error("the detail pane hides below 100 columns")
	}
	press(t, m, "enter")
	if m.pk.detail {
		t.Error("no item list to focus while its pane is hidden")
	}
}

func TestGoldenPacks(t *testing.T) {
	cases := map[string]func(t *testing.T) *Model{
		"packs": func(t *testing.T) *Model { m, _ := packsModel(t); return m },
		"packs-detail-focus": func(t *testing.T) *Model {
			m, _ := packsModel(t)
			press(t, m, "enter", "down")
			return m
		},
		"packs-global": func(t *testing.T) *Model {
			m, _ := packsModel(t)
			press(t, m, "g", "down", "down", "down")
			return m
		},
		"packs-filtered": func(t *testing.T) *Model {
			m, _ := packsModel(t)
			press(t, m, "/")
			typeText(t, m, "doc")
			return m
		},
		"packs-narrow": func(t *testing.T) *Model {
			m, _ := packsModel(t)
			m.Update(tea.WindowSizeMsg{Width: 72, Height: 20})
			return m
		},
		"packs-error": func(t *testing.T) *Model {
			m, f := newTestWith(t, packsSetup)
			f.p.listErr = errors.New("ipm.toml is broken")
			press(t, m, "down", "2")
			return m
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) { golden(t, name, build(t)) })
	}
}
