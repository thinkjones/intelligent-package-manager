package tui

import (
	"strings"
	"testing"
)

func TestTabSwitchKeys(t *testing.T) {
	m, _ := newTest(t)
	for _, c := range []struct {
		keys []string
		want tab
	}{
		{[]string{"2"}, tabPacks},
		{[]string{"]"}, tabInstalls},
		{[]string{"4"}, tabMarketplace},
		{[]string{"]"}, tabCatalog},
		{[]string{"["}, tabMarketplace},
		{[]string{"[", "["}, tabPacks},
		{[]string{"1"}, tabCatalog},
	} {
		press(t, m, c.keys...)
		if m.tab != c.want {
			t.Fatalf("after %v: tab %d, want %d", c.keys, m.tab, c.want)
		}
	}
}

func TestTabKeysTypeInSearch(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "1]q")
	if m.input.Value() != "1]q" || m.tab != tabCatalog {
		t.Errorf("query %q tab %d", m.input.Value(), m.tab)
	}
}

func TestOtherTabQuitsAndKeepsCatalogState(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down", "3")
	if !strings.Contains(view(m), "INSTALLS (0)") {
		t.Errorf("tab bar:\n%s", view(m))
	}
	press(t, m, "1")
	if m.sel != 0 || m.input.Value() != "tdd" || len(m.res.Hits) == 0 {
		t.Errorf("the Catalog keeps its search: %q sel %d", m.input.Value(), m.sel)
	}
	press(t, m, "2")
	if !press(t, m, "q") {
		t.Error("q quits from any tab")
	}
}

