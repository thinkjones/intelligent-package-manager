package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceFor(t *testing.T) {
	commit := strings.Repeat("a", 40)
	tests := []struct {
		name                string
		repo, path, version string
		want                Source
	}{
		{"whole github repo, default branch", "kepano/obsidian-skills", "", "",
			Source{Source: "github", Repo: "kepano/obsidian-skills", SHA: commit}},
		{"subdirectory at a tag", "cloudflare/skills", "skills", "v1.2.0",
			Source{Source: "git-subdir", URL: "cloudflare/skills", Path: "skills", Ref: "v1.2.0", SHA: commit}},
		{"version is a commit: no ref", "o/r", "", commit,
			Source{Source: "github", Repo: "o/r", SHA: commit}},
		{"non-github git url", "https://gitlab.example.com/g/p.git", "", "",
			Source{Source: "url", URL: "https://gitlab.example.com/g/p.git", SHA: commit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SourceFor(tt.repo, tt.path, tt.version, commit); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWriteAndIsOurs(t *testing.T) {
	root := t.TempDir()
	path := Path(root)
	if IsOurs(path) {
		t.Fatal("a missing file is not ours")
	}
	m := Build([]any{Entry{Name: "x", Source: SourceFor("o/r", "", "", strings.Repeat("a", 40)), Tags: []string{OwnerTag}}}, []string{"p"}, Options{Name: "m", Owner: "o"})
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
