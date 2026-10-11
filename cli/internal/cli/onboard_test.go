package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/paths"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

func tempPaths(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()
	return paths.Paths{Home: filepath.Join(root, "home"), Cache: filepath.Join(root, "cache")}
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

var onboardNow = time.Date(2026, 10, 10, 15, 30, 0, 0, time.UTC)

func TestDetectEmpty(t *testing.T) {
	p := tempPaths(t)
	os.MkdirAll(p.Home, 0o755)
	if _, found, err := detect(p); err != nil || found {
		t.Errorf("detect = %v, %v; want nothing found", found, err)
	}
}

func TestDetectSummary(t *testing.T) {
	p := tempPaths(t)
	if _, err := sources.Add(p.Sources(), []string{"o/a", "o/b"}); err != nil {
		t.Fatal(err)
	}
	cat := p.Catalog()
	write(t, filepath.Join(cat, "repos.jsonl"), "{\"id\":\"repo:o/a\"}\n")
	write(t, filepath.Join(cat, "skills.jsonl"), "{\"id\":\"s1\"}\n{\"id\":\"s2\"}\n{\"id\":\"s3\"}\n")
	synced := time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local)
	os.Chtimes(filepath.Join(cat, "repos.jsonl"), synced, synced)
	e, found, err := detect(p)
	if err != nil || !found {
		t.Fatalf("detect = %v, %v", found, err)
	}
	if got := e.String(); got != "2 sources · 3 artifacts · synced 2026-10-09" {
		t.Errorf("summary = %q", got)
	}
	if got := (existing{Sources: 26, Artifacts: 3280}).String(); got != "26 sources · 3,280 artifacts" {
		t.Errorf("summary without a sync = %q", got)
	}
}

func makeData(t *testing.T, p paths.Paths) {
	t.Helper()
	write(t, filepath.Join(p.Home, "sources", "package.json"), "{}")
	write(t, filepath.Join(p.Cache, "catalog", "repos.jsonl"), "")
}

func TestBackupRenamesBoth(t *testing.T) {
	p := tempPaths(t)
	makeData(t, p)
	moved, err := backup(p, onboardNow)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{p.Home + ".backup-20261010-1530", p.Cache + ".backup-20261010-1530"}
	if strings.Join(moved, ",") != strings.Join(want, ",") {
		t.Errorf("moved = %v, want %v", moved, want)
	}
	if exists(p.Home) || exists(p.Cache) || !exists(filepath.Join(want[0], "sources", "package.json")) || !exists(filepath.Join(want[1], "catalog")) {
		t.Error("home and cache should now live at the backup paths only")
	}
	hint := restoreHint(p, moved)
	if !strings.Contains(hint, "ipm uninstall") || !strings.Contains(hint, "mv "+want[0]+" "+p.Home) || !strings.Contains(hint, "mv "+want[1]+" "+p.Cache) {
		t.Errorf("restore hint = %q", hint)
	}
}

func TestBackupClash(t *testing.T) {
	p := tempPaths(t)
	makeData(t, p)
	os.MkdirAll(p.Home+".backup-20261010-1530", 0o755)
	moved, err := backup(p, onboardNow)
	if err != nil {
		t.Fatal(err)
	}
	if moved[0] != p.Home+".backup-20261010-1530-2" {
		t.Errorf("home moved to %s, want the -2 suffix", moved[0])
	}
}

func TestBackupCacheInsideHome(t *testing.T) {
	p := tempPaths(t)
	p.Cache = filepath.Join(p.Home, "cache")
	makeData(t, p)
	moved, err := backup(p, onboardNow)
	if err != nil || len(moved) != 1 {
		t.Fatalf("moved = %v, err = %v; want only home moved", moved, err)
	}
	if !exists(filepath.Join(moved[0], "cache", "catalog")) {
		t.Error("the cache should have moved with home")
	}
}

func TestBackupRollsBack(t *testing.T) {
	root := t.TempDir()
	p := paths.Paths{Home: filepath.Join(root, "home"), Cache: filepath.Join(root, "locked", "cache")}
	makeData(t, p)
	os.Chmod(filepath.Join(root, "locked"), 0o555)
	defer os.Chmod(filepath.Join(root, "locked"), 0o755)
	if _, err := backup(p, onboardNow); err == nil {
		t.Fatal("want an error when the cache can't be renamed")
	}
	if !exists(filepath.Join(p.Home, "sources", "package.json")) {
		t.Error("home should be back where it was")
	}
}
