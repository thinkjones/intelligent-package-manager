package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
)

func searchFlags() (*flag.FlagSet, func(System) error) {
	var q search.Query
	var types, repos, tags multi
	var asJSON, all, explain, ids bool
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.Var(&types, "type", "only this artifact type (repeatable): repo, marketplace, plugin, copilot-plugin, skill, agent, command, hook, mcp")
	fs.Var(&repos, "repo", "only artifacts from this repo (repeatable)")
	fs.Var(&tags, "tag", "only artifacts with this tag or topic (repeatable)")
	fs.BoolVar(&q.Keyword, "keyword", false, "keyword ranking only, without semantic matching")
	fs.IntVar(&q.Limit, "limit", 20, "results to show (max 250)")
	fs.IntVar(&q.Page, "page", 0, "page of results to show, --limit per page")
	fs.BoolVar(&all, "all", false, "every result, fetched 250 at a time (hybrid: keyword matches plus ~100 semantic neighbours)")
	fs.BoolVar(&explain, "explain", false, "add KW (keyword match ✓) and SEM (semantic similarity) columns")
	fs.BoolVar(&asJSON, "json", false, "print each hit as a JSON line")
	fs.BoolVar(&ids, "ids", false, "print only each hit's ID, one per line")
	return fs, func(sys System) error {
		q.Text = strings.TrimSpace(strings.Join(fs.Args(), " "))
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
	}
}

// interspersed lets flags follow positional words (`search c# --type skill
// tests`); everything after "--" is a word, even if it starts with "-".
func interspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	rest, tail := args, []string(nil)
	if i := slices.Index(args, "--"); i >= 0 {
		rest, tail = args[:i], args[i+1:]
	}
	var words []string
	for len(rest) > 0 {
		if strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
			if err := fs.Parse(rest); err != nil {
				return nil, err
			}
			rest = fs.Args()
			continue
		}
		words = append(words, rest[0])
		rest = rest[1:]
	}
	return append(words, tail...), nil
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
