package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

func TestRecordInstallReplaces(t *testing.T) {
	f, root := installFake(t)
	scope, _ := currentScope(f.system(), false)
	other := filepath.Join(root, "other")
	for _, in := range []project.Install{
		{Target: "claude", Path: root, Scope: "project"},
		{Target: "claude", Path: root, Scope: "local"},
		{Target: "claude", Path: other, Scope: "project"},
	} {
		if err := recordInstall(f.system(), scope, "kit", in); err != nil {
			t.Fatal(err)
		}
	}
	got, err := loadInstalls(f.system(), scope)
	want := []installRecord{
		{"kit", project.Install{Target: "claude", Path: root, Scope: "local"}},
		{"kit", project.Install{Target: "claude", Path: other, Scope: "project"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v %v", got, err)
	}
}

func TestRecordInstallRelativePath(t *testing.T) {
	f, root := installFake(t)
	scope, _ := currentScope(f.system(), false)
	outside := t.TempDir()
	for _, p := range []string{root, filepath.Join(root, "sub"), outside} {
		if err := recordInstall(f.system(), scope, "kit", project.Install{Target: "claude", Path: p, Scope: "project"}); err != nil {
			t.Fatal(err)
		}
	}
	packs, _ := project.LoadPacks(root)
	var paths []string
	for _, in := range packs[0].Installs {
		paths = append(paths, in.Path)
	}
	if want := []string{".", "sub", outside}; !reflect.DeepEqual(paths, want) {
		t.Errorf("stored %q want %q", paths, want)
	}
}

func TestRecordInstallGlobal(t *testing.T) {
	f, _ := installFake(t)
	scope, _ := currentScope(f.system(), true)
	if err := f.run("pack", "-g", "new", "gk"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(scope.Root, project.File))
	for _, in := range []project.Install{
		{Target: "gascity", Path: "/city/kit", Name: "kit"},
		{Target: "claude", Scope: "user"},
		{Target: "claude", Scope: "user"},
	} {
		if err := recordInstall(f.system(), scope, "gk", in); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(f.env["IPM_HOME"], "installs.json"))
	want := `[
  {
    "pack": "gk",
    "target": "claude",
    "path": "",
    "scope": "user"
  },
  {
    "pack": "gk",
    "target": "gascity",
    "path": "/city/kit",
    "name": "kit"
  }
]
`
	if string(data) != want {
		t.Errorf("installs.json:\n%s", data)
	}
	if after, _ := os.ReadFile(filepath.Join(scope.Root, project.File)); string(after) != string(before) {
		t.Errorf("global ipm.toml changed:\n%s", after)
	}
	got, _ := loadInstalls(f.system(), scope)
	if len(got) != 2 || got[0].Target != "claude" {
		t.Errorf("loaded %+v", got)
	}
}

func TestInstallRecordKeepsStampFresh(t *testing.T) {
	f, root := installFake(t)
	if err := f.run("pack", "build"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	scope, _ := currentScope(f.system(), false)
	if err := recordInstall(f.system(), scope, "kit", project.Install{Target: "claude", Path: root, Scope: "project"}); err != nil {
		t.Fatal(err)
	}
	if !buildCurrent(f.system(), scope) {
		t.Error("recording an install must not make the build stale")
	}
}

func TestPackShowListsInstalls(t *testing.T) {
	f, root := installFake(t)
	scope, _ := currentScope(f.system(), false)
	recordInstall(f.system(), scope, "kit", project.Install{Target: "claude", Path: root, Scope: "project"})
	recordInstall(f.system(), scope, "kit", project.Install{Target: "gascity", Path: "/city/kit", Name: "kit"})
	f.out.Reset()
	if err := f.run("pack", "show", "kit"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(f.out.String(), "installed: claude project "+root+"\ninstalled: gascity kit /city/kit\n") {
		t.Errorf("show:\n%s", f.out.String())
	}
}
