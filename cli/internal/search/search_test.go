package search

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/typesense"
)

// fixture writes a small catalog and returns its dir.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"repos.jsonl": `{"id":"repo:o/r","type":"repo","name":"o/r","repo":"o/r","followed":true}` + "\n" +
			`{"id":"repo:x/c","type":"repo","name":"x/c","repo":"x/c","followed":true}` + "\n",
		"plugins.jsonl": `{"id":"plugin:o/r#c","type":"plugin","name":"c","repo":"o/r","source_repo":"x/c","description":"ext"}` + "\n" +
			`{"id":"plugin:o/r:p","type":"plugin","name":"tools","repo":"o/r","path":"p","commit":"c1"}` + "\n",
		"skills.jsonl": `{"id":"skill:o/r:p/skills/s","type":"skill","name":"s","description":"Test first","tags":["testing"],"repo":"o/r","path":"p/skills/s","commit":"c1","plugins":["plugin:o/r:p"]}` + "\n",
		"github.jsonl": `{"repo":"o/r","stars":10,"description":"D","topics":["t"]}` + "\n" + `{"repo":"x/c","stars":99,"description":"","topics":[]}` + "\n",
	}
	for _, typ := range catalog.Types {
		if _, ok := files[catalog.Files[typ]]; !ok {
			files[catalog.Files[typ]] = ""
		}
	}
	for name, body := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
	}
	return dir
}

func TestSchema(t *testing.T) {
	s := Schema()
	if !reflect.DeepEqual(s["symbols_to_index"], []string{"#", "+"}) {
		t.Errorf("c#, f#, c++ must stay whole tokens: %v", s["symbols_to_index"])
	}
	if s["name"] != Collection || s["default_sorting_field"] != "stars" {
		t.Errorf("schema %v", s)
	}
	fields := map[string]map[string]any{}
	for _, f := range s["fields"].([]map[string]any) {
		fields[f["name"].(string)] = f
	}
	emb := fields["embedding"]
	embed := emb["embed"].(map[string]any)
	if emb["type"] != "float[]" || !reflect.DeepEqual(embed["from"], []string{"name", "description", "tags"}) ||
		embed["model_config"].(map[string]any)["model_name"] != Model {
		t.Errorf("embedding %v", emb)
	}
	for _, f := range []string{"type", "tags", "repo"} {
		if fields[f]["facet"] != true {
			t.Errorf("%s not a facet", f)
		}
	}
	if fields["stars"]["type"] != "int32" || fields["plugins"]["index"] != false {
		t.Errorf("stars/plugins %v %v", fields["stars"], fields["plugins"])
	}
}

func TestDocuments(t *testing.T) {
	docs, err := Documents(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]map[string]any{}
	var ids []string
	for _, d := range docs {
		by[d["id"].(string)] = d
		ids = append(ids, d["id"].(string))
		if _, ok := d["embedding"]; ok {
			t.Error("documents must not carry embeddings")
		}
	}
	if !sortedStrings(ids) {
		t.Errorf("not sorted: %v", ids)
	}
	s := by["skill:o/r:p/skills/s"]
	if s["stars"] != 10 || !reflect.DeepEqual(s["plugin_names"], []string{"tools"}) || !reflect.DeepEqual(s["plugins"], []string{"plugin:o/r:p"}) {
		t.Errorf("skill doc %v", s)
	}
	if e := by["plugin:o/r#c"]; e["stars"] != 99 || e["source_repo"] != "x/c" {
		t.Errorf("external plugin doc %v", e)
	}
	if r := by["repo:o/r"]; r["description"] != "D" || !reflect.DeepEqual(r["tags"], []string{"t"}) || r["stars"] != 10 {
		t.Errorf("repo doc %v", r)
	}
	if _, err := Documents(t.TempDir()); err == nil || !strings.Contains(err.Error(), "ipm catalog rebuild") {
		t.Errorf("missing catalog: %v", err)
	}
}

