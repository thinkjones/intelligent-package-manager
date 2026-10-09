package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDir(t *testing.T) {
	root := t.TempDir()
	touch(t, root,
		".claude-plugin/marketplace.json",
		"plugins/a/.claude-plugin/plugin.json",
		"plugins/a/skills/one/SKILL.md",
		"plugins/a/skills/two/SKILL.md",
		"plugins/a/skills/not-a-skill/notes.md",
		"plugins/a/skills/group/three/SKILL.md",
		"plugins/a/skills/group/three/agents/openai.yaml",
		"plugins/a/skills/inner/skills/four/SKILL.md",
		"plugins/a/hooks/hooks.json",
		"plugins/a/agents/reviewer.md",
		"plugins/a/agents/README.md",
		"copilot/b/plugin.json",
		"copilot/b/.codex-plugin/plugin.json",
		"node_modules/x/skills/three/SKILL.md",
		".git/skills/four/SKILL.md",
	)
	finds, err := Dir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Find{
		{Plugin, "plugins/a", 0},
		{CopilotPlugin, "copilot/b", 0},
		{Marketplace, ".", 0},
		{Skills, "plugins/a/skills", 3},
		{Skills, "plugins/a/skills/inner/skills", 1},
		{Hooks, "plugins/a/hooks", 1},
		{Agents, "plugins/a/agents", 1},
	}
	if len(finds) != len(want) {
		t.Fatalf("got %v, want %v", finds, want)
	}
	for i := range want {
		if finds[i] != want[i] {
			t.Errorf("find %d: got %v, want %v", i, finds[i], want[i])
		}
	}
}
