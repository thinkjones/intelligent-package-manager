package tui

import (
	"errors"
	"strings"
	"testing"
)

// formModel is the Packs tab with pack kit selected and the install form open.
func formModel(t *testing.T) (*Model, fakes) {
	t.Helper()
	m, f := newTestWith(t, withPacks(Pack{Name: "kit", Items: []string{tddID}}))
	press(t, m, "down", "2", "i")
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
	press(t, m, "down", "enter")
	if m.form.step != formFolder || m.form.folder.Value() != "/work" || !strings.Contains(view(m), "parent folder") {
		t.Fatalf("gascity asks for the parent folder first:\n%s", view(m))
	}
	press(t, m, "enter")
	if m.form.step != formName || m.form.name.Value() != "kit" {
		t.Fatalf("name defaults to the pack: step %v name %q", m.form.step, m.form.name.Value())
	}
	press(t, m, "backspace", "backspace", "backspace")
	typeText(t, m, "crew")
	press(t, m, "enter")
	if m.form.step != formConfirm {
		t.Fatalf("step %v", m.form.step)
	}
	press(t, m, "y")
	v := view(m)
	for _, want := range []string{"ipm install gascity kit -C /work --name crew", "✓ installed kit (gascity /work/crew)"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal lacks %q:\n%s", want, v)
		}
	}
	if len(f.in.targets) != 1 || f.in.targets[0] != (Target{Kind: "gascity", Path: "/work", Name: "crew"}) {
		t.Errorf("installed %+v", f.in.targets)
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
	press(t, m, "esc", "i", "down", "enter", "enter", "enter")
	if m.form == nil || m.form.step != formFolder || !strings.Contains(view(m), "/nope is not a folder") {
		t.Errorf("gascity returns to the folder: step %v:\n%s", m.form, view(m))
	}
	f.in.planErr = nil
	press(t, m, "enter", "enter")
	if m.form.step != formConfirm {
		t.Errorf("a fixed folder goes on: step %v", m.form.step)
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
	for _, keys := range [][]string{{"esc"}, {"enter", "esc"}, {"enter", "enter", "esc"}, {"enter", "enter", "enter", "esc"}, {"down", "enter", "esc"}, {"down", "enter", "enter", "esc"}, {"down", "enter", "enter", "enter", "esc"}} {
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
	press(t, m, "down", "2", "b")
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
	press(t, m, "down", "2")
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
	if c := m.reinstall(Install{Pack: "kit", Target: Target{Kind: "claude", Scope: "user"}}); c != nil || m.status != busy {
		t.Errorf("reinstall: cmd %v status %q", c, m.status)
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