func TestParamsHybridAndKeyword(t *testing.T) {
	p := Query{Text: "c# testing"}.Params()
	for k, want := range map[string]string{
		"q": "c# testing", "query_by": "name,repo,plugin_names,description,tags,embedding", "query_by_weights": "4,3,3,2,2,0",
		"exclude_fields": "embedding", "sort_by": "_text_match:desc,used_by:desc,stars:desc", "facet_by": "type", "num_typos": "2", "per_page": "20",
	} {
		if p.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, p.Get(k), want)
		}
	}
	k := Query{Text: "x", Keyword: true}.Params()
	if k.Get("query_by") != "name,repo,plugin_names,description,tags" || k.Get("query_by_weights") != "4,3,3,2,2" {
		t.Errorf("keyword %v", k)
	}
	if p.Has("filter_by") {
		t.Error("no filters → no filter_by")
	}
}

func TestParamsFiltersAreQuoted(t *testing.T) {
	p := Query{Text: "*", Types: []string{"skill", "agent"}, Repos: []string{"o/r.x-y"}, Tags: []string{"a,b && c"}}.Params()
	want := "type:=[`skill`,`agent`] && repo:=[`o/r.x-y`] && tags:=[`a,b && c`]"
	if p.Get("filter_by") != want {
		t.Errorf("got  %s\nwant %s", p.Get("filter_by"), want)
	}
}

func TestParamsLimitCapped(t *testing.T) {
	if (Query{Limit: 999}).Params().Get("per_page") != "250" || (Query{Limit: 5}).Params().Get("per_page") != "5" {
		t.Error("limit not capped or not passed")
	}
}

func TestStars(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1k", 31234: "31.2k", 296686: "297k", 1234567: "1.2M"} {
		if got := Stars(n); got != want {
			t.Errorf("Stars(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTable(t *testing.T) {
	res := typesense.Result{Found: 41, SearchTimeMS: 7,
		Facets: map[string][]typesense.FacetCount{"type": {{Value: "skill", Count: 30}, {Value: "agent", Count: 11}}},
		Hits: []typesense.Hit{
			{Document: map[string]any{"type": "skill", "name": "tdd", "repo": "o/r", "stars": float64(31234), "plugin_names": []any{"tools"},
				"description": strings.Repeat("very long description ", 20)}},
			{Document: map[string]any{"type": "agent", "name": "reviewer", "repo": "x/y", "stars": float64(0), "plugin_names": []any{}, "description": "Reviews"}},
		}}
	var b bytes.Buffer
	Table(&b, res, TableOptions{Width: 120})
	out := b.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[0], "SCORE  TYPE") || !strings.Contains(lines[0], "DESCRIPTION") || !strings.Contains(lines[1], "31.2k") || !strings.Contains(lines[2], "—") {
		t.Errorf("table:\n%s", out)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, " ") && utf8.RuneCountInString(l) > 120 { // ID lines are never cut
			t.Errorf("line longer than 120: %q", l)
		}
	}
	if lines[len(lines)-1] != "41 match(es) · skill 30 · agent 11 · 7 ms" {
		t.Errorf("summary %q", lines[len(lines)-1])
	}
	b.Reset()
	Table(&b, typesense.Result{}, TableOptions{Width: 120})
	if strings.TrimSpace(b.String()) != "no matches" {
		t.Errorf("zero hits: %q", b.String())
	}
}

func sortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] < s[i-1] {
			return false
		}
	}
	return true
}

func TestParamsPage(t *testing.T) {
	if got := (Query{Text: "x", Page: 3}).Params().Get("page"); got != "3" {
		t.Errorf("page %q", got)
	}
	if (Query{Text: "x"}).Params().Has("page") {
		t.Error("no page unless asked")
	}
}

func hit(name string, textMatch int64, fusion, dist float64) typesense.Hit {
	h := typesense.Hit{Document: map[string]any{"type": "skill", "name": name, "repo": "o/r", "stars": float64(1)}, TextMatch: textMatch}
	if fusion >= 0 {
		h.Hybrid = &typesense.HybridInfo{RankFusionScore: fusion}
	}
	if dist >= 0 {
		h.VectorDistance = &dist
	}
	return h
}

