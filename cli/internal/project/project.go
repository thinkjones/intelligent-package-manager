// Package project is a scope ipm builds plugins for: a project (the nearest
// folder holding ipm.toml) or the user's global scope ($IPM_HOME/global). It
// reads and writes ipm.toml, the packs you pick, and .ipm/ipm.lock, every
// picked item resolved to a repo, path and commit.
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
)

const (
	// File holds a scope's packs.
	File = "ipm.toml"
	// LockFile, in the .ipm folder, resolves every picked item.
	LockFile = "ipm.lock"
	// StampFile, in the .ipm folder, records what the last build was made from.
	StampFile = "build.stamp"
)

// Scope is where packs are read and plugins built.
type Scope struct {
	Root   string
	Global bool
}

// IPMDir is the folder ipm generates into.
func (s Scope) IPMDir() string { return filepath.Join(s.Root, ".ipm") }

// Find is the scope for a command run in cwd: with global, home/global;
// otherwise the nearest ancestor of cwd holding ipm.toml. found reports
// whether that ipm.toml exists; when it doesn't, Root is cwd (or
// home/global), where `ipm pack new` creates one.
func Find(cwd string, global bool, home string) (Scope, bool, error) {
	if global {
		root := filepath.Join(home, "global")
		return Scope{Root: root, Global: true}, exists(filepath.Join(root, File)), nil
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if exists(filepath.Join(dir, File)) {
			return Scope{Root: dir}, true, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return Scope{Root: cwd}, false, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Pack is one [packs.<name>] table.
type Pack struct {
	Name        string
	Description string
	Items       []string
	Installs    []Install
}

// Install is one place a pack was installed: a Claude Code scope or a Gas City
// pack folder. Path is relative to the project root when inside it.
type Install struct {
	Target string `toml:"target"`
	Path   string `toml:"path"`
	Scope  string `toml:"scope"`
	Name   string `toml:"name"`
}

type packsFile struct {
	Packs map[string]struct {
		Description string    `toml:"description"`
		Items       []string  `toml:"items"`
		Installs    []Install `toml:"installs"`
	} `toml:"packs"`
}

// PacksDigest is a hex SHA-256 over each pack's name, description and items,
// in name order. Installs are left out, so recording one keeps a build fresh.
func PacksDigest(packs []Pack) string {
	sorted := append([]Pack(nil), packs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	for _, p := range sorted {
		fmt.Fprintf(h, "pack %q %q %d\n", p.Name, p.Description, len(p.Items))
		for _, it := range p.Items {
			fmt.Fprintf(h, "%q\n", it)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// LoadPacks reads root's ipm.toml, packs sorted by name; no file is none.
func LoadPacks(root string) ([]Pack, error) {
	path := filepath.Join(root, File)
	var f packsFile
	md, err := toml.DecodeFile(path, &f)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s: unknown key(s) %s", path, strings.Join(keys, ", "))
	}
	var out []Pack
	for name, p := range f.Packs {
		// The name becomes a folder under .ipm/plugins, so a teammate's
		// ipm.toml must not be able to name one elsewhere.
		if !pack.ValidName(name) || pack.Reserved(name) {
			return nil, fmt.Errorf("%s: pack name %q must start with a letter or digit, use only letters, digits, '.', '_' and '-', and not be one Claude Code reserves", path, name)
		}
		out = append(out, Pack{Name: name, Description: p.Description, Items: p.Items, Installs: p.Installs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// tomlKey is name as a TOML key: bare when it can be, else quoted (a pack
// name may hold '.').
func tomlKey(name string) string {
	if bareKey.MatchString(name) {
		return name
	}
	return strconv.Quote(name)
}

// SavePacks writes root's ipm.toml: packs sorted by name, items in order.
func SavePacks(root string, packs []Pack) error {
	sorted := append([]Pack(nil), packs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var b strings.Builder
	b.WriteString("# ipm packs: each becomes a Claude Code plugin under .ipm/plugins/ when you run `ipm pack build`.\n")
	for _, p := range sorted {
		fmt.Fprintf(&b, "\n[packs.%s]\ndescription = %q\nitems = [\n", tomlKey(p.Name), p.Description)
		for _, it := range p.Items {
			fmt.Fprintf(&b, "  %q,\n", it)
		}
		b.WriteString("]\n")
		for _, in := range p.Installs {
			fmt.Fprintf(&b, "\n[[packs.%s.installs]]\ntarget = %q\npath = %q\n", tomlKey(p.Name), in.Target, in.Path)
			if in.Scope != "" {
				fmt.Fprintf(&b, "scope = %q\n", in.Scope)
			}
			if in.Name != "" {
				fmt.Fprintf(&b, "name = %q\n", in.Name)
			}
		}
	}
	return writeFile(filepath.Join(root, File), b.String())
}

// LockItem is one picked item, resolved. Source is set for whole plugins:
// the marketplace source (JSON) their entry uses. Parts is a local whole
// plugin's child item IDs, sorted: they share its repo and commit. Nil means
// not recorded (an old lock); empty non-nil means recorded, none.
type LockItem struct {
	ID     string   `toml:"id"`
	Type   string   `toml:"type"`
	Name   string   `toml:"name"`
	Repo   string   `toml:"repo"`
	Path   string   `toml:"path"`
	SHA    string   `toml:"sha"`
	Source string   `toml:"source"`
	Parts  []string `toml:"parts"`
}

// LoadLock reads dir/ipm.lock, keyed by item ID; no file is empty.
func LoadLock(dir string) (map[string]LockItem, error) {
	out := map[string]LockItem{}
	var f struct {
		Item []LockItem `toml:"item"`
	}
	path := filepath.Join(dir, LockFile)
	if _, err := toml.DecodeFile(path, &f); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	for _, it := range f.Item {
		out[it.ID] = it
	}
	return out, nil
}

// SaveLock writes dir/ipm.lock, items sorted by ID.
func SaveLock(dir string, items map[string]LockItem) error {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("# Generated by `ipm pack build` — commit it. Every picked item, resolved to a repo, path and commit.\n")
	for _, id := range ids {
		it := items[id]
		fmt.Fprintf(&b, "\n[[item]]\nid = %q\ntype = %q\nname = %q\nrepo = %q\n", it.ID, it.Type, it.Name, it.Repo)
		if it.Path != "" {
			fmt.Fprintf(&b, "path = %q\n", it.Path)
		}
		if it.SHA != "" {
			fmt.Fprintf(&b, "sha = %q\n", it.SHA)
		}
		if it.Source != "" {
			fmt.Fprintf(&b, "source = %q\n", it.Source)
		}
		if it.Parts != nil {
			quoted := make([]string, len(it.Parts))
			for i, p := range it.Parts {
				quoted[i] = fmt.Sprintf("%q", p)
			}
			fmt.Fprintf(&b, "parts = [%s]\n", strings.Join(quoted, ", "))
		}
	}
	return writeFile(filepath.Join(dir, LockFile), b.String())
}

func writeFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
