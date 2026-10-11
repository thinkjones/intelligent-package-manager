package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/tui"

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

const onboardList = `[[repos]]
list = "AI · Agentic Engineering Methods"
repo = "o/tdd"
stars = 900
topics = ["ai", "tdd", "testing"]

[[repos]]
list = "AI · Agentic Engineering Methods"
repo = "o/hooks"
stars = 50
topics = ["tdd", "hooks"]

[[repos]]
list = "AI · Skills, Plugins & Registries"
repo = "o/skills"
stars = 70
topics = ["testing"]

[[repos]]
list = "AI · Diagrams"
repo = "o/draw"
stars = 5
`

// onboardFake is a fake with pnpm 12 on PATH and the curated list on disk;
// it returns the fake and the list's path.
func onboardFake(t *testing.T) (*fake, string) {
	t.Helper()
	root := checkout(t)
	f := newFake(root)
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	list := filepath.Join(root, "list.toml")
	write(t, list, onboardList)
	return f, list
}

// installed writes what pnpm would leave for repos: a lockfile pinning each
// and a checkout holding one skill. It doesn't touch package.json.
func installed(t *testing.T, f *fake, repos ...string) {
	t.Helper()
	dir := filepath.Join(f.env["IPM_HOME"], "sources")
	var lock strings.Builder
	lock.WriteString("lockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    dependencies:\n")
	for i, r := range repos {
		a := sources.Alias(r)
		fmt.Fprintf(&lock, "      %s:\n        specifier: github:%s\n        version: https://codeload.github.com/%s/tar.gz/%s\n", a, r, r, strings.Repeat(string(rune('a'+i)), 40))
		write(t, filepath.Join(dir, "node_modules", a, "skills", "one", "SKILL.md"), "---\nname: one\n---\n")
	}
	write(t, filepath.Join(dir, "pnpm-lock.yaml"), lock.String())
}

func followed(t *testing.T, f *fake) []string {
	t.Helper()
	srcs, _ := sources.Load(filepath.Join(f.env["IPM_HOME"], "sources"))
	var out []string
	for _, s := range srcs {
		out = append(out, s.Repo)
	}
	sort.Strings(out)
	return out
}

