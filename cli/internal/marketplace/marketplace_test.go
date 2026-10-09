package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

func TestSourceFor(t *testing.T) {
	commit := strings.Repeat("a", 40)
	tests := []struct {
		name string
		in   pack.Plugin
		want Source
	}{
		{"whole github repo, default branch", pack.Plugin{Repo: "kepano/obsidian-skills"},
			Source{Source: "github", Repo: "kepano/obsidian-skills", SHA: commit}},
		{"subdirectory at a tag", pack.Plugin{Repo: "cloudflare/skills", Path: "skills", Version: "v1.2.0"},
			Source{Source: "git-subdir", URL: "cloudflare/skills", Path: "skills", Ref: "v1.2.0", SHA: commit}},
		{"version is a commit: no ref", pack.Plugin{Repo: "o/r", Version: commit},
			Source{Source: "github", Repo: "o/r", SHA: commit}},
		{"non-github git url", pack.Plugin{Repo: "https://gitlab.example.com/g/p.git"},
			Source{Source: "url", URL: "https://gitlab.example.com/g/p.git", SHA: commit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SourceFor(tt.in, commit); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEntriesAndJSON(t *testing.T) {
	cflr := pack.Plugin{Name: "cflr", Repo: "cloudflare/skills", Path: "skills", Skills: []string{"./"}, Description: "CF"}
	packs := []pack.Pack{
		{Name: "base", Plugins: []pack.Plugin{cflr}},
		{
			Name:    "eng",
			Meta:    pack.Meta{Description: "Engineering", Version: "1.0.0", Includes: []string{"base"}},
			Plugins: []pack.Plugin{{Name: "obsidian", Repo: "kepano/obsidian-skills"}, cflr},
		},
	}
	sha := func(c string) string { return strings.Repeat(c, 40) }
	entries := Entries(packs, map[string]string{"cloudflare/skills": sha("c"), "kepano/obsidian-skills": sha("d")})
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, " "); got != "base cflr eng obsidian" {
		t.Fatalf("entries %s — want each pack's stack entry, then its plugins, each published once", got)
	}

	m := Build(entries, []string{"base", "eng"}, Options{Name: "x-marketplace", Owner: "Me"})
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`"description":"Plugins from base, eng"`,
		`"owner":{"name":"Me"}`,
		// a stack entry: in place, no components, depends on its plugins then its includes
		`{"name":"eng","description":"Engineering (2 plugins; includes base)","source":"./eng","category":"eng","tags":["ipm","eng"],"strict":false,"dependencies":["obsidian","cflr","base"],"commands":[],"agents":[],"version":"1.0.0"}`,
		`{"name":"base","description":"The base pack (1 plugin)","source":"./base","category":"base","tags":["ipm","base"],"strict":false,"dependencies":["cflr"],"commands":[],"agents":[]}`,
		// upstream plugins, pinned to their locked commits; no strict/commands/agents,
		// so an upstream plugin.json stays in charge
		`{"name":"cflr","description":"CF","source":{"source":"git-subdir","url":"cloudflare/skills","path":"skills","sha":"cccccccccccccccccccccccccccccccccccccccc"},"category":"base","tags":["ipm","base"],"skills":["./"]}`,
		`{"name":"obsidian","source":{"source":"github","repo":"kepano/obsidian-skills","sha":"dddddddddddddddddddddddddddddddddddddddd"},"category":"eng","tags":["ipm","eng"]}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON missing\n  %s\nin\n  %s", want, got)
		}
	}
	if strings.Contains(got, `"Pack"`) {
		t.Error("Entry.Pack must not be written")
	}
}

func TestWriteAndIsOurs(t *testing.T) {
	root := t.TempDir()
	path := Path(root)
	if IsOurs(path) {
		t.Fatal("a missing file is not ours")
	}
	m := Build(Entries([]pack.Pack{{Name: "p", Plugins: []pack.Plugin{{Name: "x", Repo: "o/r"}}}}, nil), []string{"p"}, Options{Name: "m", Owner: "o"})
	if err := Write(path, m); err != nil {
		t.Fatal(err)
	}
	if !IsOurs(path) {
		t.Fatal("a file Write produced is ours")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, ".claude-plugin", ".*.tmp")); len(leftovers) > 0 {
		t.Fatalf("temp file left behind: %v", leftovers)
	}
	if err := os.WriteFile(path, []byte(`{"plugins":[{"name":"hand-written"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsOurs(path) {
		t.Fatal("an entry without the ipm tag is not ours")
	}
}

func TestIsOursAcceptsLegacyTag(t *testing.T) {
	dir := t.TempDir()
	for tag, want := range map[string]bool{"ipm": true, "aihs": true, "other": false} {
		path := filepath.Join(dir, tag+".json")
		os.WriteFile(path, []byte(`{"plugins":[{"name":"x","tags":["`+tag+`"]}]}`), 0o644)
		if got := IsOurs(path); got != want {
			t.Errorf("tag %q: IsOurs = %v, want %v", tag, got, want)
		}
	}
}
