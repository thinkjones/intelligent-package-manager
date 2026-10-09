package catalog

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMarketplaceEntries(t *testing.T) {
	c := co(t)
	file(t, c.Dir, ".claude-plugin/marketplace.json", `{"name":"m","metadata":{"pluginRoot":"./plugins"},"plugins":[
		{"name":"a","source":"./a","skills":["./extra"]},
		{"name":"b","source":"./b","strict":false,"skills":["./s"]},
		{"name":"c","source":{"source":"github","repo":"x/c"},"description":"External C","category":"dev"},
		{"name":"d","source":"./missing"}]}`)
	file(t, c.Dir, "plugins/a/.claude-plugin/plugin.json", `{"name":"a"}`)
	file(t, c.Dir, "plugins/a/extra/e/SKILL.md", "---\nname: e\n---\n")
	file(t, c.Dir, "plugins/b/s/q/SKILL.md", "---\nname: q\n---\n")
	res, err := IndexRepo(c)
	if err != nil {
		t.Fatal(err)
	}
	recs := byID(t, res)
	if got := list(recs["marketplace:o/r:."], "plugins"); !reflect.DeepEqual(got, []string{"plugin:o/r:plugins/a", "plugin:o/r:plugins/b#b", "plugin:o/r#c"}) {
		t.Errorf("marketplace plugins %v", got)
	}
	a := recs["plugin:o/r:plugins/a"]
	if got := list(a, "skills"); !reflect.DeepEqual(got, []string{"skill:o/r:plugins/a/extra/e"}) || a.Fields["entry"] == nil {
		t.Errorf("strict entry should append its skills and be kept: %+v", a)
	}
	b := recs["plugin:o/r:plugins/b#b"]
	if b.Name != "b" || !reflect.DeepEqual(list(b, "skills"), []string{"skill:o/r:plugins/b/s/q"}) {
		t.Errorf("inline plugin %+v", b)
	}
	ext := recs["plugin:o/r#c"]
	if ext.Fields["source_repo"] != "x/c" || ext.Commit != "" || ext.Path != "" || ext.Description != "External C" ||
		!reflect.DeepEqual(ext.Tags, []string{"dev"}) || ext.Fields["entry"] == nil {
		t.Errorf("external plugin %+v", ext)
	}
	if !reflect.DeepEqual(res.Sources, []string{"x/c"}) {
		t.Errorf("sources %v", res.Sources)
	}
	if w := strings.Join(res.Warnings, "\n"); !strings.Contains(w, `"d"`) {
		t.Errorf("want a warning naming entry d: %v", res.Warnings)
	}
}

func twoRepos(t *testing.T) []Checkout {
	t.Helper()
	or := Checkout{Repo: "o/r", Commit: "c1", Dir: t.TempDir()}
	xc := Checkout{Repo: "x/c", Commit: "c2", Dir: t.TempDir()}
	file(t, or.Dir, ".claude-plugin/marketplace.json", `{"name":"m","plugins":[
		{"name":"tools","source":"./p"},
		{"name":"c","source":{"source":"url","url":"https://github.com/x/c.git"}},
		{"name":"z","source":{"source":"github","repo":"y/z"}}]}`)
	file(t, or.Dir, "p/.claude-plugin/plugin.json", `{"name":"tools"}`)
	file(t, or.Dir, "p/skills/s/SKILL.md", "---\nname: s\ndescription: <b>bold</b>\n---\n")
	file(t, or.Dir, "p/hooks/hooks.json", `{"hooks":{"Stop":[]}}`)
	file(t, xc.Dir, "skills/k/SKILL.md", "---\nname: k\n---\n")
	return []Checkout{or, xc}
}

