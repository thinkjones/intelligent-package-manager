package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// museFake is installFake with the whole plugin plugin:o/r:p added to the
// pack, built, and muse on PATH.
func museFake(t *testing.T) (*fake, string) {
	t.Helper()
	f, root := installFake(t)
	write(t, filepath.Join(pinnedRepo(f, "o--r", installSHA), "p", ".claude-plugin", "plugin.json"), `{"name":"p"}`)
	if err := f.run("pack", "add", "kit", "plugin:o/r:p"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if err := f.run("pack", "build"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	f.onPath["muse"] = true
	f.calls, f.dirs = nil, nil
	f.out.Reset()
	return f, root
}

// musePin is where the pin store holds plugin:o/r:p, with symlinks resolved
// as repoFolders resolves them.
func musePin(t *testing.T, f *fake) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(filepath.Join(pinnedRepo(f, "o--r", installSHA), "p"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// museRuns is the muse commands f ran.
func museRuns(f *fake) []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "muse ") {
			out = append(out, c)
		}
	}
	return out
}

func TestInstallMuseInstallsPackAndPlugins(t *testing.T) {
	f, root := museFake(t)
	if err := f.run("install", "muse", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	want := []string{
		"muse plugins install " + filepath.Join(root, ".ipm", "plugins", "kit"),
		"muse plugins install " + musePin(t, f),
	}
	if got := museRuns(f); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
	out := f.out.String()
	for _, s := range []string{"build is current", "skipped plugin:o/r#e: its upstream entry names no commit", "✓ installed kit (muse)"} {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q:\n%s", s, out)
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, project.File))
	if !strings.Contains(string(data), `target = "muse"`) {
		t.Errorf("not recorded:\n%s", data)
	}
}

func TestInstallMuseNeedsMuse(t *testing.T) {
	f, _ := museFake(t)
	f.onPath["muse"] = false
	if err := f.run("install", "muse", "kit"); err == nil || !strings.Contains(err.Error(), "muse (Muse Code) on PATH") {
		t.Errorf("got %v", err)
	}
}

func TestInstallMuseSkipsFolderWithoutManifest(t *testing.T) {
	f, _ := museFake(t)
	os.Remove(filepath.Join(pinnedRepo(f, "o--r", installSHA), "p", ".claude-plugin", "plugin.json"))
	if err := f.run("install", "muse", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if runs := museRuns(f); len(runs) != 1 {
		t.Errorf("only the pack is installed: %v", runs)
	}
	if !strings.Contains(f.out.String(), "skipped plugin:o/r:p: its folder has no plugin manifest") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestInstallMuseFails(t *testing.T) {
	f, root := museFake(t)
	f.results["muse plug"] = result{"invalid bundle", errors.New("exit 1")}
	if err := f.run("install", "muse", "kit"); err == nil || !strings.Contains(err.Error(), "muse plugins install") {
		t.Errorf("got %v", err)
	}
	if !strings.Contains(f.out.String(), "invalid bundle") {
		t.Errorf("muse's output is shown:\n%s", f.out.String())
	}
	if data, _ := os.ReadFile(filepath.Join(root, project.File)); strings.Contains(string(data), `target = "muse"`) {
		t.Errorf("a failed install is not recorded:\n%s", data)
	}
	if runs := museRuns(f); len(runs) != 1 {
		t.Errorf("a failed pack stops before its plugins: %v", runs)
	}
}

// A whole plugin Muse refuses doesn't stop the others; the pack is installed,
// so it is recorded, and the install ends with an error naming the plugin.
func TestInstallMuseKeepsGoingPastAPlugin(t *testing.T) {
	f, root := museFake(t)
	f.full = map[string]result{"muse plugins install " + musePin(t, f): {"skills must be an array", errors.New("exit 1")}}
	err := f.run("install", "muse", "kit")
	if err == nil || !strings.Contains(err.Error(), "1 of 1 whole plugins") || !strings.Contains(err.Error(), "plugin:o/r:p") {
		t.Errorf("got %v", err)
	}
	out := f.out.String()
	if !strings.Contains(out, "skills must be an array") || !strings.Contains(out, "✗ plugin:o/r:p:") {
		t.Errorf("output:\n%s", out)
	}
	if data, _ := os.ReadFile(filepath.Join(root, project.File)); !strings.Contains(string(data), `target = "muse"`) {
		t.Errorf("the pack is installed, so it is recorded:\n%s", data)
	}
}

func TestInstallMuseDryRun(t *testing.T) {
	f, root := museFake(t)
	before, _ := os.ReadFile(filepath.Join(root, project.File))
	if err := f.run("install", "muse", "kit", "--dry-run"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	if !strings.Contains(out, "$ muse plugins install "+filepath.Join(root, ".ipm", "plugins", "kit")) || !strings.Contains(out, "$ muse plugins install "+musePin(t, f)) {
		t.Errorf("output:\n%s", out)
	}
	if runs := museRuns(f); len(runs) != 0 {
		t.Errorf("a dry run runs nothing: %v", runs)
	}
	if after, _ := os.ReadFile(filepath.Join(root, project.File)); string(after) != string(before) {
		t.Errorf("ipm.toml changed:\n%s", after)
	}
	os.Remove(filepath.Join(root, ".ipm", project.StampFile))
	if err := f.run("install", "muse", "kit", "--dry-run"); err == nil || !strings.Contains(err.Error(), "current build") {
		t.Errorf("a stale build: %v", err)
	}
}

func TestReplayInstallsMuse(t *testing.T) {
	f, _ := museFake(t)
	if err := f.run("install", "muse", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	f.calls = nil
	if err := f.run("install"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if runs := museRuns(f); len(runs) != 2 {
		t.Errorf("replay runs %v", runs)
	}
}
