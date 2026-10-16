//go:build integration

package main

// The real binary with real pnpm and Node, against local git repos served
// over file://. Run with mise run test:integration.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/testtool"
)

// TestSourceSyncIndexWithPnpm runs the real binary and real pnpm against a
// local git repo served over file://, standing in for GitHub.
func TestSourceSyncIndexWithPnpm(t *testing.T) {
	pnpm, node := testtool.Need(t, "pnpm"), testtool.Need(t, "node")
	upstream := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		base := []string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", upstream}
		out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(upstream, "skills", "one"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(upstream, "skills", "one", "SKILL.md"), []byte("---\nname: one\ndescription: d\n---\n"), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "one")
	sha := git("rev-parse", "HEAD")

	home, cache := t.TempDir(), t.TempDir()
	path := filepath.Dir(pnpm) + ":" + filepath.Dir(node) + ":/usr/bin:/bin"
	run := func(args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = []string{"PATH=" + path, "HOME=" + t.TempDir(), "IPM_HOME=" + home, "IPM_CACHE_DIR=" + cache}
		out, err := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return string(out), exitErr.ExitCode()
		}
		return string(out), 0
	}
	if out, code := run("marketplace", "add", "file://"+upstream); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run("catalog", "sync"); code != 0 {
		t.Fatalf("sync: exit %d\n%s", code, out)
	}
	data, err := os.ReadFile(filepath.Join(cache, "catalog", "skills.jsonl"))
	if err != nil || !strings.Contains(string(data), "skills/one") || !strings.Contains(string(data), sha) {
		t.Fatalf("skills.jsonl should hold skills/one at %s: %v\n%s", sha, err, data)
	}
}

// TestSyncUpgradeMovesBranchesNotTags: with real pnpm, `catalog sync --upgrade`
// moves a source tracking a branch to its new commit, leaves a tag-pinned
// source on the tag, and leaves package.json's specs alone.
func TestSyncUpgradeMovesBranchesNotTags(t *testing.T) {
	pnpm, node := testtool.Need(t, "pnpm"), testtool.Need(t, "node")
	repo := func(tag string) (dir string, commit func(msg string) string) {
		dir = t.TempDir()
		git := func(args ...string) string {
			t.Helper()
			base := []string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", dir}
			out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
			return strings.TrimSpace(string(out))
		}
		git("init", "-q", "-b", "main")
		commit = func(msg string) string {
			os.MkdirAll(filepath.Join(dir, "skills", msg), 0o755)
			os.WriteFile(filepath.Join(dir, "skills", msg, "SKILL.md"), []byte("---\nname: "+msg+"\n---\n"), 0o644)
			git("add", ".")
			git("commit", "-q", "-m", msg)
			return git("rev-parse", "HEAD")
		}
		commit("one")
		if tag != "" {
			git("tag", tag)
		}
		return dir, commit
	}
	branchRepo, branchCommit := repo("")
	tagRepo, tagCommit := repo("v1")
	tagged := strings.TrimSpace(func() string {
		out, _ := exec.Command("git", "-C", tagRepo, "rev-parse", "v1").Output()
		return string(out)
	}())

	home, cache := t.TempDir(), t.TempDir()
	path := filepath.Dir(pnpm) + ":" + filepath.Dir(node) + ":/usr/bin:/bin"
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = []string{"PATH=" + path, "HOME=" + t.TempDir(), "IPM_HOME=" + home, "IPM_CACHE_DIR=" + cache}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ipm %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("marketplace", "add", "file://"+branchRepo, "file://"+tagRepo+"#v1")
	run("catalog", "sync")
	specs, _ := os.ReadFile(filepath.Join(home, "sources", "package.json"))
	newBranch := branchCommit("two")
	tagCommit("two")
	run("catalog", "sync", "--upgrade")
	lock, _ := os.ReadFile(filepath.Join(home, "sources", "pnpm-lock.yaml"))
	if !strings.Contains(string(lock), newBranch) {
		t.Errorf("branch source did not move to %s:\n%s", newBranch, lock)
	}
	if !strings.Contains(string(lock), tagged) {
		t.Errorf("tag source moved off v1 (%s):\n%s", tagged, lock)
	}
	after, _ := os.ReadFile(filepath.Join(home, "sources", "package.json"))
	if string(after) != string(specs) {
		t.Errorf("package.json changed:\nbefore %s\nafter  %s", specs, after)
	}
}

