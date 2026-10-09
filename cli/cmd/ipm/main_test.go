package main

// The real binary against a throwaway checkout. These cover only what the
// in-process tests in internal/cli cannot: exit codes and real wiring.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ipm-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin = filepath.Join(dir, "ipm")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build ipm: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func ipm(t *testing.T, root string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	if root != "" {
		cmd.Env = append(cmd.Env, "IPM_HOME="+filepath.Join(root, "home"), "IPM_CACHE_DIR="+filepath.Join(root, "cache"))
	}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return o.String(), e.String(), code
}

// TestSourceSyncIndexWithPnpm runs the real binary and real pnpm against a
// local git repo served over file://, standing in for GitHub. Skipped when
// pnpm isn't installed.
func TestSourceSyncIndexWithPnpm(t *testing.T) {
	pnpm, err := exec.LookPath("pnpm")
	if err != nil {
		t.Skip("pnpm not installed")
	}
	node, _ := exec.LookPath("node")
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
	if out, code := run("source", "add", "file://"+upstream); code != 0 {
		t.Fatalf("source add: exit %d\n%s", code, out)
	}
	if out, code := run("sync"); code != 0 {
		t.Fatalf("sync: exit %d\n%s", code, out)
	}
	data, err := os.ReadFile(filepath.Join(cache, "catalog", "skills.jsonl"))
	if err != nil || !strings.Contains(string(data), "skills/one") || !strings.Contains(string(data), sha) {
		t.Fatalf("skills.jsonl should hold skills/one at %s: %v\n%s", sha, err, data)
	}
}

func TestExitCodes(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name     string
		root     string
		args     []string
		code     int
		inStderr string
	}{
		{"ok", "", []string{"version"}, 0, ""},
		{"usage error", root, []string{"--upgrade"}, 2, "Try `ipm help`"},
		{"unknown command", root, []string{"nope"}, 2, "unknown command"},
		{"bad flag", root, []string{"sync", "--bogus"}, 2, "flag provided but not defined"},
		{"no pnpm", root, []string{"sync"}, 1, "pnpm"},
		{"no sources", root, []string{"index"}, 1, "ipm source add"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errOut, code := ipm(t, tt.root, tt.args...)
			if code != tt.code || !strings.Contains(errOut, tt.inStderr) {
				t.Fatalf("exit %d (want %d), stderr %q (want %q)", code, tt.code, errOut, tt.inStderr)
			}
		})
	}
}
