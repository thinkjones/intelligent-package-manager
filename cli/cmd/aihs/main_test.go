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
	dir, err := os.MkdirTemp("", "aihs-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin = filepath.Join(dir, "aihs")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build aihs: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func aihs(t *testing.T, root string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	if root != "" {
		cmd.Env = append(cmd.Env, "AIHS_ROOT="+root)
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

// TestSyncThenCreatePlugin runs the real binary and real git against a local
// repo served over file://, standing in for GitHub.
func TestSyncThenCreatePlugin(t *testing.T) {
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
	git("commit", "-q", "--allow-empty", "-m", "one")
	sha := git("rev-parse", "HEAD")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("[[plugin]]\nname = \"notes-plugin\"\nrepo = \"file://%s\"\n", upstream)
	if err := os.WriteFile(filepath.Join(root, "notes", "manifest.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, errOut, code := aihs(t, root, "create-plugin", "--no-validate"); code != 1 || !strings.Contains(errOut, "run `aihs sync`") {
		t.Fatalf("before sync: exit %d, %s", code, errOut)
	}
	if out, errOut, code := aihs(t, root, "sync"); code != 0 || !strings.Contains(out, "fetched") {
		t.Fatalf("sync: exit %d\n%s%s", code, out, errOut)
	}
	if _, errOut, code := aihs(t, root, "create-plugin", "--no-validate", "--owner", "me"); code != 0 {
		t.Fatalf("create-plugin: exit %d: %s", code, errOut)
	}
	data, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "marketplace.json"))
	if err != nil || !strings.Contains(string(data), `"sha": "`+sha+`"`) {
		t.Fatalf("marketplace.json should pin %s: %v\n%s", sha, err, data)
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
		{"usage error", root, []string{"--upgrade"}, 2, "Try `aihs help`"},
		{"unknown command", root, []string{"nope"}, 2, "unknown command"},
		{"bad flag", root, []string{"create-plugin", "--bogus"}, 2, "flag provided but not defined"},
		{"missing config", "", []string{"create-plugin"}, 1, "AIHS_ROOT is not set"},
		{"nothing to publish", root, []string{"create-plugin"}, 1, "nothing to publish"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errOut, code := aihs(t, tt.root, tt.args...)
			if code != tt.code || !strings.Contains(errOut, tt.inStderr) {
				t.Fatalf("exit %d (want %d), stderr %q (want %q)", code, tt.code, errOut, tt.inStderr)
			}
		})
	}
}
