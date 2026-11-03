package assemble

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

func TestPlanAgentSkills(t *testing.T) {
	items := []project.LockItem{
		{ID: "skill:o/r:skills/a", Type: "skill", Name: "a", Repo: "o/r", Path: "skills/a", SHA: shaA},
		{ID: "agent:o/r:agents/x.md", Type: "agent", Name: "x", Repo: "o/r", Path: "agents/x.md", SHA: shaA},
		{ID: "plugin:o/r:p", Type: "plugin", Name: "p", Repo: "o/r", Path: "p", SHA: shaA, Parts: []string{"skill:o/r:p/skills/b", "hook:o/r:p/hooks/hooks.json", "skill:o/r:skills/a"}},
		{ID: "plugin:o/r#ext", Type: "plugin", Name: "ext", Repo: "o/r", Source: `{"name":"ext","source":{"source":"github","repo":"x/y"}}`},
		{ID: "plugin:o/r:q", Type: "plugin", Name: "q", Repo: "o/r", Path: "q", SHA: shaA},
	}
	p := project.Pack{Name: "kit"}
	byID := map[string]project.LockItem{}
	for _, it := range items {
		p.Items = append(p.Items, it.ID)
		byID[it.ID] = it
	}
	plan, err := PlanAgentSkills(p, byID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range plan.Entries {
		got = append(got, e.Alias+":"+e.From)
	}
	if want := []string{"o--r:skills/a", "o--r:p/skills/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("entries %v", got)
	}
	skips := strings.Join(plan.Skips, "\n")
	for _, s := range []string{
		"skipped agent:o/r:agents/x.md: an Agent Skills folder holds only skills",
		"skipped 1 agent(s), command(s), hook(s) and MCP server(s) of plugin:o/r:p",
		"skipped plugin:o/r#ext: external plugin",
		"skipped plugin:o/r:q: its parts aren't recorded",
	} {
		if !strings.Contains(skips, s) {
			t.Errorf("skips lack %q:\n%s", s, skips)
		}
	}
	if plan.Repos()["o--r"] != "github:o/r#"+shaA {
		t.Errorf("repos %v", plan.Repos())
	}
}

func skillsPlan(froms ...string) SkillsPlan {
	var sp SkillsPlan
	for _, f := range froms {
		sp.Entries = append(sp.Entries, SkillEntry{Alias: "o--r", From: f, Item: project.LockItem{ID: "skill:o/r:" + f, Type: "skill", Repo: "o/r", Path: f, SHA: shaA}})
	}
	return sp
}

func TestRenderAgentSkills(t *testing.T) {
	repo := t.TempDir()
	writeTree(t, repo, map[string]string{
		"skills/one/SKILL.md":    "---\nname: Go Expert\ndescription: Go.\n---\nBody\n",
		"skills/one/ref/a.md":    "a\n",
		"skills/two/SKILL.md":    "---\nname: go-expert\ndescription: A second.\n---\n",
		"skills/bare/SKILL.md":   "# Bare\n",
		"skills/nodesc/SKILL.md": "---\nname: nodesc\n---\n",
		"skills/hint/SKILL.md":   "---\nname: hint\ndescription: x\nargument-hint: [a] [b]\n---\n",
	})
	sp := skillsPlan("skills/one", "skills/two", "skills/bare", "skills/nodesc", "skills/hint")
	skills, warns, err := RenderAgentSkills("kit", sp, map[string]string{"o--r": repo})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
	}
	if want := []string{"go-expert", "nodesc", "hint"}; !reflect.DeepEqual(names, want) {
		t.Errorf("names %v", names)
	}
	files := map[string]string{}
	for _, f := range skills[0].Files {
		files[f.Path] = string(f.Data)
	}
	if files["SKILL.md"] != "---\nname: go-expert\ndescription: Go.\n---\nBody\n" || files["ref/a.md"] != "a\n" {
		t.Errorf("go-expert files %v", files)
	}
	all := strings.Join(warns, "\n")
	for _, s := range []string{
		`renamed skill:o/r:skills/one to "go-expert"`,
		`kit: skipped skill:o/r:skills/two: the pack already has a skill named "go-expert" (skill:o/r:skills/one)`,
		"skipped skill:o/r:skills/bare: its SKILL.md has no frontmatter",
		"skill:o/r:skills/nodesc: its SKILL.md has no description",
		"fixed skill:o/r:skills/hint: quoted argument-hint in SKILL.md",
	} {
		if !strings.Contains(all, s) {
			t.Errorf("warnings lack %q:\n%s", s, all)
		}
	}
}

func TestSpecName(t *testing.T) {
	for in, want := range map[string]string{"Go Expert": "go-expert", "pdf--processing": "pdf-processing", "-x-": "x", "a_b.c": "a-b-c", strings.Repeat("a", 70): strings.Repeat("a", 64)} {
		if got := SpecName(in); got != want {
			t.Errorf("SpecName(%q) = %q, want %q", in, got, want)
		}
	}
}

func agentSkill(name string) AgentSkill {
	return AgentSkill{Name: name, Item: project.LockItem{ID: "skill:o/r:" + name}, Files: []GasFile{{Path: "SKILL.md", Data: []byte("---\nname: " + name + "\n---\n"), Mode: 0o644}}}
}

func TestWriteAgentSkills(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAgentSkills(dir, "kit", []AgentSkill{agentSkill("a"), agentSkill("b")}, false); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(dir, n, "SKILL.md")); err != nil {
			t.Errorf("missing %s", n)
		}
	}
	// A second install with fewer skills removes the one dropped.
	removed, err := WriteAgentSkills(dir, "kit", []AgentSkill{agentSkill("a")}, false)
	if err != nil || !reflect.DeepEqual(removed, []string{filepath.Join(dir, "b")}) {
		t.Fatalf("removed %v %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b")); err == nil {
		t.Error("b is still there")
	}
	// Another pack's skill, and a folder ipm didn't write, need --force.
	if _, err := WriteAgentSkills(dir, "other", []AgentSkill{agentSkill("a")}, false); err == nil || !strings.Contains(err.Error(), "holds pack kit's skill — use --force") {
		t.Errorf("got %v", err)
	}
	writeTree(t, dir, map[string]string{"mine/SKILL.md": "mine\n"})
	if _, err := WriteAgentSkills(dir, "kit", []AgentSkill{agentSkill("a"), agentSkill("mine")}, false); err == nil || !strings.Contains(err.Error(), "was not written by ipm — use --force") {
		t.Errorf("got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "mine", "SKILL.md")); string(data) != "mine\n" {
		t.Error("a refused install wrote")
	}
	if _, err := WriteAgentSkills(dir, "other", []AgentSkill{agentSkill("a")}, true); err != nil {
		t.Fatal(err)
	}
	m, _ := readSkillsManifest(dir)
	if !reflect.DeepEqual(m.Packs, map[string][]string{"other": {"a"}}) {
		t.Errorf("a takeover moves the skill: %v", m.Packs)
	}
}
