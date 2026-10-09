package pack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "eng")
	write(t, filepath.Join(dir, ManifestFile), `
[pack]
description = "Engineering"
version = "1.2.0"
includes = ["base"]

[[plugin]]
name = "cflr"
repo = "cloudflare/skills"
path = "skills"
version = "v1.0.0"
skills = ["./"]

[[plugin]]
name = "obsidian"
repo = "kepano/obsidian-skills"

[[copilot-plugin]]
repo = "github/awesome-copilot"
path = "plugins/arize-ax"
`)
	write(t, filepath.Join(dir, ".DS_Store"), "")

	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "eng" || p.Meta.Version != "1.2.0" || !reflect.DeepEqual(p.Meta.Includes, []string{"base"}) || len(p.Plugins) != 2 {
		t.Fatalf("pack %+v", p)
	}
	if got := p.Plugins[0]; got.Path != "skills" || got.Version != "v1.0.0" || !reflect.DeepEqual(got.Skills, []string{"./"}) {
		t.Fatalf("plugin[0] %+v", got)
	}
	if c := p.Copilot(); len(c) != 1 || c[0].Kind != CopilotPlugin {
		t.Fatalf("copilot items %+v", c)
	}
	ups := Upstreams([]Pack{p})
	keys := []string{}
	for _, u := range ups {
		keys = append(keys, u.Key())
	}
	if want := []string{"cloudflare/skills@v1.0.0", "github/awesome-copilot", "kepano/obsidian-skills"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("upstreams %v, want %v", keys, want)
	}
}

func TestPackHoldsOnlyItsManifest(t *testing.T) {
	for _, extra := range []string{"skills/x/SKILL.md", "plugins/x/agents/a.md", "README.md"} {
		dir := filepath.Join(t.TempDir(), "p")
		write(t, filepath.Join(dir, ManifestFile), "")
		write(t, filepath.Join(dir, extra), "x")
		if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "a pack holds only manifest.toml") {
			t.Errorf("%s: got %v", extra, err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{"unknown key", "[[plugin]]\nname = \"x\"\nrepo = \"o/r\"\nsource = \"typo\"\n", "unknown key(s) plugin.source"},
		{"missing name", "[[plugin]]\nrepo = \"o/r\"\n", "`name` is required"},
		{"missing repo", "[[plugin]]\nname = \"x\"\n", "`repo` is required"},
		{"bad name", "[[plugin]]\nname = \"a b\"\nrepo = \"o/r\"\n", "may use only"},
		{"reserved name", "[[plugin]]\nname = \"claude-code-setup\"\nrepo = \"o/r\"\n", "reserved"},
		{"bad skill path", "[[plugin]]\nname = \"x\"\nrepo = \"o/r\"\nskills = [\"skills/x\"]\n", "must start with ./"},
		{"retired key", "[[plugin]]\nname = \"x\"\nrepo = \"o/r\"\nsha = \"abc\"\n", "unknown key(s) plugin.sha"},
		{"retired table", "[[instruction]]\nrepo = \"o/r\"\npath = \"x.instructions.md\"\n", "unknown key(s) instruction"},
		{"copilot item without path", "[[copilot-plugin]]\nrepo = \"o/r\"\n", "`path` is required"},
		{"retired agent table", "[[agent]]\nrepo = \"o/r\"\npath = \"a.agent.md\"\n", "unknown key(s) agent"},
		{"not toml", "[[plugin", "manifest.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, ManifestFile), tt.body)
			_, err := Load(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadAll(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "one", ManifestFile), "[[plugin]]\nname = \"x\"\nrepo = \"o/a\"\n")
	write(t, filepath.Join(root, "empty", ManifestFile), "")
	write(t, filepath.Join(root, "docs", "README.md"), "not a pack")

	packs, err := LoadAll(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 2 || packs[0].Name != "empty" || !packs[0].Empty() || packs[1].Empty() {
		t.Fatalf("packs %+v", packs)
	}
	if _, err := LoadAll(root, []string{"nope"}); err == nil || !strings.Contains(err.Error(), `no pack "nope"`) {
		t.Fatalf("got %v", err)
	}
	named, err := LoadAll(root, []string{"one"})
	if err != nil || len(named) != 1 || named[0].Name != "one" {
		t.Fatalf("named %+v, %v", named, err)
	}
}

func TestProblems(t *testing.T) {
	cflr := Plugin{Name: "cflr", Repo: "cloudflare/skills", Path: "skills", Skills: []string{"./"}, Description: "one wording"}
	reworded := cflr
	reworded.Description = "another wording"
	pinned := cflr
	pinned.Version = "v2"
	tests := []struct {
		name  string
		packs []Pack
		want  []string
	}{
		{"same plugin in two packs, reworded", []Pack{
			{Name: "a", Plugins: []Plugin{cflr}},
			{Name: "b", Plugins: []Plugin{reworded}},
		}, nil},
		{"same name, different definition", []Pack{
			{Name: "a", Plugins: []Plugin{cflr}},
			{Name: "b", Plugins: []Plugin{pinned}},
		}, []string{`plugin "cflr" is defined differently in a and b`}},
		{"plugin named like a pack", []Pack{
			{Name: "a", Plugins: []Plugin{{Name: "b", Repo: "o/r"}}},
			{Name: "b"},
		}, []string{`plugin "b" in a has the same name as a pack`}},
		{"include of a missing pack", []Pack{
			{Name: "a", Meta: Meta{Includes: []string{"nope"}}},
		}, []string{`a includes "nope", which is not a pack being published`}},
		{"include cycle", []Pack{
			{Name: "a", Meta: Meta{Includes: []string{"b"}}},
			{Name: "b", Meta: Meta{Includes: []string{"a"}}},
		}, []string{"includes form a cycle: a → b → a"}},
		{"self include", []Pack{
			{Name: "a", Meta: Meta{Includes: []string{"a"}}},
		}, []string{"includes form a cycle: a → a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Problems(tt.packs); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReserved(t *testing.T) {
	for name, want := range map[string]bool{
		"claude-code-setup":     true,
		"Claude_MD-Management":  true,
		"anthropics-skills":     true,
		"claude":                true,
		"claude-mods":           true,
		"cc-plugin-x":           true,
		"tools-official-claude": true,
		"skill-creator":         false,
		"claudemd-management":   false,
		"skills-anthropics":     false, // only a warning in Claude Code
		"cc-tools":              false,
	} {
		if got := Reserved(name); got != want {
			t.Errorf("Reserved(%q) = %v, want %v", name, got, want)
		}
	}
}
