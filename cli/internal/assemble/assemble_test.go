package assemble

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/marketplace"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
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
		catalog.TypePlugin: `{"id":"plugin:o/r:plugins/tools","type":"plugin","name":"tools","repo":"o/r","path":"plugins/tools","commit":"` + shaA + `","skills":["skill:o/r:plugins/tools/skills/a"],"agents":["agent:o/r:plugins/tools/agents/b.md"],"mcp":["mcp:o/r:plugins/tools/.mcp.json#notes"],"hooks":["hook:o/r:plugins/tools/hooks/hooks.json#PreToolUse"]}
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
	if !reflect.DeepEqual(items["agent:o/r:agents/rev.md"], locked) {
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
	if len(deps) != 3 || len(plan.Repos) != 1 {
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

// folders maps each repo folder in dir (named by alias) to its path, as the
// build's repo lookup does.
func folders(dir string) map[string]string {
	out := map[string]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		out[e.Name()] = filepath.Join(dir, e.Name())
	}
	return out
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
		if err := Place(plan.Packs[0], folders(nm), dir, link); err != nil {
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
	if err := Place(plan.Packs[0], folders(nodeModules(t)), dir, os.Link); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "old")); !os.IsNotExist(err) {
		t.Error("files of removed items must go")
	}
}

func lockItems(items ...project.LockItem) map[string]project.LockItem {
	m := map[string]project.LockItem{}
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

func TestResolveRefusesUnsafeLockItems(t *testing.T) {
	for name, it := range map[string]project.LockItem{
		"path escapes":  {ID: "agent:o/r:x", Type: "agent", Name: "x", Repo: "o/r", Path: "../../etc/x.md", SHA: shaA},
		"absolute path": {ID: "agent:o/r:x", Type: "agent", Name: "x", Repo: "o/r", Path: "/etc/x.md", SHA: shaA},
		"bad sha":       {ID: "agent:o/r:x", Type: "agent", Name: "x", Repo: "o/r", Path: "agents/x.md", SHA: "main"},
		"bad source":    {ID: "plugin:o/r#x", Type: "plugin", Name: "x", Repo: "o/r", Source: "null"},
	} {
		_, err := Resolve([]project.Pack{{Name: "kit", Items: []string{it.ID}}}, lockItems(it), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), it.ID) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestBuildRefusesUnpickableAndEmptyNames(t *testing.T) {
	for name, it := range map[string]project.LockItem{
		"hook":       {ID: "hook:o/r:h.json", Type: "hook", Name: "h", Repo: "o/r", Path: "h.json", SHA: shaA},
		"empty name": {ID: "agent:o/r:x", Type: "agent", Name: "!!!", Repo: "o/r", Path: "agents/!!!.md", SHA: shaA},
	} {
		if _, err := Build([]project.Pack{{Name: "kit", Items: []string{it.ID}}}, lockItems(it)); err == nil || !strings.Contains(err.Error(), it.ID) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestBuildRefusesEntryNameCollisions(t *testing.T) {
	a := project.LockItem{ID: "plugin:o/r:a/p", Type: "plugin", Name: "p", Repo: "o/r", Path: "a/p", SHA: shaA}
	b := project.LockItem{ID: "plugin:o/r:b/p", Type: "plugin", Name: "p", Repo: "o/r", Path: "b/p", SHA: shaA}
	_, err := Build([]project.Pack{{Name: "a", Items: []string{a.ID, b.ID}}}, lockItems(a, b))
	if err == nil || !strings.Contains(err.Error(), "a-p") {
		t.Errorf("same pack: %v", err)
	}
	_, err = Build([]project.Pack{{Name: "a", Items: []string{a.ID}}, {Name: "a-p"}}, lockItems(a))
	if err == nil || !strings.Contains(err.Error(), "a-p") {
		t.Errorf("pack named like an entry: %v", err)
	}
}

func TestBuildWarnsOnUnpinnedExternalPlugin(t *testing.T) {
	ext := project.LockItem{ID: "plugin:o/r#x", Type: "plugin", Name: "x", Repo: "o/r", Source: `{"name":"x","source":{"source":"github","repo":"x/x"}}`}
	plan, err := Build([]project.Pack{{Name: "kit", Items: []string{ext.ID}}}, lockItems(ext))
	if err != nil || len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], ext.ID) {
		t.Errorf("warnings %v %v", plan.Warnings, err)
	}
}

func TestBuildDedupesWithinPack(t *testing.T) {
	packs, items := resolved(t, "skill:o/r:skills/go", "copilot-plugin:o/r:plugins/go")
	plan, _ := Build(packs, items)
	if n := len(plan.Packs[0].Files); n != 1 {
		t.Errorf("a skill picked twice is placed %d times", n)
	}
}

func TestPlaceRefusesEscapes(t *testing.T) {
	nm := nodeModules(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	os.WriteFile(outside, []byte("SECRET"), 0o644)
	os.Symlink(outside, filepath.Join(nm, "o--r", "agents", "link.md"))
	for name, from := range map[string]string{"dotdot": "../../" + filepath.Base(filepath.Dir(outside)) + "/secret.md", "symlinked file": "agents/link.md"} {
		pp := PackPlan{Name: "kit", Files: []Placed{{Alias: "o--r", From: from, To: "agents/x.md", Item: project.LockItem{ID: "agent:o/r:" + from}}}}
		if err := Place(pp, folders(nm), filepath.Join(t.TempDir(), "kit"), os.Link); err == nil {
			t.Errorf("%s: escaped the repo", name)
		}
	}
}

func TestPlaceNeedsEveryRepo(t *testing.T) {
	packs, items := resolved(t, "agent:o/r:agents/rev.md")
	plan, _ := Build(packs, items)
	err := Place(plan.Packs[0], map[string]string{}, filepath.Join(t.TempDir(), "kit"), os.Link)
	if err == nil || !strings.Contains(err.Error(), "o/r") {
		t.Errorf("got %v", err)
	}
}

func TestPlanReposPinSpecs(t *testing.T) {
	packs, items := resolved(t, "agent:o/r:agents/rev.md")
	plan, _ := Build(packs, items)
	it := items["agent:o/r:agents/rev.md"]
	if want := map[string]string{"o--r": "github:o/r#" + it.SHA}; !reflect.DeepEqual(plan.Repos, want) || PinSpec(it) != want["o--r"] {
		t.Errorf("repos %v", plan.Repos)
	}
	if got := PinSpec(project.LockItem{Repo: "https://git.test/o/r", SHA: it.SHA}); got != "git+https://git.test/o/r#"+it.SHA {
		t.Errorf("git spec %q", got)
	}
}

func TestPlaceCopyKeepsMode(t *testing.T) {
	nm := nodeModules(t)
	script := filepath.Join(nm, "o--r", "skills", "code-review", "run.sh")
	os.WriteFile(script, []byte("#!/bin/sh"), 0o755)
	packs, items := resolved(t, "skill:o/r:skills/code-review")
	plan, _ := Build(packs, items)
	dir := filepath.Join(t.TempDir(), "kit")
	Place(plan.Packs[0], folders(nm), dir, func(string, string) error { return errors.New("EXDEV") })
	if info, err := os.Stat(filepath.Join(dir, "skills", "code-review", "run.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("mode %v %v", info, err)
	}
}

var toolsParts = []string{
	"agent:o/r:plugins/tools/agents/b.md",
	"hook:o/r:plugins/tools/hooks/hooks.json#PreToolUse",
	"mcp:o/r:plugins/tools/.mcp.json#notes",
	"skill:o/r:plugins/tools/skills/a",
}

func TestResolveRecordsPluginParts(t *testing.T) {
	_, items := resolved(t, "plugin:o/r:plugins/tools", "plugin:o/r#stripe")
	if got := items["plugin:o/r:plugins/tools"].Parts; !reflect.DeepEqual(got, toolsParts) {
		t.Errorf("parts %v", got)
	}
	if got := items["plugin:o/r#stripe"].Parts; got != nil {
		t.Errorf("external plugin parts %v", got)
	}
}

func TestResolveFillsPartsForOldLock(t *testing.T) {
	for _, tc := range []struct {
		sha   string
		parts []string
	}{
		{shaA, toolsParts}, // catalog is at the locked commit
		{shaB, nil},        // catalog moved on: its parts may not exist at the pin
	} {
		locked := project.LockItem{ID: "plugin:o/r:plugins/tools", Type: "plugin", Name: "tools", Repo: "o/r", Path: "plugins/tools", SHA: tc.sha}
		items, err := Resolve([]project.Pack{{Name: "p", Items: []string{locked.ID}}}, map[string]project.LockItem{locked.ID: locked}, fixtureCatalog(t))
		if err != nil {
			t.Fatal(err)
		}
		got := items[locked.ID]
		if got.SHA != tc.sha || len(got.Parts) != len(tc.parts) || (tc.parts != nil && !reflect.DeepEqual(got.Parts, tc.parts)) {
			t.Errorf("sha %s: got %+v", tc.sha[:4], got)
		}
	}
}

func TestResolveRefusesForeignParts(t *testing.T) {
	locked := project.LockItem{ID: "plugin:o/r:plugins/tools", Type: "plugin", Name: "tools", Repo: "o/r", Path: "plugins/tools", SHA: shaA, Parts: []string{"skill:x/y:skills/a"}}
	_, err := Resolve([]project.Pack{{Name: "p", Items: []string{locked.ID}}}, map[string]project.LockItem{locked.ID: locked}, fixtureCatalog(t))
	if err == nil || !strings.Contains(err.Error(), "skill:x/y:skills/a") {
		t.Errorf("got %v", err)
	}
}

func TestBuildFetchesWholePluginRepo(t *testing.T) {
	packs, items := resolved(t, "plugin:o/r:plugins/tools")
	plan, err := Build(packs, items)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Repos["o--r"] != "github:o/r#"+shaA || len(plan.Repos) != 1 {
		t.Errorf("repos %v", plan.Repos)
	}
	packs, items = resolved(t, "plugin:o/r#stripe")
	plan, _ = Build(packs, items)
	if len(plan.Repos) != 0 {
		t.Errorf("external plugin repos %v", plan.Repos)
	}
}

func TestBuildClashRule(t *testing.T) {
	sk := func(repo string) project.LockItem {
		id := "skill:" + repo + ":skills/x"
		return project.LockItem{ID: id, Type: "skill", Name: "x", Repo: repo, Path: "skills/x", SHA: shaA}
	}
	a, b, c := sk("o/a"), sk("o/b"), sk("o/c")
	plan, err := Build([]project.Pack{{Name: "kit", Items: []string{a.ID, b.ID, c.ID}}}, map[string]project.LockItem{a.ID: a, b.ID: b, c.ID: c})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range plan.Packs[0].Files {
		got = append(got, f.To)
	}
	if want := []string{"skills/x", "skills/x-o", "skills/x-2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestResolveRecordsEmptyPartsAsNonNil(t *testing.T) {
	cat := fixtureCatalog(t)
	f, err := os.OpenFile(filepath.Join(cat, catalog.Files[catalog.TypePlugin]), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"id":"plugin:o/r:plugins/bare","type":"plugin","name":"bare","repo":"o/r","path":"plugins/bare","commit":"` + shaA + `"}` + "\n")
	f.Close()
	id := "plugin:o/r:plugins/bare"
	packs := []project.Pack{{Name: "p", Items: []string{id}}}
	items, err := Resolve(packs, nil, cat)
	if err != nil || items[id].Parts == nil || len(items[id].Parts) != 0 {
		t.Fatalf("fresh: %#v %v", items[id].Parts, err)
	}
	// An old lock (parts not recorded) at the catalog's commit is filled with "none".
	old := project.LockItem{ID: id, Type: "plugin", Name: "bare", Repo: "o/r", Path: "plugins/bare", SHA: shaA}
	items, err = Resolve(packs, map[string]project.LockItem{id: old}, cat)
	if err != nil || items[id].Parts == nil {
		t.Fatalf("old lock: %#v %v", items[id].Parts, err)
	}
	// A lock that records none stays recorded.
	old.Parts = []string{}
	items, err = Resolve(packs, map[string]project.LockItem{id: old}, cat)
	if err != nil || items[id].Parts == nil {
		t.Fatalf("recorded none: %#v %v", items[id].Parts, err)
	}
}
