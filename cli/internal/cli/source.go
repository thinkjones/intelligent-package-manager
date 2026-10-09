package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/paths"
	"github.com/thinkjones/fs-skills/cli/internal/sources"
)

// ipmPaths is where this user's ipm keeps things.
func ipmPaths(sys System) (paths.Paths, error) {
	return paths.Resolve(sys.Getenv, runtime.GOOS)
}

func sourceFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("source", flag.ContinueOnError)
	return fs, func(sys System) error {
		args := fs.Args()
		if len(args) == 0 {
			return usagef("source: say add, remove, list or import. Try `ipm help source`.")
		}
		p, err := ipmPaths(sys)
		if err != nil {
			return err
		}
		verb, rest := args[0], args[1:]
		switch verb {
		case "add":
			return sourceAdd(sys, p, rest)
		case "remove":
			if len(rest) == 0 {
				return usagef("source remove: name at least one repo")
			}
			if err := sources.Remove(p.Sources(), rest); err != nil {
				return err
			}
			fmt.Fprintf(sys.Stdout, "%s removed %s — run `ipm sync` to reindex\n", ok, strings.Join(rest, ", "))
			return nil
		case "list":
			return sourceList(sys, p)
		case "import":
			if len(rest) != 1 {
				return usagef("source import: give the path of one manifest.toml")
			}
			return sourceImport(sys, p, rest[0])
		}
		return usagef("source: unknown action %q — use add, remove, list or import", verb)
	}
}

// sourceAdd adds repos; an external plugin's catalog ID adds the repo it
// lives in.
func sourceAdd(sys System, p paths.Paths, args []string) error {
	if len(args) == 0 {
		return usagef("source add: name at least one repo (owner/repo[@ref], a git URL, or an external plugin ID)")
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
	fmt.Fprintln(sys.Stdout, "\nNext: ipm sync")
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
	return "", fmt.Errorf("no plugin %s in the catalog — run `ipm index`", id)
}

func sourceList(sys System, p paths.Paths) error {
	srcs, err := sources.Load(p.Sources())
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		fmt.Fprintln(sys.Stdout, "no sources — add one with `ipm source add owner/repo`")
		return nil
	}
	commits := map[string]string{}
	if cos, err := sources.Checkouts(p.Sources()); err == nil {
		for _, c := range cos {
			commits[c.Alias] = c.Commit[:7]
		}
	}
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ALIAS\tREPO\tREF\tCOMMIT")
	for _, s := range srcs {
		ref, commit := s.Ref, commits[s.Alias]
		if ref == "" {
			ref = "—"
		}
		if commit == "" {
			commit = "not synced"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Alias, s.Repo, ref, commit)
	}
	tw.Flush()
	fmt.Fprintf(sys.Stdout, "\n%d source(s) in %s\n", len(srcs), p.Sources())
	return nil
}

// sourceImport adds the [[repo]] entries of an aihs-era manifest.toml, and
// copies its sibling catalog/github.jsonl so stats needn't be refetched.
func sourceImport(sys System, p paths.Paths, file string) error {
	if filepath.Base(file) != pack.ManifestFile {
		return usagef("source import: expected a file named %s, got %s", pack.ManifestFile, file)
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
	fmt.Fprintln(sys.Stdout, "\nNext: ipm sync")
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
