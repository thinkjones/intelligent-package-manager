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
	formTarget                 // claude or gascity
	formScope                  // claude: user, project or local
	formFolder                 // claude: the project folder; gascity: the parent folder
	formName                   // gascity: the pack folder's name
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
}

// installForm is the install form before it runs: pick a target, where it
// goes, then confirm the commands.
type installForm struct {
	pack    string
	global  bool     // the pack is of the user scope, installed from a project
	packs   []string // the choices of the pack step
	step    formStep
	sel     int
	kind    string
	folder  textinput.Model
	name    textinput.Model
	target  Target
	plan    []string
	err     string // why the last plan was refused
	targets []string
}

// openInstallForm starts installing the named pack. global marks a pack of the
// user scope listed from a project.
func (m *Model) openInstallForm(pack string, global bool) {
	if m.refuseBusy() {
		return
	}
	m.form = &installForm{pack: pack, global: global, step: formTarget, folder: newInput("folder"), name: newInput("name"), targets: m.d.Installer.Targets()}
}

// openInstallPicker starts an install that first asks which pack.
func (m *Model) openInstallPicker() {
	if m.refuseBusy() {
		return
	}
	m.openInstallForm("", false)
	m.form.step = formPack
	for _, p := range m.packs {
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
	f.err = ""
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
		switch {
		case k == "enter" && f.kind == "gascity":
			f.step = formName
			f.folder.Blur()
			f.name.Focus()
		case k == "enter":
			m.planForm(Target{Kind: "claude", Scope: claudeScopes[f.sel][0], Path: f.folder.Value(), Global: f.global})
		default:
			f.folder, _ = f.folder.Update(msg)
		}
	case formName:
		if k == "enter" {
			m.planForm(Target{Kind: "gascity", Path: f.folder.Value(), Name: f.name.Value(), Global: f.global})
			return nil
		}
		f.name, _ = f.name.Update(msg)
	case formConfirm:
		if k == "y" {
			pack, t := f.pack, f.target
			m.form = nil
			return m.runInstall(pack, t)
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
// the installer's default.
func (m *Model) chooseKind(kind string) {
	f := m.form
	def := m.d.Installer.Default(f.pack, kind)
	f.kind, f.sel = kind, 0
	f.folder.SetValue(def.Path)
	if kind == "gascity" {
		f.name.SetValue(def.Name)
		if def.Name == "" {
			f.name.SetValue(f.pack)
		}
		f.step = formFolder
		f.folder.Focus()
		return
	}
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

// runInstall installs pack at t in the output modal.
func (m *Model) runInstall(pack string, t Target) tea.Cmd {
	if m.refuseBusy() {
		return nil
	}
	return m.stream("install", installCommand(pack, t), "✓ installed "+pack+" ("+describe(t)+")", func(w io.Writer) error {
		return m.d.Installer.Install(context.Background(), pack, t, w)
	})
}

// reinstall installs a recorded install again, without the form.
func (m *Model) reinstall(in Install) tea.Cmd { return m.runInstall(in.Pack, in.Target) }

// describe is a target as people read it: "claude user", "claude project /work"
// or "gascity /work/kit".
func describe(t Target) string {
	switch {
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
	if t.Kind == "gascity" {
		parts = append(parts, "-C", quote(t.Path), "--name", quote(t.Name))
	} else {
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
		if f.kind == "gascity" {
			label, head = "parent folder  ", "Install "+f.pack+" as a Gas City pack folder"
		}
		lines = append(lines, stTitle.Render(head), "", stMuted.Render(label)+f.folder.View())
	case formName:
		lines = append(lines, stTitle.Render("Install "+f.pack+" as a Gas City pack folder"), "",
			stMuted.Render("parent folder  ")+stText.Render(f.folder.Value()),
			stMuted.Render("name           ")+f.name.View())
	case formConfirm:
		head := "Install " + f.pack + " to "
		if f.target.Kind == "gascity" {
			head += "Gas City — " + filepath.Join(f.target.Path, f.target.Name)
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
	if f.step == formConfirm {
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
