package cli

import (
	"flag"
	"fmt"

	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

func syncFlags() (*flag.FlagSet, func(System) error) {
	var upgrade bool
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.BoolVar(&upgrade, "upgrade", false, "move every pin to the latest commit of its branch or tag")
	return fs, func(sys System) error { return runSync(sys, upgrade) }
}

// runSync caches every upstream the packs and the repos of interest read
// from and pins it in aihs.lock.
func runSync(sys System, upgrade bool) error {
	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		return err
	}
	packs, err := pack.LoadAll(cfg.Root, nil)
	if err != nil {
		return err
	}
	lock, err := upstream.ReadLock(cfg.Root)
	if err != nil {
		return err
	}
	interest, err := pack.LoadInterest(cfg.Root)
	if err != nil {
		return err
	}
	ups := pack.Merge(pack.Upstreams(packs), interest)
	git := func(dir string, args ...string) (string, error) { return sys.Output(dir, "git", args...) }
	results, next := upstream.Sync(git, cfg.Root, ups, lock, upgrade)
	if err := upstream.WriteLock(cfg.Root, next, ups); err != nil {
		return err
	}

	width := 0
	for _, u := range ups {
		width = max(width, len(u.Key()))
	}
	failed, fetched, moved := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Err != nil:
			failed++
			fmt.Fprintf(sys.Stdout, "%s %-*s  %v\n", bad, width, r.Upstream.Key(), r.Err)
		case r.Was != "":
			moved++
			fmt.Fprintf(sys.Stdout, "%s %-*s  %s → %s\n", ok, width, r.Upstream.Key(), r.Was[:7], r.Commit[:7])
		default:
			note := "cached"
			if r.Fetched {
				fetched++
				note = "fetched"
			}
			fmt.Fprintf(sys.Stdout, "%s %-*s  %s  %s\n", ok, width, r.Upstream.Key(), r.Commit[:7], note)
		}
	}
	fmt.Fprintf(sys.Stdout, "\n%d upstream(s): %d fetched, %d moved, %d failed — %s\n", len(ups), fetched, moved, failed, upstream.LockFile)
	if failed > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}
