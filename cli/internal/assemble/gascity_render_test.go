package assemble

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

var update = flag.Bool("update", false, "rewrite testdata/gascity")

const (
	rendSkill  = "skill:o/r:skills/code-review"
	rendAgent  = "agent:o/r:agents/rev.md"
	rendNotes  = "mcp:o/r:.mcp.json#notes"
	rendWeb    = "mcp:o/r:.mcp.json#web"
	rendAgentM = "---\nname: Reviewer\ndescription: Reviews code\nmodel: sonnet\ntools: Read, Grep\ncolor: red\n---\nReview {{ this }}\n"
)

func writeFile(t *testing.T, root, rel, data string, mode fs.FileMode) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
}

func lockItem(id, typ, path string) project.LockItem {
	return project.LockItem{ID: id, Type: typ, Repo: "o/r", Path: path, SHA: shaA}
}

func rendEntry(kind GasKind, name, from, server string, it project.LockItem) GasEntry {
	return GasEntry{Kind: kind, Name: name, Alias: "o--r", From: from, Server: server, Item: it}
}

// rendRepo is a node_modules with a skill, an agent and an MCP file.
func rendRepo(t *testing.T, mcp string) (string, GascityPlan) {
	t.Helper()
	nm := t.TempDir()
	repo := filepath.Join(nm, "o--r")
	writeFile(t, repo, "skills/code-review/SKILL.md", "---\nname: code-review\n---\nReview.\n", 0o644)
	writeFile(t, repo, "skills/code-review/scripts/run.sh", "#!/bin/sh\necho hi\n", 0o755)
	writeFile(t, repo, "agents/rev.md", rendAgentM, 0o644)
	writeFile(t, repo, ".mcp.json", mcp, 0o644)
	skill, agent := lockItem(rendSkill, "skill", "skills/code-review"), lockItem(rendAgent, "agent", "agents/rev.md")
	notes, web := lockItem(rendNotes, "mcp", ".mcp.json"), lockItem(rendWeb, "mcp", ".mcp.json")
	return nm, GascityPlan{
		Name: "kit", Description: "My kit",
		Picked: []project.LockItem{skill, agent},
		Entries: []GasEntry{
			rendEntry(GasSkill, "code-review", "skills/code-review", "", skill),
			rendEntry(GasAgent, "reviewer", "agents/rev.md", "", agent),
			rendEntry(GasMCP, "notes", ".mcp.json", "notes", notes),
			rendEntry(GasMCP, "web", ".mcp.json", "web", web),
		},
	}
}

const rendMCP = `{"notes": {"command": "node", "args": ["a.js"], "env": {"K": "v"}, "type": "stdio"},
 "web": {"type": "http", "url": "https://x.test/mcp", "headers": {"Authorization": "Bearer t"}}}`

func TestRenderGascityGolden(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	files, warns, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"agents/reviewer: dropped color, tools — Gas City agents have no equivalent"}; !reflect.DeepEqual(warns, want) {
		t.Errorf("warnings %q", warns)
	}
	if files[0].Path != "pack.toml" {
		t.Errorf("first file %s", files[0].Path)
	}
	golden := filepath.Join("testdata", "gascity", "kit")
	if *update {
		if err := os.RemoveAll(golden); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			writeFile(t, golden, f.Path, string(f.Data), f.Mode)
		}
	}
	got := map[string]GasFile{}
	for _, f := range files {
		got[f.Path] = f
	}
	err = filepath.WalkDir(golden, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(golden, p)
		rel = filepath.ToSlash(rel)
		f, ok := got[rel]
		if !ok {
			t.Errorf("%s is not rendered", rel)
			return nil
		}
		delete(got, rel)
		want, _ := os.ReadFile(p)
		info, _ := d.Info()
		if string(f.Data) != string(want) {
			t.Errorf("%s:\n%s\nwant:\n%s", rel, f.Data, want)
		}
		if f.Mode != info.Mode().Perm() {
			t.Errorf("%s mode %v want %v", rel, f.Mode, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("%v (run go test -run RenderGascityGolden -update)", err)
	}
	for rel := range got {
		t.Errorf("%s is not in the golden tree", rel)
	}
	for _, f := range files {
		if f.Path == "skills/code-review/scripts/run.sh" && f.Mode != 0o755 {
			t.Errorf("run.sh mode %v", f.Mode)
		}
	}
}

func TestRenderGascityDeterministic(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	a, wa, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	b, wb, _ := RenderGascity(plan, folders(nm))
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(wa, wb) {
		t.Error("two renders differ")
	}
}

func TestRenderGascityPluginRoot(t *testing.T) {
	nm, plan := rendRepo(t, `{"notes": {"command": "node", "args": ["${CLAUDE_PLUGIN_ROOT}/server.js"], "cwd": "/x"}, "web": {"url": "https://x.test"}}`)
	_, warns, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"mcp/notes: uses ${CLAUDE_PLUGIN_ROOT}, which Gas City does not set", "mcp/notes: dropped cwd"} {
		if !strings.Contains(strings.Join(warns, "\n"), w) {
			t.Errorf("missing %q in %q", w, warns)
		}
	}
}

