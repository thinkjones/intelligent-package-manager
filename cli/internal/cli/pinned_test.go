package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// pinnedFake has pnpm 12 on PATH and the repo o/r synced as a source at
// commit aaaa….
func pinnedFake(t *testing.T) *fake {
	t.Helper()
	f := newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"agents/a.md": "a"}})
	return f
}

var shaOfA, shaOfB = strings.Repeat("a", 40), strings.Repeat("b", 40)

func TestRepoFoldersUsesSyncedSources(t *testing.T) {
	f := pinnedFake(t)
	got, err := repoFolders(f.system(), map[string]string{"o--r": "github:o/r#" + shaOfA})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(f.env["IPM_HOME"], "sources", "node_modules", "o--r"))
	if got["o--r"] != want {
		t.Errorf("folder %q, want %q", got["o--r"], want)
	}
	if c := callWith(f, "pnpm install"); c != "" {
		t.Errorf("no pnpm when the sources hold the pin: %q", c)
	}
}

func TestRepoFoldersFetchesOtherPinsOnce(t *testing.T) {
	f := pinnedFake(t)
	home := f.env["IPM_HOME"]
	pinned := filepath.Join(home, "pinned")
	repos := map[string]string{"o--r": "github:o/r#" + shaOfB}
	if _, err := repoFolders(f.system(), repos); err == nil || !strings.Contains(err.Error(), "o--r--bbbbbbbbbbbb") {
		t.Errorf("pnpm that installs nothing: %v", err)
	}
	c := callWith(f, "pnpm install")
	if !strings.Contains(c, "--dir "+pinned) || !strings.Contains(c, "--store-dir "+filepath.Join(home, "store")) ||
		!strings.Contains(c, "--config.pnpmfile="+filepath.Join(pinned, ".pnpmfile.cjs")) || !strings.Contains(c, "--ignore-scripts") {
		t.Errorf("pnpm call %q", c)
	}
	if !strings.Contains(f.out.String(), "pnpm install --no-frozen-lockfile") {
		t.Errorf("the pnpm command line is printed:\n%s", f.out.String())
	}
	pkg, _ := os.ReadFile(filepath.Join(pinned, "package.json"))
	if !strings.Contains(string(pkg), `"o--r--bbbbbbbbbbbb": "github:o/r#`+shaOfB+`"`) {
		t.Errorf("package.json:\n%s", pkg)
	}
	if _, err := os.Stat(filepath.Join(pinned, ".pnpmfile.cjs")); err != nil {
		t.Error("the hooks must be written to the pin store")
	}

	// pnpm installed it: another build, in any project, runs no pnpm
	write(t, filepath.Join(pinned, "node_modules", "o--r--bbbbbbbbbbbb", "agents", "a.md"), "b")
	f.calls = nil
	got, err := repoFolders(f.system(), repos)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(filepath.Join(pinned, "node_modules", "o--r--bbbbbbbbbbbb")); got["o--r"] != want {
		t.Errorf("folder %q, want %q", got["o--r"], want)
	}
	if c := callWith(f, "pnpm install"); c != "" {
		t.Errorf("no pnpm for a pin already fetched: %q", c)
	}
}

func TestRepoFoldersKeepsEarlierPins(t *testing.T) {
	f := pinnedFake(t)
	pinned := filepath.Join(f.env["IPM_HOME"], "pinned")
	repoFolders(f.system(), map[string]string{"o--r": "github:o/r#" + shaOfB})
	repoFolders(f.system(), map[string]string{"x--y": "github:x/y#" + shaOfA})
	pkg, _ := os.ReadFile(filepath.Join(pinned, "package.json"))
	if !strings.Contains(string(pkg), `"o--r--bbbbbbbbbbbb"`) || !strings.Contains(string(pkg), `"x--y--aaaaaaaaaaaa"`) {
		t.Errorf("package.json:\n%s", pkg)
	}
}

func TestRepoFoldersPnpmFailure(t *testing.T) {
	f := pinnedFake(t)
	f.results["pnpm inst"] = result{"ERR_PNPM_FETCH_404", errors.New("exit 1")}
	_, err := repoFolders(f.system(), map[string]string{"o--r": "github:o/r#" + shaOfB})
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 || !strings.Contains(f.out.String(), "ERR_PNPM_FETCH_404") || !strings.Contains(f.out.String(), "✗ pnpm failed") {
		t.Errorf("got %v\n%s", err, f.out.String())
	}
}

// Two builds wanting the same new pin at once: the pin store's lock lets
// only one run pnpm, and the other finds the pin fetched.
func TestRepoFoldersLocksThePinStore(t *testing.T) {
	f := pinnedFake(t)
	pinned := filepath.Join(f.env["IPM_HOME"], "pinned")
	sys := f.system()
	orig := sys.Output
	var mu sync.Mutex
	installs := 0
	sys.Output = func(dir, name string, args ...string) (string, error) {
		if name == "pnpm" && len(args) > 0 && args[0] == "install" {
			mu.Lock()
			installs++
			mu.Unlock()
			os.MkdirAll(filepath.Join(pinned, "node_modules", "o--r--bbbbbbbbbbbb"), 0o755)
		}
		return orig(dir, name, args...)
	}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := sys
			s.Stdout = io.Discard
			_, errs[i] = repoFolders(s, map[string]string{"o--r": "github:o/r#" + shaOfB})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if installs != 1 {
		t.Errorf("pnpm ran %d times, want 1", installs)
	}
}

// A pin pnpm can't fetch (a commit force-pushed away) must not stay in the
// pin store, or every later fetch would fail on it.
func TestRepoFoldersFailedFetchRestoresPackageJSON(t *testing.T) {
	f := pinnedFake(t)
	pinned := filepath.Join(f.env["IPM_HOME"], "pinned")
	pkg := filepath.Join(pinned, "package.json")
	f.results["pnpm inst"] = result{"ERR_PNPM_FETCH_404", errors.New("exit 1")}
	repoFolders(f.system(), map[string]string{"o--r": "github:o/r#" + shaOfB})
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Errorf("a first failed fetch leaves no package.json: %v", err)
	}

	f.results["pnpm inst"] = result{}
	repoFolders(f.system(), map[string]string{"x--y": "github:x/y#" + shaOfA})
	before, _ := os.ReadFile(pkg)
	f.results["pnpm inst"] = result{"ERR_PNPM_FETCH_404", errors.New("exit 1")}
	repoFolders(f.system(), map[string]string{"o--r": "github:o/r#" + shaOfB})
	if after, _ := os.ReadFile(pkg); string(after) != string(before) {
		t.Errorf("package.json changed:\n%s\nwant:\n%s", after, before)
	}

	f.results["pnpm inst"] = result{}
	repoFolders(f.system(), map[string]string{"z--z": "github:z/z#" + shaOfA})
	if data, _ := os.ReadFile(pkg); strings.Contains(string(data), "o--r--") || !strings.Contains(string(data), "z--z--") {
		t.Errorf("a later fetch must not carry the bad pin:\n%s", data)
	}
}
