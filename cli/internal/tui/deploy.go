package tui

import (
	"context"
	"io"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type deployStep int

const (
	stepTarget deployStep = iota
	stepPath
	stepConfirm
)

// deployScopes are Claude Code's install scopes, in the order offered.
var deployScopes = [][2]string{
	{"user", "every project, just you"},
	{"project", "a folder, shared through its .claude/settings.json"},
	{"local", "a folder, just you"},
}

// deployModal is the deploy modal before it runs: pick a target, a folder
// for project or local scope, then confirm the commands.
type deployModal struct {
	pack   string
	step   deployStep
	sel    int
	path   textinput.Model
	target Target
	plan   []string
}

// openDeploy starts deploying the active pack.
func (m *Model) openDeploy() {
	p := m.activePack()
	if p == nil {
		m.status = "no pack to deploy — press p to create one"
		return
	}
	def := m.d.Deployer.Default(p.Name)
	dm := &deployModal{pack: p.Name, path: newInput("folder")}
	for i, s := range deployScopes {
		if s[0] == def.Scope {
			dm.sel = i
		}
	}
	dm.path.SetValue(def.Path)
	m.deploy = dm
}

// deployKey handles keys while the deploy modal is open.
func (m *Model) deployKey(msg keyMsg) tea.Cmd {
	dm, k := m.deploy, msg.String()
	if k == "esc" {
		m.deploy = nil
		return nil
	}
	switch dm.step {
	case stepTarget:
		switch k {
		case "up", "k":
			dm.sel = max(0, dm.sel-1)
		case "down", "j":
			dm.sel = min(len(deployScopes)-1, dm.sel+1)
		case "enter":
			if scope := deployScopes[dm.sel][0]; scope == "user" {
				m.planFor(Target{Scope: "user"})
			} else {
				dm.step = stepPath
				dm.path.Focus()
			}
		}
	case stepPath:
		if k == "enter" {
			m.planFor(Target{Scope: deployScopes[dm.sel][0], Path: dm.path.Value()})
			return nil
		}
		dm.path, _ = dm.path.Update(msg)
	case stepConfirm:
		if k == "y" {
			return m.startDeploy()
		}
	}
	return nil
}

// planFor asks the deployer what it will run for t, and shows it to confirm.
func (m *Model) planFor(t Target) {
	plan, err := m.d.Deployer.Plan(m.deploy.pack, t)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.deploy.target, m.deploy.plan, m.deploy.step = t, plan, stepConfirm
}

func (m *Model) startDeploy() tea.Cmd {
	pack, t := m.deploy.pack, m.deploy.target
	m.deploy = nil
	return m.stream("deploy", "✓ deployed "+pack+" ("+describe(t)+")", func(w io.Writer) error {
		return m.d.Deployer.Deploy(context.Background(), pack, t, w)
	})
}

// describe is a target as people read it: "user" or "project /work".
func describe(t Target) string {
	if t.Scope == "user" {
		return "user"
	}
	return t.Scope + " " + t.Path
}

// deployLines is the modal in place of the body: exactly rows lines.
func (m *Model) deployLines(w, rows int) []string {
	dm := m.deploy
	var lines []string
	switch dm.step {
	case stepTarget:
		lines = append(lines, stTitle.Render("Deploy "+dm.pack+" to Claude Code"), "")
		for i, s := range deployScopes {
			if i == dm.sel {
				lines = append(lines, stSelected.Render("▶"+fit(s[0], 9))+stText.Render(s[1]))
			} else {
				lines = append(lines, " "+stText.Render(fit(s[0], 9))+stMuted.Render(s[1]))
			}
		}
		lines = append(lines, "", stMuted.Render("enter next · esc cancel"))
	case stepPath:
		lines = append(lines, stTitle.Render("Deploy "+dm.pack+" to Claude Code — "+deployScopes[dm.sel][0]+" scope"), "",
			stMuted.Render("folder  ")+dm.path.View(), "", stMuted.Render("enter next · esc cancel"))
	case stepConfirm:
		head := "Deploy " + dm.pack + " to Claude Code — " + dm.target.Scope + " scope"
		if dm.target.Path != "" {
			head += " in " + dm.target.Path
		}
		lines = append(lines, stTitle.Render(head), "")
		for _, p := range dm.plan {
			lines = append(lines, "  "+stAmber.Render(p))
		}
		lines = append(lines, "", stMuted.Render("y deploy · esc cancel"))
	}
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
