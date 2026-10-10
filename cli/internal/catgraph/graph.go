// Package catgraph reads the catalog as a graph — repos contain marketplaces,
// marketplaces list plugins, plugins own components, external plugins point
// at their source repo — and renders neighbourhoods as trees, Mermaid or
// Graphviz. It reads only catalog/, never the search server.
package catgraph

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/github"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
)

// componentKeys are the plugin fields listing owned components.
var componentKeys = []string{"skills", "agents", "commands", "hooks", "mcp"}

// Graph is the catalog's records and the edges between them.
type Graph struct {
	nodes    map[string]map[string]any
	down, up map[string][]string
	refs     catalog.Refs
	stars    map[string]int // normalized owner/repo → stars
}

// Load reads every catalog file under catalogDir and github.jsonl.
func Load(catalogDir string) (*Graph, error) {
	g := &Graph{nodes: map[string]map[string]any{}, down: map[string][]string{}, up: map[string][]string{}, stars: map[string]int{}}
	records := map[string][]map[string]any{}
	for _, typ := range catalog.Types {
		recs, err := catalog.Load(catalogDir, typ)
		if err != nil {
			return nil, err
		}
		records[typ] = recs
		for _, r := range recs {
			g.nodes[str(r, "id")] = r
		}
	}
	g.refs = catalog.BuildRefs(records)
	stats, err := github.Read(filepath.Join(catalogDir, "github.jsonl"))
	if err != nil {
		return nil, err
	}
	for repo, s := range stats {
		g.stars[repo] = s.Stars
	}
	g.link()
	return g, nil
}

func (g *Graph) edge(from, to string) {
	if g.nodes[from] == nil || g.nodes[to] == nil || slices.Contains(g.down[from], to) {
		return
	}
	g.down[from] = append(g.down[from], to)
	g.up[to] = append(g.up[to], from)
}

// link builds the edges. A repo's direct children are its marketplaces, any
// plugin none of its marketplaces lists, and any component no plugin owns,
// so a tree from a repo shows each artifact once.
func (g *Graph) link() {
	repoID := map[string]string{} // normalized repo → repo record ID
	for id, n := range g.nodes {
		if str(n, "type") == catalog.TypeRepo {
			repoID[pack.NormalizeRepo(str(n, "repo"))] = id
		}
	}
	listed := map[string]bool{}
	for _, id := range g.sortedIDs() {
		n := g.nodes[id]
		switch str(n, "type") {
		case catalog.TypeMarketplace:
			for _, p := range strs(n["plugins"]) {
				g.edge(id, p)
				listed[p] = true
			}
		case catalog.TypePlugin, catalog.TypeCopilotPlugin:
			for _, key := range componentKeys {
				for _, c := range strs(n[key]) {
					g.edge(id, c)
				}
			}
		}
	}
	for _, id := range g.sortedIDs() {
		n := g.nodes[id]
		repo := repoID[pack.NormalizeRepo(str(n, "repo"))]
		switch typ := str(n, "type"); {
		case typ == catalog.TypeRepo:
		case typ == catalog.TypeMarketplace:
			g.edge(repo, id)
		case typ == catalog.TypePlugin || typ == catalog.TypeCopilotPlugin:
			if !listed[id] {
				g.edge(repo, id)
			}
		case len(strs(n["plugins"])) == 0:
			g.edge(repo, id)
		}
	}
	for _, m := range []map[string][]string{g.down, g.up} {
		for id, ids := range m {
			slices.SortFunc(ids, g.order)
			m[id] = ids
		}
	}
}

func (g *Graph) sortedIDs() []string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// order sorts by type (catalog order), then name, then ID.
func (g *Graph) order(a, b string) int {
	ta, tb := slices.Index(catalog.Types, str(g.nodes[a], "type")), slices.Index(catalog.Types, str(g.nodes[b], "type"))
	if ta != tb {
		return ta - tb
	}
	if c := strings.Compare(str(g.nodes[a], "name"), str(g.nodes[b], "name")); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// Node is one record, as written in the catalog.
func (g *Graph) Node(id string) (map[string]any, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

// Children are id's neighbours: what it contains, or with up, what contains it.
func (g *Graph) Children(id string, up bool) []string {
	if up {
		return g.up[id]
	}
	return g.down[id]
}

// RefTarget is what an external entry references in another repo.
func (g *Graph) RefTarget(id string) (string, bool) {
	t, ok := g.refs.Target[id]
	return t, ok
}

// RefsTo is the external entries, in other repos, that reference id.
func (g *Graph) RefsTo(id string) []string { return g.refs.By[id] }

// UsedBy is the other repos whose external entries reference anything in repo.
func (g *Graph) UsedBy(repo string) []string {
	return g.refs.UsedBy[strings.ToLower(pack.NormalizeRepo(repo))]
}

// External reports whether id is a marketplace entry for a plugin that lives
// in another repo.
func (g *Graph) External(id string) bool { return g.refs.External[id] }

// Stars is the repo's star count, if GitHub stats know it.
func (g *Graph) Stars(repo string) (int, bool) {
	n, ok := g.stars[pack.NormalizeRepo(repo)]
	return n, ok
}

func (g *Graph) check(id string) error {
	if _, ok := g.nodes[id]; !ok {
		return fmt.Errorf("no artifact %q in the catalog — find IDs with `ipm search`", id)
	}
	return nil
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func strs(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
