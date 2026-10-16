package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"

	"github.com/spf13/cobra"
)

const (
	ok   = "✓"
	warn = "!"
	bad  = "✗"
)

func newDoctorCmd(sys System) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "check home, pnpm, sources, pin store, catalog and search — exits 1 on problems",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return doctor(sys) },
	}
}

// doctor reports where ipm keeps things and whether each piece is ready.
// Problems (no home, no pnpm) fail it; warnings do not.
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

	p, err := ipmPaths(sys)
	if err != nil {
		row(bad, "home", err.Error())
		return &ExitError{Code: 1}
	}
	row(ok, "home", p.Home)
	row(ok, "cache", p.Cache)

	if pn, err := findPnpm(sys); err != nil {
		row(bad, "pnpm", err.Error())
	} else if pn.name == "pnpm" {
		row(ok, "pnpm", pn.version)
	} else {
		row(ok, "pnpm", fmt.Sprintf("%s via `%s` (pnpm on PATH is older than %d or missing)", pn.version, pn, pnpmMajor))
	}

	srcs, err := sources.Load(p.Sources())
	switch {
	case err != nil:
		row(bad, "sources", err.Error())
	case len(srcs) == 0:
		row(warn, "sources", "none — add some with `ipm marketplace add owner/repo`")
	default:
		if _, err := sources.Checkouts(p.Sources()); err != nil {
			row(warn, "sources", fmt.Sprintf("%d, not all synced — run `ipm catalog sync`", len(srcs)))
		} else {
			row(ok, "sources", fmt.Sprintf("%d synced, in %s", len(srcs), p.Sources()))
		}
	}

	if pins, err := sources.Load(p.Pinned()); err != nil {
		row(warn, "pinned", err.Error())
	} else if len(pins) == 0 {
		row(ok, "pinned", "none yet — pack builds fetch here the commits your sources aren't at")
	} else {
		row(ok, "pinned", fmt.Sprintf("%d commit(s), in %s", len(pins), p.Pinned()))
	}

	records := 0
	for _, typ := range catalog.Types {
		recs, err := catalog.Load(p.Catalog(), typ)
		if err != nil {
			records = -1
			break
		}
		records += len(recs)
	}
	if records < 0 {
		row(warn, "catalog", "not built — run `ipm catalog sync`")
	} else {
		row(ok, "catalog", fmt.Sprintf("%d record(s), in %s", records, p.Catalog()))
	}

	dockerRow, tsRow := searchHealth(sys)
	row(dockerRow[0], "docker", dockerRow[1])
	row(tsRow[0], "typesense", tsRow[1])

	if path, err := sys.LookPath("claude"); err != nil {
		row(warn, "claude", "not on PATH — plugins you build can't be validated")
	} else {
		row(ok, "claude", path)
	}

	if problems > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}
