package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

// skillsFake is installFake built: pack kit holds skill s1, an agent and an
// external plugin.
func skillsFake(t *testing.T) (*fake, string) {
	t.Helper()
	f, root := installFake(t)
	if err := f.run("pack", "build"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	f.out.Reset()
	return f, root
}

func TestInstallAgentSkillsWritesSkills(t *testing.T) {
	f, root := skillsFake(t)
	if err := f.run("install", "agentskills", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	dir := filepath.Join(root, ".agents", "skills")
	if data, err := os.ReadFile(filepath.Join(dir, "s1", "SKILL.md")); err != nil || string(data) != "---\nname: s1\n---\n" {
		t.Errorf("s1: %q %v", data, err)
	}
	out := f.out.String()
	for _, s := range []string{
		"skipped agent:o/r:p/agents/rev.md: an Agent Skills folder holds only skills",
		"skipped plugin:o/r#e: external plugin",
		"wrote " + filepath.Join(dir, "s1"),
		"✓ installed kit (agentskills " + dir + ")",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q:\n%s", s, out)
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, project.File))
	if !strings.Contains(string(data), `target = "agentskills"`) || strings.Contains(string(data), "path") {
		t.Errorf("record:\n%s", data)
	}

	// The skill leaves the pack, and its folder goes on the next install.
	if err := f.run("pack", "remove", "kit", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := f.run("install"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "s1")); err == nil || !strings.Contains(f.out.String(), "removed "+filepath.Join(dir, "s1")+": no longer in kit") {
		t.Errorf("s1 stays:\n%s", f.out.String())
	}
}

func TestInstallAgentSkillsRefusesAFolderNotOurs(t *testing.T) {
	f, root := skillsFake(t)
	mine := filepath.Join(root, ".agents", "skills", "s1", "SKILL.md")
	write(t, mine, "mine\n")
	if err := f.run("install", "agentskills", "kit"); err == nil || !strings.Contains(err.Error(), "was not written by ipm — use --force to replace it") {
		t.Errorf("got %v", err)
	}
	if data, _ := os.ReadFile(mine); string(data) != "mine\n" {
		t.Error("the folder was replaced")
	}
	if err := f.run("install", "agentskills", "kit", "--force"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if data, _ := os.ReadFile(mine); string(data) == "mine\n" {
		t.Error("--force didn't replace it")
	}
}

func TestInstallAgentSkillsDir(t *testing.T) {
	f, root := skillsFake(t)
	if err := f.run("install", "agentskills", "kit", "-C", "nope"); err == nil || !strings.Contains(err.Error(), "is not a folder") {
		t.Errorf("got %v", err)
	}
	dir := filepath.Join(root, "skills-here")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.run("install", "agentskills", "kit", "-C", dir); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "s1", "SKILL.md")); err != nil {
		t.Error(err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, project.File)); !strings.Contains(string(data), `path = "skills-here"`) {
		t.Errorf("record:\n%s", data)
	}
}

func TestInstallAgentSkillsGlobalGoesHome(t *testing.T) {
	f, root := skillsFake(t)
	home := filepath.Join(root, "home")
	f.env["HOME"] = home
	if err := f.run("pack", "new", "-g", "mine"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "add", "-g", "mine", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("install", "agentskills", "-g", "mine"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "s1", "SKILL.md")); err != nil {
		t.Error(err)
	}
}

func TestInstallAgentSkillsDryRun(t *testing.T) {
	f, root := skillsFake(t)
	before, _ := os.ReadFile(filepath.Join(root, project.File))
	if err := f.run("install", "agentskills", "kit", "--dry-run"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	dir := filepath.Join(root, ".agents", "skills")
	if !strings.Contains(f.out.String(), "would write "+filepath.Join(dir, "s1")) {
		t.Errorf("output:\n%s", f.out.String())
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("a dry run wrote the folder")
	}
	if after, _ := os.ReadFile(filepath.Join(root, project.File)); string(after) != string(before) {
		t.Errorf("ipm.toml changed:\n%s", after)
	}
}
