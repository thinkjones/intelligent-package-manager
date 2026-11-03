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

// museCopy is the Muse copy of the plugin name that ipm install muse kit
// installs.
func museCopy(root, name string) string {
	return filepath.Join(root, ".ipm", "muse", "kit", name)
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
		"muse plugins install " + museCopy(root, "kit"),
		"muse plugins install " + museCopy(root, "p"),
	}
	if got := museRuns(f); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
	for _, p := range []string{"kit/.claude-plugin/plugin.json", "kit/skills/s1/SKILL.md", "p/.claude-plugin/plugin.json"} {
		if _, err := os.Stat(museCopy(root, p)); err != nil {
			t.Errorf("copy lacks %s", p)
		}
	}
	if pj, _ := os.ReadFile(museCopy(root, "kit/.claude-plugin/plugin.json")); strings.Contains(string(pj), "dependencies") {
		t.Errorf("the pack's copy keeps dependencies:\n%s", pj)
	}
	if gi, _ := os.ReadFile(filepath.Join(root, ".ipm", ".gitignore")); !strings.Contains(string(gi), "muse/\n") {
		t.Errorf(".gitignore %q", gi)
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
	f.full = map[string]result{"muse plugins install " + museCopy(root, "p"): {"skills must be an array", errors.New("exit 1")}}
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

// Muse's warnings are counted, not shown; its errors and other lines are.
func TestInstallMuseHidesWarnings(t *testing.T) {
	f, root := museFake(t)
	pack := museCopy(root, "kit")
	f.full = map[string]result{"muse plugins install " + pack: {
		"diagnostic=unsupported-field severity=warning path=a message=one\n" +
			"diagnostic=unsupported-field severity=warning path=b message=two\n" +
			"diagnostic=duplicate-capability-id severity=error path=c message=duplicate skill capability id `x`\n" +
			"installed kit\n", nil}}
	if err := f.run("install", "muse", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	if strings.Contains(out, "severity=warning") {
		t.Errorf("warnings shown:\n%s", out)
	}
	for _, s := range []string{"duplicate skill capability id `x`", "installed kit\n", "2 muse warning(s) hidden — `muse plugins validate " + pack + "` shows them"} {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q:\n%s", s, out)
		}
	}
}

// The copies Muse installs are fixed where Muse differs from Claude Code;
// the plugin in the pin store is not.
func TestInstallMuseFixesCopies(t *testing.T) {
	f, root := museFake(t)
	pin := filepath.Join(pinnedRepo(f, "o--r", installSHA), "p")
	write(t, filepath.Join(pin, ".claude-plugin", "plugin.json"), `{"name":"p","skills":"./skills/"}`)
	write(t, filepath.Join(pin, ".mcp.json"), `{"a":{"command":"x"},"web":{"type":"http","url":"https://x"}}`)
	if err := f.run("install", "muse", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	for _, s := range []string{
		"fixed plugin:o/r:p for Muse Code: `skills` in .claude-plugin/plugin.json is now a list",
		"fixed plugin:o/r:p for Muse Code: wrapped .mcp.json in `mcpServers`",
		`! plugin:o/r:p: left out MCP server "web" (.mcp.json): it is http`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q:\n%s", s, out)
		}
	}
	if mcp, _ := os.ReadFile(museCopy(root, "p/.mcp.json")); !strings.Contains(string(mcp), `"mcpServers"`) || strings.Contains(string(mcp), "web") {
		t.Errorf("copy's .mcp.json %s", mcp)
	}
	if mcp, _ := os.ReadFile(filepath.Join(pin, ".mcp.json")); strings.Contains(string(mcp), "mcpServers") {
		t.Error("the pin store was edited")
	}
}

// Muse refuses to install a plugin it holds from another folder. ipm
// replaces one it installed (from a .ipm/ folder or its home), and with
// --force one it didn't.
func TestInstallMuseReplacesItsOwnInstall(t *testing.T) {
	for _, tc := range []struct {
		from    string
		force   bool
		replace bool
	}{
		{"/work/.ipm/plugins/p", false, true},
		{"/elsewhere/p", false, false},
		{"/elsewhere/p", true, true},
	} {
		f, root := museFake(t)
		install := "muse plugins install " + museCopy(root, "p")
		f.queue = map[string][]result{install: {{"plugin `p` is already installed from a different source", errors.New("exit 1")}, {"installed\tp", nil}}}
		f.full = map[string]result{"muse plugins inspect p --json": {`{"record":{"id":"p","source":{"path":"` + tc.from + `"}}}`, nil}}
		args := []string{"install", "muse", "kit"}
		if tc.force {
			args = append(args, "--force")
		}
		err := f.run(args...)
		runs := museRuns(f)
		if tc.replace {
			want := []string{"muse plugins install " + museCopy(root, "kit"), install, "muse plugins inspect p --json", "muse plugins remove p", install}
			if err != nil || !reflect.DeepEqual(runs, want) {
				t.Errorf("%+v: %v\ngot  %v\nwant %v\n%s", tc, err, runs, want, f.out.String())
			}
			if !strings.Contains(f.out.String(), "replacing p in Muse Code: it was installed from "+tc.from) {
				t.Errorf("%+v: output:\n%s", tc, f.out.String())
			}
			continue
		}
		if err == nil || !strings.Contains(f.out.String(), "p is installed in Muse Code from /elsewhere/p, which ipm didn't install — use --force to replace it") {
			t.Errorf("%+v: %v\n%s", tc, err, f.out.String())
		}
		if strings.Contains(strings.Join(runs, "\n"), "remove") {
			t.Errorf("%+v: removed: %v", tc, runs)
		}
	}
}

func TestInstallMuseDryRun(t *testing.T) {
	f, root := museFake(t)
	before, _ := os.ReadFile(filepath.Join(root, project.File))
	if err := f.run("install", "muse", "kit", "--dry-run"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	if !strings.Contains(out, "$ muse plugins install "+museCopy(root, "kit")) || !strings.Contains(out, "$ muse plugins install "+museCopy(root, "p")) {
		t.Errorf("output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".ipm", "muse")); err == nil {
		t.Error("a dry run wrote the Muse copies")
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
