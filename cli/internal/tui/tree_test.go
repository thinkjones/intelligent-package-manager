package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// mapCatalog is a Catalog over an in-memory set of nodes.
type mapCatalog map[string]Node

func (c mapCatalog) Node(id string) (Node, error) {
	n, ok := c[id]
	if !ok {
		return Node{}, fmt.Errorf("no artifact %s", id)
	}
	return n, nil
}

func (c mapCatalog) add(id, typ, name, desc string) {
	c[id] = Node{ID: id, Type: typ, Name: name, Description: desc}
}

func (c mapCatalog) ref(id string) Ref {
	n := c[id]
	return Ref{ID: n.ID, Type: n.Type, Name: n.Name, Description: n.Description}
}

// link makes child a child of parent, in order.
func (c mapCatalog) link(parent, child string) {
	p, ch := c[parent], c[child]
	p.Children = append(p.Children, c.ref(child))
	ch.Parents = append(ch.Parents, c.ref(parent))
	c[parent], c[child] = p, ch
}

// sample is repo → marketplace → plugin → skills a, b, c; b is also in p2.
func sample() mapCatalog {
	c := mapCatalog{}
	c.add("repo:o/r", "repo", "o/r", "The repo")
	c.add("marketplace:o/r:.", "marketplace", "m", "The marketplace")
	c.add("plugin:o/r:p", "plugin", "p", "The plugin")
	c.add("plugin:o/r:p2", "plugin", "p2", "Another plugin")
	c.add("skill:o/r:a", "skill", "a", "Skill a")
	c.add("skill:o/r:b", "skill", "b", "Skill b does a great many things, described at length so that it has to wrap onto more than one line in the tree, and then keeps going well past three lines of text so it must be cut")
	c.add("skill:o/r:c", "skill", "c", "Skill c")
	c.link("repo:o/r", "marketplace:o/r:.")
	c.link("marketplace:o/r:.", "plugin:o/r:p")
	c.link("marketplace:o/r:.", "plugin:o/r:p2")
	for _, s := range []string{"a", "b", "c"} {
		c.link("plugin:o/r:p", "skill:o/r:"+s)
	}
	c.link("plugin:o/r:p2", "skill:o/r:b")
	return c
}

// caretRow is the tree line carrying the caret, without styling.
func caretRow(lines []string) string {
	for _, l := range lines {
		if p := ansi.Strip(l); strings.HasPrefix(p, "▶ ") {
			return p
		}
	}
	return ""
}

func ids(refs []Ref) string {
	var out []string
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return strings.Join(out, ",")
}

func TestBuildTreePath(t *testing.T) {
	tv, err := buildTree(sample(), "skill:o/r:b", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ids(tv.Path) != "repo:o/r,marketplace:o/r:.,plugin:o/r:p" {
		t.Errorf("path %s", ids(tv.Path))
	}
	if ids(tv.Siblings) != "skill:o/r:a,skill:o/r:b,skill:o/r:c" || tv.Cursor != 1 {
		t.Errorf("siblings %s cursor %d", ids(tv.Siblings), tv.Cursor)
	}
	if ids(tv.AlsoIn) != "plugin:o/r:p2" || tv.Node.ID != "skill:o/r:b" {
		t.Errorf("also in %s node %s", ids(tv.AlsoIn), tv.Node.ID)
	}
}

func TestBuildTreeChosenParent(t *testing.T) {
	tv, _ := buildTree(sample(), "skill:o/r:b", map[string]string{"skill:o/r:b": "plugin:o/r:p2"})
	if ids(tv.Path) != "repo:o/r,marketplace:o/r:.,plugin:o/r:p2" || ids(tv.AlsoIn) != "plugin:o/r:p" {
		t.Errorf("path %s also %s", ids(tv.Path), ids(tv.AlsoIn))
	}
	if ids(tv.Siblings) != "skill:o/r:b" {
		t.Errorf("siblings under p2: %s", ids(tv.Siblings))
	}
}

func TestBuildTreeRoot(t *testing.T) {
	tv, _ := buildTree(sample(), "repo:o/r", nil)
	if len(tv.Path) != 0 || ids(tv.Siblings) != "repo:o/r" || ids(tv.Children) != "marketplace:o/r:." {
		t.Errorf("path %s siblings %s children %s", ids(tv.Path), ids(tv.Siblings), ids(tv.Children))
	}
}

func TestBuildTreeDepthCap(t *testing.T) {
	c := mapCatalog{}
	for i := range 15 {
		c.add(fmt.Sprintf("n%02d", i), "plugin", fmt.Sprintf("n%02d", i), "")
		if i > 0 {
			c.link(fmt.Sprintf("n%02d", i-1), fmt.Sprintf("n%02d", i))
		}
	}
	tv, _ := buildTree(c, "n14", nil)
	if len(tv.Path) != 10 {
		t.Errorf("path length %d, want 10", len(tv.Path))
	}
}

// descCol is the display cell where text starts on a row whose prefix ends
// at the first occurrence of name.
func descCol(t *testing.T, line, desc string) int {
	t.Helper()
	plain := ansi.Strip(line)
	i := strings.Index(plain, desc)
	if i < 0 {
		t.Fatalf("%q not on line %q", desc, plain)
	}
	return lipgloss.Width(plain[:i])
}

func TestTreeLines(t *testing.T) {
	tv, _ := buildTree(sample(), "skill:o/r:b", nil)
	lines := tv.lines(100, 14, -1, false)
	if len(lines) != 14 {
		t.Fatalf("%d lines", len(lines))
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w != 100 {
			t.Errorf("line width %d: %q", w, ansi.Strip(l))
		}
	}
	text := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"repo    o/r", "market  m", "plugin  p", "skill   a", "also in: plugin p2"} {
		if !strings.Contains(text, want) {
			t.Errorf("tree lacks %q:\n%s", want, text)
		}
	}
	col := descCol(t, lines[0], "The repo")
	for _, d := range []string{"The marketplace", "The plugin", "Skill a", "Skill c"} {
		for _, l := range lines {
			if strings.Contains(ansi.Strip(l), d) && descCol(t, l, d) != col {
				t.Errorf("%q starts at %d, want %d", d, descCol(t, l, d), col)
			}
		}
	}
	if cursor := caretRow(lines); !strings.Contains(cursor, "skill   b") || !strings.Contains(cursor, "Skill b") || strings.Contains(cursor, "─▶") {
		t.Errorf("the caret row is the cursor, with its description and no inline arrow: %q", cursor)
	}
	wrapped := 0
	for _, l := range lines {
		p := ansi.Strip(l)
		if len(p) > col && strings.TrimSpace(p[:col]) == "" && strings.TrimSpace(p) != "" {
			wrapped++
		}
	}
	if wrapped != 2 || !strings.Contains(text, "…") {
		t.Errorf("the cursor's description should wrap to 3 lines and be cut (%d continuation lines):\n%s", wrapped, text)
	}
}

