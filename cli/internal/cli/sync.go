package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func syncFlags() (*flag.FlagSet, func(System) error) {
	var upgrade bool
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.BoolVar(&upgrade, "upgrade", false, "move every source to the latest commit of its branch or tag")
	return fs, func(sys System) error { return runSync(sys, upgrade) }
}

// runSync has pnpm fetch and pin every source (never running their
// scripts), then reindexes. A failed pnpm run leaves the catalog as it was.
func runSync(sys System, upgrade bool) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	pn, err := findPnpm(sys)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(p.Sources(), "package.json")); err != nil {
		return fmt.Errorf("no sources yet — add some with `ipm source add owner/repo`")
	}
	args := []string{"install"}
	if upgrade {
		args = []string{"update", "--latest"}
	}
	args = append(args, "--ignore-scripts", "--store-dir", p.Store(), "--dir", p.Sources())
	if err := writePnpmfile(p.Sources()); err != nil {
		return err
	}
	fmt.Fprintf(sys.Stdout, "%s %s  (pnpm %s)\n", pn, strings.Join(args, " "), pn.version)
	out, err := sys.Output(p.Sources(), pn.name, append(append([]string{}, pn.prefix...), args...)...)
	if err != nil {
		fmt.Fprintln(sys.Stdout, strings.TrimSpace(out))
		fmt.Fprintf(sys.Stdout, "%s pnpm failed — the catalog is unchanged\n", bad)
		return &ExitError{Code: 1}
	}
	fmt.Fprintf(sys.Stdout, "%s sources synced\n\n", ok)
	return runIndex(sys)
}
