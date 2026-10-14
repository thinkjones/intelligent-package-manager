package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/github"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/paths"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"

	"github.com/spf13/cobra"
)

// ipmPaths is where this user's ipm keeps things.
func ipmPaths(sys System) (paths.Paths, error) {
	return paths.Resolve(sys.Getenv, runtime.GOOS)
}

func newMarketplaceAddCmd(sys System) *cobra.Command {
	return &cobra.Command{
		Use:   "add <repo>...",
		Short: "follow repos: owner/repo[@ref], a git URL[#ref], or an external plugin ID",
		Args:  minArgs(1, "at least one repo"),
		RunE: func(_ *cobra.Command, args []string) error {
			p, err := ipmPaths(sys)
			if err != nil {
				return err
			}
			return sourceAdd(sys, p, args)
		},
	}
}

func newMarketplaceRemoveCmd(sys System) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <repo>...",
		Short: "stop following repos",
		Args:  minArgs(1, "at least one repo"),
		RunE: func(_ *cobra.Command, args []string) error {
			p, err := ipmPaths(sys)
			if err != nil {
				return err
			}
			return sourceRemove(sys, p, args)
		},
	}
}

// sourceRemove stops following repos.
func sourceRemove(sys System, p paths.Paths, repos []string) error {
	if err := sources.Remove(p.Sources(), repos); err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s removed %s — run `ipm catalog sync` to reindex\n", ok, strings.Join(repos, ", "))
	return nil
}

func newMarketplaceImportCmd(sys System) *cobra.Command {
	return &cobra.Command{
		Use:   "import <path/to/manifest.toml>",
		Short: "one-time: add the [[repo]] entries of an aihs-era manifest.toml",
		Args:  exactArgs(1, "the path of one manifest.toml"),
		RunE: func(_ *cobra.Command, args []string) error {
			p, err := ipmPaths(sys)
			if err != nil {
				return err
			}
			return sourceImport(sys, p, args[0])
		},
	}
}

// sourceAdd adds repos; an external plugin's catalog ID adds the repo it
// lives in.
func sourceAdd(sys System, p paths.Paths, args []string) error {
	if len(args) == 0 {
		return usagef("marketplace add: name at least one repo (owner/repo[@ref], a git URL, or an external plugin ID)")
	}
	var plugins []map[string]any
	repos := make([]string, 0, len(args))
	for _, a := range args {
		if !strings.HasPrefix(a, catalog.TypePlugin+":") {
			repos = append(repos, a)
			continue
		}
		if plugins == nil {
			var err error
			if plugins, err = catalog.Load(p.Catalog(), catalog.TypePlugin); err != nil {
				return err
			}
		}
		repo, err := sourceRepo(plugins, a)
		if err != nil {
			return err
		}
		repos = append(repos, repo)
	}
	added, err := sources.Add(p.Sources(), repos)
	if err != nil {
		return err
	}
	for _, s := range added {
		fmt.Fprintf(sys.Stdout, "%s %s (%s)\n", ok, s.Repo, s.Alias)
	}
	fmt.Fprintln(sys.Stdout, "\nNext: ipm catalog sync")
	return nil
}

// sourceRepo is the repo an external plugin record lives in.
func sourceRepo(plugins []map[string]any, id string) (string, error) {
	for _, pl := range plugins {
		if pl["id"] != id {
			continue
		}
		if repo, _ := pl["source_repo"].(string); repo != "" {
			return repo, nil
		}
		return "", fmt.Errorf("%s is not an external plugin — its repo is already a source", id)
	}
	return "", fmt.Errorf("no plugin %s in the catalog — run `ipm catalog rebuild`", id)
}

// packsMaxAge is how old the saved source packs may be before ipm marketplace
// --followed fetches them again.
const packsMaxAge = 7 * 24 * time.Hour

// sourcePacks is the default curated list, which groups ipm marketplace --followed:
// the saved copy, fetched again when it is missing or old. ok is false when
// there is no copy and none could be fetched.
func sourcePacks(sys System, p paths.Paths) (lists []curated.List, ok bool) {
	data, saved, ok := curated.Cached(curated.DefaultURL, p.Cache)
	now := time.Now()
	if sys.Now != nil {
		now = sys.Now()
	}
	if (!ok || now.Sub(saved) > packsMaxAge) && sys.HTTP != nil {
		client := *sys.HTTP
		client.Timeout = 5 * time.Second
		if fresh, _, err := curated.Fetch(&client, curated.DefaultURL, p.Cache); err == nil {
			data, ok = fresh, true
		}
	}
	if !ok {
		return nil, false
	}
	lists, _, err := curated.Parse(data)
	return lists, err == nil
}

// sourceRow is one source as ipm marketplace --followed shows it.
type sourceRow struct {
	alias, repo, stars, ref, commit string
	description, homepage           string
	topics                          []string
}

