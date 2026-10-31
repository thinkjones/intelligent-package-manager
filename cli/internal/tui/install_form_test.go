package tui

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"strings"
	"testing"
)

// formModel is the Packs tab with pack kit selected and the install form open.
func formModel(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withPacks(Pack{Name: "kit", Items: []string{tddID}}))
	press(t, m, "2", "tab", "i")
	return m, f
}

func TestInstallFormClaudeFlow(t *testing.T) {
	m, f := formModel(t)
	f.in.lines = []string{"$ claude plugin install kit@work-ipm --scope project"}
	f.in.installs = []Install{{Pack: "kit", Target: Target{Kind: "claude", Scope: "project", Path: "/work"}}}
	v := view(m)
	for _, want := range []string{"Install kit", "▶claude  ", "gascity"} {
		if !strings.Contains(v, want) {
			t.Fatalf("target step lacks %q:\n%s", want, v)
		}
	}
	press(t, m, "enter")
	if !strings.Contains(view(m), "▶project") {
		t.Fatalf("scope step should preselect the default:\n%s", view(m))
	}
	press(t, m, "enter")
	if m.form.folder.Value() != "/work" {
		t.Fatalf("folder step prefilled %q", m.form.folder.Value())
	}
	press(t, m, "enter")
	v = view(m)
	for _, want := range append([]string{"Install kit to Claude Code — project scope in /work", "y install · esc cancel"}, f.in.plan...) {
		if !strings.Contains(v, want) {
			t.Errorf("confirm lacks %q:\n%s", want, v)
		}
	}
	press(t, m, "y")
	v = view(m)
	for _, want := range []string{"ipm install claude kit --scope project --path /work", "$ claude plugin install", "✓ installed kit (claude project /work)"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal lacks %q:\n%s", want, v)
		}
	}
	if foot := strings.Split(strings.TrimRight(view(m), "\n "), "\n"); strings.TrimSpace(foot[len(foot)-1]) != "esc close" {
		t.Errorf("a finished run's footer: %q", foot[len(foot)-1])
	}
	if m.inst.kind != runInstall || m.inst.title != "install" || strings.Join(f.in.installed, ";") != "kit project /work" {
		t.Errorf("title %q installed %v", m.inst.title, f.in.installed)
	}
	if m.counts[tabInstalls] != 1 {
		t.Errorf("installs reload after an install: count %d", m.counts[tabInstalls])
	}
	press(t, m, "esc")
	if m.inst != nil || m.form != nil {
		t.Error("esc should close the finished install")
	}
}

func TestInstallFormGascityFlow(t *testing.T) {
	m, f := formModel(t)
	f.in.def = Target{Kind: "gascity", Path: "/work/.ipm/.gascity", Name: "kit"}
	f.in.cleaned = &Target{Kind: "gascity", Path: "/work/.ipm/.gascity", Name: "kit"}
	f.in.plan = []string{"ipm install gascity kit"}
	press(t, m, "down", "enter")
	v := view(m)
	if m.form.step != formConfirm || strings.Contains(v, "folder") {
		t.Fatalf("gascity goes straight to confirm: step %v\n%s", m.form.step, v)
	}
	for _, want := range []string{"Install kit to Gas City — /work/.ipm/.gascity/kit", "ipm install gascity kit", "y install · esc cancel"} {
		if !strings.Contains(v, want) {
			t.Errorf("confirm lacks %q:\n%s", want, v)
		}
	}
	// Plan gets the default name and no folder, so it picks the pack's
	// scope's default folder.
	if want := (Target{Kind: "gascity", Name: "kit"}); f.in.planned[0] != want {
		t.Errorf("planned %+v, want %+v", f.in.planned[0], want)
	}
	press(t, m, "y")
	if v := view(m); !strings.Contains(v, "✓ installed kit (gascity /work/.ipm/.gascity/kit)") || strings.Contains(v, "-C") {
		t.Errorf("modal:\n%s", v)
	}
	if m.inst.command != "ipm install gascity kit" {
		t.Errorf("the run shows the confirmed command, got %q", m.inst.command)
	}
	if len(f.in.targets) != 1 || f.in.targets[0] != *f.in.cleaned {
		t.Errorf("installed %+v", f.in.targets)
	}
}

func TestInstallFormMuseFlow(t *testing.T) {
	m, f := formModel(t)
	f.in.plan = []string{"ipm install muse kit"}
	press(t, m, "down", "down", "enter")
	v := view(m)
	if m.form.step != formConfirm {
		t.Fatalf("muse goes straight to confirm: step %v\n%s", m.form.step, v)
	}
	for _, want := range []string{"Install kit to Muse Code", "ipm install muse kit", "y install · esc cancel"} {
		if !strings.Contains(v, want) {
			t.Errorf("confirm lacks %q:\n%s", want, v)
		}
	}
	if want := (Target{Kind: "muse"}); f.in.planned[0] != want {
		t.Errorf("planned %+v, want %+v", f.in.planned[0], want)
	}
	press(t, m, "y")
	if v := view(m); !strings.Contains(v, "✓ installed kit (muse)") {
		t.Errorf("modal:\n%s", v)
	}
	if m.inst.command != "ipm install muse kit" {
		t.Errorf("the run shows the confirmed command, got %q", m.inst.command)
	}
}

func TestInstallFormGascityNameFallsBackToPack(t *testing.T) {
	m, f := formModel(t)
	press(t, m, "down", "enter")
	if f.in.planned[0].Name != "kit" {
		t.Errorf("planned %+v", f.in.planned[0])
	}
}

