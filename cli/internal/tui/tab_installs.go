package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// installsTab is the Installs tab's state: the recorded installs, selection
// and filter.
type installsTab struct {
	rows   []Install
	err    error // the last load's error
	loaded bool
	global bool // include the user scope's records
	filter filterBox

	sel, offset int
}

var installCols = []column{
	{Title: "PACK", Min: 8, Max: 24},
	{Title: "TARGET", Min: 6, Max: 7},
	{Title: "AS", Min: 4, Max: 16},
	{Title: "PATH", Min: 8, Max: 40},
	{Title: "FROM", Min: 4, Max: 7},
}

// installAs is the AS cell: the claude scope or the gascity folder name.
func installAs(in Install) string {
	if in.Kind == "gascity" {
		return in.Name
	}
	return in.Scope
}

// installFrom is the FROM cell: the scope whose ipm.toml records the install.
func installFrom(in Install) string {
	if in.Global {
		return "global"
	}
	return "project"
}

// cells are an install's visible column text.
func (in Install) cells() []string {
	return []string{in.Pack, in.Kind, installAs(in), in.Path, installFrom(in)}
}

// loadInstallsTab reads the recorded installs, keeping the selection on the
// same record when it is still there.
func (m *Model) loadInstallsTab() {
	ins := &m.ins
	keep := m.selectedInstall()
	rows, err := m.d.Installer.List(ins.global)
	ins.loaded, ins.err = true, err
	if err != nil {
		ins.rows = nil
		m.counts[tabInstalls] = notLoaded
		return
	}
	ins.rows = rows
	m.counts[tabInstalls] = len(rows)
	ins.sel = 0
	for i, in := range m.visibleInstalls() {
		if keep != nil && in == *keep {
			ins.sel = i
		}
	}
	m.clampInstalls()
}

// visibleInstalls are the rows the filter keeps.
func (m *Model) visibleInstalls() []Install {
	var out []Install
	for _, in := range m.ins.rows {
		if m.ins.filter.match(in.cells()) {
			out = append(out, in)
		}
	}
	return out
}

// selectedInstall is the record under the cursor, nil when there is none.
func (m *Model) selectedInstall() *Install {
	vis := m.visibleInstalls()
	if m.ins.sel < 0 || m.ins.sel >= len(vis) {
		return nil
	}
	return &vis[m.ins.sel]
}

func (m *Model) clampInstalls() {
	m.ins.sel = max(0, min(m.ins.sel, len(m.visibleInstalls())-1))
}

// installsKey handles a key press on the Installs tab.
func (m *Model) installsKey(msg tea.KeyPressMsg) tea.Cmd {
	ins, k := &m.ins, msg.String()
	if ins.filter.typing {
		if ins.filter.key(msg) {
			ins.sel, ins.offset = 0, 0
			m.clampInstalls()
		}
		return nil
	}
	switch k {
	case "q":
		return tea.Quit
	case "?":
		m.help = true
	case "/":
		ins.filter.focus()
	case "g":
		if m.d.Packs.Scope() == "global" {
			m.note("already in global scope")
			return nil
		}
		ins.global = !ins.global
		m.loadInstallsTab()
	case "down", "j":
		ins.sel = max(0, min(ins.sel+1, len(m.visibleInstalls())-1))
	case "up", "k":
		ins.sel = max(0, ins.sel-1)
	case "esc":
		if ins.filter.value() != "" {
			ins.filter.clear()
			ins.sel, ins.offset = 0, 0
			m.clampInstalls()
		}
	case "enter", "i":
		if in := m.selectedInstall(); in != nil {
			if m.readOnlyInstall(in) {
				return nil
			}
			return m.reinstall(*in)
		}
		m.status = "no install selected"
	case "a":
		m.newInstall()
	}
	return nil
}

// readOnlyInstall reports whether in is a global record shown from a project,
// saying so in the status: those change from ipm tui -g.
func (m *Model) readOnlyInstall(in *Install) bool {
	if in.Global && m.d.Packs.Scope() != "global" {
		m.status = readOnlyGlobal
		return true
	}
	return false
}