// TestPackBuildWithPnpm: real ipm + real pnpm from a local git source to a
// built plugin. A build reads the synced source when it is at the pinned
// commit, else fetches the pin into $IPM_HOME/pinned once for every project;
// .ipm holds no pnpm project.
func TestPackBuildWithPnpm(t *testing.T) {
	pnpm, node := testtool.Need(t, "pnpm"), testtool.Need(t, "node")
	upstream := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		base := []string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", upstream}
		out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	for rel, body := range map[string]string{
		"skills/one/SKILL.md":  "---\nname: one\ndescription: d\n---\n",
		"agents/helper.md":     "---\nname: helper\ndescription: h\n---\nfirst\n",
		"skills/one/refs/x.md": "x",
	} {
		os.MkdirAll(filepath.Join(upstream, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(upstream, rel), []byte(body), 0o644)
	}
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-q", "-m", "one")
	first := git("rev-parse", "HEAD")

	home, cache, project := t.TempDir(), t.TempDir(), t.TempDir()
	path := filepath.Dir(pnpm) + ":" + filepath.Dir(node) + ":/usr/bin:/bin"
	run := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + path, "HOME=" + t.TempDir(), "IPM_HOME=" + home, "IPM_CACHE_DIR=" + cache}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ipm %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	sameFile := func(a, b string) bool {
		t.Helper()
		ia, errA := os.Stat(a)
		ib, errB := os.Stat(b)
		if errA != nil || errB != nil {
			t.Fatalf("stat: %v %v", errA, errB)
		}
		return os.SameFile(ia, ib)
	}
	repo := "file://" + upstream
	alias := sources.Alias(repo)
	run(project, "marketplace", "add", repo)
	run(project, "catalog", "sync")
	run(project, "pack", "new", "kit")
	run(project, "pack", "add", "kit", "skill:"+repo+":skills/one", "agent:"+repo+":agents/helper.md")

	// The source is at the pinned commit: no pnpm, no pin store.
	if out := run(project, "pack", "build", "--no-validate"); strings.Contains(out, "pnpm") {
		t.Errorf("a build from synced sources runs no pnpm:\n%s", out)
	}
	plugin := filepath.Join(project, ".ipm", "plugins", "kit")
	for _, rel := range []string{"skills/one/SKILL.md", "skills/one/refs/x.md", "agents/helper.md", ".claude-plugin/plugin.json"} {
		if _, err := os.Stat(filepath.Join(plugin, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"node_modules", "package.json", "pnpm-lock.yaml", ".pnpmfile.cjs"} {
		if _, err := os.Stat(filepath.Join(project, ".ipm", rel)); !os.IsNotExist(err) {
			t.Errorf(".ipm/%s: %v", rel, err)
		}
	}
	if !sameFile(filepath.Join(home, "sources", "node_modules", alias, "agents", "helper.md"), filepath.Join(plugin, "agents", "helper.md")) {
		t.Error("plugin files must be hard links to the synced source")
	}

	// The source moves on; the lock keeps the first commit, which is
	// fetched into the pin store.
	os.WriteFile(filepath.Join(upstream, "agents", "helper.md"), []byte("---\nname: helper\ndescription: h\n---\nsecond\n"), 0o644)
	git("commit", "-q", "-am", "two")
	run(project, "catalog", "sync", "--upgrade")
	run(project, "pack", "build", "--no-validate")
	pinned := filepath.Join(home, "pinned")
	pkg, _ := os.ReadFile(filepath.Join(pinned, "package.json"))
	if key := alias + "--" + first[:12]; !strings.Contains(string(pkg), `"`+key+`"`) {
		t.Errorf("pinned/package.json lacks %s:\n%s", key, pkg)
	}
	if data, _ := os.ReadFile(filepath.Join(plugin, "agents", "helper.md")); !strings.Contains(string(data), "first") {
		t.Errorf("the build must keep the pinned commit:\n%s", data)
	}
	if !sameFile(filepath.Join(pinned, "node_modules", alias+"--"+first[:12], "agents", "helper.md"), filepath.Join(plugin, "agents", "helper.md")) {
		t.Error("plugin files must be hard links into the pin store")
	}

	// Another project with the same pin builds without pnpm.
	other := t.TempDir()
	for _, rel := range []string{"ipm.toml", filepath.Join(".ipm", "ipm.lock")} {
		data, _ := os.ReadFile(filepath.Join(project, rel))
		os.MkdirAll(filepath.Dir(filepath.Join(other, rel)), 0o755)
		os.WriteFile(filepath.Join(other, rel), data, 0o644)
	}
	if out := run(other, "pack", "build", "--no-validate"); strings.Contains(out, "pnpm") {
		t.Errorf("a pin already fetched runs no pnpm:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(other, ".ipm", "plugins", "kit", "agents", "helper.md")); err != nil {
		t.Error(err)
	}
}
