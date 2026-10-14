package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInstallGascityPassesGcLint proves the folder is one Gas City accepts. It
// needs the real gc, which CI lacks, so it skips instead of failing when gc is
// not on PATH.
func TestInstallGascityPassesGcLint(t *testing.T) {
	gc, err := exec.LookPath("gc")
	if err != nil {
		t.Skip("gc is not on PATH")
	}
	f, _, tmp := gascityFake(t)
	if err := f.run("install", "gascity", "kit", "-C", tmp); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	folder := filepath.Join(tmp, "kit")
	cmd := exec.Command(gc, "lint", folder)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gc lint %s: %v\n%s", folder, err, out)
	}
}
