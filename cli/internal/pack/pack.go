// Package pack reads the packs at a checkout root.
//
// A pack is a directory at the root holding exactly one file, manifest.toml.
// A pack never holds content of its own: every [[plugin]] names a Claude Code
// plugin or skills that live upstream, and [[copilot-plugin]] names a GitHub
// Copilot plugin whose skills are published upstream as they are, unmodified.
// [pack] describes the pack and may include other packs.
//
// Versions follow npm's git-dependency convention: `version` is a tag,
// branch or commit (npm's "#commit-ish"); omitted, it is the default branch.
// aihs.lock records the commit each one resolved to, as package-lock does.
//
// The same plugin may be listed by several packs, as long as every listing
// defines it identically (description aside) — it is published once.
package pack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestFile is the file that makes a directory a pack.
const ManifestFile = "manifest.toml"

// nameRE is what Claude Code accepts in a plugin or marketplace name.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidName reports whether s is usable as a plugin or marketplace name.
func ValidName(s string) bool { return nameRE.MatchString(s) }

// Reserved reports whether Claude Code refuses s as a third-party plugin
// name because it passes as one of Anthropic's own (`claude plugin validate`
// fails it). Case and separators are ignored, as Claude Code does.
func Reserved(s string) bool {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	if len(words) == 0 {
		return false
	}
	joined := strings.Join(words, "-")
	switch joined {
	case "claude", "anthropic", "anthropics", "claude-code", "claude-mods":
		return true
	}
	switch words[0] {
	case "claude", "anthropic", "anthropics":
		return true
	}
	if strings.HasPrefix(joined, "cc-plugin-") {
		return true
	}
	for i := 0; i+1 < len(words); i++ {
		pair := [2]string{words[i], words[i+1]}
		for _, brand := range []string{"claude", "anthropic"} {
			if pair == [2]string{"official", brand} || pair == [2]string{brand, "official"} {
				return true
			}
		}
	}
	return false
}

// Upstream is one repo at one version that packs read from. ipm sync pins
// each to a commit in aihs.lock.
type Upstream struct {
	// Repo is a GitHub owner/repo, or a full git URL.
	Repo string
	// Version is a tag, branch or full commit SHA; empty means the default
	// branch.
	Version string
}

// Key identifies the upstream in aihs.lock, npm style: repo@version.
func (u Upstream) Key() string {
	if u.Version == "" {
		return u.Repo
	}
	return u.Repo + "@" + u.Version
}

// Upstream is where the plugin comes from.
func (pl Plugin) Upstream() Upstream { return Upstream{pl.Repo, pl.Version} }

