package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/paths"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/tui"
)

// existing is what an earlier ipm setup holds.
type existing struct {
	Sources, Artifacts int
	Synced             time.Time
}

// detect reports whether home has sources or the cache has a catalog, and
// what they hold.
func detect(p paths.Paths) (existing, bool, error) {
	var e existing
	srcs, err := sources.Load(p.Sources())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return e, false, err
	}
	e.Sources = len(srcs)
	info, err := os.Stat(filepath.Join(p.Catalog(), catalog.Files[catalog.TypeRepo]))
	hasCatalog := err == nil
	if hasCatalog {
		e.Synced = info.ModTime()
		for _, typ := range catalog.Types {
			if typ == catalog.TypeRepo {
				continue
			}
			if recs, err := catalog.Load(p.Catalog(), typ); err == nil {
				e.Artifacts += len(recs)
			}
		}
	}
	return e, e.Sources > 0 || hasCatalog, nil
}

// String is the one-line summary: 26 sources · 3,280 artifacts · synced
// 2026-10-09.
func (e existing) String() string {
	s := fmt.Sprintf("%s %s · %s %s", thousands(e.Sources), plural(e.Sources, "source"), thousands(e.Artifacts), plural(e.Artifacts, "artifact"))
	if !e.Synced.IsZero() {
		s += " · synced " + e.Synced.Format("2006-01-02")
	}
	return s
}

// plural is word, with an s unless n is 1.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// thousands formats n with commas: 3280 → 3,280.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// backup renames home, then the cache, to <dir>.backup-YYYYMMDD-HHMM (with
// -2, -3… when taken), skipping one that doesn't exist and a cache that
// lived inside home. It returns the new paths. If the cache can't be
// moved, home is moved back, so a failure changes nothing.
func backup(p paths.Paths, now time.Time) ([]string, error) {
	var moved []string
	var undo func()
	for i, dir := range []string{p.Home, p.Cache} {
		if i == 1 && inside(dir, p.Home) {
			continue
		}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		}
		to := backupName(dir, now)
		if err := os.Rename(dir, to); err != nil {
			if undo != nil {
				undo()
			}
			return nil, fmt.Errorf("can't back up %s: %v — nothing was changed", dir, err)
		}
		from := dir
		undo = func() { _ = os.Rename(to, from) }
		moved = append(moved, to)
	}
	return moved, nil
}

// inside reports whether path is dir or below it.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func backupName(dir string, now time.Time) string {
	base := dir + ".backup-" + now.Format("20060102-1504")
	name := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(name); errors.Is(err, os.ErrNotExist) {
			return name
		}
		name = base + "-" + strconv.Itoa(n)
	}
}

// restoreHint says how to put backed-up folders back: remove what ipm made
// since (mv would otherwise nest the backup inside it), move the backups
// back, and restart the search server on the restored data.
func restoreHint(p paths.Paths, moved []string) string {
	var origs, mvs []string
	for _, m := range moved {
		orig := p.Cache
		if strings.HasPrefix(m, p.Home+".backup-") {
			orig = p.Home
		}
		origs = append(origs, orig)
		mvs = append(mvs, "mv "+m+" "+orig)
	}
	return "to restore: rm -rf " + strings.Join(origs, " ") + " && " + strings.Join(mvs, " && ") + " && ipm catalog serve"
}

// defaultLists are pre-ticked when onboarding starts from nothing.
var defaultLists = []string{"AI · Agentic Engineering Methods", "AI · Skills, Plugins & Registries"}

// genericTopics are too broad to suggest as a first search.
var genericTopics = map[string]bool{"ai": true, "llm": true, "llms": true, "agents": true, "ai-agents": true, "agent": true, "claude": true, "claude-code": true, "anthropic": true, "openai": true}

type onboardOpts struct {
	List             string
	Lists            []string
	Keep, Fresh, Yes bool
}

