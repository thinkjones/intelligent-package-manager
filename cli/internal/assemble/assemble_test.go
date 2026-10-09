package assemble

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/marketplace"
	"github.com/thinkjones/fs-skills/cli/internal/project"
)

var shaA, shaB = strings.Repeat("a", 40), strings.Repeat("b", 40)

// fixtureCatalog writes a catalog with skills from two repos sharing a name,
// an agent, a command, a local plugin, an inline plugin, an external plugin
// and a copilot plugin.
func fixtureCatalog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		catalog.TypeSkill: `{"id":"skill:o/r:skills/code-review","type":"skill","name":"code-review","repo":"o/r","path":"skills/code-review","commit":"` + shaA + `"}
{"id":"skill:mattpocock/skills:skills/code-review","type":"skill","name":"code-review","repo":"mattpocock/skills","path":"skills/code-review","commit":"` + shaB + `"}
{"id":"skill:o/r:skills/go","type":"skill","name":"Go Expert","repo":"o/r","path":"skills/go","commit":"` + shaA + `"}`,
		catalog.TypeAgent:   `{"id":"agent:o/r:agents/rev.md","type":"agent","name":"reviewer","repo":"o/r","path":"agents/rev.md","commit":"` + shaA + `"}`,
		catalog.TypeCommand: `{"id":"command:o/r:commands/ship.md","type":"command","name":"ship","repo":"o/r","path":"commands/ship.md","commit":"` + shaA + `"}`,
		catalog.TypePlugin: `{"id":"plugin:o/r:plugins/tools","type":"plugin","name":"tools","repo":"o/r","path":"plugins/tools","commit":"` + shaA + `"}
{"id":"plugin:o/r:plugins/lsp#lsp","type":"plugin","name":"lsp","repo":"o/r","path":"plugins/lsp","commit":"` + shaA + `","entry":{"name":"lsp","source":"./plugins/lsp","strict":false,"lspServers":{"x":{"command":"x"}}}}
{"id":"plugin:o/r#stripe","type":"plugin","name":"stripe","repo":"o/r","source_repo":"stripe/ai","entry":{"name":"stripe","description":"Stripe","source":{"source":"git-subdir","url":"https://github.com/stripe/ai.git","path":"p","sha":"` + shaB + `"}}}`,
		catalog.TypeCopilotPlugin: `{"id":"copilot-plugin:o/r:plugins/go","type":"copilot-plugin","name":"go","repo":"o/r","path":"plugins/go","commit":"` + shaA + `","skills":["skill:o/r:skills/go"]}`,
	}
	for _, typ := range catalog.Types {
		body := files[typ]
		if body != "" {
			body += "\n"
		}
		os.WriteFile(filepath.Join(dir, catalog.Files[typ]), []byte(body), 0o644)
	}
	return dir
}

