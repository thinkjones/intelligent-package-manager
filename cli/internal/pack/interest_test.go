package pack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInterest(t *testing.T) {
	root := t.TempDir()
	if ups, err := LoadInterest(root); err != nil || ups != nil {
		t.Fatalf("missing file: %v %v", ups, err)
	}
	path := filepath.Join(root, ManifestFile)
	os.WriteFile(path, []byte("[[repo]]\nrepo = \"z/z\"\n\n[[repo]]\nrepo = \"a/a\"\nversion = \"v1\"\n"), 0o644)
	ups, err := LoadInterest(root)
	if err != nil || len(ups) != 2 || ups[0].Key() != "a/a@v1" || ups[1].Key() != "z/z" {
		t.Fatalf("got %v %v", ups, err)
	}
	for body, want := range map[string]string{
		"[[repo]]\nversion = \"v1\"\n":                         "repo is required",
		"[[repo]]\nrepo = \"a/a\"\n[[repo]]\nrepo = \"a/a\"\n": "listed twice",
		"[[repo]]\nrepo = \"a/a\"\nnote = \"x\"\n":             "unknown key",
	} {
		os.WriteFile(path, []byte(body), 0o644)
		if _, err := LoadInterest(root); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want %q, got %v", body, want, err)
		}
	}
}

func TestMerge(t *testing.T) {
	got := Merge([]Upstream{{"b/b", ""}, {"a/a", ""}}, []Upstream{{"a/a", ""}, {"a/a", "v1"}})
	if len(got) != 3 || got[0].Key() != "a/a" || got[1].Key() != "a/a@v1" || got[2].Key() != "b/b" {
		t.Errorf("got %v", got)
	}
}

func TestNormalizeRepo(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/o/r":     "o/r",
		"https://github.com/o/r.git": "o/r",
		"git@github.com:o/r.git":     "o/r",
		"https://github.com/o/r/":    "o/r",
		"o/r":                        "o/r",
		"https://gitlab.com/g/p":     "https://gitlab.com/g/p",
	} {
		if got := NormalizeRepo(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

func TestAppendInterestKeepsComments(t *testing.T) {
	root := t.TempDir()
	orig := "# keep me\n\n[[repo]]\nrepo = \"a/a\"\n"
	os.WriteFile(filepath.Join(root, ManifestFile), []byte(orig), 0o644)
	if err := AppendInterest(root, []string{"b/b"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ManifestFile))
	if !strings.HasPrefix(string(data), orig) {
		t.Errorf("original bytes lost:\n%s", data)
	}
	ups, err := LoadInterest(root)
	if err != nil || len(ups) != 2 || ups[1].Repo != "b/b" {
		t.Errorf("got %v %v", ups, err)
	}
	if err := AppendInterest(t.TempDir(), []string{"c/c"}); err != nil {
		t.Errorf("a missing manifest should be created: %v", err)
	}
}

func TestAppendInterestRefusesDuplicate(t *testing.T) {
	root := t.TempDir()
	orig := "[[repo]]\nrepo = \"a/a\"\n"
	os.WriteFile(filepath.Join(root, ManifestFile), []byte(orig), 0o644)
	err := AppendInterest(root, []string{"b/b", "a/a"})
	if err == nil || !strings.Contains(err.Error(), "a/a") {
		t.Errorf("got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ManifestFile)); string(data) != orig {
		t.Errorf("file changed:\n%s", data)
	}
}
