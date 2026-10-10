package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Tree layout, from the spec.
const (
	labelWidth   = 6  // type labels: repo, market, plugin, …
	nameWidth    = 24 // names are cut to this, so descriptions line up
	maxDepth     = 10 // the path walk stops here
	descMaxLines = 3  // the cursor's description wraps to at most this
	childRowsMin = 10 // a tree shorter than this shows no children
	maxChildren  = 8  // children shown under the cursor before "↓ N more"
)

// treeView is the tree around a cursor node: the path from its root to its
// parent, its siblings (Siblings[Cursor] is the node), its children, and its
// other parents.
type treeView struct {
	Path     []Ref
	Siblings []Ref
	Cursor   int
	Node     Node
	Children []Ref
	AlsoIn   []Ref
}

// buildTree centres a tree on id. chosen maps a node to the parent the user
// picked for it; otherwise a node's first parent is followed.
func buildTree(c Catalog, id string, chosen map[string]string) (treeView, error) {
	n, err := c.Node(id)
	if err != nil {
		return treeView{}, err
	}
	tv := treeView{Node: n, Children: n.Children, Siblings: []Ref{refOf(n)}}
	parent := pickParent(n, chosen)
	for _, p := range n.Parents {
		if p.ID != parent {
			tv.AlsoIn = append(tv.AlsoIn, p)
		}
	}
	if parent == "" {
		return tv, nil
	}
	pn, err := c.Node(parent)
	if err != nil {
		return tv, err
	}
	tv.Siblings, tv.Cursor = pn.Children, -1
	for i, s := range pn.Children {
		if s.ID == id {
			tv.Cursor = i
		}
	}
	if tv.Cursor < 0 {
		tv.Siblings = append(append([]Ref(nil), pn.Children...), refOf(n))
		tv.Cursor = len(tv.Siblings) - 1
	}
	tv.Path = []Ref{refOf(pn)}
	for cur := pn; len(tv.Path) < maxDepth; {
		pid := pickParent(cur, chosen)
		if pid == "" {
			break
		}
		next, err := c.Node(pid)
		if err != nil {
			break
		}
		tv.Path = append([]Ref{refOf(next)}, tv.Path...)
		cur = next
	}
	return tv, nil
}

func pickParent(n Node, chosen map[string]string) string {
	if want, ok := chosen[n.ID]; ok {
		for _, p := range n.Parents {
			if p.ID == want {
				return want
			}
		}
	}
	if len(n.Parents) > 0 {
		return n.Parents[0].ID
	}
	return ""
}

func refOf(n Node) Ref { return Ref{ID: n.ID, Type: n.Type, Name: n.Name, Description: n.Description} }

// Row kinds.
const (
	rowNode = iota
	rowCursor
	rowText // a continuation of the cursor's description
	rowAlso
	rowMore
)

type treeRow struct {
	kind  int
	guide string
	ref   Ref
	text  string
	sel   int // index among selectable rows, or -1
}

// selectable is the rows tree mode moves through with ↑ ↓: the siblings,
// with the node's other parents right after it.
func (tv treeView) selectable() []Ref {
	var out []Ref
	for i, s := range tv.Siblings {
		out = append(out, s)
		if i == tv.Cursor {
			out = append(out, tv.AlsoIn...)
		}
	}
	return out
}