func newOnboardCmd(sys System) *cobra.Command {
	var o onboardOpts
	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "learn the concepts, pick repos from the marketplace, build your catalog",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return runOnboard(sys, o) },
	}
	f := cmd.Flags()
	f.StringVar(&o.List, "list", curated.DefaultURL, "the curated list: a URL or a file")
	f.StringArrayVar(&o.Lists, "lists", nil, "follow every repo in this list, by exact name (repeatable); skips the checklist")
	f.BoolVar(&o.Keep, "keep", false, "use existing ipm data without asking")
	f.BoolVar(&o.Fresh, "fresh", false, "back up existing ipm data and start fresh without asking")
	f.BoolVar(&o.Yes, "yes", false, "follow the pre-ticked lists without showing the checklist")
	return cmd
}

// onboardIntro is the four concepts, printed before anything else.
const onboardIntro = `ipm in four steps:
  marketplace  curated repos you can follow — pick the ones you care about
  catalog      the repos you follow, fetched, pinned and searchable
  pack         the skills, agents and plugins you pick from the catalog
  install      a pack, built and installed into Claude Code

`

// runOnboard takes a user from nothing (or an old setup) to a searchable
// library: keep or back up existing data, pick curated sources, then add,
// sync, fetch stats and load search.
func runOnboard(sys System, o onboardOpts) error {
	if o.Keep && o.Fresh {
		return usagef("onboard: use --keep or --fresh, not both")
	}
	interactive := sys.Interactive != nil && sys.Interactive()
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	in := bufio.NewReader(orEmpty(sys.Stdin))
	now := time.Now()
	if sys.Now != nil {
		now = sys.Now()
	}

	fmt.Fprint(sys.Stdout, onboardIntro)

	// Everything that can fail without changing anything comes first: the
	// flags, then the curated list and the names in --lists. Only then is
	// existing data backed up.
	e, found, err := detect(p)
	if err != nil {
		return err
	}
	if found && !o.Keep && !o.Fresh && !interactive {
		return usagef("onboard: ipm data exists — pass --keep to use it or --fresh to back it up")
	}
	if len(o.Lists) == 0 && !o.Yes && !interactive {
		return usagef("onboard: pick sources with --lists <name> or --yes when not in a terminal")
	}
	data, cached, err := curated.Fetch(sys.HTTP, o.List, p.Cache)
	if err != nil {
		return err
	}
	lists, skipped, err := curated.Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %v", o.List, err)
	}
	var picks []string
	switch {
	case len(o.Lists) > 0:
		if picks, err = reposIn(lists, o.Lists); err != nil {
			return err
		}
	case o.Yes:
		if picks, _ = reposIn(lists, presentLists(lists, defaultLists)); len(picks) == 0 {
			return fmt.Errorf("%s has none of the pre-ticked lists (%s) — name lists with --lists", o.List, strings.Join(defaultLists, ", "))
		}
	}

	var moved []string
	if found {
		fmt.Fprintf(sys.Stdout, "found existing ipm data: %s\n", e)
		fresh := o.Fresh
		if !o.Keep && !o.Fresh {
			switch ask(sys, in, "[u]se it · [b]ack it up and start fresh · [q]uit: ", "u", "b", "q") {
			case "q":
				fmt.Fprintln(sys.Stdout, "nothing changed")
				return nil
			case "b":
				fresh = true
			}
		}
		if fresh {
			if moved, err = backup(p, now); err != nil {
				return err
			}
			for _, m := range moved {
				fmt.Fprintf(sys.Stdout, "%s backed up to %s\n", ok, m)
			}
			fmt.Fprintln(sys.Stdout, restoreHint(p, moved))
		}
	}
	if cached {
		fmt.Fprintf(sys.Stdout, "%s offline: using the copy of %s saved earlier\n", warn, o.List)
	}
	if skipped > 0 {
		fmt.Fprintf(sys.Stdout, "%s skipped %d %s without a valid owner/repo\n", warn, skipped, plural(skipped, "row"))
	}

	if len(o.Lists) == 0 && !o.Yes {
		have, err := sources.Load(p.Sources())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		locked := map[string]bool{}
		for _, s := range have {
			locked[strings.ToLower(s.Repo)] = true
		}
		ticked := map[string]bool{}
		if len(have) == 0 {
			pre, _ := reposIn(lists, presentLists(lists, defaultLists))
			for _, r := range pre {
				ticked[r] = true
			}
		}
		c := tui.NewChecklist(checklistLists(lists), ticked, locked, now)
		run := sys.RunChecklist
		if run == nil {
			run = func(c *tui.Checklist) error { _, err := tea.NewProgram(c).Run(); return err }
		}
		err = run(c)
		if err != nil || !c.Confirmed() {
			if len(moved) > 0 {
				fmt.Fprintf(sys.Stdout, "no sources added; your data was backed up to %s\n%s\n", strings.Join(moved, " and "), restoreHint(p, moved))
			} else {
				fmt.Fprintln(sys.Stdout, "nothing changed")
			}
			return err
		}
		picks = c.Picked()
	}

	return onboardPipeline(sys, p, lists, picks)
}

