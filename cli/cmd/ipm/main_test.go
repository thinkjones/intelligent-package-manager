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
		{"bad flag", root, []string{"sync", "--bogus"}, 2, "unknown flag: --bogus"},
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