// sourceList (ipm marketplace --followed) prints the sources under the source pack each belongs to (the
// first, for a repo in several), then the rest under Other. Stars,
// description and topics come from the pack, else from ipm marketplace stats.
func sourceList(sys System, p paths.Paths) error {
	srcs, err := sources.Load(p.Sources())
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		fmt.Fprintln(sys.Stdout, "no sources — add one with `ipm marketplace add owner/repo`")
		return nil
	}
	commits := map[string]string{}
	if cos, err := sources.Checkouts(p.Sources()); err == nil {
		for _, c := range cos {
			commits[c.Alias] = c.Commit[:7]
		}
	}
	stats := map[string]github.Stats{}
	if read, err := github.Read(filepath.Join(p.Catalog(), "github.jsonl")); err == nil {
		for repo, s := range read {
			stats[strings.ToLower(repo)] = s
		}
	}
	lists, grouped := sourcePacks(sys, p)
	packOf := map[string]int{}
	detail := map[string]curated.Repo{}
	for i, l := range lists {
		for _, r := range l.Repos {
			if k := strings.ToLower(r.Repo); detail[k].Repo == "" {
				packOf[k], detail[k] = i, r
			}
		}
	}
	groups := make([][]sourceRow, len(lists)+1)
	width := [4]int{}
	for _, s := range srcs {
		k := strings.ToLower(s.Repo)
		row := sourceRow{alias: s.Alias, repo: s.Repo, stars: "—", ref: s.Ref, commit: commits[s.Alias]}
		if row.ref == "" {
			row.ref = "—"
		}
		if row.commit == "" {
			row.commit = "not synced"
		}
		g := len(lists)
		if d, ok := detail[k]; ok {
			g = packOf[k]
			row.description, row.homepage, row.topics = d.Description, d.Homepage, d.Topics
			if d.Stars > 0 {
				row.stars = search.Stars(d.Stars)
			}
		}
		if st, ok := stats[k]; ok {
			if row.stars == "—" {
				row.stars = search.Stars(st.Stars)
			}
			if row.description == "" {
				row.description = st.Description
			}
			if row.topics == nil {
				row.topics = st.Topics
			}
		}
		groups[g] = append(groups[g], row)
		for i, cell := range []string{row.alias, row.repo, row.stars, row.ref} {
			width[i] = max(width[i], utf8.RuneCountInString(cell))
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
	for g, rows := range groups {
		if len(rows) == 0 {
			continue
		}
		name := "Other"
		switch {
		case g < len(lists):
			name = lists[g].Name
		case !grouped:
			name = "Sources"
		}
		line(fmt.Sprintf("%s (%d)", name, len(rows)))
		for _, r := range rows {
			line(fmt.Sprintf("  %s  %s  ★ %s  %s  %s", pad(r.alias, width[0]), pad(r.repo, width[1]), pad(r.stars, width[2]), pad(r.ref, width[3]), r.commit))
			if d := strings.Join(strings.Fields(r.description), " "); d != "" {
				line("      " + d)
			}
			var more []string
			if len(r.topics) > 0 {
				more = append(more, "topics: "+strings.Join(r.topics, ", "))
			}
			if r.homepage != "" {
				more = append(more, r.homepage)
			}
			if len(more) > 0 {
				line("      " + strings.Join(more, " · "))
			}
		}
		fmt.Fprintln(sys.Stdout)
	}
	fmt.Fprintf(sys.Stdout, "%d source(s) in %s\n", len(srcs), p.Sources())
	if !grouped {
		fmt.Fprintln(sys.Stdout, "- not grouped: no copy of the source packs yet — `ipm onboard` fetches one")
	}
	return nil
}

// sourceImport adds the [[repo]] entries of an aihs-era manifest.toml, and
// copies its sibling catalog/github.jsonl so stats needn't be refetched.
func sourceImport(sys System, p paths.Paths, file string) error {
	if filepath.Base(file) != pack.ManifestFile {
		return usagef("marketplace import: expected a file named %s, got %s", pack.ManifestFile, file)
	}
	ups, err := pack.LoadInterest(filepath.Dir(file))
	if err != nil {
		return err
	}
	if len(ups) == 0 {
		return fmt.Errorf("%s lists no [[repo]] entries", file)
	}
	repos := make([]string, 0, len(ups))
	for _, u := range ups {
		r := u.Repo
		if u.Version != "" {
			r += "@" + u.Version
		}
		repos = append(repos, r)
	}
	added, skipped, err := sources.AddSkippingExisting(p.Sources(), repos)
	if err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s imported %d source(s) from %s\n", ok, len(added), file)
	if len(skipped) > 0 {
		fmt.Fprintf(sys.Stdout, "- %d already a source: %s\n", len(skipped), strings.Join(skipped, ", "))
	}
	stats := filepath.Join(filepath.Dir(file), "catalog", "github.jsonl")
	dest := filepath.Join(p.Catalog(), "github.jsonl")
	if _, err := os.Stat(dest); err != nil {
		if err := copyFile(stats, dest); err == nil {
			fmt.Fprintf(sys.Stdout, "%s copied GitHub stats to %s\n", ok, dest)
		}
	}
	fmt.Fprintln(sys.Stdout, "\nNext: ipm catalog sync")
	return nil
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
