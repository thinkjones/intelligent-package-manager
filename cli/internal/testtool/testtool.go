// Package testtool finds the programs a test needs outside Go: pnpm, node,
// docker.
package testtool

import (
	"os"
	"os/exec"
	"testing"
)

// Strict is the environment variable that turns a missing tool from a skip
// into a failure. The mise tasks set it, so CI can't pass having run nothing.
const Strict = "IPM_TEST_STRICT"

// Need returns the path of the program called name. Without it the test is
// skipped, or fails when IPM_TEST_STRICT is 1.
func Need(t testing.TB, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err == nil {
		return path
	}
	if os.Getenv(Strict) == "1" {
		t.Fatalf("%s is required when %s=1", name, Strict)
	} else {
		t.Skipf("%s is not installed", name)
	}
	return ""
}