func TestTreeScrollsLongSiblingLists(t *testing.T) {
	c := mapCatalog{}
	c.add("plugin:p", "plugin", "p", "")
	for i := range 80 {
		id := fmt.Sprintf("agent:a%02d", i)
		c.add(id, "agent", fmt.Sprintf("a%02d", i), "")
		c.link("plugin:p", id)
	}
	tv, _ := buildTree(c, "agent:a60", nil)
	lines := tv.lines(80, 14, -1, false)
	text := ansi.Strip(strings.Join(lines, "\n"))
	if len(lines) != 14 || !strings.Contains(caretRow(lines), "a60") || !strings.Contains(text, "↑ ") || !strings.Contains(text, "↓ ") || !strings.Contains(text, " more") {
		t.Errorf("want the cursor between ↑/↓ more rows:\n%s", text)
	}
}

func TestTreeColumnAlignedWithWideNames(t *testing.T) {
	c := mapCatalog{}
	c.add("plugin:p", "plugin", "p", "parent")
	c.add("skill:x", "skill", "日本語のスキル", "wide one")
	c.add("skill:y", "skill", "🚀 rocket", "emoji one")
	c.add("skill:z", "skill", "plain", "plain one")
	c.link("plugin:p", "skill:x")
	c.link("plugin:p", "skill:y")
	c.link("plugin:p", "skill:z")
	tv, _ := buildTree(c, "skill:z", nil)
	lines := tv.lines(90, 10, -1, false)
	var cols []int
	for _, d := range []string{"wide one", "emoji one"} {
		for _, l := range lines {
			if strings.Contains(ansi.Strip(l), d) {
				cols = append(cols, descCol(t, l, d))
			}
		}
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Errorf("description columns %v", cols)
	}
}

func TestTreeNameCut(t *testing.T) {
	c := sample()
	long := strings.Repeat("n", 40)
	a := c["skill:o/r:a"]
	a.Name = long
	c["skill:o/r:a"] = a
	p := c["plugin:o/r:p"]
	p.Children[0].Name = long
	c["plugin:o/r:p"] = p
	tv, _ := buildTree(c, "skill:o/r:c", nil)
	text := ansi.Strip(strings.Join(tv.lines(100, 12, -1, false), "\n"))
	if !strings.Contains(text, strings.Repeat("n", 23)+"…") || strings.Contains(text, strings.Repeat("n", 25)) {
		t.Errorf("names cut to 24:\n%s", text)
	}
}

