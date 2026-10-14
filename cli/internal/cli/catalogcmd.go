package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"

	"github.com/spf13/cobra"
)

// newCatalogCmd is `ipm catalog`: alone, a summary of what is cataloged and
// searchable; with a word, the commands that build and serve the catalog.
func newCatalogCmd(sys System) *cobra.Command {
	cmd := verbs(&cobra.Command{
		Use:   "catalog",
		Short: "what's in your catalog (sync, rebuild, serve, scan, search)",
	}, "sync, rebuild, serve, scan, search")
	cmd.RunE = func(*cobra.Command, []string) error { return catalogSummary(sys) }
	cmd.AddCommand(
		newSyncCmd(sys),
		&cobra.Command{
			Use:   "rebuild",
			Short: "rebuild the catalog from your synced sources, then reload search",
			Args:  noArgs,
			RunE:  func(*cobra.Command, []string) error { return runRebuild(sys) },
		},
		newServeCmd(sys),
		newScanCmd(sys),
		newSearchCmd(sys),
	)
	return cmd
}

// searchUp reports whether the search server can take a load: Docker is
// ready, the API key exists and Typesense answers its health check.
func searchUp(sys System) bool {
	if dockerReady(sys) != nil {
		return false
	}
	c, err := tsClient(sys)
	return err == nil && c.Health() == nil
}

// loadIfUp loads search when the server is up, else says how to get there.
func loadIfUp(sys System) error {
	if searchUp(sys) {
		return runLoad(sys)
	}
	fmt.Fprintln(sys.Stdout, "- search not loaded: run `ipm catalog serve` then `ipm catalog rebuild`")
	return nil
}

// runRebuild indexes the synced sources, then loads search if it is up.
func runRebuild(sys System) error {
	if err := runIndex(sys); err != nil {
		return err
	}
	return loadIfUp(sys)
}

// catalogSummary is bare `ipm catalog`: sources, items, last index, search.
func catalogSummary(sys System) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	followed, synced, err := sources.Synced(p.Sources())
	if err != nil {
		return err
	}
	var items []string
	for _, typ := range catalog.Types {
		if recs, err := catalog.Load(p.Catalog(), typ); err == nil && len(recs) > 0 {
			items = append(items, fmt.Sprintf("%s %d", typ, len(recs)))
		}
	}
	itemsText := "none"
	if len(items) > 0 {
		itemsText = strings.Join(items, " · ")
	}
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "sources\t%d followed, %d synced\n", followed, synced)
	fmt.Fprintf(tw, "items\t%s\n", itemsText)
	fmt.Fprintf(tw, "last index\t%s\n", lastIndex(sys, p.Catalog()))
	fmt.Fprintf(tw, "search\t%s\n", searchStatus(sys))
	return tw.Flush()
}

// lastIndex is when the catalog was last written: the newest file in dir
// other than github.jsonl (which `ipm marketplace` refreshes on its own).
func lastIndex(sys System, dir string) string {
	entries, _ := os.ReadDir(dir)
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || e.Name() == "github.jsonl" {
			continue
		}
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		return "never — run `ipm catalog sync`"
	}
	return fmt.Sprintf("%s (%s)", newest.Format("2006-01-02 15:04"), ago(sys.Now().Sub(newest)))
}

// ago is d as a rough age.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// searchStatus is the summary's search row.
func searchStatus(sys System) string {
	if _, tsRow := searchHealth(sys); tsRow[0] != ok {
		return tsRow[1]
	}
	c, err := tsClient(sys)
	if err != nil {
		return err.Error()
	}
	n, err := c.NumDocuments(search.Collection)
	if err != nil {
		return tsError(err).Error()
	}
	return fmt.Sprintf("running, %d document(s)", n)
}
