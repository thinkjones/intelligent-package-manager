package catgraph

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	var p2skills, p2ids []string
	for i := range 30 {
		id := fmt.Sprintf("skill:o/r:p2/skills/k%02d", i)
		p2ids = append(p2ids, `"`+id+`"`)
		p2skills = append(p2skills, fmt.Sprintf(`{"id":%q,"type":"skill","name":"k%02d","repo":"o/r","plugins":["plugin:o/r:p2"]}`, id, i))
	}
	files := map[string]string{
		"repos.jsonl": `{"id":"repo:o/r","type":"repo","name":"o/r","repo":"o/r","followed":true}
{"id":"repo:x/c","type":"repo","name":"x/c","repo":"x/c","followed":false}
`,
		"marketplaces.jsonl": `{"id":"marketplace:o/r:.","type":"marketplace","name":"m","repo":"o/r","plugins":["plugin:o/r:p","plugin:o/r#e"]}
`,
		"plugins.jsonl": `{"id":"plugin:o/r#e","type":"plugin","name":"e","repo":"o/r","source_repo":"x/c","marketplaces":["marketplace:o/r:."]}
{"id":"plugin:o/r:p","type":"plugin","name":"p","repo":"o/r","skills":["skill:o/r:p/skills/s1","skill:o/r:p/skills/s2"],"hooks":["hook:o/r:p/hooks/hooks.json"],"marketplaces":["marketplace:o/r:."]}
{"id":"plugin:o/r:p2","type":"plugin","name":"p2 \"big\" [x]","repo":"o/r","skills":[` + strings.Join(p2ids, ",") + `]}
{"id":"plugin:o/r:q","type":"plugin","name":"q","repo":"o/r"}
`,
		"skills.jsonl": `{"id":"skill:o/r:p/skills/s1","type":"skill","name":"s1","repo":"o/r","plugins":["plugin:o/r:p"]}
{"id":"skill:o/r:p/skills/s2","type":"skill","name":"s2","repo":"o/r","plugins":["plugin:o/r:p"]}
{"id":"skill:o/r:s3","type":"skill","name":"s3","repo":"o/r","plugins":[]}
` + strings.Join(p2skills, "\n") + "\n",
		"hooks.jsonl":  `{"id":"hook:o/r:p/hooks/hooks.json","type":"hook","name":"p","repo":"o/r","plugins":["plugin:o/r:p"]}` + "\n",
		"github.jsonl": `{"repo":"o/r","stars":10}` + "\n",
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

func load(t *testing.T) *Graph {
	t.Helper()
	g, err := Load(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestChildrenDown(t *testing.T) {
	g := load(t)
	for id, want := range map[string][]string{
		"repo:o/r":              {"marketplace:o/r:.", "plugin:o/r:p2", "plugin:o/r:q", "skill:o/r:s3"},
		"marketplace:o/r:.":     {"plugin:o/r#e", "plugin:o/r:p"},
		"plugin:o/r:p":          {"skill:o/r:p/skills/s1", "skill:o/r:p/skills/s2", "hook:o/r:p/hooks/hooks.json"},
		"plugin:o/r#e":          {"repo:x/c"},
		"skill:o/r:p/skills/s1": nil,
	} {
		if got := g.Children(id, false); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", id, got, want)
		}
	}
}

func TestChildrenUp(t *testing.T) {
	g := load(t)
	for id, want := range map[string][]string{
		"skill:o/r:p/skills/s1": {"plugin:o/r:p"},
		"plugin:o/r:p":          {"marketplace:o/r:."},
		"plugin:o/r#e":          {"marketplace:o/r:."},
		"marketplace:o/r:.":     {"repo:o/r"},
	} {
		if got := g.Children(id, true); !reflect.DeepEqual(got, want) {
			t.Errorf("%s up: got %v want %v", id, got, want)
		}
	}
}

func TestTree(t *testing.T) {
	var b bytes.Buffer
	if err := load(t).Tree(&b, "repo:o/r", Options{Depth: 2}); err != nil {
		t.Fatal(err)
	}
	want := `repo        o/r ★ 10
├─ marketplace m
│  ├─ plugin      e
│  └─ plugin      p
├─ plugin      p2 "big" [x]
│  ├─ skill       k00
`
	if !strings.HasPrefix(b.String(), want) {
		t.Errorf("got:\n%s\nwant prefix:\n%s", b.String(), want)
	}
	if !strings.HasSuffix(b.String(), "├─ plugin      q\n└─ skill       s3\n") {
		t.Errorf("tail:\n%s", b.String())
	}
}

func TestTreeCutsAt25(t *testing.T) {
	g := load(t)
	var b bytes.Buffer
	g.Tree(&b, "plugin:o/r:p2", Options{})
	if lines := strings.Split(strings.TrimSpace(b.String()), "\n"); len(lines) != 27 || lines[26] != "└─ … 5 more" {
		t.Errorf("got %d lines, last %q", len(lines), lines[len(lines)-1])
	}
	b.Reset()
	g.Tree(&b, "plugin:o/r:p2", Options{All: true})
	if n := strings.Count(b.String(), "\n"); n != 31 {
		t.Errorf("--all: %d lines", n)
	}
}

func TestTreeUp(t *testing.T) {
	var b bytes.Buffer
	load(t).Tree(&b, "skill:o/r:p/skills/s1", Options{Up: true})
	want := "skill       s1\n└─ plugin      p\n   └─ marketplace m\n      └─ repo        o/r ★ 10\n"
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestTreeMarksCycles(t *testing.T) {
	g := load(t)
	g.down["skill:o/r:p/skills/s1"] = []string{"plugin:o/r:p"}
	var b bytes.Buffer
	g.Tree(&b, "plugin:o/r:p", Options{})
	if !strings.Contains(b.String(), "(cycle)") {
		t.Errorf("got:\n%s", b.String())
	}
}

func TestMermaid(t *testing.T) {
	var b bytes.Buffer
	if err := load(t).Mermaid(&b, "marketplace:o/r:.", Options{Depth: 1}); err != nil {
		t.Fatal(err)
	}
	want := "flowchart LR\n  n0[\"marketplace: m\"]\n  n1[\"plugin: e\"]\n  n2[\"plugin: p\"]\n  n0 --> n1\n  n0 --> n2\n"
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
	b.Reset()
	load(t).Mermaid(&b, "plugin:o/r:p2", Options{Depth: 1})
	if !strings.Contains(b.String(), `n0["plugin: p2 #quot;big#quot; [x]"]`) || !strings.Contains(b.String(), `["… 5 more"]`) {
		t.Errorf("escaping/cut:\n%s", b.String())
	}
}

func TestDOT(t *testing.T) {
	var b bytes.Buffer
	load(t).DOT(&b, "plugin:o/r:p2", Options{Depth: 1, All: true})
	out := b.String()
	if !strings.HasPrefix(out, "digraph catalog {\n  rankdir=LR;\n  n0 [label=\"plugin: p2 \\\"big\\\" [x]\"];\n") ||
		!strings.Contains(out, "  n0 -> n1;\n") || !strings.HasSuffix(out, "}\n") {
		t.Errorf("got:\n%s", out)
	}
}

func TestUnknownID(t *testing.T) {
	g := load(t)
	var b bytes.Buffer
	for _, err := range []error{g.Tree(&b, "skill:nope", Options{}), g.Mermaid(&b, "skill:nope", Options{})} {
		if err == nil || !strings.Contains(err.Error(), "ipm search") {
			t.Errorf("got %v", err)
		}
	}
	if _, err := Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "ipm index") {
		t.Errorf("missing catalog: %v", err)
	}
}
