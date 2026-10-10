// Package search turns the catalog into Typesense documents, builds the
// collection schema and query parameters, and renders results. The index is
// disposable: everything here derives from catalog/*.jsonl.
package search

import (
	"fmt"
	"io"
	"math"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/github"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/typesense"
)

const (
	// Collection is the one Typesense collection the catalog loads into.
	Collection = "artifacts"
	// Model is Typesense's built-in embedding model, run inside the server.
	Model = "ts/all-MiniLM-L12-v2"
)

// Schema is the artifacts collection: keyword fields, facets, stars for
// ordering, and an embedding Typesense computes from name, description and
// tags at import.
func Schema() map[string]any {
	f := func(name, typ string, extra ...any) map[string]any {
		m := map[string]any{"name": name, "type": typ}
		for i := 0; i+1 < len(extra); i += 2 {
			m[extra[i].(string)] = extra[i+1]
		}
		return m
	}
	return map[string]any{
		"name":                  Collection,
		"default_sorting_field": "stars",
		// Keep # and + inside words, so c#, f# and c++ are searchable
		// rather than collapsing to c and f.
		"symbols_to_index": []string{"#", "+"},
		// Split names on these too, so "cloudflare" finds cloudflare/skills
		// and "tdd" finds tdd-guard.
		"token_separators": []string{"/", "-", "_"},
		"fields": []map[string]any{
			f("type", "string", "facet", true),
			f("name", "string"),
			f("description", "string"),
			f("tags", "string[]", "facet", true),
			f("repo", "string", "facet", true),
			f("plugin_names", "string[]"),
			f("stars", "int32", "sort", true),
			f("used_by", "int32", "sort", true),
			f("external", "bool"),
			f("plugins", "string[]", "index", false, "optional", true),
			f("source_repo", "string", "index", false, "optional", true),
			f("path", "string", "index", false, "optional", true),
			f("commit", "string", "index", false, "optional", true),
			f("embedding", "float[]", "embed", map[string]any{
				"from":         []string{"name", "description", "tags"},
				"model_config": map[string]any{"model_name": Model},
			}),
		},
	}
}

// Documents is every catalog record as a search document, sorted by ID.
// Stars come from github.jsonl: the record's repo's, or for an external
// plugin, its source repo's.
func Documents(catalogDir string) ([]map[string]any, error) {
	recs := map[string][]map[string]any{}
	names := map[string]string{}
	for _, typ := range catalog.Types {
		rs, err := catalog.Load(catalogDir, typ)
		if err != nil {
			return nil, err
		}
		recs[typ] = rs
		if typ == catalog.TypePlugin || typ == catalog.TypeCopilotPlugin {
			for _, r := range rs {
				names[str(r, "id")] = str(r, "name")
			}
		}
	}
	stats, err := github.Read(filepath.Join(catalogDir, "github.jsonl"))
	if err != nil {
		return nil, err
	}
	refs := catalog.BuildRefs(recs)
	var docs []map[string]any
	for _, typ := range catalog.Types {
		for _, r := range recs[typ] {
			d := map[string]any{
				"id": str(r, "id"), "type": typ, "name": str(r, "name"), "description": str(r, "description"),
				"tags": strs(r["tags"]), "repo": str(r, "repo"), "plugin_names": []string{}, "stars": 0,
			}
			if d["name"] == "" {
				d["name"] = d["id"]
			}
			starsFrom := str(r, "repo")
			if src := str(r, "source_repo"); src != "" {
				d["source_repo"] = src
				starsFrom = src
			}
			// used_by: how many other repos reference this repo (for an
			// external entry, the repo it points at).
			d["external"] = refs.External[str(r, "id")]
			usedFrom := str(r, "repo")
			if d["external"] == true {
				usedFrom = str(r, "source_repo")
			}
			d["used_by"] = len(refs.UsedBy[catalog.NormRepo(usedFrom)])
			st, known := stats[pack.NormalizeRepo(starsFrom)]
			if known {
				d["stars"] = st.Stars
			}
			if typ == catalog.TypeRepo && known {
				d["description"] = st.Description
				d["tags"] = append([]string{}, st.Topics...)
			}
			if plugins := strs(r["plugins"]); len(plugins) > 0 {
				d["plugins"] = plugins
				var pn []string
				for _, id := range plugins {
					if n := names[id]; n != "" {
						pn = append(pn, n)
					}
				}
				if pn != nil {
					d["plugin_names"] = pn
				}
			}
			for _, k := range []string{"path", "commit"} {
				if v := str(r, k); v != "" {
					d[k] = v
				}
			}
			docs = append(docs, d)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i]["id"].(string) < docs[j]["id"].(string) })
	return docs, nil
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func strs(v any) []string {
	out := []string{}
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	}
	return out
}

