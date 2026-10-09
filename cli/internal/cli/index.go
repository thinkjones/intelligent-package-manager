package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

func indexFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	return fs, runIndex
}

// runIndex writes catalog/*.jsonl from the repos of interest at their locked
// commits. It reads only aihs.lock and the cache, and writes nothing unless
// every repo is pinned and cached.
func runIndex(sys System) error {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	ups, err := pack.LoadInterest(cfg.Root)
	if err != nil {
		return err
	}
	if len(ups) == 0 {
		return fmt.Errorf("no repos of interest: list them as [[repo]] in %s, then run `aihs sync`", filepath.Join(cfg.Root, pack.ManifestFile))
	}
	lock, err := upstream.ReadLock(cfg.Root)
	if err != nil {
		return err
	}
	cos := make([]catalog.Checkout, 0, len(ups))
	for _, u := range ups {
		dir, err := upstream.Checkout(cfg.Root, u, lock)
		if err != nil {
			return err
		}
		cos = append(cos, catalog.Checkout{Repo: u.Repo, Commit: lock[u.Key()], Dir: dir})
	}
	recs, warnings, err := catalog.Build(cos)
	if err != nil {
		return err
	}
	if err := catalog.Write(filepath.Join(cfg.Root, "catalog"), recs); err != nil {
		return err
	}

	counts := map[string]int{}
	for _, r := range recs {
		counts[r.Type]++
	}
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	for _, typ := range catalog.Types {
		fmt.Fprintf(tw, "%s\t%d\n", typ, counts[typ])
	}
	tw.Flush()
	for _, w := range warnings {
		fmt.Fprintf(sys.Stdout, "%s %s\n", warn, w)
	}
	fmt.Fprintf(sys.Stdout, "\ncatalog: %d record(s), %d warning(s) — catalog/\n", len(recs), len(warnings))
	return nil
}
