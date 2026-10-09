// Package search turns the catalog into Typesense documents, builds the
// collection schema and query parameters, and renders results. The index is
// disposable: everything here derives from catalog/*.jsonl.
package search

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/github"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/typesense"
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
		"fields": []map[string]any{
			f("type", "string", "facet", true),
			f("name", "string"),
			f("description", "string"),
			f("tags", "string[]", "facet", true),
			f("repo", "string", "facet", true),
			f("plugin_names", "string[]"),
			f("stars", "int32", "sort", true),
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
	Limit              int
}

// Params are the Typesense search parameters for q. Hybrid unless Keyword;
// Typesense 30.2 wants one weight per query_by field, so the embedding
// field takes 0.
func (q Query) Params() url.Values {
	p := url.Values{}
	p.Set("q", q.Text)
	if q.Keyword {
		p.Set("query_by", "name,description,tags,plugin_names")
		p.Set("query_by_weights", "4,2,2,1")
	} else {
		p.Set("query_by", "name,description,tags,plugin_names,embedding")
		p.Set("query_by_weights", "4,2,2,1,0")
	}
	p.Set("exclude_fields", "embedding")
	p.Set("sort_by", "_text_match:desc,stars:desc")
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

// Table writes hits as columns, best first, every line within width, then a
// summary of matches per type and the search time.
func Table(w io.Writer, res typesense.Result, width int) {
	if len(res.Hits) == 0 {
		fmt.Fprintln(w, "no matches")
		return
	}
	header := []string{"TYPE", "NAME", "REPO", "★", "PLUGINS", "DESCRIPTION"}
	caps := []int{14, 32, 30, 6, 24}
	rows := [][]string{header}
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
		rows = append(rows, []string{str(d, "type"), str(d, "name"), str(d, "repo"), Stars(stars), plugins, str(d, "description")})
	}
	widths := make([]int, len(caps))
	for _, r := range rows {
		for i := range caps {
			r[i] = cut(r[i], caps[i])
			widths[i] = max(widths[i], utf8.RuneCountInString(r[i]))
		}
	}
	used := 0
	for _, wd := range widths {
		used += wd + 2
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		r[5] = cut(r[5], width-used)
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()
	parts := []string{fmt.Sprintf("%d match(es)", res.Found)}
	for _, f := range res.Facets["type"] {
		parts = append(parts, fmt.Sprintf("%s %d", f.Value, f.Count))
	}
	parts = append(parts, fmt.Sprintf("%d ms", res.SearchTimeMS))
	fmt.Fprintln(w, strings.Join(parts, " · "))
}
