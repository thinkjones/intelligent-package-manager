package copilot

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const repo = "github/awesome-copilot"

// cache lays out repo at a commit, as ipm sync does, and returns the lock.
func cache(t *testing.T, root string, files map[string]string) upstream.Lock {
	t.Helper()
	commit := strings.Repeat("c", 40)
	dir := upstream.Dir(root, repo, commit)
	write(t, filepath.Join(dir, ".git", "HEAD"), commit+"\n")
	for p, body := range files {
		write(t, filepath.Join(dir, p), body)
	}
	return upstream.Lock{repo: commit}
}

func item(path string) pack.CopilotItem {
	return pack.CopilotItem{Kind: pack.CopilotPlugin, Repo: repo, Path: path}
}

func TestExpand(t *testing.T) {
	root := t.TempDir()
	lock := cache(t, root, map[string]string{
		"plugins/team/plugin.json": `{"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
			"name": "team", "description": "A team",
			"extensions": {"com.github.awesome-copilot": {
				"agents": ["./agents/planner.md"], "skills": ["./skills/plan/", "./skills/ship/"],
				"hooks": ["./hooks/x"]}, "com.example": {}}}`,
		"plugins/solo/plugin.json": `{"name": "solo", "extensions": {"com.github.awesome-copilot": {"skills": ["./skills/plan/"]}}}`,
		"skills/plan/SKILL.md":     "---\nname: plan\n---\n",
		"skills/ship/SKILL.md":     "---\nname: ship\n---\n",
	})
	tests := []struct {
		path    string
		want    []pack.Plugin
		skipped []string
	}{
		// Skills are published upstream under the plugin's name; agents,
		// hooks and other extensions are reported, not published.
		{"plugins/team", []pack.Plugin{
			{Name: "team", Repo: repo, Path: "skills", Skills: []string{"./plan", "./ship"}, Description: "A team"},
		}, []string{"agents", "extension com.example", "hooks"}},
		{"plugins/solo", []pack.Plugin{
			{Name: "solo", Repo: repo, Path: "skills", Skills: []string{"./plan"}},
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			res, err := Expand(root, item(tt.path), lock)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(res.Plugins, tt.want) || !reflect.DeepEqual(res.Skipped, tt.skipped) {
				t.Fatalf("got %+v skipped %v\nwant %+v skipped %v", res.Plugins, res.Skipped, tt.want, tt.skipped)
			}
		})
	}

	// A version and a name override carry through.
	it := item("plugins/solo")
	it.Name, it.Version = "single", "v1"
	lock[pack.Upstream{Repo: repo, Version: "v1"}.Key()] = lock[repo]
	res, err := Expand(root, it, lock)
	if err != nil || res.Plugins[0].Name != "single" || res.Plugins[0].Version != "v1" {
		t.Fatalf("override: %+v %v", res.Plugins, err)
	}
}

func TestExpandErrors(t *testing.T) {
	root := t.TempDir()
	lock := cache(t, root, map[string]string{
		"plugins/claude/.claude-plugin/plugin.json": `{"name": "claude"}`,
		"plugins/gone/plugin.json":                  `{"name": "gone", "extensions": {"com.github.awesome-copilot": {"skills": ["./skills/missing/"]}}}`,
		"plugins/agents-only/plugin.json":           `{"name": "a", "extensions": {"com.github.awesome-copilot": {"agents": ["./agents/x.md"]}}}`,
	})
	tests := []struct{ path, want string }{
		{"plugins/claude", "list it as a [[plugin]]"},
		{"plugins/gone", "./skills/missing/ not found upstream"},
		{"plugins/agents-only", "publishes nothing: its plugin.json lists no skills"},
		{"plugins/nope", "no plugin.json found"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if _, err := Expand(root, item(tt.path), lock); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}

	if _, err := Expand(root, item("plugins/solo"), upstream.Lock{repo: strings.Repeat("e", 40)}); err == nil || !strings.Contains(err.Error(), "run `ipm sync`") {
		t.Fatalf("a cache not at the locked commit must be refused: %v", err)
	}
}

func TestSkillPaths(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "plugins/go/plugin.json"), `{"name":"go","extensions":{"com.github.awesome-copilot":{"skills":["./skills/go-sdk"],"agents":["./agents/x.md"]}}}`)
	write(t, filepath.Join(dir, "skills/go-sdk/SKILL.md"), "---\nname: go-sdk\n---\n")
	got, err := SkillPaths(dir, "plugins/go")
	if err != nil || !reflect.DeepEqual(got, []string{"skills/go-sdk"}) {
		t.Errorf("got %v %v", got, err)
	}
	if _, err := SkillPaths(dir, "nowhere"); err == nil {
		t.Error("want an error for a missing plugin.json")
	}
}