func TestBuildRepoRecordsAndReverseLinks(t *testing.T) {
	recs, _, err := Build(twoRepos(t))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]Record{}
	for _, r := range recs {
		m[r.ID] = r
	}
	if r := m["repo:o/r"]; r.Fields["followed"] != true || r.Commit != "c1" ||
		!reflect.DeepEqual(r.Fields["counts"], map[string]int{"hook": 1, "marketplace": 1, "plugin": 3, "skill": 1}) {
		t.Errorf("repo:o/r %+v", r)
	}
	if r, ok := m["repo:x/c"]; !ok || r.Fields["followed"] != true {
		t.Errorf("repo:x/c should be one followed record: %+v", r)
	}
	if _, dup := m["repo:https://github.com/x/c.git"]; dup {
		t.Error("URL form of a followed repo must not get its own record")
	}
	if r := m["repo:y/z"]; r.Fields["followed"] != false || r.Commit != "" {
		t.Errorf("repo:y/z %+v", r)
	}
	if got := list(m["skill:o/r:p/skills/s"], "plugins"); !reflect.DeepEqual(got, []string{"plugin:o/r:p"}) {
		t.Errorf("skill plugins %v", got)
	}
	if got := list(m["skill:x/c:skills/k"], "plugins"); got == nil || len(got) != 0 {
		t.Errorf("unowned skill plugins %#v", got)
	}
	if got := list(m["plugin:o/r:p"], "marketplaces"); !reflect.DeepEqual(got, []string{"marketplace:o/r:."}) {
		t.Errorf("plugin marketplaces %v", got)
	}
}

func TestHookNamedForOwningPlugin(t *testing.T) {
	recs, _, _ := Build(twoRepos(t))
	for _, r := range recs {
		if r.ID == "hook:o/r:p/hooks/hooks.json" && r.Name != "tools" {
			t.Errorf("hook name %q", r.Name)
		}
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	cos := twoRepos(t)
	dirs := [2]string{t.TempDir(), t.TempDir()}
	for _, d := range dirs {
		recs, _, err := Build(cos)
		if err != nil {
			t.Fatal(err)
		}
		if err := Write(d, recs); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range Files {
		a, _ := os.ReadFile(filepath.Join(dirs[0], name))
		b, _ := os.ReadFile(filepath.Join(dirs[1], name))
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs between runs", name)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dirs[0], Files[TypeSkill]))
	if !bytes.Contains(data, []byte("<b>bold</b>")) {
		t.Errorf("HTML should not be escaped:\n%s", data)
	}
}

func TestEntriesSharingASourceFolderStayDistinct(t *testing.T) {
	c := co(t)
	file(t, c.Dir, ".claude-plugin/marketplace.json", `{"name":"m","plugins":[
		{"name":"docs","source":"./","strict":false,"skills":["./skills/pdf"]},
		{"name":"examples","source":"./","strict":false,"skills":["./skills/art"]}]}`)
	file(t, c.Dir, "skills/pdf/SKILL.md", "---\nname: pdf\n---\n")
	file(t, c.Dir, "skills/art/SKILL.md", "---\nname: art\n---\n")
	res, err := IndexRepo(c)
	if err != nil {
		t.Fatal(err)
	}
	recs := byID(t, res)
	for id, want := range map[string]string{"plugin:o/r:.#docs": "skill:o/r:skills/pdf", "plugin:o/r:.#examples": "skill:o/r:skills/art"} {
		if got := list(recs[id], "skills"); !reflect.DeepEqual(got, []string{want}) {
			t.Errorf("%s skills %v", id, got)
		}
	}
	if got := list(recs["marketplace:o/r:."], "plugins"); !reflect.DeepEqual(got, []string{"plugin:o/r:.#docs", "plugin:o/r:.#examples"}) {
		t.Errorf("marketplace plugins %v", got)
	}
}

func TestSymlinkedFileOutsideCheckoutIsNotRead(t *testing.T) {
	c := co(t)
	secret := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(secret, []byte("AWS_SECRET=hunter2\n"), 0o644)
	file(t, c.Dir, "p/.claude-plugin/plugin.json", `{"name":"p","hooks":"./cfg/h.json"}`)
	os.MkdirAll(filepath.Join(c.Dir, "p", "hooks"), 0o755)
	os.MkdirAll(filepath.Join(c.Dir, "p", "cfg"), 0o755)
	os.Symlink(secret, filepath.Join(c.Dir, "p", "hooks", "hooks.json"))
	os.Symlink(secret, filepath.Join(c.Dir, "p", "cfg", "h.json"))
	recs, warns, err := Build([]Checkout{c})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		if b, _ := r.MarshalJSON(); bytes.Contains(b, []byte("hunter2")) {
			t.Errorf("%s leaked the file: %s", r.ID, b)
		}
	}
	if !strings.Contains(strings.Join(warns, "\n"), "leaves the checkout") {
		t.Errorf("want a warning, got %v", warns)
	}
}
