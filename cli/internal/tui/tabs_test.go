package tui

import (
	"strings"
	"testing"
)

func TestTabSwitchKeys(t *testing.T) {
	m, _ := newTest(t)
	press(t, m, "down")
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
