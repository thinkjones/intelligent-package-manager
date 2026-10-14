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
	if err := f.run("install"); err == nil || !strings.Contains(err.Error(), "zed") {
		t.Errorf("got %v", err)
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