func TestTabBarCounts(t *testing.T) {
	m, _ := newTestWith(t, func(f fakes) {
		f.c.counts = map[string]int{"skill": 2000, "plugin": 314}
		f.p.packs = []Pack{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	})
	want := "CATALOG (2,314)  PACKS (3)  INSTALLS (…)  MARKETPLACE (…)"
	if !strings.Contains(view(m), want) {
		t.Errorf("tab bar lacks %q:\n%s", want, view(m))
	}
}

func TestCommas(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 2314: "2,314", 1234567: "1,234,567"} {
		if got := commas(n); got != want {
			t.Errorf("commas(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestEscLeavesSearchForTabKeys(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "esc", "3")
	if m.tab != tabInstalls {
		t.Errorf("esc leaves the search box so 3 switches tabs: tab %d", m.tab)
	}
}

func TestStartsOnTabBar(t *testing.T) {
	m, _ := newTest(t)
	if !m.bar || m.tab != tabCatalog {
		t.Fatalf("bar %v tab %d", m.bar, m.tab)
	}
	if v := view(m); !strings.Contains(v, "▶ CATALOG") || !strings.Contains(v, "←→ tabs") {
		t.Errorf("the focused tab bar is marked and the footer names its keys:\n%s", v)
	}
	press(t, m, "tab")
	if m.bar || strings.Contains(view(m), "▶ CATALOG") {
		t.Errorf("a pane has focus: the bar loses its marker:\n%s", view(m))
	}
}

func TestBarArrowsSwitchTabsAndWrap(t *testing.T) {
	m, _ := newTest(t)
	for _, c := range []struct {
		key  string
		want tab
	}{{"right", tabPacks}, {"l", tabInstalls}, {"right", tabMarketplace}, {"right", tabCatalog}, {"left", tabMarketplace}, {"h", tabInstalls}} {
		press(t, m, c.key)
		if m.tab != c.want || !m.bar {
			t.Fatalf("after %s: tab %d bar %v, want %d", c.key, m.tab, m.bar, c.want)
		}
	}
}

func TestBarEntersFirstPane(t *testing.T) {
	for _, k := range []string{"tab", "down", "enter"} {
		m, _ := newTest(t)
		press(t, m, k)
		if m.bar || m.focus != focusSearch {
			t.Errorf("%s from the bar: bar %v focus %v", k, m.bar, m.focus)
		}
	}
	m, _ := newTest(t)
	press(t, m, "/")
	typeText(t, m, "tdd")
	if m.bar || m.focus != focusSearch || m.input.Value() != "tdd" {
		t.Errorf("/ from the bar: bar %v focus %v query %q", m.bar, m.focus, m.input.Value())
	}
}

// catalogPane names where the Catalog's focus is: bar, search, results or tree.
func catalogPane(m *Model) string {
	switch {
	case m.bar:
		return "bar"
	case m.focus == focusSearch:
		return "search"
	case m.focus == focusTree:
		return "tree"
	}
	return "results"
}

func TestCatalogPaneCycle(t *testing.T) {
	m, f := newTest(t)
	withDetails(f)
	typeText(t, m, "tdd")
	press(t, m, "esc", "esc")
	var got []string
	for range 5 {
		press(t, m, "tab")
		got = append(got, catalogPane(m))
	}
	if strings.Join(got, ",") != "search,results,tree,bar,search" {
		t.Errorf("tab: %v", got)
	}
	got = nil
	for range 5 {
		press(t, m, "shift+tab")
		got = append(got, catalogPane(m))
	}
	if strings.Join(got, ",") != "bar,tree,results,search,bar" {
		t.Errorf("shift+tab: %v", got)
	}
	if m.input.Value() != "tdd" {
		t.Errorf("the search survives the cycle: %q", m.input.Value())
	}
}

func TestCatalogPaneCycleSkipsEmptyTree(t *testing.T) {
	m, _ := newTest(t)
	press(t, m, "tab", "tab", "tab")
	if !m.bar {
		t.Errorf("no result, no tree pane: %s", catalogPane(m))
	}
}

func TestPacksPaneCycle(t *testing.T) {
	m, _ := newTestWith(t, packsSetup)
	press(t, m, "right", "tab")
	if m.bar || m.pk.detail {
		t.Fatalf("tab: the packs table, bar %v detail %v", m.bar, m.pk.detail)
	}
	press(t, m, "tab")
	if !m.pk.detail {
		t.Fatal("tab again: the items")
	}
	press(t, m, "tab")
	if !m.bar || m.pk.detail {
		t.Fatalf("tab from the items: the bar, bar %v detail %v", m.bar, m.pk.detail)
	}
	press(t, m, "shift+tab")
	if m.bar || !m.pk.detail {
		t.Errorf("shift+tab from the bar: the items, bar %v detail %v", m.bar, m.pk.detail)
	}
}

func TestOneTablePaneCycle(t *testing.T) {
	for _, tb := range []string{"3", "4"} {
		m, _ := newTest(t)
		press(t, m, tb, "tab")
		if m.bar {
			t.Errorf("tab %s: the table has focus", tb)
		}
		press(t, m, "tab")
		if !m.bar {
			t.Errorf("tab %s: tab again is the bar", tb)
		}
		press(t, m, "shift+tab", "shift+tab")
		if !m.bar {
			t.Errorf("tab %s: shift+tab twice is the bar again", tb)
		}
	}
}

func TestEscReturnsToBar(t *testing.T) {
	m, _ := newTestWith(t, packsSetup)
	typeText(t, m, "tdd")
	press(t, m, "esc")
	if m.bar || m.focus != focusResults {
		t.Fatalf("esc in the search box: the results, %s", catalogPane(m))
	}
	press(t, m, "esc")
	if !m.bar {
		t.Errorf("esc in the results: the bar, %s", catalogPane(m))
	}
	for _, tb := range []string{"2", "3", "4"} {
		press(t, m, tb, "tab", "esc")
		if !m.bar {
			t.Errorf("tab %s: esc in the table goes to the bar", tb)
		}
	}
	press(t, m, "2", "tab", "tab", "esc")
	if m.bar || m.pk.detail {
		t.Errorf("esc in the items: the packs table, bar %v detail %v", m.bar, m.pk.detail)
	}
}

func TestFilterTabKeepsText(t *testing.T) {
	m, _ := newTestWith(t, packsSetup)
	press(t, m, "right", "/", "d", "o", "c", "tab")
	if m.bar || m.pk.filter.typing || m.pk.filter.value() != "doc" {
		t.Errorf("tab in the filter: bar %v typing %v value %q", m.bar, m.pk.filter.typing, m.pk.filter.value())
	}
	press(t, m, "esc")
	if m.bar || m.pk.filter.value() != "" {
		t.Errorf("esc clears the filter first: bar %v value %q", m.bar, m.pk.filter.value())
	}
}

func TestNumberKeysLandOnBar(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "tdd")
	press(t, m, "down", "2")
	if m.tab != tabPacks || !m.bar {
		t.Errorf("2 from the results: tab %d bar %v", m.tab, m.bar)
	}
}

func TestSearchBoxTypesNavigationLetters(t *testing.T) {
	m, _ := newTest(t)
	typeText(t, m, "hlq1[")
	if m.input.Value() != "hlq1[" || m.tab != tabCatalog || m.bar {
		t.Errorf("query %q tab %d bar %v", m.input.Value(), m.tab, m.bar)
	}
}
