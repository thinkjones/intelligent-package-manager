package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

var pinnedSHA = regexp.MustCompile(`#([0-9a-f]{40})$`)

// repoFolders resolves each repo a build reads (alias → pnpm spec pinned to a
// commit) to a folder holding it at that commit: the synced source when it is
// at that commit, else the pin store ($IPM_HOME/pinned), one pnpm project every
// scope shares, keyed <alias>--<sha12>. pnpm runs once, under the pin store's
// lock, for the pins neither holds; a failed run prints pnpm's output and
// exits 1.
func repoFolders(sys System, repos map[string]string) (map[string]string, error) {
	return findRepoFolders(sys, repos, true)
}

// findRepoFolders is repoFolders; without fetch it runs no pnpm and names a
// pin not fetched yet by its place in the pin store.
func findRepoFolders(sys System, repos map[string]string, fetch bool) (map[string]string, error) {
	p, err := ipmPaths(sys)
	if err != nil {
		return nil, err
	}
	synced, err := sources.Installed(p.Sources())
	if err != nil {
		return nil, err
	}
	at := map[string]string{} // alias@commit → folder
	for _, c := range synced {
		at[c.Alias+"@"+c.Commit] = c.Dir
	}
	out := map[string]string{}
	want := map[string]string{} // pin key → spec, for repos the sources lack
	keys := map[string]string{} // alias → pin key
	for alias, spec := range repos {
		m := pinnedSHA.FindStringSubmatch(spec)
		if m == nil {
			return nil, fmt.Errorf("%s: %q is not pinned to a commit", alias, spec)
		}
		if dir, ok := at[alias+"@"+m[1]]; ok {
			out[alias] = dir
			continue
		}
		key := alias + "--" + m[1][:12]
		want[key], keys[alias] = spec, key
	}
	if len(want) == 0 {
		return out, nil
	}
	store := p.Pinned()
	fetched := func(key string) (string, bool) {
		dir, err := filepath.EvalSymlinks(filepath.Join(store, "node_modules", key))
		return dir, err == nil
	}
	if !fetch {
		for alias, key := range keys {
			if dir, ok := fetched(key); ok {
				out[alias] = dir
			} else {
				out[alias] = filepath.Join(store, "node_modules", key)
			}
		}
		return out, nil
	}
	missing := func() map[string]string {
		m := map[string]string{}
		for key, spec := range want {
			if _, ok := fetched(key); !ok {
				m[key] = spec
			}
		}
		return m
	}
	// Unlocked: a pin pnpm has installed is never removed, so when every pin
	// is already there no build can be changing them.
	if len(missing()) > 0 {
		if err := os.MkdirAll(store, 0o755); err != nil {
			return nil, err
		}
		unlock, err := lockFile(filepath.Join(store, ".lock"))
		if err != nil {
			return nil, err
		}
		defer unlock()
		// Another build may have fetched them while this one waited.
		if m := missing(); len(m) > 0 {
			if err := fetchPins(sys, store, p.Store(), m); err != nil {
				return nil, err
			}
		}
	}
	for alias, key := range keys {
		dir, ok := fetched(key)
		if !ok {
			return nil, fmt.Errorf("pnpm did not install %s (%s) in %s", key, want[key], filepath.Join(store, "node_modules"))
		}
		out[alias] = dir
	}
	return out, nil
}

// fetchPins adds pins (key → spec) to the pin store's package.json, keeping
// the ones there, and has pnpm install them with the hooks and flags
// ipm catalog sync uses. If pnpm fails, package.json goes back to what it
// was, so a pin pnpm can't fetch doesn't break every later fetch.
func fetchPins(sys System, dir, pnpmStore string, pins map[string]string) error {
	pn, err := findPnpm(sys)
	if err != nil {
		return err
	}
	pkg := filepath.Join(dir, "package.json")
	before, err := os.ReadFile(pkg)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	had := err == nil
	restore := func() error {
		if !had {
			return os.Remove(pkg)
		}
		return os.WriteFile(pkg, before, 0o644)
	}
	root, err := sources.AddPins(dir, pins)
	if err != nil {
		return err
	}
	hooks, err := writePnpmfile(dir, root)
	if err != nil {
		return err
	}
	var keys []string
	for k := range pins {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(sys.Stdout, "fetching %s into %s\n", strings.Join(keys, ", "), dir)
	args := []string{"install", "--no-frozen-lockfile", "--ignore-scripts", "--config.ignore-pnpmfile=false", "--config.pnpmfile=" + hooks,
		"--store-dir", pnpmStore, "--dir", dir}
	fmt.Fprintf(sys.Stdout, "%s %s\n", pn, strings.Join(args, " "))
	if out, err := sys.Output(dir, pn.name, append(append([]string{}, pn.prefix...), args...)...); err != nil {
		fmt.Fprintln(sys.Stdout, strings.TrimSpace(out))
		fmt.Fprintf(sys.Stdout, "%s pnpm failed\n", bad)
		if err := restore(); err != nil {
			return fmt.Errorf("pnpm failed, and %s could not be restored: %v", pkg, err)
		}
		return &ExitError{Code: 1}
	}
	return nil
}
