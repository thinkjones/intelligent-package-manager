package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// twoPacksFake is gascityFake with a second pack, second, built.
func twoPacksFake(t *testing.T) (*fake, string, string) {
	t.Helper()
	f, root, tmp := gascityFake(t)
	if err := f.run("pack", "new", "second"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "add", "second", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "build"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	f.calls = nil
	f.out.Reset()
	return f, root, tmp
}

func TestInstallGascityAllPacks(t *testing.T) {
	f, root, _ := twoPacksFake(t)
	dir := filepath.Join(root, ".ipm", ".gascity")
	if err := f.run("install", "gascity"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	for _, name := range []string{"kit", "second"} {
		if _, err := os.Stat(filepath.Join(dir, name, "pack.toml")); err != nil {
			t.Error(err)
		}
	}
	out := f.out.String()
	for _, want := range []string{
		"✓ installed kit (gascity " + filepath.Join(dir, "kit") + ")\n",
		"✓ installed second (gascity " + filepath.Join(dir, "second") + ")\n",
		"source = \"" + filepath.Join(dir, "second") + "\"\n",
		"✓ installed 2 packs\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if n := strings.Count(out, "build is current"); n != 1 {
		t.Errorf("build checked %d times:\n%s", n, out)
	}
	data, _ := os.ReadFile(filepath.Join(root, project.File))
	for _, name := range []string{"kit", "second"} {
		if !strings.Contains(string(data), "[[packs."+name+".installs]]\ntarget = \"gascity\"\npath = \".ipm/.gascity\"\nname = \""+name+"\"\n") {
			t.Errorf("ipm.toml lacks %s:\n%s", name, data)
		}
	}
}

func TestInstallGascityAllBuildsOnce(t *testing.T) {
	f, root, _ := twoPacksFake(t)
	stamp := filepath.Join(root, ".ipm", project.StampFile)
	os.Remove(stamp)
	if err := f.run("pack", "build"); err != nil {
		t.Fatal(err)
	}
	one := pnpmCalls(f)
	os.Remove(stamp)
	f.calls = nil
	if err := f.run("install", "gascity"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if one == 0 || pnpmCalls(f) != one {
		t.Errorf("%d pnpm calls, one build makes %d", pnpmCalls(f), one)
	}
}

func TestInstallGascityAllKeepsGoing(t *testing.T) {
	f, root, _ := twoPacksFake(t)
	dir := filepath.Join(root, ".ipm", ".gascity")
	write(t, filepath.Join(dir, "kit", "pack.toml"), "[pack]\nname = \"mine\"\n")
	err := f.run("install", "gascity")
	var exit *ExitError
	var usage *UsageError
	if !errors.As(err, &exit) || exit.Code != 1 || errors.As(err, &usage) {
		t.Fatalf("got %v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "second", "pack.toml")); err != nil {
		t.Error(err)
	}
	out := f.out.String()
	for _, want := range []string{
		"✗ kit: " + filepath.Join(dir, "kit") + " exists and was not written by ipm — use --force to replace it\n",
		"✓ installed second",
		"✗ 1 of 2 packs failed\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	// --force applies to every pack.
	f.out.Reset()
	if err := f.run("install", "gascity", "--force"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "kit", "pack.toml")); !strings.HasPrefix(string(data), gasHeader) {
		t.Errorf("not replaced: %s", data)
	}
}

func TestInstallGascityAllNoPacks(t *testing.T) {
	f, _, tmp := gascityFake(t)
	f.cwd = tmp
	err := f.run("install", "gascity")
	if err == nil || !strings.Contains(err.Error(), "no packs in "+filepath.Join(tmp, project.File)+" — create one with `ipm pack new <name>`") {
		t.Errorf("got %v", err)
	}
	err = f.run("install", "gascity", "-g")
	if err == nil || !strings.Contains(err.Error(), "no packs in "+filepath.Join(f.env["IPM_HOME"], "global", project.File)) {
		t.Errorf("-g: got %v", err)
	}
}

func TestInstallGascityNameNeedsPack(t *testing.T) {
	f, _, _ := gascityFake(t)
	err := f.run("install", "gascity", "--name", "x")
	var usage *UsageError
	if !errors.As(err, &usage) || err.Error() != "install gascity: --name needs a pack — it names one pack folder" {
		t.Errorf("got %v", err)
	}
}

func TestInstallGascityAllDryRun(t *testing.T) {
	f, root, _ := twoPacksFake(t)
	dir := filepath.Join(root, ".ipm", ".gascity")
	before, _ := os.ReadFile(filepath.Join(root, project.File))
	if err := f.run("install", "gascity", "--dry-run"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	for _, want := range []string{"would write " + filepath.Join(dir, "kit") + "\n", "would write " + filepath.Join(dir, "second") + "\n", "  skills/s1/SKILL.md\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "installed") {
		t.Errorf("a dry run installs nothing:\n%s", out)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("a dry run created .ipm/.gascity")
	}
	if after, _ := os.ReadFile(filepath.Join(root, project.File)); string(after) != string(before) {
		t.Errorf("ipm.toml changed:\n%s", after)
	}
}

// globalPacksFake is gascityFake with two user-scope packs, gk and gk2.
func globalPacksFake(t *testing.T) (*fake, string, string) {
	t.Helper()
	f, root, tmp := gascityFake(t)
	for _, name := range []string{"gk", "gk2"} {
		if err := f.run("pack", "-g", "new", name); err != nil {
			t.Fatal(err)
		}
	}
	f.out.Reset()
	return f, root, tmp
}

func TestInstallGascityGlobalIntoProject(t *testing.T) {
	f, root, _ := globalPacksFake(t)
	sub := filepath.Join(root, "sub")
	os.MkdirAll(sub, 0o755)
	f.cwd = sub
	before, _ := os.ReadFile(filepath.Join(root, project.File))
	if err := f.run("install", "gascity", "-g"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	dir := filepath.Join(root, ".ipm", ".gascity")
	for _, name := range []string{"gk", "gk2"} {
		if _, err := os.Stat(filepath.Join(dir, name, "pack.toml")); err != nil {
			t.Error(err)
		}
	}
	if !strings.Contains(f.out.String(), "✓ installed 2 packs\n") {
		t.Errorf("output:\n%s", f.out.String())
	}
	if _, err := os.Stat(filepath.Join(f.env["IPM_HOME"], "global", ".ipm", ".gascity")); err == nil {
		t.Error("wrote into the user scope's .ipm")
	}
	got, err := loadInstalls(f.system(), scopeOf(t, f, true))
	want := []installRecord{
		{"gk", project.Install{Target: "gascity", Path: dir, Name: "gk"}},
		{"gk2", project.Install{Target: "gascity", Path: dir, Name: "gk2"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v %v", got, err)
	}
	if after, _ := os.ReadFile(filepath.Join(root, project.File)); string(after) != string(before) {
		t.Errorf("ipm.toml changed:\n%s", after)
	}

	// Replay recreates the folder, from anywhere.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	f.cwd = t.TempDir()
	f.out.Reset()
	if err := f.run("install", "-g"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "gk", "pack.toml")); err != nil {
		t.Errorf("not redone: %v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(f.cwd, ".ipm")); err == nil {
		t.Error("replay wrote into the working directory")
	}
}

func TestInstallGascityGlobalOutsideProject(t *testing.T) {
	f, _, tmp := globalPacksFake(t)
	f.cwd = tmp
	if err := f.run("install", "gascity", "-g", "gk"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	dir := filepath.Join(tmp, ".ipm", ".gascity")
	if _, err := os.Stat(filepath.Join(dir, "gk", "pack.toml")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gk2")); err == nil {
		t.Error("a named pack installed the others")
	}
	if !strings.Contains(f.out.String(), "✓ installed gk (gascity "+filepath.Join(dir, "gk")+")\n") || strings.Contains(f.out.String(), "packs") {
		t.Errorf("output:\n%s", f.out.String())
	}
	got, err := loadInstalls(f.system(), scopeOf(t, f, true))
	want := []installRecord{{"gk", project.Install{Target: "gascity", Path: dir, Name: "gk"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v %v", got, err)
	}
}

func TestReplayOldGlobalGascityRecord(t *testing.T) {
	f, _, _ := globalPacksFake(t)
	old := filepath.Join(f.env["IPM_HOME"], "global", ".ipm", ".gascity")
	data, _ := json.Marshal([]installJSON{{Pack: "gk", Target: "gascity", Path: old, Name: "gk"}})
	write(t, filepath.Join(f.env["IPM_HOME"], installsFile), string(data))
	if err := f.run("install", "-g"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(old, "gk", "pack.toml")); err != nil {
		t.Errorf("not redone: %v\n%s", err, f.out.String())
	}
}

func TestGascityRefusalGlobalLocalProject(t *testing.T) {
	f, _, tmp := gascityFake(t)
	f.cwd = filepath.Join(tmp, "here")
	os.MkdirAll(f.cwd, 0o755)
	want := "refusing to replace " + tmp + ": it holds the project"
	if err := gascityRefusal(f.system(), scopeOf(t, f, true), tmp); err == nil || err.Error() != want {
		t.Errorf("got %v", err)
	}
}