func TestTreeKeepsSiblingsAroundBigCursor(t *testing.T) {
	c := mapCatalog{}
	c.add("market:m", "marketplace", "m", "")
	for i := range 80 {
		id := fmt.Sprintf("plugin:p%02d", i)
		c.add(id, "plugin", fmt.Sprintf("p%02d", i), "A plugin")
		c.link("market:m", id)
	}
	cur := c["plugin:p40"]
	cur.Description = strings.Repeat("A long description that wraps. ", 12)
	c["plugin:p40"] = cur
	for i := range 20 {
		id := fmt.Sprintf("skill:s%02d", i)
		c.add(id, "skill", fmt.Sprintf("s%02d", i), "")
		c.link("plugin:p40", id)
	}
	tv, _ := buildTree(c, "plugin:p40", nil)
	for _, rows := range []int{10, 12, 14} {
		ls := tv.lines(76, rows, -1, false)
		text := ansi.Strip(strings.Join(ls, "\n"))
		if !strings.Contains(caretRow(ls), "p40") || !strings.Contains(text, "↑ ") || !strings.Contains(text, "more") || (!strings.Contains(text, "p39") && !strings.Contains(text, "p41")) {
			t.Errorf("rows %d: want the cursor, a sibling and the scroll rows:\n%s", rows, text)
		}
		if strings.Count(text, "↓ ") < 1 {
			t.Errorf("rows %d: no ↓ more row:\n%s", rows, text)
		}
	}
}

// external builds a marketplace in a/m listing a local plugin and an external
// entry for cloudflare/skills.
func external() mapCatalog {
	c := mapCatalog{}
	c.add("repo:a/m", "repo", "a/m", "A marketplace repo")
	c.add("marketplace:a/m:.", "marketplace", "m", "Plugins")
	c.add("plugin:a/m:local", "plugin", "local", "A local plugin")
	c.add("plugin:a/m#cloudflare", "plugin", "cloudflare", "Skills for Cloudflare")
	c.link("repo:a/m", "marketplace:a/m:.")
	c.link("marketplace:a/m:.", "plugin:a/m:local")
	c.link("marketplace:a/m:.", "plugin:a/m#cloudflare")
	p := c["marketplace:a/m:."]
	p.Children[1].External = true
	c["marketplace:a/m:."] = p
	e := c["plugin:a/m#cloudflare"]
	e.External = &Ref{ID: "plugin:cloudflare/skills:.", Type: "plugin", Name: "cloudflare", Repo: "cloudflare/skills"}
	c["plugin:a/m#cloudflare"] = e
	return c
}

func TestTreeExternalRow(t *testing.T) {
	tv, _ := buildTree(external(), "plugin:a/m#cloudflare", nil)
	ls := tv.lines(110, 12, -1, false)
	text := ansi.Strip(strings.Join(ls, "\n"))
	if !strings.Contains(caretRow(ls), "↗cloudflare") || !strings.Contains(text, "↗ cloudflare/skills — Skills for Cloudflare") {
		t.Errorf("cursor external row:\n%s", text)
	}
	tv, _ = buildTree(external(), "plugin:a/m:local", nil)
	text = ansi.Strip(strings.Join(tv.lines(110, 12, -1, false), "\n"))
	if !strings.Contains(text, " ↗cloudflare") {
		t.Errorf("sibling external row:\n%s", text)
	}
}

func TestTreeUnresolvedExternal(t *testing.T) {
	c := external()
	e := c["plugin:a/m#cloudflare"]
	e.External, e.Unresolved = nil, "c/missing"
	c["plugin:a/m#cloudflare"] = e
	tv, _ := buildTree(c, "plugin:a/m#cloudflare", nil)
	text := ansi.Strip(strings.Join(tv.lines(140, 12, -1, false), "\n"))
	if !strings.Contains(text, "↗ c/missing (not a source — ipm marketplace add plugin:a/m#cloudflare)") {
		t.Errorf("unresolved:\n%s", text)
	}
}

func TestTreeUsedBySuffix(t *testing.T) {
	c := sample()
	b := c["skill:o/r:b"]
	b.UsedByRepos, b.Description = 3, "Skill b"
	c["skill:o/r:b"] = b
	p := c["plugin:o/r:p"]
	p.Children[0].UsedBy = 2
	c["plugin:o/r:p"] = p
	tv, _ := buildTree(c, "skill:o/r:b", nil)
	text := ansi.Strip(strings.Join(tv.lines(120, 12, -1, false), "\n"))
	if !strings.Contains(text, "↗ used by 3") || !strings.Contains(text, "↗ used by 2") {
		t.Errorf("used-by suffixes:\n%s", text)
	}
}

func TestTreePathRowShowsUsedBy(t *testing.T) {
	c := sample()
	p := c["plugin:o/r:p"]
	p.UsedBy = []Ref{{ID: "plugin:x/y#p", Repo: "x/y", External: true}, {ID: "plugin:z/w#p", Repo: "z/w", External: true}}
	c["plugin:o/r:p"] = p
	tv, _ := buildTree(c, "skill:o/r:a", nil)
	lines := tv.lines(120, 12, -1, false)
	for _, l := range lines {
		if p := ansi.Strip(l); strings.Contains(p, "plugin  p ") && !strings.Contains(p, "↗ used by 2") {
			t.Errorf("the referenced plugin on the path should say so: %q", p)
		}
	}
}
