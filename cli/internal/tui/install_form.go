package tui

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type formStep int

const (
	formPack   formStep = iota // which pack, when none was preset
	formTarget                 // claude, gascity or muse
	formScope                  // claude: user, project or local
	formFolder                 // claude: the project folder
	formConfirm
)

// claudeScopes are Claude Code's install scopes, in the order offered.
var claudeScopes = [][2]string{
	{"user", "every project, just you"},
	{"project", "a folder, shared through its .claude/settings.json"},
	{"local", "a folder, just you"},
}

// targetHelp says what each install target is, for the target step.
var targetHelp = map[string]string{
	"claude":  "Claude Code, as a plugin",
	"gascity": "a Gas City pack folder",
	"muse":    "Muse Code, as plugins",
}

// installForm is the install form before it runs: pick a target, where it
// goes (claude only: gascity goes to its default folder, and muse has one
// set of plugins per user), then confirm the commands.
type installForm struct {
	pack    string
	global  bool     // the pack is of the user scope, installed from a project
	packs   []string // the choices of the pack step
	step    formStep
	sel     int
	folder  textinput.Model
	target  Target
	plan    []string
	err     string // why the last plan was refused
	refused bool   // a reinstall's plan was refused: nothing to confirm
	targets []string
}

// openInstallForm starts installing the named pack. global marks a pack of the
// user scope listed from a project.
func (m *Model) openInstallForm(pack string, global bool) {
	if m.refuseBusy() {
		return
	}
	m.form = &installForm{pack: pack, global: global, step: formTarget, folder: newInput("folder"), targets: m.d.Installer.Targets()}
}

// openInstallPicker starts an install that first asks which pack, with preset
// highlighted when it is one of them.
func (m *Model) openInstallPicker(preset string) {
	if m.refuseBusy() {
		return
	}
	m.openInstallForm("", false)
	m.form.step = formPack
	for _, p := range m.packs {
		if p.Name == preset {
			m.form.sel = len(m.form.packs)
		}
		m.form.packs = append(m.form.packs, p.Name)
	}
}

// formKey handles keys while the install form is open.
func (m *Model) formKey(msg keyMsg) tea.Cmd {
	f, k := m.form, msg.String()
	if k == "esc" {
		m.form = nil
		return nil
	}
	if !f.refused {
		f.err = ""
	}
	switch f.step {
	case formPack:
		if f.move(k, len(f.packs)) {
			return nil
		}
		if k == "enter" {
			f.pack, f.sel, f.step = f.packs[f.sel], 0, formTarget
		}
	case formTarget:
		if f.move(k, len(f.targets)) {
			return nil
		}
		if k == "enter" {
			m.chooseKind(f.targets[f.sel])
		}
	case formScope:
		if f.move(k, len(claudeScopes)) {
			return nil
		}
		if k == "enter" {
			if scope := claudeScopes[f.sel][0]; scope == "user" {
				m.planForm(Target{Kind: "claude", Scope: "user", Global: f.global})
			} else {
				f.step = formFolder
				f.folder.Focus()
			}
		}
	case formFolder:
		if k == "enter" {
			m.planForm(Target{Kind: "claude", Scope: claudeScopes[f.sel][0], Path: f.folder.Value(), Global: f.global})
		} else {
			f.folder, _ = f.folder.Update(msg)
		}
	case formConfirm:
		if k == "y" && !f.refused {
			pack, t, command := f.pack, f.target, installCommand(f.pack, f.target)
			if (t.Kind == "gascity" || t.Kind == "muse") && len(f.plan) == 1 {
				// Its plan is the one command the user confirmed.
				command = f.plan[0]
			}
			m.form = nil
			return m.runInstall(pack, t, command)
		}
	}
	return nil
}

// move moves the selection of a list of n on up/down, and reports whether k
// was one of those keys.
func (f *installForm) move(k string, n int) bool {
	switch k {
	case "up", "k":
		f.sel = max(0, f.sel-1)
	case "down", "j":
		f.sel = min(n-1, f.sel+1)
	default:
		return false
	}
	return true
}

// chooseKind takes the target kind and opens its first step, prefilled from
// the installer's default. Gascity has no steps: it plans the default name
// with no folder, which the installer reads as the default folder of the
// pack's own scope, and goes to confirm. Muse has no steps either.
func (m *Model) chooseKind(kind string) {
	f := m.form
	def := m.d.Installer.Default(f.pack, kind)
	if kind == "muse" {
		m.planForm(Target{Kind: "muse", Global: f.global})
		return
	}
	if kind == "gascity" {
		name := def.Name
		if name == "" {
			name = f.pack
		}
		m.planForm(Target{Kind: "gascity", Name: name, Global: f.global})
		return
	}
	f.sel = 0
	f.folder.SetValue(def.Path)
	for i, s := range claudeScopes {
		if s[0] == def.Scope {
			f.sel = i
		}
	}
	f.step = formScope
}

// planForm asks the installer what it will run for t, and shows it to
// confirm; a refusal shows in the form, which stays on its step.
func (m *Model) planForm(t Target) {
	f := m.form
	cleaned, plan, err := m.d.Installer.Plan(f.pack, t)
	if err != nil {
		f.err = err.Error()
		return
	}
	f.target, f.plan, f.step = cleaned, plan, formConfirm
}

