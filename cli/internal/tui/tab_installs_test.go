package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func withInstalls(f fakes) {
	packsSetup(f)
	f.in.installs = []Install{
		{Pack: "csharp-sdlc", Target: Target{Kind: "claude", Scope: "project", Path: "/x"}},
		{Pack: "docs", Target: Target{Kind: "gascity", Path: "/x", Name: "kit"}},
	}
	f.in.globalInstalls = []Install{{Pack: "mine", Target: Target{Kind: "claude", Scope: "user", Global: true}}}
}

func installsModel(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withInstalls)
	press(t, m, "down", "3")
	return m, f
}

func TestInstallsTabLoadsOnFirstVisit(t *testing.T) {
	m, f := newTestWith(t, withInstalls)
	if !strings.Contains(view(m), "INSTALLS (…)") || f.in.listCalls != 0 {
		t.Fatalf("unloaded:\n%s", view(m))
	}
	press(t, m, "down", "3")
	if !strings.Contains(view(m), "INSTALLS (2)") || f.in.listCalls != 1 {
		t.Errorf("loaded:\n%s", view(m))
	}
	press(t, m, "1", "3")
	if f.in.listCalls != 1 {
		t.Errorf("a loaded tab is not read again: %d", f.in.listCalls)
	}
}

func TestInstallsTable(t *testing.T) {
	m, _ := installsModel(t)
	v := view(m)
	for _, want := range []string{"PACK", "TARGET", "AS", "PATH", "FROM", "csharp-sdlc", "claude", "project", "/x", "docs", "gascity", "kit", "2 installs · claude 1 · gascity 1"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
}

func TestInstallsDetailShowsTheCommand(t *testing.T) {
	m, _ := installsModel(t)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	if v := view(m); !strings.Contains(v, "ipm install claude csharp-sdlc --scope project --path /x") {
		t.Errorf("claude detail:\n%s", v)
	}
	press(t, m, "down")
	if v := view(m); !strings.Contains(v, "ipm install gascity docs -C /x --name kit") || !strings.Contains(v, "/x/kit") {
		t.Errorf("gascity detail:\n%s", v)
	}
}

func TestInstallsReinstall(t *testing.T) {
	for _, k := range []string{"enter", "i"} {
		m, f := installsModel(t)
		press(t, m, "down", k)
		if len(f.in.targets) != 1 || f.in.targets[0] != (Target{Kind: "gascity", Path: "/x", Name: "kit"}) || m.form != nil {
			t.Errorf("%s: installed %+v form %v", k, f.in.targets, m.form)
		}
		if v := view(m); !strings.Contains(v, "✓ installed docs (gascity /x/kit)") {
			t.Errorf("%s:\n%s", k, v)
		}
	}
}

func TestInstallsNewInstall(t *testing.T) {
	m, _ := installsModel(t)
	press(t, m, "down", "a")
	if m.form == nil || m.form.pack != "docs" || m.form.step != formTarget {
		t.Errorf("a uses the pack under the selection: %+v", m.form)
	}
}

func TestInstallsNewInstallPicksAPack(t *testing.T) {
	m, _ := newTestWith(t, packsSetup)
	press(t, m, "down", "3")
	if !strings.Contains(view(m), "no installs yet — press a to install a pack") {
		t.Fatalf("empty:\n%s", view(m))
	}
	press(t, m, "a")
	if m.form == nil || m.form.step != formPack || !strings.Contains(view(m), "csharp-sdlc") {
		t.Fatalf("several packs and none selected: pick one first:\n%s", view(m))
	}
	press(t, m, "down", "enter")
	if m.form.pack != "docs" || m.form.step != formTarget {
		t.Errorf("pack %q step %v", m.form.pack, m.form.step)
	}
}

func TestInstallsNewInstallOnlyPack(t *testing.T) {
	m, _ := newTestWith(t, withPacks(Pack{Name: "kit"}))
	press(t, m, "down", "3", "a")
	if m.form == nil || m.form.pack != "kit" || m.form.step != formTarget {
		t.Errorf("the only pack needs no picker: %+v", m.form)
	}
}

func TestInstallsNewInstallWithoutPacks(t *testing.T) {
	m, _ := newTest(t)
	press(t, m, "down", "3", "a")
	if m.form != nil || !strings.Contains(m.status, "no pack to install") {
		t.Errorf("form %v status %q", m.form, m.status)
	}
}

func TestInstallsGlobalToggle(t *testing.T) {
	m, _ := installsModel(t)
	press(t, m, "g")
	v := view(m)
	if m.counts[tabInstalls] != 3 || !strings.Contains(v, "mine") || !strings.Contains(v, "global") {
		t.Errorf("global records:\n%s", v)
	}
	press(t, m, "down", "down")
	for _, k := range []string{"enter", "i", "a"} {
		press(t, m, k)
		if m.status != readOnlyGlobal || m.form != nil || m.inst != nil || len(m.d.Installer.(*fakeInstaller).targets) != 0 {
			t.Errorf("%s on a global record from a project: status %q", k, m.status)
		}
	}
	press(t, m, "g")
	if m.counts[tabInstalls] != 2 {
		t.Errorf("g again drops them: %d", m.counts[tabInstalls])
	}
}

func TestInstallsGlobalInGlobalScope(t *testing.T) {
	m, _ := newTestWith(t, func(f fakes) {
		withInstalls(f)
		f.p.scope = "global"
	})
	press(t, m, "down", "3", "g")
	if m.ins.global || !m.statusOK {
		t.Errorf("g does nothing in global scope: global %v status %q", m.ins.global, m.status)
	}
}

func TestInstallsFilter(t *testing.T) {
	m, _ := installsModel(t)
	press(t, m, "/")
	typeText(t, m, "GAS")
	v := view(m)
	if strings.Contains(v, "csharp-sdlc") || !strings.Contains(v, "docs") || !strings.Contains(v, "filter: GAS") {
		t.Errorf("filtered:\n%s", v)
	}
	press(t, m, "esc")
	if !strings.Contains(view(m), "csharp-sdlc") {
		t.Errorf("esc clears:\n%s", view(m))
	}
	// Keys with a meaning elsewhere type into the filter.
	press(t, m, "/")
	typeText(t, m, "a1]qgi")
	if m.ins.filter.value() != "a1]qgi" || m.tab != tabInstalls || m.form != nil || m.inst != nil {
		t.Errorf("filter %q tab %d", m.ins.filter.value(), m.tab)
	}
	if !strings.Contains(view(m), "no install matches") {
		t.Errorf("no match:\n%s", view(m))
	}
}

func TestInstallsLoadError(t *testing.T) {
	m, f := newTestWith(t, withInstalls)
	f.in.listErr = errors.New("ipm.toml is broken")
	press(t, m, "down", "3")
	if v := view(m); !strings.Contains(v, "could not read installs: ipm.toml is broken") || !strings.Contains(v, "INSTALLS (…)") {
		t.Errorf("error body:\n%s", v)
	}
	press(t, m, "down", "2")
	if !strings.Contains(view(m), "csharp-sdlc") {
		t.Errorf("other tabs keep working:\n%s", view(m))
	}
	f.in.listErr = nil
	press(t, m, "down", "3")
	if !strings.Contains(view(m), "INSTALLS (2)") {
		t.Errorf("a retry loads:\n%s", view(m))
	}
}

func TestInstallsHelp(t *testing.T) {
	m, _ := installsModel(t)
	press(t, m, "?")
	v := view(m)
	for _, want := range []string{"reinstall", "new install", "include the global"} {
		if !strings.Contains(v, want) {
			t.Errorf("help lacks %q:\n%s", want, v)
		}
	}
}

func TestInstallsGlobalRecordsWorkInGlobalScope(t *testing.T) {
	m, f := newTestWith(t, func(f fakes) {
		withInstalls(f)
		f.in.installs = f.in.globalInstalls
		f.p.scope = "global"
	})
	press(t, m, "down", "3", "enter")
	if len(f.in.targets) != 1 || !f.in.targets[0].Global {
		t.Errorf("reinstall in global scope: %+v status %q", f.in.targets, m.status)
	}
	press(t, m, "esc", "a")
	if m.form == nil || m.form.pack != "mine" {
		t.Errorf("a opens the form: %v status %q", m.form, m.status)
	}
}
