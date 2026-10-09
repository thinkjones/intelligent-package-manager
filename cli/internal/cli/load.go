package cli

import (
	"flag"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/search"
)

// importBatch is how many documents go in one import request.
const importBatch = 500

func loadFlags() (*flag.FlagSet, func(System) error) {
	fs := flag.NewFlagSet("load", flag.ContinueOnError)
	return fs, runLoad
}

// runLoad rebuilds the search index from catalog/: drop, create, import.
func runLoad(sys System) error {
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	docs, err := search.Documents(p.Catalog())
	if err != nil {
		return err
	}
	c, err := tsClient(sys)
	if err != nil {
		return err
	}
	start := sys.Now()
	if err := c.DeleteCollection(search.Collection); err != nil {
		return tsError(err)
	}
	if err := c.CreateCollection(search.Schema()); err != nil {
		return tsError(err)
	}
	fmt.Fprintf(sys.Stdout, "importing %d document(s) — the first load embeds each one, so it takes a minute or two\n", len(docs))
	counts := map[string]int{}
	failed := 0
	for i := 0; i < len(docs); i += importBatch {
		batch := docs[i:min(i+importBatch, len(docs))]
		results, err := c.Import(search.Collection, batch)
		if err != nil {
			return tsError(err)
		}
		for j, d := range batch {
			if j < len(results) && !results[j].Success {
				failed++
				fmt.Fprintf(sys.Stdout, "%s %s: %s\n", bad, d["id"], results[j].Error)
				continue
			}
			counts[d["type"].(string)]++
		}
	}
	tw := tabwriter.NewWriter(sys.Stdout, 0, 0, 2, ' ', 0)
	for _, typ := range catalog.Types {
		fmt.Fprintf(tw, "%s\t%d\n", typ, counts[typ])
	}
	tw.Flush()
	fmt.Fprintf(sys.Stdout, "\n%d imported, %d failed in %s — collection %q\n",
		len(docs)-failed, failed, sys.Now().Sub(start).Round(time.Second), search.Collection)
	if failed > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}
