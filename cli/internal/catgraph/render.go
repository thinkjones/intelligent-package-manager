package catgraph

import (
	"fmt"
	"io"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/search"
)

// maxChildren is how many children a node shows before "… N more".
const maxChildren = 25

// Options shape a walk: direction, how deep (0 = unlimited), and whether to
// show every child.
type Options struct {
	Up    bool
	Depth int
	All   bool
}

// Label is a node's "type  name" line, with stars for a repo.
func (g *Graph) Label(id string) string {
	n := g.nodes[id]
	label := fmt.Sprintf("%-11s %s", str(n, "type"), str(n, "name"))
	if str(n, "type") == catalog.TypeRepo {
		if s, ok := g.Stars(str(n, "repo")); ok {
			label += " ★ " + search.Stars(s)
		}
	}
	return label
}

// shown is the children to draw and how many were left out.
func (g *Graph) shown(id string, o Options) ([]string, int) {
	kids := g.Children(id, o.Up)
	if o.All || len(kids) <= maxChildren {
		return kids, 0
	}
	return kids[:maxChildren], len(kids) - maxChildren
}

// Tree writes id and its neighbourhood as a box-drawing tree. A node already
// on the current path is marked (cycle) and not expanded.
func (g *Graph) Tree(w io.Writer, id string, o Options) error {
	if err := g.check(id); err != nil {
		return err
	}
	fmt.Fprintln(w, g.Label(id))
	g.tree(w, id, "", 1, o, map[string]bool{id: true})
	return nil
}

func (g *Graph) tree(w io.Writer, id, prefix string, depth int, o Options, path map[string]bool) {
	if o.Depth > 0 && depth > o.Depth {
		return
	}
	kids, more := g.shown(id, o)
	for i, k := range kids {
		last := i == len(kids)-1 && more == 0
		branch, next := "├─ ", "│  "
		if last {
			branch, next = "└─ ", "   "
		}
		if path[k] {
			fmt.Fprintf(w, "%s%s%s (cycle)\n", prefix, branch, g.Label(k))
			continue
		}
		fmt.Fprintf(w, "%s%s%s\n", prefix, branch, g.Label(k))
		path[k] = true
		g.tree(w, k, prefix+next, depth+1, o, path)
		delete(path, k)
	}
	if more > 0 {
		fmt.Fprintf(w, "%s└─ … %d more\n", prefix, more)
	}
}

// walk visits id's neighbourhood breadth-first, numbering nodes in visit
// order; edges always point from container to contained.
func (g *Graph) walk(id string, o Options) (nodes []string, labels map[string]string, edges [][2]string) {
	num := map[string]string{}
	labels = map[string]string{}
	add := func(key, label string) string {
		if n, ok := num[key]; ok {
			return n
		}
		n := fmt.Sprintf("n%d", len(num))
		num[key] = n
		nodes = append(nodes, n)
		labels[n] = label
		return n
	}
	typeName := func(id string) string { return str(g.nodes[id], "type") + ": " + str(g.nodes[id], "name") }
	add(id, typeName(id))
	frontier := []string{id}
	for depth := 1; len(frontier) > 0 && (o.Depth == 0 || depth <= o.Depth); depth++ {
		var next []string
		for _, f := range frontier {
			kids, more := g.shown(f, o)
			for _, k := range kids {
				_, seen := num[k]
				kn := add(k, typeName(k))
				if o.Up {
					edges = append(edges, [2]string{kn, num[f]})
				} else {
					edges = append(edges, [2]string{num[f], kn})
				}
				if !seen {
					next = append(next, k)
				}
			}
			if more > 0 {
				mn := add(f+"#more", fmt.Sprintf("… %d more", more))
				edges = append(edges, [2]string{num[f], mn})
			}
		}
		frontier = next
	}
	return nodes, labels, edges
}

// Mermaid writes the neighbourhood as a Mermaid flowchart.
func (g *Graph) Mermaid(w io.Writer, id string, o Options) error {
	if err := g.check(id); err != nil {
		return err
	}
	nodes, labels, edges := g.walk(id, o)
	fmt.Fprintln(w, "flowchart LR")
	for _, n := range nodes {
		fmt.Fprintf(w, "  %s[\"%s\"]\n", n, strings.ReplaceAll(labels[n], `"`, "#quot;"))
	}
	for _, e := range edges {
		fmt.Fprintf(w, "  %s --> %s\n", e[0], e[1])
	}
	return nil
}

// DOT writes the neighbourhood as a Graphviz digraph.
func (g *Graph) DOT(w io.Writer, id string, o Options) error {
	if err := g.check(id); err != nil {
		return err
	}
	nodes, labels, edges := g.walk(id, o)
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	fmt.Fprintln(w, "digraph catalog {\n  rankdir=LR;")
	for _, n := range nodes {
		fmt.Fprintf(w, "  %s [label=\"%s\"];\n", n, esc.Replace(labels[n]))
	}
	for _, e := range edges {
		fmt.Fprintf(w, "  %s -> %s;\n", e[0], e[1])
	}
	fmt.Fprintln(w, "}")
	return nil
}
