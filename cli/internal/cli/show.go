package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/github"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"

	"github.com/spf13/cobra"
)

func loadGraph(sys System) (*catgraph.Graph, string, error) {
	p, err := ipmPaths(sys)
	if err != nil {
		return nil, "", err
	}
	dir := p.Catalog()
	g, err := catgraph.Load(dir)
	return g, dir, err
}

func newShowCmd(sys System) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "an artifact's fields, GitHub stats and neighbours, or its tree or graph (reads the catalog, no search server)",
		Args:  exactArgs(1, "one artifact ID (find IDs with `ipm search`)"),
		RunE:  func(_ *cobra.Command, args []string) error { return runShow(sys, args[0], asJSON) },
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the catalog record as JSON")
	cmd.AddCommand(newTreeCmd(sys), newGraphCmd(sys))
	return cmd
}

// runShow prints one artifact's fields, its repo's GitHub stats and its
// neighbours one level up and down.
func runShow(sys System, id string, asJSON bool) error {
	g, dir, err := loadGraph(sys)
	if err != nil {
		return err
	}
	n, found := g.Node(id)
	if !found {
		return fmt.Errorf("no artifact %q in the catalog — find IDs with `ipm search`", id)
	}
	if asJSON {
		enc := json.NewEncoder(sys.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(n)
	}
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	field := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "%s\t%s\n", k, v)
		}
	}
	s := func(k string) string { v, _ := n[k].(string); return v }
	list := func(k string) string {
		var out []string
		if l, ok := n[k].([]any); ok {
			for _, x := range l {
				out = append(out, fmt.Sprint(x))
			}
		}
		return strings.Join(out, ", ")
	}
	field("name", s("name"))
	field("type", s("type"))
	field("id", id)
	field("description", s("description"))
	field("repo", s("repo"))
	field("source_repo", s("source_repo"))
	field("commit", s("commit"))
	field("path", s("path"))
	field("tags", list("tags"))
	field("errors", list("errors"))
	stats, err := github.Read(filepath.Join(dir, "github.jsonl"))
	if err != nil {
		return err
	}
	statsRepo := s("repo")
	if src := s("source_repo"); src != "" {
		statsRepo = src
	}
	if st, ok := stats[pack.NormalizeRepo(statsRepo)]; ok {
		field("stars", fmt.Sprintf("%s (%d)", search.Stars(st.Stars), st.Stars))
		field("forks", fmt.Sprint(st.Forks))
		field("license", st.License)
		field("pushed", st.PushedAt)
		field("topics", strings.Join(st.Topics, ", "))
	}
	tw.Flush()
	neighbours(sys.Stdout, g, "up:", g.Children(id, true))
	neighbours(sys.Stdout, g, "down:", g.Children(id, false))
	refs := g.RefsTo(id)
	if s("type") == catalog.TypeRepo {
		refs = g.RefsInto(s("repo"))
	}
	if len(refs) > 0 {
		fmt.Fprintln(sys.Stdout, "\nused by:")
		for _, r := range refs {
			n, _ := g.Node(r)
			fmt.Fprintf(sys.Stdout, "  %s  in %s  (%s)\n", g.Label(r), n["repo"], r)
		}
	}
	if _, resolved := g.RefTarget(id); g.External(id) && !resolved {
		fmt.Fprintf(sys.Stdout, "\nadd its source: ipm marketplace add %s\n", id)
	}
	return nil
}

func neighbours(w io.Writer, g *catgraph.Graph, title string, ids []string) {
	fmt.Fprintf(w, "\n%s\n", title)
	if len(ids) == 0 {
		fmt.Fprintln(w, "  —")
	}
	for _, id := range ids {
		fmt.Fprintf(w, "  %s  (%s)\n", g.Label(id), id)
	}
}

func newTreeCmd(sys System) *cobra.Command {
	var o catgraph.Options
	cmd := &cobra.Command{
		Use:   "tree <id>",
		Short: "an artifact's relationships as a tree",
		Args:  exactArgs(1, "one artifact ID (find IDs with `ipm search`)"),
		RunE: func(_ *cobra.Command, args []string) error {
			id := args[0]
			g, _, err := loadGraph(sys)
			if err != nil {
				return err
			}
			if o.Depth < 0 {
				o.Depth = 0
				if strings.HasPrefix(id, catalog.TypeRepo+":") && !o.Up {
					o.Depth = 3
				}
			}
			return g.Tree(sys.Stdout, id, o)
		},
	}
	cmd.Flags().BoolVar(&o.Up, "up", false, "walk to what contains the artifact instead of what it contains")
	cmd.Flags().IntVar(&o.Depth, "depth", -1, "levels to show (default: 3 from a repo, else all)")
	cmd.Flags().BoolVar(&o.All, "all", false, "show every child, not the first 25")
	return cmd
}

func newGraphCmd(sys System) *cobra.Command {
	var o catgraph.Options
	var format string
	cmd := &cobra.Command{
		Use:   "graph <id>",
		Short: "an artifact's neighbourhood as Mermaid or Graphviz",
		Args:  exactArgs(1, "one artifact ID (find IDs with `ipm search`)"),
		RunE: func(_ *cobra.Command, args []string) error {
			if format != "mermaid" && format != "dot" {
				return usagef("graph: --format is mermaid or dot, not %q", format)
			}
			g, _, err := loadGraph(sys)
			if err != nil {
				return err
			}
			if format == "dot" {
				return g.DOT(sys.Stdout, args[0], o)
			}
			return g.Mermaid(sys.Stdout, args[0], o)
		},
	}
	cmd.Flags().BoolVar(&o.Up, "up", false, "walk to what contains the artifact instead of what it contains")
	cmd.Flags().IntVar(&o.Depth, "depth", 2, "levels to include")
	cmd.Flags().BoolVar(&o.All, "all", false, "include every child, not the first 25")
	cmd.Flags().StringVar(&format, "format", "mermaid", "mermaid or dot")
	return cmd
}