func TestInstallFormUserSkipsFolder(t *testing.T) {
	m, _ := formModel(t)
	press(t, m, "enter", "up", "enter")
	v := view(m)
	if m.form.step != formConfirm || !strings.Contains(v, "Install kit to Claude Code — user scope") || strings.Contains(v, "user scope in") {
		t.Errorf("user scope goes straight to confirm:\n%s", v)
	}
	press(t, m, "y")
	if !strings.Contains(view(m), "✓ installed kit (claude user)") {
		t.Errorf("modal:\n%s", view(m))
	}
}

func TestInstallFormPlanErrorStaysOnStep(t *testing.T) {
	m, f := formModel(t)
	f.in.planErr = errors.New("/nope is not a folder")
	press(t, m, "enter", "enter", "enter")
	if m.form == nil || m.form.step != formFolder || !strings.Contains(view(m), "/nope is not a folder") {
		t.Errorf("claude: step %v:\n%s", m.form, view(m))
	}
	f.in.planErr = errors.New("/work/.ipm/.gascity/kit exists and was not written by ipm — use --force to replace it")
	press(t, m, "esc", "i", "down", "enter")
	v := view(m)
	if m.form == nil || m.form.step != formTarget || !strings.Contains(v, "was not written by ipm") || !strings.Contains(v, "▶gascity") {
		t.Errorf("gascity stays on the target step: step %v:\n%s", m.form, v)
	}
	press(t, m, "esc")
	if m.form != nil || len(f.in.installed) != 0 {
		t.Errorf("esc cancels: form %v installed %v", m.form, f.in.installed)
	}
}

func TestInstallFormFailure(t *testing.T) {
	m, f := formModel(t)
	f.in.err = errors.New("`claude plugin install kit@work-ipm --scope project` failed: exit status 1")
	press(t, m, "enter", "enter", "enter", "y")
	if !strings.Contains(view(m), "✗ `claude plugin install") {
		t.Errorf("modal:\n%s", view(m))
	}
	press(t, m, "esc")
	if m.inst != nil {
		t.Error("esc should close")
	}
}

func TestInstallFormCancelAtEveryStep(t *testing.T) {
	for _, keys := range [][]string{{"esc"}, {"enter", "esc"}, {"enter", "enter", "esc"}, {"enter", "enter", "enter", "esc"}, {"down", "enter", "esc"}} {
		m, f := formModel(t)
		press(t, m, keys...)
		if m.form != nil || m.inst != nil || len(f.in.installed) != 0 {
			t.Errorf("%v: form %v installed %v", keys, m.form, f.in.installed)
		}
	}
}

func TestInstallFormShowsCleanedTarget(t *testing.T) {
	m, f := formModel(t)
	f.in.cleaned = &Target{Kind: "claude", Scope: "project", Path: "/home/me/work"}
	press(t, m, "enter", "enter")
	m.form.folder.SetValue("~/work/")
	press(t, m, "enter")
	if !strings.Contains(view(m), "project scope in /home/me/work") {
		t.Errorf("confirm should show the folder claude runs in:\n%s", view(m))
	}
	press(t, m, "y")
	if !strings.Contains(view(m), "✓ installed kit (claude project /home/me/work)") || f.in.installed[0] != "kit project /home/me/work" {
		t.Errorf("installed %v:\n%s", f.in.installed, view(m))
	}
}

func TestBuildModalEchoesTheCommand(t *testing.T) {
	m, _ := newTestWith(t, withPacks(Pack{Name: "kit"}))
	press(t, m, "2", "tab", "b")
	v := view(m)
	if !strings.Contains(v, "ipm pack build") || strings.Contains(v, "ipm build") {
		t.Errorf("build header:\n%s", v)
	}
}

// While a run streams, b, i and a reinstall are refused with the same words.
func TestBusyRefusesNewRuns(t *testing.T) {
	const busy = "busy — wait for the current run to finish"
	m, f := newTestWith(t, func(f fakes) {
		withPacks(Pack{Name: "kit", Items: []string{tddID}})(f)
		withInstalls(f)
	})
	f.b.gate = make(chan struct{})
	press(t, m, "2", "tab")
	cmd := m.startBuild()
	for _, k := range []string{"b", "i", "3", "enter", "a"} {
		_, c := m.Update(key(k))
		if c != nil || m.form != nil || m.status != busy || len(f.in.installed) != 0 {
			t.Errorf("%s while building: cmd %v form %v status %q installed %v", k, c, m.form, m.status, f.in.installed)
		}
	}
	// The guard is in the starters too, not only in the modal's key handling.
	m.status = ""
	m.openInstallForm("kit", false)
	if m.form != nil || m.status != busy {
		t.Errorf("openInstallForm: form %v status %q", m.form, m.status)
	}
	m.status = ""
	m.reinstall(Install{Pack: "kit", Target: Target{Kind: "claude", Scope: "user"}})
	if m.form != nil || m.status != busy {
		t.Errorf("reinstall: form %v status %q", m.form, m.status)
	}
	m.status = ""
	if c := m.startBuild(); c != nil || m.status != busy {
		t.Errorf("startInstall: cmd %v status %q", c, m.status)
	}
	close(f.b.gate)
	drain(t, m, cmd)
	if len(f.in.installed) != 0 {
		t.Errorf("installed %v", f.in.installed)
	}
}

func TestInstallFormLongFolderShowsTheEnd(t *testing.T) {
	m, f := formModel(t)
	f.in.def.Path = "/" + strings.Repeat("deep/", 40) + "tail-end"
	press(t, m, "enter", "enter")
	if v := view(m); !strings.Contains(v, "tail-end") {
		t.Errorf("the folder field scrolls to the cursor:\n%s", v)
	}
	// A narrower terminal re-fits the field around the cursor.
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	if v := view(m); !strings.Contains(v, "tail-end") {
		t.Errorf("after a resize:\n%s", v)
	}
}
