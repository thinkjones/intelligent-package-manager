package cli

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/paths"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

// marketOpts are the flags of a bare `ipm marketplace`.
type marketOpts struct {
	List     string
	Lists    []string
	Followed bool
}

// marketActions is what `ipm marketplace` takes after its name.
const marketActions = "add, remove, import or stats"

func newMarketplaceCmd(sys System) *cobra.Command {
	var o marketOpts
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "browse the curated repos and follow, unfollow or import your own",
		Long: `Browse the curated lists of repos, with a check mark on the ones you follow.
Run alone it lists them; add, remove, import and stats change what you follow.`,
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usagef("%s: unknown action %q%s — use %s. Try `%s`.", c.CommandPath(), args[0], didYouMean(c, args[0]), marketActions, helpFor(c))
			}
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			p, err := ipmPaths(sys)
			if err != nil {
				return err
			}
			return marketplaceList(sys, p, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.List, "list", curated.DefaultURL, "the curated list: a URL or a file")
	f.StringArrayVar(&o.Lists, "lists", nil, "show only this list, by exact name (repeatable)")
	f.BoolVar(&o.Followed, "followed", false, "show only the repos you follow, with their ref and pinned commit")
	cmd.AddCommand(
		newMarketplaceAddCmd(sys),
		newMarketplaceRemoveCmd(sys),
		newMarketplaceImportCmd(sys),
		newStatsCmd(sys),
	)
	return cmd
}

// marketplaceList prints the curated lists with a ✓ on the repos you
// follow, then the followed repos no list holds under Other. --followed
// prints only the followed repos instead.
func marketplaceList(sys System, p paths.Paths, o marketOpts) error {
	if o.Followed {
		return sourceList(sys, p)
	}
	data, cached, err := curated.Fetch(sys.HTTP, o.List, p.Cache)
	if err != nil {
		return err
	}
	lists, skipped, err := curated.Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %v", o.List, err)
	}
	shown := lists
	if len(o.Lists) > 0 {
		shown = nil
		for _, n := range o.Lists {
			i := 0
			for i < len(lists) && lists[i].Name != n {
				i++
			}
			if i == len(lists) {
				var names []string
				for _, l := range lists {
					names = append(names, l.Name)
				}
				return usagef("marketplace: no list named %q; the lists are: %s", n, strings.Join(names, ", "))
			}
			shown = append(shown, lists[i])
		}
	}
	have, err := sources.Load(p.Sources())
	if err != nil {
		return err
	}
	followed := map[string]bool{}
	for _, s := range have {
		followed[strings.ToLower(s.Repo)] = true
	}
	inList := map[string]bool{}
	for _, l := range lists {
		for _, r := range l.Repos {
			inList[strings.ToLower(r.Repo)] = true
		}
	}
	var other []curated.Repo
	for _, s := range have {
		if !inList[strings.ToLower(s.Repo)] {
			other = append(other, curated.Repo{Repo: s.Repo})
		}
	}
	if len(o.Lists) == 0 && len(other) > 0 {
		shown = append(shown[:len(shown):len(shown)], curated.List{Name: "Other", Repos: other})
	}

	width, starW := 0, 0
	stars := func(r curated.Repo) string {
		if r.Stars > 0 {
			return search.Stars(r.Stars)
		}
		return "—"
	}
	for _, l := range shown {
		for _, r := range l.Repos {
			width = max(width, utf8.RuneCountInString(r.Repo))
			starW = max(starW, utf8.RuneCountInString(stars(r)))
		}
	}
	term := 0
	if sys.TermWidth != nil {
		term = sys.TermWidth()
	}
	line := func(s string) {
		if term > 0 {
			s = ansi.Truncate(s, term, "…")
		}
		fmt.Fprintln(sys.Stdout, s)
	}
	pad := func(s string, w int) string { return s + strings.Repeat(" ", w-utf8.RuneCountInString(s)) }
	if cached {
		fmt.Fprintf(sys.Stdout, "%s offline: using the copy of %s saved earlier\n", warn, o.List)
	}
	if skipped > 0 {
		fmt.Fprintf(sys.Stdout, "%s skipped %d %s without a valid owner/repo\n", warn, skipped, plural(skipped, "row"))
	}
	count := 0
	for _, l := range shown {
		line(fmt.Sprintf("%s (%d)", l.Name, len(l.Repos)))
		for _, r := range l.Repos {
			mark := " "
			if followed[strings.ToLower(r.Repo)] {
				mark = "✓"
				count++
			}
			row := fmt.Sprintf("  %s %s  ★ %s", mark, pad(r.Repo, width), pad(stars(r), starW))
			if d := strings.Join(strings.Fields(r.Description), " "); d != "" {
				row += "  " + d
			}
			line(strings.TrimRight(row, " "))
		}
		fmt.Fprintln(sys.Stdout)
	}
	fmt.Fprintf(sys.Stdout, "✓ follows %d of the repos shown — `ipm marketplace add <repo>` follows one\n", count)
	return nil
}
