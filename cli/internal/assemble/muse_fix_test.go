package assemble

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A plugin with each difference Muse Code refuses, as found in real plugins.
func musePluginTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".claude-plugin/plugin.json":  `{"name":"p","skills":"./extra/","mcpServers":{"docs":{"type":"http","url":"https://x/mcp"},"local":{"command":"node","args":["s.js"]}}}`,
		".mcp.json":                   `{"playwright":{"command":"npx","args":["@playwright/mcp@latest"]},"remote":{"type":"sse","url":"https://y/sse"}}`,
		"commands/modernize.md":       "---\ndescription: Start here\nargument-hint: [system] [--source <path>]\n---\nBody\n",
		"commands/fine.md":            "---\ndescription: \"Fine\"\n---\nBody\n",
		"skills/good/SKILL.md":        "---\nname: good\ndescription: Good.\n---\nBody\n",
		"skills/good/ref.md":          "ref\n",
		"skills/bare/SKILL.md":        "# Bare\n\nNo frontmatter.\n",
		"extra/twin/SKILL.md":         "---\nname: good\ndescription: Same name.\n---\nBody\n",
		"README.md":                   "readme\n",
		"assets/skills/good/SKILL.md": "---\nname: good\ndescription: A copy the plugin doesn't load.\n---\n",
		"docs/commands/x.md":          "---\nk: [a] [b]\n---\n",
	})
	return dir
}

func TestFixForMuse(t *testing.T) {
	dir := musePluginTree(t)
	fix, err := FixForMuse(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(fix.Files[".claude-plugin/plugin.json"], &manifest); err != nil {
		t.Fatalf("manifest %s: %v", fix.Files[".claude-plugin/plugin.json"], err)
	}
	if !reflect.DeepEqual(manifest["skills"], []any{"./extra/"}) {
		t.Errorf("skills %v", manifest["skills"])
	}
	if servers := manifest["mcpServers"].(map[string]any); len(servers) != 1 || servers["local"] == nil {
		t.Errorf("manifest servers %v", servers)
	}
	var mcp map[string]map[string]any
	if err := json.Unmarshal(fix.Files[".mcp.json"], &mcp); err != nil {
		t.Fatal(err)
	}
	if len(mcp) != 1 || len(mcp["mcpServers"]) != 1 || mcp["mcpServers"]["playwright"] == nil {
		t.Errorf(".mcp.json %s", fix.Files[".mcp.json"])
	}
	if got := string(fix.Files["commands/modernize.md"]); got != "---\ndescription: Start here\nargument-hint: \"[system] [--source <path>]\"\n---\nBody\n" {
		t.Errorf("modernize.md %q", got)
	}
	for _, p := range []string{"commands/fine.md", "skills/good/SKILL.md", "README.md", "docs/commands/x.md"} {
		if _, ok := fix.Files[p]; ok {
			t.Errorf("%s rewritten", p)
		}
	}
	if want := []string{"extra/twin", "skills/bare"}; !reflect.DeepEqual(fix.Drop, want) {
		t.Errorf("drop %v", fix.Drop)
	}
	notes := strings.Join(append(append([]string{}, fix.Fixed...), fix.Dropped...), "\n")
	for _, s := range []string{
		"`skills` in .claude-plugin/plugin.json is now a list",
		"wrapped .mcp.json in `mcpServers`",
		"quoted argument-hint in commands/modernize.md",
		`left out MCP server "docs" (.claude-plugin/plugin.json): it is http, and Muse Code runs only stdio servers`,
		`left out MCP server "remote" (.mcp.json): it is sse, and Muse Code runs only stdio servers`,
		"left out skills/bare: its SKILL.md has no frontmatter",
		`left out extra/twin: skills/good already has the skill name "good"`,
	} {
		if !strings.Contains(notes, s) {
			t.Errorf("notes lack %q:\n%s", s, notes)
		}
	}
}

func TestFixForMuseLeavesAGoodPluginAlone(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".claude-plugin/plugin.json": `{"name":"p","skills":["./skills/"]}`,
		".mcp.json":                  `{"mcpServers":{"a":{"command":"x"}}}`,
		"skills/a/SKILL.md":          "---\nname: a\ndescription: A.\n---\n",
	})
	fix, err := FixForMuse(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fix.Files) != 0 || len(fix.Drop) != 0 || len(fix.Fixed) != 0 || len(fix.Dropped) != 0 {
		t.Errorf("fix %+v", fix)
	}
}

// The pack's own plugin leaves out dependencies: Muse ignores them, and ipm
// installs each whole plugin itself.
func TestFixForMuseDropsPackDependencies(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{".claude-plugin/plugin.json": `{"name":"kit","dependencies":["kit-p"]}`})
	fix, err := FixForMuse(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fix.Files[".claude-plugin/plugin.json"]); strings.Contains(got, "dependencies") || !strings.Contains(got, `"kit"`) {
		t.Errorf("manifest %s", got)
	}
	if len(fix.Fixed) != 0 {
		t.Errorf("an ipm-made change is not reported: %v", fix.Fixed)
	}
}

// A plugin with its own Muse manifest is Muse's already: only its files'
// frontmatter is checked.
func TestFixForMuseKeepsAMuseManifest(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		".muse-plugin/plugin.json":   `{"name":"p"}`,
		".claude-plugin/plugin.json": `{"name":"p","skills":"./skills/"}`,
		".mcp.json":                  `{"a":{"command":"x"}}`,
	})
	fix, err := FixForMuse(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(fix.Files) != 0 {
		t.Errorf("files %v", fix.Files)
	}
}

func TestWriteMuseCopy(t *testing.T) {
	src := musePluginTree(t)
	fix, err := FixForMuse(src, false)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "muse", "p")
	writeTree(t, dst, map[string]string{"stale.md": "old\n"})
	if err := WriteMuseCopy(src, dst, fix); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"skills/bare", "extra/twin", "stale.md"} {
		if _, err := os.Stat(filepath.Join(dst, p)); err == nil {
			t.Errorf("%s is in the copy", p)
		}
	}
	got, _ := os.ReadFile(filepath.Join(dst, "commands", "modernize.md"))
	if string(got) != string(fix.Files["commands/modernize.md"]) {
		t.Errorf("modernize.md %q", got)
	}
	a, _ := os.Stat(filepath.Join(src, "skills", "good", "ref.md"))
	b, _ := os.Stat(filepath.Join(dst, "skills", "good", "ref.md"))
	if !os.SameFile(a, b) {
		t.Error("an unchanged file is linked, not copied")
	}
	orig, _ := os.ReadFile(filepath.Join(src, "commands", "modernize.md"))
	if strings.Contains(string(orig), `"[system]`) {
		t.Error("the source was edited")
	}
}
