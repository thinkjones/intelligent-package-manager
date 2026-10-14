package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// filterBox is the filter of a tab other than Catalog: a text input and the
// substring match that narrows the tab's rows.
type filterBox struct {
	in     textinput.Model
	typing bool // the input has focus
}

func newFilterBox() filterBox {
	return filterBox{in: newInput("type to filter")}
}

// value is the text rows must contain, "" when unfiltered.
func (f *filterBox) value() string { return strings.TrimSpace(f.in.Value()) }

// focus starts typing into the filter.
func (f *filterBox) focus() {
	f.typing = true
	f.in.Focus()
}

func (f *filterBox) blur() {
	f.typing = false
	f.in.Blur()
}

// clear empties the filter and leaves it.
func (f *filterBox) clear() {
	f.in.SetValue("")
	f.blur()
}

// match reports whether any visible cell contains the filter text, ignoring
// case. An empty filter matches every row.
func (f *filterBox) match(cells []string) bool {
	q := strings.ToLower(f.value())
	if q == "" {
		return true
	}
	for _, c := range cells {
		if strings.Contains(strings.ToLower(c), q) {
			return true
		}
	}
	return false
}

// key handles a key press while the filter has focus, and reports whether the
// filter text changed. esc clears the filter; enter keeps it and leaves the
// input; everything else types.
func (f *filterBox) key(msg tea.KeyPressMsg) (changed bool) {
	switch msg.String() {
	case "esc":
		changed = f.in.Value() != ""
		f.clear()
	case "enter":
		f.blur()
	default:
		before := f.in.Value()
		f.in, _ = f.in.Update(msg)
		changed = f.in.Value() != before
	}
	return changed
}

// line is the filter as the title of a panel: the input while typing, the
// text while it is set, nothing otherwise.
func (f *filterBox) line() string {
	switch {
	case f.typing:
		return stMuted.Render("filter: ") + f.in.View()
	case f.value() != "":
		return stMuted.Render("filter: ") + stAmber.Render(f.value())
	}
	return ""
}
