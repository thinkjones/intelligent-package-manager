package cli

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
)

func packFlags() (*flag.FlagSet, func(System) error) {
	var global bool
	var description string
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.BoolVar(&global, "g", false, "use your user scope ($IPM_HOME/global) instead of the project")
	fs.StringVar(&description, "description", "", "pack description (pack new)")
	return fs, func(sys System) error {
		args := fs.Args()
		if len(args) == 0 {
			return usagef("pack: say new, add, remove, list or show. Try `ipm help pack`.")
		}
		scope, err := currentScope(sys, global)
		if err != nil {
			return err
		}
		verb, rest := args[0], args[1:]
		switch verb {
		case "new":
			if len(rest) != 1 {
				return usagef("pack new: give one pack name")
			}
			return packNew(sys, scope, rest[0], description)
		case "add", "remove":
			if len(rest) < 2 {
				return usagef("pack %s: give a pack name and at least one item ID", verb)
			}
			return packEdit(sys, scope, verb, rest[0], rest[1:])
		case "list":
			return packList(sys, scope)
		case "show":
			if len(rest) != 1 {
				return usagef("pack show: give one pack name")
			}
			return packShow(sys, scope, rest[0])
		}
		return usagef("pack: unknown action %q — use new, add, remove, list or show", verb)
	}
}

// currentScope is the project around the working directory, or with global
// the user scope.
func currentScope(sys System, global bool) (project.Scope, error) {
	p, err := ipmPaths(sys)
	if err != nil {
		return project.Scope{}, err
	}
	cwd := ""
	if sys.Getwd != nil {
		if cwd, err = sys.Getwd(); err != nil {
			return project.Scope{}, err
		}
	}
	s, _, err := project.Find(cwd, global, p.Home)
	return s, err
}

func findPack(packs []project.Pack, name string) int {
	return slices.IndexFunc(packs, func(p project.Pack) bool { return p.Name == name })
}

func packNew(sys System, scope project.Scope, name, description string) error {
	if !pack.ValidName(name) || pack.Reserved(name) {
		return fmt.Errorf("%q can't be a pack name: use letters, digits, '.', '_' and '-', and avoid names Claude Code reserves (claude-…, anthropic-…, \"official\" beside claude or anthropic)", name)
	}
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return err
	}
	if findPack(packs, name) >= 0 {
		return fmt.Errorf("pack %s already exists", name)
	}
	packs = append(packs, project.Pack{Name: name, Description: description})
	if err := project.SavePacks(scope.Root, packs); err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s pack %s created in %s\n\nNext: ipm pack add %s <item id>…  (find IDs with `ipm search`)\n", ok, name, scope.Root+"/"+project.File, name)
	return nil
}

// pickable checks that an item exists and can be picked on its own; when it
// can't, the error says what to pick instead.
func pickable(g *catgraph.Graph, packName, id string) error {
	n, found := g.Node(id)
	if !found {
		return fmt.Errorf("no item %s in the catalog — find IDs with `ipm search`", id)
	}
	typ, _ := n["type"].(string)
	switch typ {
	case catalog.TypeSkill, catalog.TypeAgent, catalog.TypePlugin, catalog.TypeCopilotPlugin:
		return nil
	case catalog.TypeCommand:
		if !strings.Contains(id, "#") {
			return nil
		}
	case catalog.TypeRepo, catalog.TypeMarketplace:
		return fmt.Errorf("%s is a %s — pick the plugins, skills, agents or commands it holds (see `ipm tree %s`)", id, typ, id)
	}
	var owners []string
	if l, ok := n["plugins"].([]any); ok {
		for _, o := range l {
			owners = append(owners, fmt.Sprint(o))
		}
	}
	if len(owners) == 0 {
		return fmt.Errorf("%s can't be picked on its own, and no plugin in the catalog owns it", id)
	}
	return fmt.Errorf("%s comes only with its plugin: `ipm pack add %s %s`", id, packName, strings.Join(owners, " "))
}

func packEdit(sys System, scope project.Scope, verb, name string, ids []string) error {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return err
	}
	i := findPack(packs, name)
	if i < 0 {
		return fmt.Errorf("no pack %s — create it with `ipm pack new %s`", name, name)
	}
	if verb == "add" {
		p, err := ipmPaths(sys)
		if err != nil {
			return err
		}
		g, err := catgraph.Load(p.Catalog())
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := pickable(g, name, id); err != nil {
				return err
			}
		}
		for _, id := range ids {
			if !slices.Contains(packs[i].Items, id) {
				packs[i].Items = append(packs[i].Items, id)
			}
		}
	} else {
		for _, id := range ids {
			j := slices.Index(packs[i].Items, id)
			if j < 0 {
				return fmt.Errorf("%s is not in pack %s", id, name)
			}
			packs[i].Items = slices.Delete(packs[i].Items, j, j+1)
		}
	}
	if err := project.SavePacks(scope.Root, packs); err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s pack %s: %d item(s)\n\nNext: ipm install\n", ok, name, len(packs[i].Items))
	return nil
}

func packList(sys System, scope project.Scope) error {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return err
	}
	if len(packs) == 0 {
		fmt.Fprintf(sys.Stdout, "no packs in %s — create one with `ipm pack new <name>`\n", scope.Root)
		return nil
	}
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PACK\tITEMS\tDESCRIPTION")
	for _, p := range packs {
		fmt.Fprintf(tw, "%s\t%d\t%s\n", p.Name, len(p.Items), p.Description)
	}
	return tw.Flush()
}

func packShow(sys System, scope project.Scope, name string) error {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return err
	}
	i := findPack(packs, name)
	if i < 0 {
		return fmt.Errorf("no pack %s — `ipm pack list` shows the packs here", name)
	}
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	g, _ := catgraph.Load(p.Catalog())
	lock, _ := project.LoadLock(scope.IPMDir())
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tNAME\tREPO\t★\tID")
	for _, id := range packs[i].Items {
		typ, nm, repo, stars := "?", "", "", ""
		if g != nil {
			if n, found := g.Node(id); found {
				typ, _ = n["type"].(string)
				nm, _ = n["name"].(string)
				repo, _ = n["repo"].(string)
				if s, ok := g.Stars(repo); ok {
					stars = search.Stars(s)
				}
			}
		}
		if typ == "?" {
			if l, found := lock[id]; found {
				typ, nm, repo = l.Type, l.Name, l.Repo
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", typ, nm, repo, stars, id)
	}
	tw.Flush()
	fmt.Fprintf(sys.Stdout, "\n%s — %d item(s)\n", packs[i].Description, len(packs[i].Items))
	return nil
}