// Query is one search as the CLI states it.
type Query struct {
	Text               string
	Types, Repos, Tags []string
	Keyword            bool
	// Limit is the page size; Page (1-based, 0 = first) picks the page.
	Limit, Page int
}

// Params are the Typesense search parameters for q. Hybrid unless Keyword;
// Typesense 30.2 wants one weight per query_by field, so the embedding
// field takes 0.
func (q Query) Params() url.Values {
	p := url.Values{}
	p.Set("q", q.Text)
	if q.Keyword {
		p.Set("query_by", "name,repo,plugin_names,description,tags")
		p.Set("query_by_weights", "4,3,3,2,2")
	} else {
		p.Set("query_by", "name,repo,plugin_names,description,tags,embedding")
		p.Set("query_by_weights", "4,3,3,2,2,0")
	}
	p.Set("exclude_fields", "embedding")
	p.Set("sort_by", "_text_match:desc,used_by:desc,stars:desc")
	p.Set("facet_by", "type")
	p.Set("num_typos", "2")
	limit := q.Limit
	switch {
	case limit <= 0:
		limit = 20
	case limit > 250:
		limit = 250
	}
	p.Set("per_page", strconv.Itoa(limit))
	if q.Page > 0 {
		p.Set("page", strconv.Itoa(q.Page))
	}
	var filters []string
	for _, f := range []struct {
		field  string
		values []string
	}{{"type", q.Types}, {"repo", q.Repos}, {"tags", q.Tags}} {
		var quoted []string
		for _, v := range f.values {
			// Backticks quote a value whole; one containing a backtick
			// cannot be expressed, so it is left out.
			if !strings.Contains(v, "`") {
				quoted = append(quoted, "`"+v+"`")
			}
		}
		if quoted != nil {
			filters = append(filters, f.field+":=["+strings.Join(quoted, ",")+"]")
		}
	}
	if filters != nil {
		p.Set("filter_by", strings.Join(filters, " && "))
	}
	return p
}

// Stars abbreviates a star count: 999, 31.2k, 297k, 1.2M.
func Stars(n int) string {
	trim := func(s string) string { return strings.TrimSuffix(s, ".0") }
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 100_000:
		return trim(strconv.FormatFloat(float64(n)/1000, 'f', 1, 64)) + "k"
	case n < 1_000_000:
		return strconv.Itoa((n+500)/1000) + "k"
	}
	return trim(strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64)) + "M"
}