// runInstall installs pack at t in the output modal, titled with command.
func (m *Model) runInstall(pack string, t Target, command string) tea.Cmd {
	if m.refuseBusy() {
		return nil
	}
	return m.stream(runInstall, "install", command, "✓ installed "+pack+" ("+describe(t)+")", func(w io.Writer) error {
		return m.d.Installer.Install(context.Background(), pack, t, w)
	})
}

// reinstall opens the form at its confirm step for a recorded install, showing
// the exact commands; a refused plan shows in the form with nothing to confirm.
func (m *Model) reinstall(in Install) {
	m.openInstallForm(in.Pack, in.Global)
	if m.form == nil {
		return
	}
	f := m.form
	cleaned, plan, err := m.d.Installer.Plan(in.Pack, in.Target)
	f.step = formConfirm
	if err != nil {
		f.err, f.refused = err.Error(), true
		return
	}
	f.target, f.plan = cleaned, plan
}

// describe is a target as people read it: "claude user", "claude project
// /work", "gascity /work/kit" or "muse".
func describe(t Target) string {
	switch {
	case t.Kind == "muse":
		return "muse"
	case t.Kind == "gascity":
		return "gascity " + filepath.Join(t.Path, t.Name)
	case t.Scope == "user":
		return "claude user"
	}
	return "claude " + t.Scope + " " + t.Path
}

// installCommand is the ipm command that installs pack at t.
func installCommand(pack string, t Target) string {
	parts := []string{"ipm install", t.Kind, quote(pack)}
	switch t.Kind {
	case "muse": // one set of plugins per user: nothing to name
	case "gascity":
		parts = append(parts, "-C", quote(t.Path), "--name", quote(t.Name))
	default:
		parts = append(parts, "--scope", t.Scope)
		if t.Path != "" {
			parts = append(parts, "--path", quote(t.Path))
		}
	}
	if t.Global {
		parts = append(parts, "-g")
	}
	return strings.Join(parts, " ")
}

// quote single-quotes s for a shell when it needs it.
func quote(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\"'$\\") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}

// fitInput gives in the width it is drawn at, keeping its cursor in view: the
// width is only known at render time, after the value was set.
func fitInput(in *textinput.Model, w int) {
	if in.Width() == w {
		return
	}
	in.SetWidth(w)
	in.SetCursor(in.Position())
}

// formLines is the form in place of the body: exactly rows lines.
func (m *Model) formLines(w, rows int) []string {
	f := m.form
	var lines []string
	list := func(title string, names []string, help func(string) string) {
		lines = append(lines, stTitle.Render(title), "")
		nw := 9
		for _, n := range names {
			nw = max(nw, lipgloss.Width(n))
		}
		for i, n := range names {
			if i == f.sel {
				lines = append(lines, stSelected.Render("▶"+fit(n, nw))+"  "+stText.Render(help(n)))
			} else {
				lines = append(lines, " "+stText.Render(fit(n, nw))+"  "+stMuted.Render(help(n)))
			}
		}
	}
	packHelp := func(n string) string {
		for _, p := range m.packs {
			if p.Name == n {
				return p.Description
			}
		}
		return ""
	}
	switch f.step {
	case formPack:
		list("Install which pack?", f.packs, packHelp)
	case formTarget:
		list("Install "+f.pack+" to", f.targets, func(n string) string { return targetHelp[n] })
	case formScope:
		names := make([]string, len(claudeScopes))
		help := map[string]string{}
		for i, s := range claudeScopes {
			names[i], help[s[0]] = s[0], s[1]
		}
		list("Install "+f.pack+" to Claude Code", names, func(n string) string { return help[n] })
	case formFolder:
		label, head := "folder  ", "Install "+f.pack+" to Claude Code — "+claudeScopes[f.sel][0]+" scope"
		fitInput(&f.folder, max(10, w-lipgloss.Width(label)-1))
		lines = append(lines, stTitle.Render(head), "", stMuted.Render(label)+f.folder.View())
	case formConfirm:
		head := "Install " + f.pack + " to "
		if f.refused {
			head = "Reinstall " + f.pack
		} else if f.target.Kind == "gascity" {
			head += "Gas City — " + filepath.Join(f.target.Path, f.target.Name)
		} else if f.target.Kind == "muse" {
			head += "Muse Code"
		} else {
			head += "Claude Code — " + f.target.Scope + " scope"
			if f.target.Path != "" {
				head += " in " + f.target.Path
			}
		}
		lines = append(lines, stTitle.Render(head), "")
		for _, p := range f.plan {
			lines = append(lines, "  "+stAmber.Render(p))
		}
	}
	if f.err != "" {
		lines = append(lines, "", stError.Render(f.err))
	}
	foot := "enter next · esc cancel"
	switch {
	case f.refused:
		foot = "esc cancel"
	case f.step == formConfirm:
		foot = "y install · esc cancel"
	}
	lines = append(lines, "", stMuted.Render(foot))
	out := make([]string, 0, rows)
	for _, l := range lines {
		if len(out) == rows {
			break
		}
		out = append(out, fit(l, w))
	}
	for len(out) < rows {
		out = append(out, spaces(w))
	}
	return out
}