// onboardPipeline adds picks as sources, syncs, fetches stats and loads
// search, stopping at the first step that fails. In a terminal the steps'
// own output feeds one progress line and is shown only when a step fails.
func onboardPipeline(sys System, p paths.Paths, lists []curated.List, picks []string) error {
	pr := newProgress(sys, 5)
	defer pr.close()
	step := sys // what the steps run with
	if pr != nil {
		step.Stdout, step.Progress = pr, pr.count
	}
	// say prints one of onboard's own lines; beside the bar they need no
	// blank lines to set them off from a step's output.
	say := func(format string, a ...any) {
		s := fmt.Sprintf(format, a...)
		if pr == nil {
			fmt.Fprint(sys.Stdout, s)
			return
		}
		pr.print(strings.Trim(s, "\n") + "\n")
	}
	fail := func(name string, err error, rerun string) error {
		out := pr.output()
		pr.close()
		fmt.Fprint(sys.Stdout, out)
		var exit *ExitError
		if errors.As(err, &exit) {
			fmt.Fprintf(sys.Stdout, "%s %s\n", bad, name)
		} else {
			fmt.Fprintf(sys.Stdout, "%s %s: %v\n", bad, name, err)
		}
		fmt.Fprintf(sys.Stdout, "rerun: %s\n", rerun)
		return &ExitError{Code: 1}
	}
	pr.begin(1, "sources")
	added, skipped, err := sources.AddSkippingExisting(p.Sources(), picks)
	if err != nil {
		return fail("add sources", err, "ipm onboard")
	}
	say("%s added %d %s (%d already followed)\n\n", ok, len(added), plural(len(added), "source"), len(skipped))

	pr.begin(2, "sync")
	if err := syncSources(step, false); err != nil {
		return fail("sync", err, "ipm catalog sync")
	}
	say("%s sync%s\n\n", ok, pr.summary())

	pr.begin(3, "stats")
	if reason := ghMissing(sys); reason != "" {
		say("- stats skipped: %s\n", reason)
	} else if err := runStats(step); err != nil {
		return fail("stats", err, "ipm marketplace stats")
	} else {
		say("%s stats%s\n\n", ok, pr.summary())
	}

	pr.begin(4, "search server")
	if dockerReady(sys) != nil {
		say("- search skipped: start Docker, then ipm catalog serve && ipm catalog rebuild\n")
	} else {
		if err := runServe(step, false); err != nil {
			return fail("search", err, "ipm catalog serve && ipm catalog rebuild")
		}
		pr.begin(5, "search index")
		if err := runLoad(step); err != nil {
			return fail("search", err, "ipm catalog rebuild")
		}
		say("%s search%s\n", ok, pr.summary())
	}
	pr.close()

	e, _, _ := detect(p)
	say("\n%s library ready — %s %s, %s %s\n", ok, thousands(e.Sources), plural(e.Sources, "source"), thousands(e.Artifacts), plural(e.Artifacts, "artifact"))
	var try []string
	for _, t := range topTopics(lists, picks, 3) {
		try = append(try, "ipm search "+t)
	}
	say("try: %s\n", strings.Join(append(try, "ipm tui"), " · "))
	return nil
}