// newInstall opens the install form for the pack under the selection, or the
// only pack; with several and none selected it asks which first.
func (m *Model) newInstall() {
	if in := m.selectedInstall(); in != nil {
		if !m.readOnlyInstall(in) {
			m.openInstallForm(in.Pack, in.Global)
		}
		return
	}
	switch len(m.packs) {
	case 0:
		m.status = "no pack to install — create one on the Packs tab"
	case 1:
		m.openInstallForm(m.packs[0].Name, false)
	default:
		m.openInstallPicker()
	}
}

// installsStats is the Installs tab's strip: the total and each target's
// share, then the filter while set.
func (m *Model) installsStats() string {
	ins := &m.ins
	if !ins.loaded || ins.err != nil {
		return ""
	}
	parts := []string{count(len(ins.rows), "install")}
	for _, kind := range m.d.Installer.Targets() {
		n := 0
		for _, in := range ins.rows {
			if in.Kind == kind {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", kind, n))
		}
	}
	s := strings.Join(parts, stMuted.Render(" · "))
	if f := ins.filter.value(); f != "" {
		s += stMuted.Render(" · filter: ") + stAmber.Render(f)
	}
	return s
}

// installsHints are the Installs footer's keys, before help and quit.
func (m *Model) installsHints() []string {
	if m.ins.filter.typing {
		return []string{"type to filter", "enter keep", "esc clear"}
	}
	return []string{"↑↓ move", "enter reinstall", "a new", "g global", "/ filter", "1-4 tabs"}
}

// installsBody is the table of installs, with the selected one's detail
// beside it when the screen is wide, h lines in all.
func (m *Model) installsBody(w, h int) []string {
	ins := &m.ins
	message := func(line string) []string {
		lines := []string{line}
		for len(lines) < h-2 {
			lines = append(lines, "")
		}
		return panel(stMuted.Render("installs"), lines, w, true)
	}
	if ins.err != nil {
		return message(stError.Render("could not read installs: " + ins.err.Error()))
	}
	vis := m.visibleInstalls()
	switch {
	case len(vis) == 0 && ins.filter.value() != "":
		return message(stMuted.Render("no install matches the filter — esc clears it"))
	case len(vis) == 0:
		return message(stMuted.Render("no installs yet — press a to install a pack"))
	}
	wide := w >= wideWidth
	tw := w
	if wide {
		tw = w * 62 / 100
	}
	rows := make([][]string, len(vis))
	styles := make([][]lipgloss.Style, len(vis))
	for i, in := range vis {
		rows[i] = in.cells()
		from := stText
		if in.Global {
			from = stMuted
		}
		styles[i] = []lipgloss.Style{stText, stText, stText, stMuted, from}
	}
	title := stMuted.Render("installs")
	if f := ins.filter.line(); f != "" {
		ins.filter.in.SetWidth(max(10, tw-26))
		title += stMuted.Render(" ── ") + f
	}
	list := panel(title, table(installCols, rows, styles, ins.sel, &ins.offset, tw-4, h-2, true), tw, true)
	if !wide {
		return list
	}
	return joinPanels(list, m.installDetail(vis[ins.sel], w-tw, h))
}

// installDetail is the panel of one install in full, and the command that
// redoes it, w wide and h lines high.
func (m *Model) installDetail(in Install, w, h int) []string {
	var lines []string
	add := func(name, value string) {
		if value != "" {
			lines = append(lines, stMuted.Render(fit(name, 8))+stText.Render(value))
		}
	}
	add("target", in.Kind)
	add("scope", in.Scope)
	add("name", in.Name)
	add("path", in.Path)
	if in.Kind == "gascity" {
		add("folder", strings.TrimPrefix(describe(in.Target), "gascity "))
	}
	add("from", installFrom(in))
	lines = append(lines, "", stMuted.Render("reinstall with"))
	for _, l := range strings.Split(ansi.Wrap(installCommand(in.Pack, in.Target), max(1, w-6), ""), "\n") {
		lines = append(lines, stAmber.Render(l))
	}
	for len(lines) < h-2 {
		lines = append(lines, "")
	}
	return panel(stTitle.Render(in.Pack), lines[:h-2], w, false)
}