func TestTableScores(t *testing.T) {
	var b bytes.Buffer
	res := typesense.Result{Found: 2, Hits: []typesense.Hit{hit("a", 200, 0.713, 0.55), hit("b", 0, 0.65, 0.6)}}
	Table(&b, res, TableOptions{Width: 120})
	lines := strings.Split(b.String(), "\n")
	if !strings.HasPrefix(lines[0], "SCORE  TYPE") || !strings.HasPrefix(lines[1], "71 ") || !strings.HasPrefix(lines[2], "65 ") {
		t.Errorf("hybrid scores:\n%s", b.String())
	}
	b.Reset()
	Table(&b, res, TableOptions{Width: 120, Explain: true})
	lines = strings.Split(b.String(), "\n")
	if !strings.HasPrefix(lines[0], "SCORE  KW  SEM ") || !strings.Contains(lines[1], "✓") || !strings.Contains(lines[1], "0.45") || !strings.Contains(lines[2], "—") {
		t.Errorf("explain:\n%s", b.String())
	}
	b.Reset()
	kw := typesense.Result{Found: 2, Hits: []typesense.Hit{hit("a", 200, -1, -1), hit("b", 100, -1, -1)}}
	Table(&b, kw, TableOptions{Width: 120})
	lines = strings.Split(b.String(), "\n")
	if !strings.HasPrefix(lines[1], "100") || !strings.HasPrefix(lines[2], "50 ") {
		t.Errorf("keyword scores:\n%s", b.String())
	}
}

func TestTablePageLabel(t *testing.T) {
	var b bytes.Buffer
	Table(&b, typesense.Result{Found: 45, SearchTimeMS: 2, Hits: []typesense.Hit{hit("a", 1, 0.5, 0.5)}}, TableOptions{Width: 120, Page: 2, PerPage: 20})
	if !strings.Contains(b.String(), "page 2 of 3 · 45 match(es)") {
		t.Errorf("got:\n%s", b.String())
	}
}

func TestTableKeywordScoreAgainstOverallTop(t *testing.T) {
	var b bytes.Buffer
	Table(&b, typesense.Result{Found: 9, Hits: []typesense.Hit{hit("a", 200, -1, -1)}}, TableOptions{Width: 120, TopTextMatch: 400})
	if !strings.HasPrefix(strings.Split(b.String(), "\n")[1], "50 ") {
		t.Errorf("got:\n%s", b.String())
	}
}

func TestTableShowsEachHitsID(t *testing.T) {
	long := "skill:o/r:" + strings.Repeat("deep/", 30) + "tdd"
	res := typesense.Result{Found: 2, Hits: []typesense.Hit{
		{Document: map[string]any{"id": long, "type": "skill", "name": "tdd", "repo": "o/r", "stars": float64(1), "description": "Test first"}},
		{Document: map[string]any{"id": "agent:x/y:agents/rev.md", "type": "agent", "name": "reviewer", "repo": "x/y", "stars": float64(2), "description": "Reviews"}},
	}}
	for _, explain := range []bool{false, true} {
		var b bytes.Buffer
		Table(&b, res, TableOptions{Width: 120, Explain: explain})
		lines := strings.Split(b.String(), "\n")
		typeCol := strings.Index(lines[0], "TYPE")
		for i, id := range []string{long, "agent:x/y:agents/rev.md"} {
			row, idLine := lines[1+2*i], lines[2+2*i]
			if idLine != strings.Repeat(" ", typeCol)+id {
				t.Errorf("explain=%v: want the full ID under TYPE, got %q", explain, idLine)
			}
			if r := []rune(row); len(r) < typeCol || !strings.HasPrefix(string(r[typeCol:]), res.Hits[i].Document["type"].(string)) {
				t.Errorf("explain=%v: rows lost their alignment:\n%s", explain, b.String())
			}
		}
	}
}

func wideHits() typesense.Result {
	return typesense.Result{Found: 100, SearchTimeMS: 18, Facets: map[string][]typesense.FacetCount{"type": {
		{Value: "skill", Count: 44}, {Value: "agent", Count: 33}, {Value: "plugin", Count: 11}, {Value: "command", Count: 8}, {Value: "copilot-plugin", Count: 3}, {Value: "repo", Count: 1}}},
		Hits: []typesense.Hit{{Document: map[string]any{
			"id": "copilot-plugin:github/awesome-copilot:plugins/csharp-dotnet-development", "type": "copilot-plugin",
			"name": "csharp-dotnet-development", "repo": "github/awesome-copilot", "stars": float64(39900),
			"plugin_names": []any{"dotnet-contribution"}, "description": strings.Repeat("Essential prompts and skills. ", 8)}}}}
}

