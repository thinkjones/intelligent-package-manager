package cli

import (
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"

	"github.com/spf13/cobra"
)

func newPackCmd(sys System) *cobra.Command {
	var global bool
	var description string
	cmd := verbs(&cobra.Command{
		Use:   "pack",
		Short: "create and edit packs of catalog items in ipm.toml (-g for your user scope)",
	}, "new, add, remove, list or show")
	cmd.PersistentFlags().BoolVarP(&global, "global", "g", false, "use your user scope ($IPM_HOME/global) instead of the project")
	// inScope runs f in the project's scope, or the user's with -g.
	inScope := func(f func(project.Scope, []string) error) func(*cobra.Command, []string) error {
		return func(_ *cobra.Command, args []string) error {
			scope, err := currentScope(sys, global)
			if err != nil {
				return err
			}
			return f(scope, args)
		}
	}
	newCmd := &cobra.Command{
		Use:   "new <name>",
		Short: "create a pack (creates ipm.toml here if no folder above has one)",
		Args:  exactArgs(1, "one pack name"),
		RunE:  inScope(func(s project.Scope, args []string) error { return packNew(sys, s, args[0], description) }),
	}
	newCmd.Flags().StringVar(&description, "description", "", "pack description")
	edit := func(verb, short string) *cobra.Command {
		return &cobra.Command{
			Use:   verb + " <pack> <id>...",
			Short: short,
			Args:  minArgs(2, "a pack name and at least one item ID"),
			RunE:  inScope(func(s project.Scope, args []string) error { return packEdit(sys, s, verb, args[0], args[1:]) }),
		}
	}
	cmd.AddCommand(
		newCmd,
		edit("add", "add catalog items: skills, agents, file commands, plugins, Copilot plugins"),
		edit("remove", "remove items"),
		&cobra.Command{
			Use:   "list",
			Short: "every pack in the scope: name, item count, description",
			Args:  noArgs,
			RunE:  inScope(func(s project.Scope, _ []string) error { return packList(sys, s) }),
		},
		&cobra.Command{
			Use:   "show <pack>",
			Short: "a pack's items: type, name, repo, stars and ID",
			Args:  exactArgs(1, "one pack name"),
			RunE:  inScope(func(s project.Scope, args []string) error { return packShow(sys, s, args[0]) }),
		},
	)
	return cmd
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
	if err := createPack(scope, name, description); err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s pack %s created in %s\n\nNext: ipm pack add %s <item id>…  (find IDs with `ipm search`)\n", ok, name, scope.Root+"/"+project.File, name)
	return nil
}

// createPack adds an empty pack to the scope's ipm.toml. ipm pack new and the
// TUI share it.
func createPack(scope project.Scope, name, description string) error {
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
	return project.SavePacks(scope.Root, packs)
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
		return fmt.Errorf("%s is a %s — pick the plugins, skills, agents or commands it holds (see `ipm show tree %s`)", id, typ, id)
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
	edit := func() (project.Pack, error) { return removeFromPack(scope, name, ids) }
	if verb == "add" {
		edit = func() (project.Pack, error) { return addToPack(sys, scope, name, ids) }
	}
	p, err := edit()
	if err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s pack %s: %d item(s)\n\nNext: ipm install\n", ok, name, len(p.Items))
	return nil
}

// addToPack adds catalog items to a pack: every ID is checked first, so a
// refusal writes nothing; IDs already in the pack are skipped. ipm pack add
// and the TUI share it.
func addToPack(sys System, scope project.Scope, name string, ids []string) (project.Pack, error) {
	return editPack(scope, name, func(p *project.Pack) error {
		paths, err := ipmPaths(sys)
		if err != nil {
			return err
		}
		g, err := catgraph.Load(paths.Catalog())
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := pickable(g, name, id); err != nil {
				return err
			}
		}
		for _, id := range ids {
			if !slices.Contains(p.Items, id) {
				p.Items = append(p.Items, id)
			}
		}
		return nil
	})
}

// removeFromPack removes items from a pack; an ID not in it is an error and
// nothing is written.
func removeFromPack(scope project.Scope, name string, ids []string) (project.Pack, error) {
	return editPack(scope, name, func(p *project.Pack) error {
		for _, id := range ids {
			j := slices.Index(p.Items, id)
			if j < 0 {
				return fmt.Errorf("%s is not in pack %s", id, name)
			}
			p.Items = slices.Delete(p.Items, j, j+1)
		}
		return nil
	})
}

// editPack loads the scope's packs, applies change to the named one and saves
// them all, unless change fails.
func editPack(scope project.Scope, name string, change func(*project.Pack) error) (project.Pack, error) {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return project.Pack{}, err
	}
	i := findPack(packs, name)
	if i < 0 {
		return project.Pack{}, fmt.Errorf("no pack %s — create it with `ipm pack new %s`", name, name)
	}
	if err := change(&packs[i]); err != nil {
		return project.Pack{}, err
	}
	return packs[i], project.SavePacks(scope.Root, packs)
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
