package catalog

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRecordJSON(t *testing.T) {
	r := Record{ID: "skill:o/r:skills/a", Type: TypeSkill, Name: "a", Description: "x < y",
		Tags: []string{"b", "a", "b"}, Repo: "o/r", Commit: "c1", Path: "skills/a",
		Meta: map[string]any{"name": "a"}, ContentHash: "h", Fields: map[string]any{"plugins": []string(nil)}}
	got, err := r.MarshalJSON()
	want := `{"commit":"c1","content_hash":"h","description":"x < y","id":"skill:o/r:skills/a","meta":{"name":"a"},"name":"a","path":"skills/a","plugins":[],"repo":"o/r","tags":["a","b"],"type":"skill"}`
	if err != nil || string(got) != want {
		t.Fatalf("got %s (%v)\nwant %s", got, err, want)
	}
}

func TestRecordOmitsEmptyOptionalFields(t *testing.T) {
	r := Record{ID: "plugin:o/r#c", Type: TypePlugin, Name: "c", Repo: "o/r", Fields: map[string]any{"source_repo": "x/c"}}
	got, err := r.MarshalJSON()
	want := `{"description":"","id":"plugin:o/r#c","name":"c","repo":"o/r","source_repo":"x/c","tags":[],"type":"plugin"}`
	if err != nil || string(got) != want {
		t.Fatalf("got %s (%v)\nwant %s", got, err, want)
	}
}

func TestWriteSortsAndWritesEveryFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "github.jsonl"), []byte("keep\n"), 0o644)
	recs := []Record{
		{ID: "skill:o/r:b", Type: TypeSkill, Name: "b", Repo: "o/r", Description: "x < y"},
		{ID: "skill:o/r:a", Type: TypeSkill, Name: "a", Repo: "o/r"},
	}
	if err := Write(dir, recs); err != nil {
		t.Fatal(err)
	}
	for _, name := range Files {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not written: %v", name, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, Files[TypeSkill]))
	lines := strings.Split(string(data), "\n")
	if len(lines) != 3 || lines[2] != "" || !strings.Contains(lines[0], `"skill:o/r:a"`) || !strings.Contains(lines[1], "x < y") {
		t.Errorf("skills.jsonl:\n%s", data)
	}
	if agents, _ := os.ReadFile(filepath.Join(dir, Files[TypeAgent])); len(agents) != 0 {
		t.Errorf("agents.jsonl should be empty, got %q", agents)
	}
	if gh, _ := os.ReadFile(filepath.Join(dir, "github.jsonl")); string(gh) != "keep\n" {
		t.Errorf("github.jsonl changed: %q", gh)
	}
	loaded, err := Load(dir, TypeSkill)
	if err != nil || len(loaded) != 2 || loaded[0]["id"] != "skill:o/r:a" {
		t.Errorf("Load: %v %v", loaded, err)
	}
	if _, err := Load(t.TempDir(), TypeSkill); err == nil || !strings.Contains(err.Error(), "aihs index") {
		t.Errorf("Load of a missing file: %v", err)
	}
}

func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWalk(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "skills/a/SKILL.md", "docs/b/SKILL.md", "agents/r.md", "agents/sub/deep.md", "agents/README.md",
		"commands/go.md", "hooks/hooks.json", ".mcp.json", "p/.claude-plugin/plugin.json",
		"c/plugin.json", "c/.codex-plugin/plugin.json", ".claude-plugin/marketplace.json",
		"node_modules/x/SKILL.md", ".git/y/SKILL.md")
	got, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	want := Found{
		Skills: []string{"docs/b", "skills/a"}, Plugins: []string{"p"}, CopilotPlugins: []string{"c"}, Marketplaces: []string{"."},
		Agents: []string{"agents/r.md", "agents/sub/deep.md"}, Commands: []string{"commands/go.md"},
		HookFiles: []string{"hooks/hooks.json"}, MCPFiles: []string{".mcp.json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestWalkDoesNotFollowSymlinkedDirs(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "skills/a/SKILL.md")
	if err := os.Symlink("..", filepath.Join(root, "skills", "loop")); err != nil {
		t.Fatal(err)
	}
	got, err := Walk(root)
	if err != nil || !reflect.DeepEqual(got.Skills, []string{"skills/a"}) {
		t.Errorf("got %v %v", got.Skills, err)
	}
}

func TestWriteEncodesEverythingBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "repos.jsonl"), []byte("old\n"), 0o644)
	recs := []Record{
		{ID: "repo:o/r", Type: TypeRepo, Repo: "o/r"},
		{ID: "skill:o/r:a", Type: TypeSkill, Repo: "o/r", Meta: map[string]any{"x": math.NaN()}},
	}
	if err := Write(dir, recs); err == nil {
		t.Fatal("want an encoding error")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "repos.jsonl")); string(data) != "old\n" {
		t.Errorf("repos.jsonl rewritten despite the error: %q", data)
	}
}
