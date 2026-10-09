package sources

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAlias(t *testing.T) {
	for in, want := range map[string]string{
		"ComposioHQ/awesome-claude-skills": "composiohq--awesome-claude-skills",
		"obra/superpowers":                 "obra--superpowers",
		"https://gitlab.com/g/p.git":       "gitlab.com--g--p",
		"Agents365-ai/drawio-skill":        "agents365-ai--drawio-skill",
	} {
		if got := Alias(in); got != want {
			t.Errorf("Alias(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddLoadRemove(t *testing.T) {
	dir := t.TempDir()
	added, err := Add(dir, []string{"obra/superpowers", "anthropics/skills@v1.2", "https://github.com/Wshobson/agents.git", "https://gitlab.com/g/p.git#main"})
	if err != nil || len(added) != 4 {
		t.Fatalf("add: %v %v", added, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "package.json"))
	want := `{
  "name": "ipm-sources",
  "private": true,
  "dependencies": {
    "anthropics--skills": "github:anthropics/skills#v1.2",
    "gitlab.com--g--p": "git+https://gitlab.com/g/p.git#main",
    "obra--superpowers": "github:obra/superpowers",
    "wshobson--agents": "github:Wshobson/agents"
  }
}
`
	if string(data) != want {
		t.Errorf("package.json:\n%s\nwant:\n%s", data, want)
	}
	got, err := Load(dir)
	if err != nil || len(got) != 4 {
		t.Fatalf("load: %v %v", got, err)
	}
	want0 := Source{Alias: "anthropics--skills", Repo: "anthropics/skills", Ref: "v1.2", Spec: "github:anthropics/skills#v1.2"}
	if got[0] != want0 || got[1].Repo != "https://gitlab.com/g/p.git" || got[1].Ref != "main" || got[3].Repo != "Wshobson/agents" {
		t.Errorf("load: %+v", got)
	}
	if err := Remove(dir, []string{"obra/superpowers", "https://github.com/wshobson/agents"}); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(dir)
	if len(got) != 2 {
		t.Errorf("after remove: %+v", got)
	}
	if err := Remove(dir, []string{"nobody/nothing"}); err == nil || !strings.Contains(err.Error(), "nobody/nothing") {
		t.Errorf("remove unknown: %v", err)
	}
	if s, err := Load(t.TempDir()); err != nil || s != nil {
		t.Errorf("missing package.json: %v %v", s, err)
	}
}

func TestAddRefusesDuplicateAndCollision(t *testing.T) {
	dir := t.TempDir()
	Add(dir, []string{"foo/bar"})
	if _, err := Add(dir, []string{"foo/bar"}); err == nil || !strings.Contains(err.Error(), "foo/bar") {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := Add(dir, []string{"Foo/Bar"}); err == nil || !strings.Contains(err.Error(), "Foo/Bar") || !strings.Contains(err.Error(), "foo/bar") {
		t.Errorf("collision: %v", err)
	}
	if _, err := Add(dir, []string{"not a repo"}); err == nil {
		t.Error("garbage accepted")
	}
}

const lock = `lockfileVersion: '9.0'

importers:

  .:
    dependencies:
      anthropics--skills:
        specifier: github:anthropics/skills
        version: https://codeload.github.com/anthropics/skills/tar.gz/683bc88e56f3e09ba94f7055977f3d3aa499f202
      obra--superpowers:
        specifier: github:obra/superpowers
        version: https://codeload.github.com/obra/superpowers/tar.gz/8ca22dba9a94f28898bbce59f2537ff4d87c747d#path:/x
`

func synced(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	Add(dir, []string{"anthropics/skills", "obra/superpowers"})
	os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(lock), 0o644)
	for _, a := range []string{"anthropics--skills", "obra--superpowers"} {
		os.MkdirAll(filepath.Join(dir, "node_modules", a), 0o755)
	}
	return dir
}

func TestCheckouts(t *testing.T) {
	dir := synced(t)
	cos, err := Checkouts(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cos {
		got[c.Repo] = c.Commit
		if c.Dir != filepath.Join(dir, "node_modules", c.Alias) {
			t.Errorf("dir %s", c.Dir)
		}
	}
	want := map[string]string{"anthropics/skills": "683bc88e56f3e09ba94f7055977f3d3aa499f202", "obra/superpowers": "8ca22dba9a94f28898bbce59f2537ff4d87c747d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestCheckoutsNeedSync(t *testing.T) {
	for name, breakIt := range map[string]func(dir string){
		"no lock": func(dir string) { os.Remove(filepath.Join(dir, "pnpm-lock.yaml")) },
		"no commit": func(dir string) {
			os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(strings.ReplaceAll(lock, "8ca22dba9a94f28898bbce59f2537ff4d87c747d", "main")), 0o644)
		},
		"no node_modules": func(dir string) { os.RemoveAll(filepath.Join(dir, "node_modules", "obra--superpowers")) },
	} {
		dir := synced(t)
		breakIt(dir)
		if _, err := Checkouts(dir); err == nil || !strings.Contains(err.Error(), "ipm sync") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
