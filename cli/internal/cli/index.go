package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

// runIndex writes the catalog from every synced source at its pinned commit.
// It reads only what pnpm installed, and writes nothing unless every source
// is installed and pinned.
func runIndex(sys System) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	srcs, err := sources.Checkouts(p.Sources())
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		return fmt.Errorf("no sources to index — add some with `ipm marketplace add owner/repo`, then `ipm catalog sync`")
	}
	cos := make([]catalog.Checkout, 0, len(srcs))
	for _, s := range srcs {
		cos = append(cos, catalog.Checkout{Repo: s.Repo, Commit: s.Commit, Dir: s.Dir})
	}
	recs, warnings, err := catalog.Build(cos)
	if err != nil {
		return err
	}
	if err := catalog.Write(p.Catalog(), recs); err != nil {
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