// lines draws the tree: exactly rows lines of width w. sel is the selected
// row among selectable() in tree mode, -1 in search mode.
func (tv treeView) lines(w, rows, sel int, focused bool) []string {
	depth := len(tv.Path)
	var head []treeRow
	for d, r := range tv.Path {
		g := ""
		if d > 0 {
			g = strings.Repeat("  ", d-1) + "└ "
		}
		head = append(head, treeRow{kind: rowNode, guide: g, ref: r, sel: -1})
	}
	if len(head) > rows-3 {
		head = head[len(head)-max(0, rows-3):]
	}

	sibGuide := func(last bool) string {
		if depth == 0 {
			return ""
		}
		if last {
			return strings.Repeat("  ", depth-1) + "└ "
		}
		return strings.Repeat("  ", depth-1) + "├ "
	}
	under := ""
	if depth > 0 {
		under = strings.Repeat("  ", depth-1) + "│ "
		if tv.Cursor == len(tv.Siblings)-1 {
			under = strings.Repeat("  ", depth)
		}
	}

	// The description column starts after the widest guide, the type label,
	// the marker and the name, so it lines up on every row.
	guideW := lipgloss.Width(under) + 2
	for _, r := range head {
		guideW = max(guideW, lipgloss.Width(r.guide))
	}
	col := guideW + labelWidth + 1 + nameWidth + 3
	descW := w - col

	var body []treeRow
	cursorStart, cursorEnd := 0, 0
	selIdx := 0
	for i, s := range tv.Siblings {
		g := sibGuide(i == len(tv.Siblings)-1)
		if i != tv.Cursor {
			body = append(body, treeRow{kind: rowNode, guide: g, ref: s, sel: selIdx})
			selIdx++
			continue
		}
		cursorStart = len(body)
		desc := tv.Node.Description
		if desc == "" {
			desc = s.Description
		}
		wrapped := wrapDesc(desc, descW)
		first := ""
		if len(wrapped) > 0 {
			first = wrapped[0]
		}
		body = append(body, treeRow{kind: rowCursor, guide: g, ref: s, text: first, sel: selIdx})
		selIdx++
		for _, l := range wrapped[min(1, len(wrapped)):] {
			body = append(body, treeRow{kind: rowText, text: l, sel: -1})
		}
		for j, a := range tv.AlsoIn {
			label := "also in: "
			if j > 0 {
				label = strings.Repeat(" ", len(label))
			}
			body = append(body, treeRow{kind: rowAlso, guide: under, text: label + a.Type + " " + a.Name, sel: selIdx})
			selIdx++
		}
		if rows >= childRowsMin {
			for j, ch := range tv.Children {
				if j == maxChildren {
					body = append(body, treeRow{kind: rowMore, guide: under + "└ ", text: fmt.Sprintf("↓ %d more", len(tv.Children)-j), sel: -1})
					break
				}
				cg := under + "├ "
				if j == len(tv.Children)-1 {
					cg = under + "└ "
				}
				body = append(body, treeRow{kind: rowNode, guide: cg, ref: ch, sel: -1})
			}
		}
		cursorEnd = len(body) - 1
	}

	// Scroll the siblings so the cursor and its description stay in view.
	room := max(1, rows-len(head))
	start := 0
	if len(body) > room {
		block := cursorEnd - cursorStart + 1
		start = cursorStart - max(0, (room-block)/2)
		start = max(0, min(start, len(body)-room))
	}
	end := min(len(body), start+room)
	window := append([]treeRow(nil), body[start:end]...)
	if start > 0 && start < cursorStart {
		window[0] = treeRow{kind: rowMore, guide: sibGuide(false), text: fmt.Sprintf("↑ %d more", start+1), sel: -1}
	}
	if end < len(body) && end-1 > cursorEnd {
		window[len(window)-1] = treeRow{kind: rowMore, guide: sibGuide(true), text: fmt.Sprintf("↓ %d more", len(body)-end+1), sel: -1}
	}

	out := make([]string, 0, rows)
	for _, r := range append(head, window...) {
		out = append(out, fit(tv.render(r, col, descW, sel, focused), w))
	}
	for len(out) < rows {
		out = append(out, spaces(w))
	}
	return out
}

// render draws one row; col is where descriptions start.
func (tv treeView) render(r treeRow, col, descW, sel int, focused bool) string {
	guide := stRule.Render(r.guide)
	switch r.kind {
	case rowText:
		return spaces(col) + stText.Render(r.text)
	case rowMore:
		return guide + stMuted.Render(r.text)
	case rowAlso:
		text := stMuted.Render(r.text)
		if focused && r.sel == sel {
			text = stSelected.Render(r.text)
		}
		return guide + spaces(labelWidth+1) + text
	}
	name := ansi.Truncate(r.ref.Name, nameWidth, "…")
	label := typeStyle(r.ref.Type).Render(fit(typeLabel(r.ref.Type), labelWidth))
	prefix := guide + label
	lead := lipgloss.Width(r.guide) + labelWidth + 1 + lipgloss.Width(name)
	if r.kind == rowCursor {
		nameStyle := stTitle
		if focused && r.sel == sel {
			nameStyle = stSelected
		}
		fill := max(1, col-lead-3)
		line := prefix + stAmber.Render("▶") + nameStyle.Render(name) + " " + stAmber.Render(strings.Repeat("─", fill)+"▶") + " "
		if descW > 0 {
			line += stText.Render(r.text)
		}
		return line
	}
	line := prefix + " " + stText.Render(name) + spaces(max(1, col-lead))
	if descW > 0 {
		line += stMuted.Render(ansi.Truncate(firstLine(r.ref.Description), descW, "…"))
	}
	return line
}

// wrapDesc wraps text to width w in at most descMaxLines lines, ending the
// last with "…" when there was more.
func wrapDesc(text string, w int) []string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" || w <= 0 {
		return nil
	}
	lines := strings.Split(lipgloss.NewStyle().Width(w).Render(text), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	if len(lines) > descMaxLines {
		last := ansi.Truncate(lines[descMaxLines-1], w-1, "")
		lines = append(lines[:descMaxLines-1], last+"…")
	}
	return lines
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// typeLabel is a type in at most labelWidth cells.
func typeLabel(t string) string {
	switch t {
	case "marketplace":
		return "market"
	case "copilot-plugin":
		return "copilt"
	case "command":
		return "cmd"
	}
	if len(t) > labelWidth {
		return t[:labelWidth]
	}
	return t
}

// typeStyle colours a type, the same everywhere it appears.
func typeStyle(t string) lipgloss.Style {
	c := colMuted
	switch t {
	case "repo":
		c = colRepo
	case "marketplace":
		c = colMarket
	case "plugin", "copilot-plugin":
		c = colAmber
	case "skill":
		c = colTeal
	case "agent":
		c = colAgent
	case "command":
		c = colCommand
	}
	return lipgloss.NewStyle().Foreground(c)
}