// Upstreams is every upstream the packs read from, each once, sorted by key.
func Upstreams(packs []Pack) []Upstream {
	seen := map[string]Upstream{}
	for _, p := range packs {
		for _, pl := range p.Plugins {
			u := pl.Upstream()
			seen[u.Key()] = u
		}
		for _, c := range p.Copilot() {
			u := c.Upstream()
			seen[u.Key()] = u
		}
	}
	out := make([]Upstream, 0, len(seen))
	for _, u := range seen {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

// Plugin is one [[plugin]] table: an upstream Claude Code plugin, or
// upstream skills or agents published as one.
type Plugin struct {
	// Name is the plugin id; for an upstream plugin, its own plugin.json name.
	Name string `toml:"name"`
	// Repo is a GitHub owner/repo, or a full git URL.
	Repo string `toml:"repo"`
	// Path is the subdirectory to fetch; empty means the whole repo.
	Path string `toml:"path"`
	// Version is a tag, branch or commit; empty means the default branch.
	Version string `toml:"version"`
	// Skills are skill dirs relative to Path ("./" is Path itself); empty
	// means the upstream plugin as-is.
	Skills []string `toml:"skills"`
	// Agents are agent files relative to Path, each under an agents/ folder
	// — Claude Code loads plugin agents from nowhere else; empty means the
	// plugin's own.
	Agents []string `toml:"agents"`
	// Dependencies are other plugins in the marketplace this one needs.
	Dependencies []string `toml:"dependencies"`
	// Description is shown in /plugin before install.
	Description string `toml:"description"`
}

// Kind is the GitHub Copilot format of a Copilot item.
type Kind string

// CopilotPlugin is an agent-plugins.org plugin.json.
const CopilotPlugin Kind = "copilot-plugin"

// CopilotItem is a GitHub Copilot plugin, published upstream as it is:
// package copilot expands it into plugin references, never copies.
type CopilotItem struct {
	Kind Kind `toml:"-"`
	// Name overrides the derived plugin name.
	Name string `toml:"name"`
	Repo string `toml:"repo"`
	// Path is the directory holding plugin.json.
	Path    string `toml:"path"`
	Version string `toml:"version"`
	// Description overrides the upstream description.
	Description string `toml:"description"`
}

// Upstream is where the item comes from.
func (c CopilotItem) Upstream() Upstream { return Upstream{c.Repo, c.Version} }

func (t CopilotItem) validate() error {
	switch {
	case t.Repo == "":
		return errors.New("`repo` is required")
	case t.Path == "":
		return errors.New("`path` is required")
	case t.Name != "" && (!ValidName(t.Name) || Reserved(t.Name)):
		return fmt.Errorf("name %q is not a valid plugin name", t.Name)
	}
	return nil
}

// Meta is the optional [pack] table.
type Meta struct {
	Description string `toml:"description"`
	Version     string `toml:"version"`
	// Includes names other packs this one builds on; installing this pack
	// installs them too.
	Includes []string `toml:"includes"`
}

// Pack is one loaded, validated pack.
type Pack struct {
	Name    string
	Dir     string
	Meta    Meta
	Plugins []Plugin

	CopilotPlugins []CopilotItem
}

// Copilot is every Copilot item in the pack.
func (p Pack) Copilot() []CopilotItem { return p.CopilotPlugins }

type manifest struct {
	Pack           Meta          `toml:"pack"`
	Plugins        []Plugin      `toml:"plugin"`
	CopilotPlugins []CopilotItem `toml:"copilot-plugin"`
}

// allowed is what a pack directory may hold: its manifest, and nothing else
// besides files the OS leaves behind.
func allowed(name string) bool { return name == ManifestFile || name == ".DS_Store" }

// Load reads and validates the pack in dir.
func Load(dir string) (Pack, error) {
	path := filepath.Join(dir, ManifestFile)
	var m manifest
	md, err := toml.DecodeFile(path, &m)
	if err != nil {
		return Pack{}, fmt.Errorf("%s: %v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return Pack{}, fmt.Errorf("%s: unknown key(s) %s", path, strings.Join(keys, ", "))
	}
	p := Pack{Name: filepath.Base(dir), Dir: dir, Meta: m.Pack, Plugins: m.Plugins}
	for i, pl := range p.Plugins {
		if err := pl.validate(); err != nil {
			return Pack{}, fmt.Errorf("%s: plugin[%d]: %v", path, i, err)
		}
	}
	for _, g := range []struct {
		kind  Kind
		items []CopilotItem
		into  *[]CopilotItem
	}{
		{CopilotPlugin, m.CopilotPlugins, &p.CopilotPlugins},
	} {
		for i, t := range g.items {
			if err := t.validate(); err != nil {
				return Pack{}, fmt.Errorf("%s: %s[%d]: %v", path, g.kind, i, err)
			}
			t.Kind = g.kind
			*g.into = append(*g.into, t)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Pack{}, err
	}
	for _, e := range entries {
		if !allowed(e.Name()) {
			return Pack{}, fmt.Errorf("%s: a pack holds only %s — %s does not belong here; reference content upstream instead", dir, ManifestFile, e.Name())
		}
	}
	if !ValidName(p.Name) || Reserved(p.Name) {
		return Pack{}, fmt.Errorf("%s: pack name %q is not a valid plugin name", path, p.Name)
	}
	return p, nil
}

func (pl Plugin) validate() error {
	switch {
	case pl.Name == "":
		return errors.New("`name` is required")
	case !ValidName(pl.Name):
		return fmt.Errorf("name %q may use only letters, digits, '.', '_' and '-'", pl.Name)
	case Reserved(pl.Name):
		return fmt.Errorf("name %q is reserved by Claude Code for Anthropic's own plugins — name it for what it does", pl.Name)
	case pl.Repo == "":
		return errors.New("`repo` is required")
	}
	for _, s := range slices.Concat(pl.Skills, pl.Agents) {
		if !strings.HasPrefix(s, ".") {
			return fmt.Errorf("path %q must start with ./", s)
		}
	}
	return nil
}

// Dirs is every pack directory under root, sorted.
func Dirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, e.Name(), ManifestFile)); err == nil {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
	}
	return dirs, nil
}

// LoadAll loads every pack under root, or only the named ones.
func LoadAll(root string, names []string) ([]Pack, error) {
	var dirs []string
	if len(names) > 0 {
		for _, n := range names {
			dir := filepath.Join(root, n)
			if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
				return nil, fmt.Errorf("no pack %q: %s does not exist", n, filepath.Join(dir, ManifestFile))
			}
			dirs = append(dirs, dir)
		}
	} else {
		var err error
		if dirs, err = Dirs(root); err != nil {
			return nil, err
		}
	}
	packs := make([]Pack, 0, len(dirs))
	for _, d := range dirs {
		p, err := Load(d)
		if err != nil {
			return nil, err
		}
		packs = append(packs, p)
	}
	return packs, nil
}