func TestResolveFromLockThenCatalog(t *testing.T) {
	cat := fixtureCatalog(t)
	locked := project.LockItem{ID: "agent:o/r:agents/rev.md", Type: "agent", Name: "reviewer-old", Repo: "o/r", Path: "agents/rev.md", SHA: shaB}
	items, err := Resolve([]project.Pack{{Name: "p", Items: []string{"agent:o/r:agents/rev.md", "skill:o/r:skills/code-review", "copilot-plugin:o/r:plugins/go", "plugin:o/r#stripe"}}},
		map[string]project.LockItem{locked.ID: locked}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if items["agent:o/r:agents/rev.md"] != locked {
		t.Errorf("the lock should win: %+v", items["agent:o/r:agents/rev.md"])
	}
	if s := items["skill:o/r:skills/code-review"]; s.SHA != shaA || s.Path != "skills/code-review" || s.Name != "code-review" {
		t.Errorf("catalog item %+v", s)
	}
	if _, ok := items["skill:o/r:skills/go"]; !ok {
		t.Error("a copilot plugin's skills must be resolved too")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(items["plugin:o/r#stripe"].Source), &entry); err != nil || entry["name"] != "stripe" {
		t.Errorf("external plugin source %q %v", items["plugin:o/r#stripe"].Source, err)
	}
}

func TestResolveUnknownItem(t *testing.T) {
	_, err := Resolve([]project.Pack{{Name: "kit", Items: []string{"skill:gone"}}}, nil, fixtureCatalog(t))
	if err == nil || !strings.Contains(err.Error(), "skill:gone") || !strings.Contains(err.Error(), "ipm pack remove kit") {
		t.Errorf("got %v", err)
	}
}

func resolved(t *testing.T, ids ...string) ([]project.Pack, map[string]project.LockItem) {
	t.Helper()
	packs := []project.Pack{{Name: "kit", Description: "My kit", Items: ids}}
	items, err := Resolve(packs, nil, fixtureCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	return packs, items
}

func TestBuildNamesAndClashes(t *testing.T) {
	packs, items := resolved(t, "skill:o/r:skills/code-review", "skill:mattpocock/skills:skills/code-review", "skill:o/r:skills/go", "agent:o/r:agents/rev.md", "command:o/r:commands/ship.md")
	plan, err := Build(packs, items)
	if err != nil {
		t.Fatal(err)
	}
	var to []string
	for _, f := range plan.Packs[0].Files {
		to = append(to, f.To)
	}
	want := []string{"skills/code-review", "skills/code-review-mattpocock", "skills/go-expert", "agents/reviewer.md", "commands/ship.md"}
	if !reflect.DeepEqual(to, want) {
		t.Errorf("got  %v\nwant %v", to, want)
	}
	if plan.Repos["o--r"] != "github:o/r#"+shaA || plan.Repos["mattpocock--skills"] != "github:mattpocock/skills#"+shaB || len(plan.Repos) != 2 {
		t.Errorf("repos %v", plan.Repos)
	}
}

func TestBuildWholePlugins(t *testing.T) {
	packs, items := resolved(t, "plugin:o/r:plugins/tools", "plugin:o/r:plugins/lsp#lsp", "plugin:o/r#stripe")
	plan, err := Build(packs, items)
	if err != nil {
		t.Fatal(err)
	}
	deps := plan.Packs[0].Deps
	if len(deps) != 3 || len(plan.Repos) != 0 {
		t.Fatalf("deps %v repos %v", deps, plan.Repos)
	}
	tools := deps[0].(marketplace.Entry)
	if tools.Name != "kit-tools" || tools.Source != (marketplace.Source{Source: "git-subdir", URL: "o/r", Path: "plugins/tools", SHA: shaA}) || !reflect.DeepEqual(tools.Tags, []string{marketplace.OwnerTag, "kit"}) {
		t.Errorf("local plugin %+v", tools)
	}
	lsp := deps[1].(map[string]any)
	if lsp["name"] != "kit-lsp" || lsp["lspServers"] == nil || !reflect.DeepEqual(lsp["source"], marketplace.Source{Source: "git-subdir", URL: "o/r", Path: "plugins/lsp", SHA: shaA}) {
		t.Errorf("inline plugin %v", lsp)
	}
	stripe := deps[2].(map[string]any)
	src, _ := stripe["source"].(map[string]any)
	if stripe["name"] != "kit-stripe" || src["sha"] != shaB || !reflect.DeepEqual(stripe["tags"], []string{marketplace.OwnerTag, "kit"}) {
		t.Errorf("external plugin %v", stripe)
	}
	if !reflect.DeepEqual(plan.Packs[0].DepNames(), []string{"kit-tools", "kit-lsp", "kit-stripe"}) {
		t.Errorf("dep names %v", plan.Packs[0].DepNames())
	}
}

func TestBuildExpandsCopilotPlugin(t *testing.T) {
	packs, items := resolved(t, "copilot-plugin:o/r:plugins/go")
	plan, _ := Build(packs, items)
	if f := plan.Packs[0].Files; len(f) != 1 || f[0].To != "skills/go-expert" || f[0].From != "skills/go" {
		t.Errorf("files %+v", f)
	}
}

func nodeModules(t *testing.T) string {
	t.Helper()
	nm := t.TempDir()
	for rel, body := range map[string]string{
		"o--r/skills/code-review/SKILL.md":               "s",
		"o--r/skills/code-review/refs/x.md":              "x",
		"o--r/agents/rev.md":                             "a",
		"mattpocock--skills/skills/code-review/SKILL.md": "m",
	} {
		p := filepath.Join(nm, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o644)
	}
	return nm
}

func TestPlaceLinksElseCopies(t *testing.T) {
	packs, items := resolved(t, "skill:o/r:skills/code-review", "agent:o/r:agents/rev.md")
	plan, _ := Build(packs, items)
	nm := nodeModules(t)
	for name, link := range map[string]func(string, string) error{
		"link": os.Link,
		"copy": func(string, string) error { return errors.New("cross-device link") },
	} {
		dir := filepath.Join(t.TempDir(), "kit")
		if err := Place(plan.Packs[0], nm, dir, link); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for rel, want := range map[string]string{"skills/code-review/SKILL.md": "s", "skills/code-review/refs/x.md": "x", "agents/reviewer.md": "a"} {
			if got, err := os.ReadFile(filepath.Join(dir, rel)); err != nil || string(got) != want {
				t.Errorf("%s %s: %q %v", name, rel, got, err)
			}
		}
	}
}

func TestPlaceRemovesStaleFiles(t *testing.T) {
	packs, items := resolved(t, "agent:o/r:agents/rev.md")
	plan, _ := Build(packs, items)
	dir := filepath.Join(t.TempDir(), "kit")
	os.MkdirAll(filepath.Join(dir, "skills", "old"), 0o755)
	os.WriteFile(filepath.Join(dir, "skills", "old", "SKILL.md"), []byte("old"), 0o644)
	if err := Place(plan.Packs[0], nodeModules(t), dir, os.Link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "old")); !os.IsNotExist(err) {
		t.Error("files of removed items must go")
	}
}