func TestTableFitsTheWidth(t *testing.T) {
	for _, tc := range []struct {
		width int
		gone  []string
	}{
		{160, nil},
		{110, []string{"DESCRIPTION"}},
		{90, []string{"DESCRIPTION", "PLUGINS"}},
		{70, []string{"DESCRIPTION", "PLUGINS", "REPO"}},
	} {
		var b bytes.Buffer
		Table(&b, wideHits(), TableOptions{Width: tc.width})
		lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
		for _, col := range []string{"DESCRIPTION", "PLUGINS", "REPO"} {
			if strings.Contains(lines[0], col) == slices.Contains(tc.gone, col) {
				t.Errorf("width %d: %s shown=%v, want dropped=%v\n%s", tc.width, col, strings.Contains(lines[0], col), slices.Contains(tc.gone, col), b.String())
			}
		}
		for _, l := range lines {
			if !strings.HasPrefix(l, " ") && utf8.RuneCountInString(l) > tc.width {
				t.Errorf("width %d: line of %d: %q", tc.width, utf8.RuneCountInString(l), l)
			}
		}
		if !strings.Contains(b.String(), "\n       copilot-plugin:github/awesome-copilot:plugins/csharp-dotnet-development\n") {
			t.Errorf("width %d: the ID must stay whole:\n%s", tc.width, b.String())
		}
	}
}

func TestTableWiderShowsMoreAndZeroCutsNothing(t *testing.T) {
	desc := func(width int) string {
		var b bytes.Buffer
		Table(&b, wideHits(), TableOptions{Width: width})
		row := strings.Split(b.String(), "\n")[1]
		return row[strings.Index(row, "Essential"):]
	}
	if len(desc(200)) <= len(desc(130)) {
		t.Error("a wider terminal should show more of the description")
	}
	if full := desc(0); strings.Contains(full, "…") || !strings.HasSuffix(full, "skills.") {
		t.Errorf("width 0 (not a terminal) must not cut: %q", full)
	}
}

func TestScoreExported(t *testing.T) {
	if got := Score(hit("x", 0, 0.42, -1), 0); got != "42" {
		t.Errorf("Score = %q", got)
	}
}

func TestSchemaReferences(t *testing.T) {
	s := Schema()
	fields := map[string]map[string]any{}
	for _, f := range s["fields"].([]map[string]any) {
		fields[f["name"].(string)] = f
	}
	if fields["used_by"]["type"] != "int32" || fields["used_by"]["sort"] != true || fields["external"]["type"] != "bool" {
		t.Errorf("used_by %v external %v", fields["used_by"], fields["external"])
	}
	if !reflect.DeepEqual(s["token_separators"], []string{"/", "-", "_"}) {
		t.Errorf("token_separators %v", s["token_separators"])
	}
}

func TestDocumentsUsedBy(t *testing.T) {
	docs, err := Documents(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		id := d["id"].(string)
		want, ext := 0, false
		switch id {
		case "repo:x/c":
			want = 1
		case "plugin:o/r#c":
			want, ext = 1, true
		}
		if d["used_by"] != want || d["external"] != ext {
			t.Errorf("%s: used_by %v external %v, want %d %v", id, d["used_by"], d["external"], want, ext)
		}
	}
}

func TestTableUsedByColumn(t *testing.T) {
	res := typesense.Result{Found: 2, Hits: []typesense.Hit{
		{Document: map[string]any{"id": "skill:b/s:x", "type": "skill", "name": "agents-sdk", "repo": "b/s", "stars": float64(40), "used_by": float64(3)}},
		{Document: map[string]any{"id": "plugin:a/m#cf", "type": "plugin", "name": "cloudflare", "repo": "a/m", "stars": float64(9), "external": true}},
	}}
	var b bytes.Buffer
	Table(&b, res, TableOptions{Width: 160})
	out := b.String()
	if !strings.Contains(out, "↗3") || !strings.Contains(out, "↗ cloudflare") {
		t.Errorf("table:\n%s", out)
	}
}