// Empty reports whether the pack publishes nothing.
func (p Pack) Empty() bool {
	return len(p.Plugins) == 0 && len(p.Meta.Includes) == 0 && len(p.Copilot()) == 0
}

// same reports whether two listings define the same plugin. Description is
// presentation only, so packs may word it differently.
func (pl Plugin) same(o Plugin) bool {
	return pl.Repo == o.Repo && pl.Path == o.Path && pl.Version == o.Version &&
		slices.Equal(pl.Skills, o.Skills) && slices.Equal(pl.Agents, o.Agents) && slices.Equal(pl.Dependencies, o.Dependencies)
}

// Problems reports what stops packs being published together: a plugin name
// defined differently by two packs, a pack named like a plugin, and an
// include of a pack that is missing, itself, or part of a cycle.
func Problems(packs []Pack) []string {
	var problems []string
	byPack := map[string]Pack{}
	for _, p := range packs {
		byPack[p.Name] = p
	}
	first := map[string]Plugin{}
	firstIn := map[string]string{}
	for _, p := range packs {
		for _, pl := range p.Plugins {
			prev, seen := first[pl.Name]
			switch {
			case !seen:
				first[pl.Name], firstIn[pl.Name] = pl, p.Name
			case !prev.same(pl):
				problems = append(problems, fmt.Sprintf("plugin %q is defined differently in %s and %s", pl.Name, firstIn[pl.Name], p.Name))
			}
			if _, clash := byPack[pl.Name]; clash {
				problems = append(problems, fmt.Sprintf("plugin %q in %s has the same name as a pack", pl.Name, p.Name))
			}
		}
	}
	for _, p := range packs {
		for _, inc := range p.Meta.Includes {
			if _, ok := byPack[inc]; !ok {
				problems = append(problems, fmt.Sprintf("%s includes %q, which is not a pack being published", p.Name, inc))
			}
		}
	}
	if cycle := includeCycle(byPack); cycle != "" {
		problems = append(problems, "includes form a cycle: "+cycle)
	}
	return slices.Compact(problems)
}

// includeCycle returns one include cycle as "a → b → a", or "".
func includeCycle(byPack map[string]Pack) string {
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	var path []string
	var visit func(string) string
	visit = func(name string) string {
		switch state[name] {
		case visiting:
			i := slices.Index(path, name)
			return strings.Join(append(slices.Clone(path[i:]), name), " → ")
		case done:
			return ""
		}
		state[name] = visiting
		path = append(path, name)
		for _, inc := range byPack[name].Meta.Includes {
			if _, ok := byPack[inc]; ok {
				if c := visit(inc); c != "" {
					return c
				}
			}
		}
		path = path[:len(path)-1]
		state[name] = done
		return ""
	}
	names := make([]string, 0, len(byPack))
	for n := range byPack {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if c := visit(n); c != "" {
			return c
		}
	}
	return ""
}
