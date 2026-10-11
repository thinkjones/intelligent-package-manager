package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// uninstallFake has docker running, a binary at a temp path and data in
// home and cache.
func uninstallFake(t *testing.T) *fake {
	t.Helper()
	f := newFake(t.TempDir())
	f.onPath["docker"] = true
	f.exe = filepath.Join(t.TempDir(), "ipm")
	write(t, f.exe, "binary")
	write(t, filepath.Join(f.env["IPM_HOME"], "sources", "package.json"), "{}")
	write(t, filepath.Join(f.env["IPM_CACHE_DIR"], "catalog", "repos.jsonl"), "")
	return f
}

func TestUninstallKeepsData(t *testing.T) {
	f := uninstallFake(t)
	if err := f.run("uninstall"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if exists(f.exe) {
		t.Error("binary should be removed")
	}
	if callWith(f, "docker rm") != "docker rm -f ipm-typesense" {
		t.Errorf("calls %v", f.calls)
	}
	if !exists(f.env["IPM_HOME"]) || !exists(f.env["IPM_CACHE_DIR"]) {
		t.Error("data should be kept")
	}
	if !strings.Contains(f.out.String(), "kept your data: "+f.env["IPM_HOME"]+", "+f.env["IPM_CACHE_DIR"]) {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestUninstallRefusesDevBuild(t *testing.T) {
	f := uninstallFake(t)
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".git"), 0o755)
	write(t, filepath.Join(repo, "cli", "go.mod"), "module x\n")
	f.exe = filepath.Join(repo, "cli", "bin", "ipm")
	write(t, f.exe, "binary")
	err := f.run("uninstall")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("got %v, want a refusal naming --force", err)
	}
	if !exists(f.exe) || len(f.calls) != 0 {
		t.Errorf("nothing should happen: exists=%v calls=%v", exists(f.exe), f.calls)
	}
	if err := f.run("uninstall", "--force"); err != nil || exists(f.exe) {
		t.Errorf("--force: err=%v exists=%v", err, exists(f.exe))
	}
}

func TestUninstallPurgeConfirm(t *testing.T) {
	f := uninstallFake(t)
	f.interactive = true
	f.stdin = "nope\n"
	var exit *ExitError
	if err := f.run("uninstall", "--purge"); !errors.As(err, &exit) {
		t.Fatalf("mismatch: got %v, want exit 1", err)
	}
	if !exists(f.env["IPM_HOME"]) || !strings.Contains(f.out.String(), "nothing deleted") {
		t.Errorf("mismatch should delete nothing:\n%s", f.out.String())
	}
	g := uninstallFake(t)
	g.interactive = true
	g.stdin = g.env["IPM_HOME"] + "\n"
	if err := g.run("uninstall", "--purge"); err != nil {
		t.Fatalf("%v\n%s", err, g.out.String())
	}
	if exists(g.env["IPM_HOME"]) || exists(g.env["IPM_CACHE_DIR"]) {
		t.Error("home and cache should be deleted")
	}
}

func TestUninstallPurgeNeedsYesWhenNotInteractive(t *testing.T) {
	f := uninstallFake(t)
	var usage *UsageError
	if err := f.run("uninstall", "--purge"); !errors.As(err, &usage) {
		t.Fatalf("got %v, want a usage error", err)
	}
	if !exists(f.exe) {
		t.Error("nothing should happen before the usage error")
	}
	f.stdin = "should not be read\n"
	if err := f.run("uninstall", "--purge", "--yes"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if exists(f.env["IPM_HOME"]) || exists(f.env["IPM_CACHE_DIR"]) {
		t.Error("--yes should delete without asking")
	}
}

func TestUninstallWithoutDocker(t *testing.T) {
	f := uninstallFake(t)
	f.onPath["docker"] = false
	if err := f.run("uninstall"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.out.String(), "- docker not available: skipped the search container") || exists(f.exe) {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestUninstallInDotfilesHome(t *testing.T) {
	f := uninstallFake(t)
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".git"), 0o755) // a dotfiles repo
	f.exe = filepath.Join(home, ".local", "bin", "ipm")
	write(t, f.exe, "binary")
	if err := f.run("uninstall"); err != nil || exists(f.exe) {
		t.Errorf("~/.local/bin/ipm under a git home is not a dev build: err=%v exists=%v", err, exists(f.exe))
	}
}
