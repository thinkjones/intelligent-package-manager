package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/thinkjones/fs-skills/cli/internal/config"
	"github.com/thinkjones/fs-skills/cli/internal/marketplace"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

const (
	ok   = "✓"
	warn = "!"
	bad  = "✗"
)

func doctorFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	return fs, doctor
}

// doctor reports on config, every pack manifest and the generated
// marketplace. Problems fail it; warnings do not.
func doctor(sys System) error {
	problems, warnings := 0, 0
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	row := func(mark, label, detail string) {
		switch mark {
		case bad:
			problems++
		case warn:
			warnings++
		}
		fmt.Fprintf(tw, "%s %s\t%s\n", mark, label, detail)
	}
	defer func() {
		tw.Flush()
		fmt.Fprintf(sys.Stdout, "\nipm doctor: %d problem(s), %d warning(s)\n", problems, warnings)
	}()

	cfg, err := config.Load(sys.Getenv)
	if err != nil {
		row(bad, config.RootVar, err.Error())
		return &ExitError{Code: 1}
	}
	row(ok, config.RootVar, cfg.Root)

	dirs, err := pack.Dirs(cfg.Root)
	if err != nil {
		row(bad, "packs", err.Error())
		return &ExitError{Code: 1}
	}
	if len(dirs) == 0 {
		row(warn, "packs", "none — a pack is a root directory holding "+pack.ManifestFile)
	}
	var loaded []pack.Pack
	for _, d := range dirs {
		p, err := pack.Load(d)
		switch {
		case err != nil:
			row(bad, filepath.Base(d), err.Error())
		case p.Empty():
			row(warn, p.Name, "publishes nothing: no [[plugin]], [[copilot-plugin]] or includes")
		default:
			loaded = append(loaded, p)
			row(ok, p.Name, fmt.Sprintf("%d plugin(s), %d Copilot item(s), %d include(s)", len(p.Plugins), len(p.Copilot()), len(p.Meta.Includes)))
		}
	}
	for _, problem := range pack.Problems(loaded) {
		row(bad, "packs", problem)
	}

	switch lock, err := upstream.ReadLock(cfg.Root); {
	case err != nil:
		row(bad, upstream.LockFile, err.Error())
	default:
		ups := pack.Upstreams(loaded)
		if missing := lock.Missing(ups); len(missing) > 0 {
			row(warn, upstream.LockFile, fmt.Sprintf("%d of %d upstream(s) not pinned — run `ipm sync`", len(missing), len(ups)))
		} else {
			row(ok, upstream.LockFile, fmt.Sprintf("%d upstream(s) pinned", len(ups)))
		}
	}

	path := marketplace.Path(cfg.Root)
	switch _, err := os.Stat(path); {
	case err != nil:
		row(warn, "marketplace", "not generated yet — run `ipm create-plugin`")
	case !marketplace.IsOurs(path):
		row(warn, "marketplace", path+" was not written by ipm")
	default:
		row(ok, "marketplace", path)
	}

	dockerRow, tsRow := searchHealth(sys, cfg.Root)
	row(dockerRow[0], "docker", dockerRow[1])
	row(tsRow[0], "typesense", tsRow[1])

	if p, err := sys.LookPath("claude"); err != nil {
		row(warn, "claude", "not on PATH — create-plugin cannot validate")
	} else {
		row(ok, "claude", p)
	}

	if problems > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}