// cut shortens s to at most n runes, ending in "…" when cut.
func cut(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// TableOptions shape the results table. Page and PerPage, when set, add
// "page P of N" to the summary; Explain adds the KW and SEM columns.
type TableOptions struct {
	Width         int
	Explain       bool
	Page, PerPage int
	// TopTextMatch is the best keyword score across all pages, so a later
	// page's SCORE is relative to the overall best; 0 uses these hits' best.
	TopTextMatch int64
}

// Score is a hit's SCORE: its hybrid rank-fusion score × 100, or for a
// keyword-only search, its text match as a percentage of the best hit's.
func Score(h typesense.Hit, top int64) string {
	switch {
	case h.Hybrid != nil:
		return strconv.Itoa(int(math.Round(h.Hybrid.RankFusionScore * 100)))
	case top > 0:
		return strconv.Itoa(int(math.Round(float64(h.TextMatch) * 100 / float64(top))))
	}
	return "—"
}

// Table writes hits as columns, best first, each with its ID on the line
// beneath, then a summary of matches per type and the search time. Rows fit
// o.Width (IDs are never cut); 0 means no limit, for output that isn't a
// terminal.
func Table(w io.Writer, res typesense.Result, o TableOptions) {
	if len(res.Hits) == 0 {
		fmt.Fprintln(w, "no matches")
		return
	}
	top := o.TopTextMatch
	for _, h := range res.Hits {
		top = max(top, h.TextMatch)
	}
	header := []string{"SCORE"}
	caps := []int{5}
	if o.Explain {
		header = append(header, "KW", "SEM")
		caps = append(caps, 2, 4)
	}
	header = append(header, "TYPE", "NAME", "REPO", "↗", "★", "PLUGINS", "DESCRIPTION")
	caps = append(caps, 14, 32, 30, 4, 6, 24)
	rows := [][]string{header}
	ids := []string{""}
	for _, h := range res.Hits {
		d := h.Document
		stars := 0
		if f, ok := d["stars"].(float64); ok {
			stars = int(f)
		}
		plugins := strings.Join(strs(d["plugin_names"]), ", ")
		if plugins == "" {
			plugins = "—"
		}
		row := []string{Score(h, top)}
		if o.Explain {
			kw, sem := "—", "—"
			if h.TextMatch > 0 {
				kw = "✓"
			}
			if h.VectorDistance != nil {
				sem = strconv.FormatFloat(1-*h.VectorDistance, 'f', 2, 64)
			}
			row = append(row, kw, sem)
		}
		name, used := str(d, "name"), ""
		if ext, _ := d["external"].(bool); ext {
			name = "↗ " + name
		}
		if n, ok := d["used_by"].(float64); ok && n > 0 {
			used = "↗" + strconv.Itoa(int(n))
		}
		rows = append(rows, append(row, str(d, "type"), name, str(d, "repo"), used, Stars(stars), plugins, str(d, "description")))
		ids = append(ids, str(d, "id"))
	}
	// Fit the terminal: columns keep their caps, the description takes what
	// is left, and when even that doesn't fit, DESCRIPTION, PLUGINS and then
	// REPO (also inside the ID) are dropped. Width 0 cuts nothing.
	n := len(header)
	desc, plugins, repo, name := n-1, n-2, n-5, n-6
	widths := make([]int, n)
	for _, r := range rows {
		for i := range n - 1 {
			if o.Width > 0 {
				r[i] = cut(r[i], caps[i])
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(r[i]))
		}
	}
	keep := make([]bool, n)
	for i := range keep {
		keep[i] = true
	}
	// others is the line length of every kept column but the description,
	// with the two-space gap after each.
	others := func() int {
		total := 0
		for i := range n - 1 {
			if keep[i] {
				total += widths[i] + 2
			}
		}
		return total
	}
	const minDesc = 20
	if o.Width > 0 {
		if others()+minDesc > o.Width {
			keep[desc] = false
		}
		for _, i := range []int{plugins, repo} {
			if !keep[desc] && others()-2 > o.Width {
				keep[i] = false
			}
		}
		if over := others() - 2 - o.Width; !keep[desc] && over > 0 {
			widths[name] = max(12, widths[name]-over)
		}
	}
	indent := 0
	for i := range n - 7 { // SCORE, and KW and SEM with --explain
		indent += widths[i] + 2
	}
	// Each hit's ID goes on its own line under TYPE, never cut, so it can be
	// copied into ipm show, tree, graph or pack add.
	for row, r := range rows {
		var line strings.Builder
		for i, cell := range r {
			if !keep[i] {
				continue
			}
			if i == desc {
				if o.Width > 0 {
					cell = cut(cell, o.Width-others())
				}
				line.WriteString(cell)
				continue
			}
			cell = cut(cell, widths[i])
			line.WriteString(cell + strings.Repeat(" ", widths[i]+2-utf8.RuneCountInString(cell)))
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
		if ids[row] != "" {
			fmt.Fprintln(w, strings.Repeat(" ", indent)+ids[row])
		}
	}
	var parts []string
	if o.Page > 0 && o.PerPage > 0 {
		parts = append(parts, fmt.Sprintf("page %d of %d", o.Page, (res.Found+o.PerPage-1)/o.PerPage))
	}
	parts = append(parts, fmt.Sprintf("%d match(es)", res.Found))
	for _, f := range res.Facets["type"] {
		parts = append(parts, fmt.Sprintf("%s %d", f.Value, f.Count))
	}
	parts = append(parts, fmt.Sprintf("%d ms", res.SearchTimeMS))
	// Wrap the summary at its separators when it is wider than the terminal.
	line := parts[0]
	for _, p := range parts[1:] {
		if o.Width > 0 && utf8.RuneCountInString(line+" · "+p) > o.Width {
			fmt.Fprintln(w, line)
			line = p
			continue
		}
		line += " · " + p
	}
	fmt.Fprintln(w, line)
}
