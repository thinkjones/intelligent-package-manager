package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/scan"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

func scanFlags() (*flag.FlagSet, func(System) error) {
	var paths bool
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.BoolVar(&paths, "paths", false, "list every folder found, not only the counts")
	return fs, func(sys System) error { return runScan(sys, fs.Args(), paths) }
}

// scanned is one repo's scan.
type scanned struct {
	up     pack.Upstream
	commit string
	finds  []scan.Find
	err    error
}

// runScan caches each repo — the repos of interest in the root manifest.toml
// when none are named — and reports what it holds. A repo aihs.lock pins is
// scanned at that commit; any other at its remote's current commit, cached
// until the next `ipm sync` prunes it.
func runScan(sys System, args []string, paths bool) error {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	var ups []pack.Upstream
	for _, a := range args {
		repo, version, _ := strings.Cut(a, "@")
		ups = append(ups, pack.Upstream{Repo: repo, Version: version})
	}
	if len(ups) == 0 {
		if ups, err = pack.LoadInterest(cfg.Root); err != nil {
			return err
		}
		if len(ups) == 0 {
			return fmt.Errorf("no repos to scan: name one (`ipm scan owner/repo`) or list repos of interest as [[repo]] in %s", filepath.Join(cfg.Root, pack.ManifestFile))
		}
	}
	lock, err := upstream.ReadLock(cfg.Root)
	if err != nil {
		return err
	}
	git := func(dir string, args ...string) (string, error) { return sys.Output(dir, "git", args...) }

	results := make([]scanned, len(ups))
	for i, u := range ups {
		r := scanned{up: u}
		dir, _, err := upstream.Ensure(git, cfg.Root, u, lock)
		if err == nil {
			r.commit = filepath.Base(dir)
			r.finds, err = scan.Dir(dir)
		}
		r.err = err
		results[i] = r
	}

	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  REPO\tCOMMIT\tPLUGINS\tCOPILOT\tMARKETPLACES\tSKILLS\tHOOKS\tAGENTS")
	failed := 0
	for _, r := range results {
		if r.err != nil {
			failed++
			fmt.Fprintf(tw, "%s %s\t%v\n", bad, r.up.Key(), r.err)
			continue
		}
		n := scan.Count(r.finds)
		fmt.Fprintf(tw, "%s %s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\n", ok, r.up.Key(), r.commit[:7],
			n[scan.Plugin], n[scan.CopilotPlugin], n[scan.Marketplace],
			folders(r.finds, scan.Skills), folders(r.finds, scan.Hooks), folders(r.finds, scan.Agents))
	}
	tw.Flush()
	fmt.Fprintln(sys.Stdout, "\nSKILLS, HOOKS and AGENTS are folders (items in them): skill folders, hook files, agent files.")

	if paths {
		for _, r := range results {
			if r.err != nil || len(r.finds) == 0 {
				continue
			}
			fmt.Fprintf(sys.Stdout, "\n%s @ %s\n", r.up.Key(), r.commit[:7])
			tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
			for _, f := range r.finds {
				items := ""
				if f.Kind == scan.Skills || f.Kind == scan.Hooks || f.Kind == scan.Agents {
					items = fmt.Sprintf("%d", f.Items)
				}
				fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Kind, f.Path, items)
			}
			tw.Flush()
		}
	}

	fmt.Fprintf(sys.Stdout, "\n%d repo(s) scanned, %d failed\n", len(results), failed)
	if failed > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}

// folders is "folders (items)" for the finds of kind k.
func folders(finds []scan.Find, k scan.Kind) string {
	dirs, items := 0, 0
	for _, f := range finds {
		if f.Kind == k {
			dirs++
			items += f.Items
		}
	}
	return fmt.Sprintf("%d (%d)", dirs, items)
}
