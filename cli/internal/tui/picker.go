package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
)

// picker is the modal for switching packs or creating one.
type picker struct {
	sel      int
	creating bool
	// step is 0 while typing the name, 1 while typing the description.
	step        int
	name, descr textinput.Model
}

func newInput(placeholder string) textinput.Model {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	in.SetStyles(st)
	in.Prompt = ""
	in.Placeholder = placeholder
	return in
}

func (m *Model) openPicker(creating bool) {
	p := &picker{creating: creating, name: newInput("pack name"), descr: newInput("description (optional)")}
	for i, pk := range m.packs {
		if pk.Name == m.active {
			p.sel = i
		}
	}
	if creating {
		p.name.Focus()
	}
	m.picker = p
}

// pickerKey handles a key while the picker is open.
func (m *Model) pickerKey(msg keyMsg) {
	p := m.picker
	k := msg.String()
	if k == "esc" {
		m.picker = nil
		return
	}
	if !p.creating {
		switch k {
		case "down", "j":
			p.sel = min(len(m.packs)-1, p.sel+1)
		case "up", "k":
			p.sel = max(0, p.sel-1)
		case "n":
			m.openPicker(true)
		case "enter":
			if p.sel < len(m.packs) {
				m.active, m.traySel = m.packs[p.sel].Name, 0
				m.note("active pack: %s", m.active)
			}
			m.picker = nil
		}
		return
	}
	if k == "enter" {
		if p.step == 0 {
			if strings.TrimSpace(p.name.Value()) == "" {
				return
			}
			p.step = 1
			p.name.Blur()
			p.descr.Focus()
			return
		}
		name := strings.TrimSpace(p.name.Value())
		if err := m.d.Packs.Create(name, strings.TrimSpace(p.descr.Value())); err != nil {
			m.status = err.Error()
			p.step = 0
			p.descr.Blur()
			p.name.Focus()
			return
		}
		m.picker = nil
		m.reloadPacks()
		m.active, m.traySel = name, 0
		m.note("created pack %s", name)
		return
	}
	if p.step == 0 {
		p.name, _ = p.name.Update(msg)
	} else {
		p.descr, _ = p.descr.Update(msg)
	}
}

// pickerLines is the picker in place of the body: exactly rows lines.
func (m *Model) pickerLines(w, rows int) []string {
	p := m.picker
	var lines []string
	if p.creating {
		lines = append(lines, stTitle.Render("New pack in "+m.d.Packs.Scope()), "",
			stMuted.Render("name         ")+p.name.View(),
			stMuted.Render("description  ")+p.descr.View(), "",
			stMuted.Render("enter next · esc cancel"))
	} else {
		lines = append(lines, stTitle.Render("Packs in "+m.d.Packs.Scope()), "")
		for i, pk := range m.packs {
			row := pk.Name + stMuted.Render("  "+pk.Description)
			if i == p.sel {
				lines = append(lines, stSelected.Render("▶"+pk.Name)+stMuted.Render("  "+pk.Description))
			} else {
				lines = append(lines, " "+row)
			}
		}
		lines = append(lines, "", stMuted.Render("enter switch · n new pack · esc cancel"))
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
