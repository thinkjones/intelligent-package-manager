package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/catgraph"
	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/github"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/search"
)

// oneID is the single artifact ID a relationship command takes; flags may
// come after it.
func oneID(name string, fs *flag.FlagSet) (string, error) {
	words := fs.Args()
	if len(words) != 1 {
		return "", usagef("%s: give one artifact ID (find IDs with `ipm search`). Try `ipm help %s`.", name, name)
	}
	return words[0], nil
}

func loadGraph(sys System) (*catgraph.Graph, string, error) {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return nil, "", err
	}
	dir := filepath.Join(cfg.Root, "catalog")
	g, err := catgraph.Load(dir)
	return g, dir, err
}

func showFlags() (*flag.FlagSet, func(System) error) {
	var asJSON bool
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.BoolVar(&asJSON, "json", false, "print the catalog record as JSON")
	return fs, func(sys System) error {
		id, err := oneID("show", fs)
		if err != nil {
			return err
		}
		return runShow(sys, id, asJSON)
	}
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
	if s("type") == catalog.TypePlugin && s("source_repo") != "" {
		fmt.Fprintf(sys.Stdout, "\nadd its source: ipm source add %s\n", id)
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

func treeFlags() (*flag.FlagSet, func(System) error) {
	var o catgraph.Options
	fs := flag.NewFlagSet("tree", flag.ContinueOnError)
	fs.BoolVar(&o.Up, "up", false, "walk to what contains the artifact instead of what it contains")
	fs.IntVar(&o.Depth, "depth", -1, "levels to show (default: 3 from a repo, else all)")
	fs.BoolVar(&o.All, "all", false, "show every child, not the first 25")
	return fs, func(sys System) error {
		id, err := oneID("tree", fs)
		if err != nil {
			return err
		}
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
	}
}

func graphFlags() (*flag.FlagSet, func(System) error) {
	var o catgraph.Options
	var format string
	fs := flag.NewFlagSet("graph", flag.ContinueOnError)
	fs.BoolVar(&o.Up, "up", false, "walk to what contains the artifact instead of what it contains")
	fs.IntVar(&o.Depth, "depth", 2, "levels to include")
	fs.BoolVar(&o.All, "all", false, "include every child, not the first 25")
	fs.StringVar(&format, "format", "mermaid", "mermaid or dot")
	return fs, func(sys System) error {
		id, err := oneID("graph", fs)
		if err != nil {
			return err
		}
		if format != "mermaid" && format != "dot" {
			return usagef("graph: --format is mermaid or dot, not %q", format)
		}
		g, _, err := loadGraph(sys)
		if err != nil {
			return err
		}
		if format == "dot" {
			return g.DOT(sys.Stdout, id, o)
		}
		return g.Mermaid(sys.Stdout, id, o)
	}
}