func TestRenderGascityPluginJSONServers(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	writeFile(t, filepath.Join(nm, "o--r"), "plugins/tools/.claude-plugin/plugin.json", `{"name": "tools", "mcpServers": {"notes": {"command": "run"}}}`, 0o644)
	plan.Entries = plan.Entries[2:3]
	plan.Entries[0].From = "plugins/tools/.claude-plugin/plugin.json"
	files, _, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "mcp/notes.toml" && !strings.Contains(string(f.Data), `command = "run"`) {
			t.Errorf("%s", f.Data)
			return
		}
	}
}

func TestRenderGascityModelInherit(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	writeFile(t, filepath.Join(nm, "o--r"), "agents/rev.md", "---\nname: R\nmodel: inherit\n---\nbody\n", 0o644)
	files, _, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "agents/reviewer/agent.toml" && strings.Contains(string(f.Data), "option_defaults") {
			t.Errorf("%s", f.Data)
		}
	}
}

func TestRenderGascitySymlinkEscape(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	outside := filepath.Join(t.TempDir(), "secret")
	writeFile(t, filepath.Dir(outside), "secret", "x", 0o644)
	if err := os.Symlink(outside, filepath.Join(nm, "o--r", "skills/code-review/leak")); err != nil {
		t.Fatal(err)
	}
	// Walk the resolved skill folder: the link is skipped, not followed.
	files, _, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "skills/code-review/leak" {
			t.Error("the symlink was rendered")
		}
	}
	// A skill folder that is itself a link out is refused.
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(nm, "o--r", "skills/evil")); err != nil {
		t.Fatal(err)
	}
	plan.Entries[0].From = "skills/evil"
	_, _, err = RenderGascity(plan, folders(nm))
	if err == nil || !strings.Contains(err.Error(), "leads outside its repo") {
		t.Errorf("got %v", err)
	}
}

func TestRenderGascityBadServer(t *testing.T) {
	nm, plan := rendRepo(t, `{"notes": {"args": ["a"]}, "web": {"url": "https://x.test"}}`)
	_, _, err := RenderGascity(plan, folders(nm))
	if err == nil || !strings.Contains(err.Error(), "notes") || !strings.Contains(err.Error(), rendNotes) {
		t.Errorf("got %v", err)
	}
	plan.Entries[2].Server = "missing"
	_, _, err = RenderGascity(plan, folders(nm))
	if err == nil || !strings.Contains(err.Error(), rendNotes) {
		t.Errorf("got %v", err)
	}
}

func TestRenderGascityMCPServersArray(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	writeFile(t, filepath.Join(nm, "o--r"), "plugins/tools/.claude-plugin/plugin.json", `{"name": "tools", "mcpServers": [{"other": {"command": "x"}}, {"notes": {"command": "run"}}]}`, 0o644)
	plan.Entries = plan.Entries[2:3]
	plan.Entries[0].From = "plugins/tools/.claude-plugin/plugin.json"
	files, _, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == "mcp/notes.toml" && strings.Contains(string(f.Data), `command = "run"`) {
			return
		}
	}
	t.Errorf("no mcp/notes.toml with the command in %d files", len(files))
}

func TestRenderGascityBadAgentFrontmatter(t *testing.T) {
	nm, plan := rendRepo(t, rendMCP)
	writeFile(t, filepath.Join(nm, "o--r"), "agents/rev.md", "---\nname: [oops\n---\nDo the thing.\n", 0o644)
	files, warns, err := RenderGascity(plan, folders(nm))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.Path] = string(f.Data)
	}
	if got["agents/reviewer/prompt.md"] != "Do the thing.\n" {
		t.Errorf("prompt %q", got["agents/reviewer/prompt.md"])
	}
	if got["agents/reviewer/agent.toml"] != "provider = \"claude\"\n" {
		t.Errorf("agent.toml %q", got["agents/reviewer/agent.toml"])
	}
	if !strings.Contains(strings.Join(warns, "\n"), "agents/reviewer: frontmatter is not valid YAML — wrote the body only") {
		t.Errorf("warns %q", warns)
	}
}

func TestGascityPlanRepos(t *testing.T) {
	_, plan := rendRepo(t, rendMCP)
	if got := plan.Repos(); !reflect.DeepEqual(got, map[string]string{"o--r": "github:o/r#" + shaA}) {
		t.Errorf("repos %v", got)
	}
}
