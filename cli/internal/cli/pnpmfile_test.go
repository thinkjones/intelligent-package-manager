package cli

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/testtool"
)

// pnpm 12 can't write a lockfile entry for a package without a name
// (OpenBMB/ChatDev's package.json holds only devDependencies), so the hook
// gives one to manifests that lack it.
func TestPnpmfileNamesNamelessPackages(t *testing.T) {
	testtool.Need(t, "node")
	path, err := writePnpmfile(t.TempDir(), "ipm-sources-x")
	if err != nil {
		t.Fatal(err)
	}
	js := `const {readPackage} = require(process.argv[1]).hooks
console.log(JSON.stringify([
  readPackage({devDependencies: {"kill-port": "^2.0.1"}}),
  readPackage({name: "tdd-guard", version: "1.7.0", dependencies: {a: "1"}}),
  readPackage({name: "ipm-sources-x", dependencies: {"o--r": "github:o/r"}}),
]))`
	out, err := exec.Command("node", "-e", js, path).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := `[{"devDependencies":{"kill-port":"^2.0.1"},"name":"unnamed","version":"0.0.0","dependencies":{},"optionalDependencies":{},"peerDependencies":{}},` +
		`{"name":"tdd-guard","version":"1.7.0","dependencies":{},"optionalDependencies":{},"peerDependencies":{}},` +
		`{"name":"ipm-sources-x","dependencies":{"o--r":"github:o/r"}}]`
	if got := strings.TrimSpace(string(out)); got != want {
		t.Errorf("readPackage =\n%s, want\n%s", got, want)
	}
}
