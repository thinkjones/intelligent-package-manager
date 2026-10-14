package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"

	"github.com/spf13/cobra"
)

func newSyncCmd(sys System) *cobra.Command {
	var upgrade bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "fetch and pin every source with pnpm, rebuild the catalog and reload search",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return runSync(sys, upgrade) },
	}
	cmd.Flags().BoolVar(&upgrade, "upgrade", false, "move every source to the latest commit of its branch or tag")
	return cmd
}

// runSync has pnpm fetch and pin every source (never running their
// scripts), then reindexes and loads search when the server is up. A failed pnpm run leaves the catalog as it was.
func runSync(sys System, upgrade bool) error {
	if err := syncSources(sys, upgrade); err != nil {
		return err
	}
	return loadIfUp(sys)
}

// syncSources is runSync without the search load, for callers (ipm onboard)
// that load search themselves after another step.
func syncSources(sys System, upgrade bool) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	pn, err := findPnpm(sys)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(p.Sources(), "package.json")); err != nil {
		return fmt.Errorf("no sources yet — add some with `ipm marketplace add owner/repo`")
	}
	args := []string{"install"}
	if upgrade {
		args = []string{"update", "--latest"}
	}
	root, err := sources.RootName(p.Sources())
	if err != nil {
		return err
	}
	hooks, err := writePnpmfile(p.Sources(), root)
	if err != nil {
		return err
	}
	// The hooks are what keep upstream files intact and sources' npm
	// dependencies out; pass them explicitly (pnpm 12 has no --pnpmfile flag,
	// so as config) so a user's pnpm config can't switch them off. CI=true
	// would otherwise freeze the lockfile.
	if !upgrade {
		args = append(args, "--no-frozen-lockfile") // update always rewrites the lockfile; install takes this
	}
	args = append(args, "--ignore-scripts", "--config.ignore-pnpmfile=false", "--config.pnpmfile="+hooks,
		"--store-dir", p.Store(), "--dir", p.Sources())
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