// ghMissing is why stats can't run, or "" when gh is ready.
func ghMissing(sys System) string {
	if _, err := sys.LookPath("gh"); err != nil {
		return "gh is not installed"
	}
	if _, err := sys.Output("", "gh", "auth", "status"); err != nil {
		return "gh is not logged in (gh auth login)"
	}
	return ""
}

// ask prints prompt and reads lines until one is a choice (case-insensitive);
// end of input is the last choice.
func ask(sys System, in *bufio.Reader, prompt string, choices ...string) string {
	for {
		fmt.Fprint(sys.Stdout, prompt)
		line, err := in.ReadString('\n')
		got := strings.ToLower(strings.TrimSpace(line))
		for _, c := range choices {
			if got == c {
				fmt.Fprintln(sys.Stdout)
				return c
			}
		}
		if err != nil {
			fmt.Fprintln(sys.Stdout)
			return choices[len(choices)-1]
		}
	}
}

func orEmpty(r io.Reader) io.Reader {
	if r == nil {
		return strings.NewReader("")
	}
	return r
}

// presentLists is the names that lists has.
func presentLists(lists []curated.List, names []string) []string {
	var out []string
	for _, n := range names {
		for _, l := range lists {
			if l.Name == n {
				out = append(out, n)
			}
		}
	}
	return out
}

// reposIn is every repo in the named lists, each once; an unknown name is an
// error listing the names there are.
func reposIn(lists []curated.List, names []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		var list *curated.List
		for i := range lists {
			if lists[i].Name == n {
				list = &lists[i]
			}
		}
		if list == nil {
			var all []string
			for _, l := range lists {
				all = append(all, "  "+l.Name)
			}
			return nil, fmt.Errorf("no list named %q; the lists are:\n%s", n, strings.Join(all, "\n"))
		}
		for _, r := range list.Repos {
			if k := strings.ToLower(r.Repo); !seen[k] {
				seen[k] = true
				out = append(out, r.Repo)
			}
		}
	}
	return out, nil
}

func checklistLists(lists []curated.List) []tui.CuratedList {
	out := make([]tui.CuratedList, 0, len(lists))
	for _, l := range lists {
		cl := tui.CuratedList{Name: l.Name}
		for _, r := range l.Repos {
			cl.Repos = append(cl.Repos, tui.CuratedRepo{Repo: r.Repo, Description: r.Description, Language: r.Language, Stars: r.Stars, LastPush: r.LastPush, Topics: r.Topics, Homepage: r.Homepage})
		}
		out = append(out, cl)
	}
	return out
}

// topTopics is the n most common topics across picks, ties alphabetical,
// leaving out generic ones.
func topTopics(lists []curated.List, picks []string, n int) []string {
	picked := map[string]bool{}
	for _, p := range picks {
		picked[strings.ToLower(p)] = true
	}
	count := map[string]int{}
	seen := map[string]bool{}
	for _, l := range lists {
		for _, r := range l.Repos {
			k := strings.ToLower(r.Repo)
			if !picked[k] || seen[k] {
				continue
			}
			seen[k] = true
			for _, t := range r.Topics {
				if !genericTopics[t] {
					count[t]++
				}
			}
		}
	}
	topics := make([]string, 0, len(count))
	for t := range count {
		topics = append(topics, t)
	}
	sort.Slice(topics, func(a, b int) bool {
		if count[topics[a]] != count[topics[b]] {
			return count[topics[a]] > count[topics[b]]
		}
		return topics[a] < topics[b]
	})
	return topics[:min(n, len(topics))]
}
