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
