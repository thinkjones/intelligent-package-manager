package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"

	"github.com/spf13/cobra"
)

func newSearchCmd(sys System) *cobra.Command {
	var q search.Query
	var types, repos, tags []string
	var asJSON, all, explain, ids bool
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "hybrid search over the catalog (needs ipm serve and ipm load)",
		RunE: func(_ *cobra.Command, args []string) error {
			q.Text = strings.TrimSpace(strings.Join(args, " "))
			if q.Text == "" {
				return usagef("search: give a query (or * for everything). Try `ipm help search`.")
			}
			for _, v := range slices.Concat(types, repos, tags) {
				if strings.Contains(v, "`") {
					return usagef("search: filter value %q contains a backtick, which Typesense filters cannot express", v)
				}
			}
			if ids && asJSON {
				return usagef("search: --ids and --json don't combine")
			}
			if all && q.Page > 0 {
				return usagef("search: --all and --page don't combine")
			}
			q.Types, q.Repos, q.Tags = types, repos, tags
			return runSearch(sys, q, all, asJSON, explain, ids)
		},
	}
	f := cmd.Flags()
	f.StringArrayVar(&types, "type", nil, "only this artifact type (repeatable): repo, marketplace, plugin, copilot-plugin, skill, agent, command, hook, mcp")
	f.StringArrayVar(&repos, "repo", nil, "only artifacts from this repo (repeatable)")
	f.StringArrayVar(&tags, "tag", nil, "only artifacts with this tag or topic (repeatable)")
	f.BoolVar(&q.Keyword, "keyword", false, "keyword ranking only, without semantic matching")
	f.IntVar(&q.Limit, "limit", 20, "results to show (max 250)")
	f.IntVar(&q.Page, "page", 0, "page of results to show, --limit per page")
	f.BoolVar(&all, "all", false, "every result, fetched 250 at a time (hybrid: keyword matches plus ~100 semantic neighbours)")
	f.BoolVar(&explain, "explain", false, "add KW (keyword match ✓) and SEM (semantic similarity) columns")
	f.BoolVar(&ids, "ids", false, "print only each hit's ID, one per line")
	f.BoolVar(&asJSON, "json", false, "print each hit as a JSON line")
	return cmd
}

// outputWidth is the width to fit tables to: the terminal's, else $COLUMNS,
// else 0 (not a terminal: cut nothing).
func outputWidth(sys System) int {
	if sys.TermWidth != nil {
		if w := sys.TermWidth(); w > 0 {
			return w
		}
	}
	if w, err := strconv.Atoi(sys.Getenv("COLUMNS")); err == nil && w > 0 {
		return w
	}
	return 0
}

// maxPage is Typesense's largest page size.
const maxPage = 250

func runSearch(sys System, q search.Query, all, asJSON, explain, ids bool) error {
	c, err := tsClient(sys)
	if err != nil {
		return err
	}
	if all {
		q.Limit, q.Page = maxPage, 1
	}
	res, err := c.Search(search.Collection, q.Params())
	if err != nil {
		return tsError(err)
	}
	for all && len(res.Hits) < res.Found && q.Page*maxPage < res.Found {
		q.Page++
		more, err := c.Search(search.Collection, q.Params())
		if err != nil {
			return tsError(err)
		}
		if len(more.Hits) == 0 {
			break
		}
		res.Hits = append(res.Hits, more.Hits...)
		res.SearchTimeMS += more.SearchTimeMS
	}
	if ids {
		for _, h := range res.Hits {
			if id, _ := h.Document["id"].(string); id != "" {
				fmt.Fprintln(sys.Stdout, id)
			}
		}
		return nil
	}
	if asJSON {
		enc := json.NewEncoder(sys.Stdout)
		enc.SetEscapeHTML(false)
		for _, h := range res.Hits {
			if err := enc.Encode(h); err != nil {
				return err
			}
		}
		return nil
	}
	o := search.TableOptions{Width: outputWidth(sys), Explain: explain}
	if q.Keyword && q.Page > 1 {
		first := q
		first.Page, first.Limit = 1, 1
		if best, err := c.Search(search.Collection, first.Params()); err == nil && len(best.Hits) > 0 {
			o.TopTextMatch = best.Hits[0].TextMatch
		}
	}
	if per, _ := strconv.Atoi(q.Params().Get("per_page")); !all && res.Found > per {
		o.Page, o.PerPage = max(q.Page, 1), per
	}
	search.Table(sys.Stdout, res, o)
	return nil
}