func TestOnboardFreshWithYes(t *testing.T) {
	f, list := onboardFake(t)
	installed(t, f, "o/hooks", "o/skills", "o/tdd")
	if err := f.run("onboard", "--yes", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if got := strings.Join(followed(t, f), ","); got != "o/hooks,o/skills,o/tdd" {
		t.Errorf("sources = %s, want the two pre-ticked lists", got)
	}
	out := f.out.String()
	for _, want := range []string{"✓ added 3 sources (0 already followed)", "✓ sync", "- stats skipped: gh is not installed", "- search skipped: start Docker, then ipm serve && ipm load", "✓ library ready — 3 sources, 3 artifacts"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestOnboardAsksWhenDataExists(t *testing.T) {
	f, list := onboardFake(t)
	withSources(t, f, map[string]map[string]string{"o/old": {"skills/x/SKILL.md": "x"}})
	f.interactive = true
	f.stdin = "x\nb\n"
	if err := f.run("onboard", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	if strings.Count(out, "[u]se it · [b]ack it up and start fresh · [q]uit") != 2 {
		t.Errorf("want the question asked twice:\n%s", out)
	}
	if !strings.Contains(out, "found existing ipm data: 1 source") {
		t.Errorf("want the summary:\n%s", out)
	}
	home := f.env["IPM_HOME"]
	if !exists(filepath.Join(home+".backup-20261009-0000", "sources", "package.json")) {
		t.Errorf("home not backed up:\n%s", out)
	}
	if !strings.Contains(out, "to restore: ipm uninstall, then mv "+home+".backup-20261009-0000 "+home) {
		t.Errorf("want the restore hint:\n%s", out)
	}
}

func TestOnboardKeepLocksExisting(t *testing.T) {
	f, list := onboardFake(t)
	withSources(t, f, map[string]map[string]string{"o/tdd": {"skills/x/SKILL.md": "x"}})
	installed(t, f, "o/draw", "o/tdd")
	f.interactive = true
	f.checklist = func(c *tui.Checklist) error {
		c.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
		c.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		v := ansi.Strip(c.View().Content)
		if !strings.Contains(v, "[✓] o/tdd") || strings.Contains(v, "[x]") {
			t.Errorf("want o/tdd locked and nothing pre-ticked:\n%s", v)
		}
		for _, k := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: ' ', Text: " "}, {Code: tea.KeyEnter}, {Code: 'y', Text: "y"}} {
			c.Update(k)
		}
		return nil
	}
	if err := f.run("onboard", "--keep", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if got := strings.Join(followed(t, f), ","); got != "o/draw,o/tdd" {
		t.Errorf("sources = %s, want o/tdd kept and o/draw added", got)
	}
}

func TestOnboardNonInteractiveNeedsFlags(t *testing.T) {
	f, list := onboardFake(t)
	withSources(t, f, map[string]map[string]string{"o/old": {"skills/x/SKILL.md": "x"}})
	var usage *UsageError
	if err := f.run("onboard", "--list", list); !errors.As(err, &usage) || !strings.Contains(err.Error(), "--keep") {
		t.Errorf("with data: got %v, want a usage error naming --keep", err)
	}
	g, list2 := onboardFake(t)
	if err := g.run("onboard", "--list", list2); !errors.As(err, &usage) || !strings.Contains(err.Error(), "--lists") {
		t.Errorf("without data: got %v, want a usage error naming --lists", err)
	}
	if err := g.run("onboard", "--keep", "--fresh", "--list", list2); !errors.As(err, &usage) {
		t.Errorf("--keep --fresh: got %v, want a usage error", err)
	}
}

func TestOnboardUnknownList(t *testing.T) {
	f, list := onboardFake(t)
	err := f.run("onboard", "--lists", "Nope", "--list", list)
	if err == nil || !strings.Contains(err.Error(), "AI · Diagrams") || !strings.Contains(err.Error(), "Nope") {
		t.Errorf("got %v, want an error naming Nope and the lists", err)
	}
	if len(followed(t, f)) != 0 {
		t.Error("nothing should be added")
	}
}

func TestOnboardLists(t *testing.T) {
	f, list := onboardFake(t)
	installed(t, f, "o/draw")
	if err := f.run("onboard", "--lists", "AI · Diagrams", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if got := strings.Join(followed(t, f), ","); got != "o/draw" {
		t.Errorf("sources = %s, want o/draw", got)
	}
}

func TestOnboardQuitAfterBackupReprintsRestore(t *testing.T) {
	f, list := onboardFake(t)
	withSources(t, f, map[string]map[string]string{"o/old": {"skills/x/SKILL.md": "x"}})
	f.interactive = true
	f.stdin = "b\n"
	if err := f.run("onboard", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	tail := out[strings.LastIndex(out, "nothing changed"):]
	if !strings.Contains(out, "nothing changed") || !strings.Contains(tail, "to restore:") {
		t.Errorf("want 'nothing changed' then the restore hint:\n%s", out)
	}
	if exists(filepath.Join(f.env["IPM_HOME"], "sources", "package.json")) {
		t.Error("no sources should be added")
	}
}

func TestOnboardSyncFailureStops(t *testing.T) {
	f, list := onboardFake(t)
	f.results["pnpm inst"] = result{"ERR_PNPM_FETCH_404 nope", errors.New("exit 1")}
	var exit *ExitError
	if err := f.run("onboard", "--yes", "--list", list); !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("got %v, want exit 1\n%s", err, f.out.String())
	}
	out := f.out.String()
	if !strings.Contains(out, "✗ sync") || !strings.Contains(out, "rerun: ipm sync") || strings.Contains(out, "stats") || strings.Contains(out, "library ready") {
		t.Errorf("want the pipeline to stop at sync:\n%s", out)
	}
	if len(followed(t, f)) != 3 {
		t.Errorf("picks should stay added, got %v", followed(t, f))
	}
}

func TestOnboardSkipsStatsWhenGhLoggedOut(t *testing.T) {
	f, list := onboardFake(t)
	installed(t, f, "o/hooks", "o/skills", "o/tdd")
	f.onPath["gh"] = true
	f.results["gh auth"] = result{"not logged in", errors.New("exit 1")}
	if err := f.run("onboard", "--yes", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if !strings.Contains(f.out.String(), "- stats skipped: gh is not logged in (gh auth login)") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestOnboardTopics(t *testing.T) {
	f, list := onboardFake(t)
	installed(t, f, "o/hooks", "o/skills", "o/tdd")
	if err := f.run("onboard", "--yes", "--list", list); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if want := "try: ipm search tdd · ipm search testing · ipm search hooks · ipm tui"; !strings.Contains(f.out.String(), want) {
		t.Errorf("want %q in:\n%s", want, f.out.String())
	}
}
