//go:build integration

package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/testtool"
)

// The search server's container and address, fixed in internal/cli/serve.go.
const (
	tsContainer = "ipm-typesense"
	tsAddr      = "127.0.0.1:8108"
)

// TestServeLoadSearch: real ipm, pnpm and Typesense in Docker, from a local
// git source to a search hit. ipm has one container name and one port, so the
// test refuses to run beside a search server that is already there.
func TestServeLoadSearch(t *testing.T) {
	docker, pnpm, node := testtool.Need(t, "docker"), testtool.Need(t, "pnpm"), testtool.Need(t, "node")
	if err := exec.Command(docker, "inspect", tsContainer).Run(); err == nil {
		t.Fatalf("an %s container already exists — this test would replace it; run it in CI, or remove yours with ipm serve --stop && docker rm %s", tsContainer, tsContainer)
	}
	if c, err := net.DialTimeout("tcp", tsAddr, time.Second); err == nil {
		c.Close()
		t.Fatalf("something already listens on %s — this test needs the port; stop it, or run the test in CI", tsAddr)
	}
	// Cleanups run last first, so the folders are made before the container's.
	home, cache := t.TempDir(), t.TempDir()
	t.Cleanup(func() {
		// The server runs as root and its files land in cache: hand them back
		// so the test can delete them.
		exec.Command(docker, "exec", tsContainer, "chmod", "-R", "a+rwX", "/data").Run()
		exec.Command(docker, "rm", "-f", tsContainer).Run()
	})

	upstream := t.TempDir()
	os.MkdirAll(filepath.Join(upstream, "skills", "one"), 0o755)
	os.WriteFile(filepath.Join(upstream, "skills", "one", "SKILL.md"), []byte("---\nname: one\ndescription: writes tests before code\n---\n"), 0o644)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "one"}} {
		base := []string{"-c", "user.name=t", "-c", "user.email=t@t", "-C", upstream}
		if out, err := exec.Command("git", append(base, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	env := []string{
		"PATH=" + strings.Join([]string{filepath.Dir(pnpm), filepath.Dir(node), filepath.Dir(docker), "/usr/bin", "/bin"}, ":"),
		"HOME=" + t.TempDir(), "IPM_HOME=" + home, "IPM_CACHE_DIR=" + cache,
	}
	// Docker finds its daemon through these when it isn't at the default socket.
	for _, k := range []string{"DOCKER_HOST", "DOCKER_CONTEXT"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ipm %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("source", "add", "file://"+upstream)
	run("sync")
	run("serve")
	run("load")
	for _, args := range [][]string{{"search", "one", "--keyword", "--json"}, {"search", "tests before code", "--json"}} {
		if out := run(args...); !strings.Contains(out, "skills/one") {
			t.Errorf("ipm %v should find skills/one:\n%s", args, out)
		}
	}
}
