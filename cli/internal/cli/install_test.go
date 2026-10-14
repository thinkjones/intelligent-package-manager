package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// pnpmCalls counts the pnpm commands f ran.
func pnpmCalls(f *fake) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "pnpm ") {
			n++
		}
	}
	return n
}

// claudeRuns is the claude commands f ran that change Claude Code's config.
func claudeRuns(f *fake) []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "claude plugin") && !strings.HasSuffix(c, "--json") && !strings.HasPrefix(c, "claude plugin validate") {
			out = append(out, c)
		}
	}
	return out
}

func TestInstallClaudeSkipsCurrentBuild(t *testing.T) {
	f, _ := claudeFake(t)
	if err := f.run("install", "claude", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if !strings.Contains(f.out.String(), "build is current") || pnpmCalls(f) != 0 {
		t.Errorf("a current build is not rebuilt: %d pnpm calls\n%s", pnpmCalls(f), f.out.String())
	}
}

func TestInstallClaudeUpdateRebuilds(t *testing.T) {
	f, _ := claudeFake(t)
	if err := f.run("install", "claude", "kit", "--update"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if strings.Contains(f.out.String(), "build is current") || pnpmCalls(f) == 0 {
		t.Errorf("--update rebuilds: %d pnpm calls\n%s", pnpmCalls(f), f.out.String())
	}
}

func TestInstallClaudeDryRun(t *testing.T) {
	f, root := installFake(t) // not built
	f.onPath["claude"] = true
	f.full = map[string]result{
		"claude plugin marketplace list --json": {"[]", nil},
		"claude plugin list --json":             {"[]", nil},
	}
	before, _ := os.ReadFile(filepath.Join(root, project.File))
	if err := f.run("install", "claude", "kit", "--dry-run"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	if !strings.Contains(out, "$ claude plugin marketplace add "+filepath.Join(root, ".ipm")) || !strings.Contains(out, "$ claude plugin install kit@work-ipm --scope project") {
		t.Errorf("output:\n%s", out)
	}
	if runs := claudeRuns(f); len(runs) != 0 || pnpmCalls(f) != 0 {
		t.Errorf("a dry run runs nothing: %v, %d pnpm calls", runs, pnpmCalls(f))
	}
	for _, rel := range []string{"plugins", ".claude-plugin", project.StampFile} {
		if _, err := os.Stat(filepath.Join(root, ".ipm", rel)); err == nil {
			t.Errorf("a dry run wrote .ipm/%s", rel)
		}
	}
	if after, _ := os.ReadFile(filepath.Join(root, project.File)); string(after) != string(before) {
		t.Errorf("ipm.toml changed:\n%s", after)
	}
}

func TestInstallClaudeRecords(t *testing.T) {
	f, root := claudeFake(t)
	if err := f.run("install", "claude", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	data, _ := os.ReadFile(filepath.Join(root, project.File))
	if !strings.Contains(string(data), "[[packs.kit.installs]]\ntarget = \"claude\"\npath = \".\"\nscope = \"project\"\n") {
		t.Errorf("ipm.toml:\n%s", data)
	}
}

func TestInstallClaudeRecordsGlobal(t *testing.T) {
	f, _ := claudeFake(t)
	if err := f.run("pack", "-g", "new", "gk"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "-g", "add", "gk", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	scope, _ := currentScope(f.system(), true)
	write(t, filepath.Join(scope.IPMDir(), "node_modules", "o--r", "p", "skills", "s1", "SKILL.md"), "---\nname: s1\n---\n")
	if err := f.run("install", "claude", "-g", "gk"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	got, err := loadInstalls(f.system(), scope)
	want := []installRecord{{"gk", project.Install{Target: "claude", Scope: "user"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v %v", got, err)
	}
}

func TestPackInstallIsInstall(t *testing.T) {
	var runs [][]string
	for _, args := range [][]string{{"pack", "install", "claude", "kit"}, {"install", "claude", "kit"}} {
		f, root := claudeFake(t)
		if err := f.run(args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, f.out.String())
		}
		var run []string
		for _, c := range claudeRuns(f) {
			run = append(run, strings.ReplaceAll(c, root, "ROOT"))
		}
		runs = append(runs, run)
	}
	if len(runs[0]) == 0 || !reflect.DeepEqual(runs[0], runs[1]) {
		t.Errorf("pack install %v\ninstall %v", runs[0], runs[1])
	}
}

func TestBareInstallReplays(t *testing.T) {
	f, root := claudeFake(t)
	scope, _ := currentScope(f.system(), false)
	sub := filepath.Join(root, "sub")
	os.MkdirAll(sub, 0o755)
	for _, in := range []project.Install{
		{Target: "claude", Path: sub, Scope: "local"},
		{Target: "claude", Path: root, Scope: "project"},
	} {
		if err := recordInstall(f.system(), scope, "kit", in); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.run("install"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	var installs []string
	for i, c := range f.calls {
		if strings.HasPrefix(c, "claude plugin install") {
			installs = append(installs, f.dirs[i]+" | "+c)
		}
	}
	want := []string{
		root + " | claude plugin install kit@work-ipm --scope project",
		sub + " | claude plugin install kit@work-ipm --scope local",
	}
	if !reflect.DeepEqual(installs, want) {
		t.Errorf("replayed %v\nwant %v", installs, want)
	}
}

func TestBareInstallNothingRecorded(t *testing.T) {
	f, _ := claudeFake(t)
	err := f.run("install")
	var usage *UsageError
	want := "install: say claude or gascity — nothing is recorded for this scope yet. Try `ipm help install`."
	if !errors.As(err, &usage) || err.Error() != want {
		t.Errorf("got %v", err)
	}
}

func TestBareInstallUnknownTarget(t *testing.T) {
	f, root := claudeFake(t)
	scope, _ := currentScope(f.system(), false)
	if err := recordInstall(f.system(), scope, "kit", project.Install{Target: "zed", Path: root}); err != nil {
		t.Fatal(err)
	}
	err := f.run("install")
	if err == nil || !strings.Contains(err.Error(), "kit → zed "+root+`: unknown target "zed" (use claude or gascity)`) ||
		!strings.Contains(err.Error(), "ipm.toml ("+filepath.Join(root, project.File)+")") {
		t.Errorf("got %v", err)
	}
}

// An unknown target anywhere stops the replay before anything runs.
func TestBareInstallUnknownTargetRunsNothing(t *testing.T) {
	f, root := claudeFake(t)
	if err := f.run("pack", "new", "zz"); err != nil {
		t.Fatal(err)
	}
	scope, _ := currentScope(f.system(), false)
	for _, r := range []installRecord{
		{"kit", project.Install{Target: "claude", Path: root, Scope: "project"}},
		{"kit", project.Install{Target: "zed", Path: root}},
		{"zz", project.Install{Target: "claude", Path: root, Scope: "project"}},
	} {
		if err := recordInstall(f.system(), scope, r.Pack, r.Install); err != nil {
			t.Fatal(err)
		}
	}
	f.calls, f.dirs = nil, nil
	err := f.run("install")
	if err == nil || !strings.Contains(err.Error(), `kit → zed `+root+`: unknown target "zed"`) || !strings.Contains(err.Error(), "nothing was installed") {
		t.Errorf("got %v", err)
	}
	if runs := claudeRuns(f); len(runs) != 0 || pnpmCalls(f) != 0 {
		t.Errorf("an unknown target runs nothing: %v, %d pnpm calls", runs, pnpmCalls(f))
	}
}

func TestBareInstallGlobalNamesInstallsFile(t *testing.T) {
	f, _ := claudeFake(t)
	scope, _ := currentScope(f.system(), true)
	if err := recordInstall(f.system(), scope, "gk", project.Install{Target: "zed", Path: "/city"}); err != nil {
		t.Fatal(err)
	}
	err := f.run("install", "-g")
	want := "$IPM_HOME/installs.json (" + filepath.Join(f.env["IPM_HOME"], "installs.json") + ")"
	if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), project.File) {
		t.Errorf("got %v\nwant it to name %s", err, want)
	}
}

// A stale record (its folder deleted) fails on its own: the records after it
// still run, and the replay ends with exit 1 and the file to fix.
func TestBareInstallStaleRecordKeepsGoing(t *testing.T) {
	f, root := claudeFake(t)
	scope, _ := currentScope(f.system(), false)
	gone, good := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, p := range []string{gone, good} {
		os.MkdirAll(p, 0o755)
		if err := recordInstall(f.system(), scope, "kit", project.Install{Target: "claude", Path: p, Scope: "local"}); err != nil {
			t.Fatal(err)
		}
	}
	os.RemoveAll(gone)
	err := f.run("install")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("got %v, want exit 1\n%s", err, f.out.String())
	}
	var installs []string
	for i, c := range f.calls {
		if strings.HasPrefix(c, "claude plugin install") {
			installs = append(installs, f.dirs[i]+" | "+c)
		}
	}
	if want := []string{good + " | claude plugin install kit@work-ipm --scope local"}; !reflect.DeepEqual(installs, want) {
		t.Errorf("ran %v\nwant %v", installs, want)
	}
	out := f.out.String()
	for _, want := range []string{
		"✗ kit → claude local " + gone + ": " + gone + " is not a folder",
		"✓ installed kit",
		"✗ 1 of 2 recorded installs failed; if a record is stale, remove it from ipm.toml (" + filepath.Join(root, project.File) + ")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestInstallClaudeShortFlags(t *testing.T) {
	var runs [][]string
	for _, args := range [][]string{{"install", "claude", "kit", "-s", "local", "-p", "."}, {"install", "claude", "kit", "--scope", "local", "--path", "."}} {
		f, root := claudeFake(t)
		if err := f.run(args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, f.out.String())
		}
		var run []string
		for _, c := range claudeRuns(f) {
			run = append(run, strings.ReplaceAll(c, root, "ROOT"))
		}
		runs = append(runs, run)
	}
	if !slices.ContainsFunc(runs[0], func(c string) bool { return strings.HasSuffix(c, "--scope local") }) || !reflect.DeepEqual(runs[0], runs[1]) {
		t.Errorf("short %v\nlong %v", runs[0], runs[1])
	}
}

func TestTargetNamesFollowInstallers(t *testing.T) {
	saved := installers
	t.Cleanup(func() { installers = saved })
	for _, c := range []struct {
		names []string
		want  string
	}{
		{[]string{"claude"}, "claude"},
		{[]string{"gascity", "claude"}, "claude or gascity"},
		{[]string{"zed", "gascity", "claude"}, "claude, gascity or zed"},
	} {
		installers = map[string]installer{}
		for _, n := range c.names {
			installers[n] = installClaude
		}
		if got := targetNames(); got != c.want {
			t.Errorf("%v: got %q want %q", c.names, got, c.want)
		}
	}
}

func TestInstallUnknownTargetNamesTargets(t *testing.T) {
	saved := installers
	t.Cleanup(func() { installers = saved })
	installers = map[string]installer{"claude": installClaude, "gascity": installClaude}
	f, _ := claudeFake(t)
	err := f.run("install", "zed")
	if err == nil || !strings.Contains(err.Error(), "— use claude or gascity.") {
		t.Errorf("got %v", err)
	}
	f.out.Reset()
	err = f.run("install")
	if err == nil || !strings.Contains(err.Error(), "install: say claude or gascity — nothing is recorded") {
		t.Errorf("got %v", err)
	}
}

func TestBuildCurrentStaleWhenLockChanges(t *testing.T) {
	f, root := claudeFake(t)
	scope, _ := currentScope(f.system(), false)
	if !buildCurrent(f.system(), scope) {
		t.Fatal("a fresh build is current")
	}
	lock := filepath.Join(root, ".ipm", project.LockFile)
	data, _ := os.ReadFile(lock)
	if err := os.WriteFile(lock, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if buildCurrent(f.system(), scope) {
		t.Error("a changed lock makes the build stale")
	}
}

func TestBuildCurrentStaleWhenVersionChanges(t *testing.T) {
	f, _ := claudeFake(t)
	scope, _ := currentScope(f.system(), false)
	sys := f.system()
	sys.Version = "other"
	if buildCurrent(sys, scope) {
		t.Error("a new ipm version makes the build stale")
	}
}

func TestBuildCurrentStaleWhenStampMissing(t *testing.T) {
	f, root := claudeFake(t)
	scope, _ := currentScope(f.system(), false)
	if err := os.Remove(filepath.Join(root, ".ipm", project.StampFile)); err != nil {
		t.Fatal(err)
	}
	if buildCurrent(f.system(), scope) {
		t.Error("no stamp means no current build")
	}
}
