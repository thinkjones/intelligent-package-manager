// Package marketplace builds and writes a Claude Code marketplace.json from
// loaded packs.
//
// Each plugin becomes an entry whose source points at its upstream repo — a
// `github` source for a whole repo, `git-subdir` (a sparse checkout) when a
// path is set, `url` for a non-GitHub git URL — pinned to the commit in
// aihs.lock. Nothing is copied: Claude Code fetches the plugin at install
// time. A plugin listed by several packs is published once.
//
// Every pack also becomes a stack entry named after it, referenced in place
// ("./<pack>", which holds only manifest.toml): it has no components of its
// own and depends on the pack's plugins and on the packs it includes, so
// installing it installs the whole stack. The file lives at
// <root>/.claude-plugin/marketplace.json because Claude Code resolves
// relative sources against that root and forbids "../".
package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

const Schema = "https://anthropic.com/claude-code/marketplace.schema.json"

// OwnerTag is on every entry; it is how an existing marketplace.json is
// recognised as ours and safe to overwrite.
const OwnerTag = "ipm"

// legacyOwnerTag marked entries written while ipm was called aihs.
const legacyOwnerTag = "aihs"

// Path is where the marketplace lives under a checkout root.
func Path(root string) string {
	return filepath.Join(root, ".claude-plugin", "marketplace.json")
}

type Marketplace struct {
	Schema      string  `json:"$schema"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Owner       Owner   `json:"owner"`
	Plugins     []Entry `json:"plugins"`
}

type Owner struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// Entry is one plugin entry. Source is a Source for an upstream plugin, or a
// "./<pack>" string for a pack's stack entry.
type Entry struct {
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Source       any       `json:"source"`
	Category     string    `json:"category"`
	Tags         []string  `json:"tags"`
	Strict       *bool     `json:"strict,omitempty"`
	Skills       []string  `json:"skills,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Commands     *[]string `json:"commands,omitempty"`
	Agents       *[]string `json:"agents,omitempty"`
	Version      string    `json:"version,omitempty"`

	// Pack is the pack the entry came from; not written.
	Pack string `json:"-"`
}

type Source struct {
	Source string `json:"source"`
	Repo   string `json:"repo,omitempty"`
	URL    string `json:"url,omitempty"`
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`
	SHA    string `json:"sha,omitempty"`
}

// Options are the marketplace-level values.
type Options struct {
	Name        string
	Description string
	Owner       string
	RepoURL     string
}

var ownerRepo = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

var commitRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// SourceFor maps a plugin onto a Claude Code plugin source at commit. A
// version that names a branch or tag is kept as `ref`, for readers.
func SourceFor(pl pack.Plugin, commit string) Source {
	var s Source
	switch {
	case pl.Path != "":
		s = Source{Source: "git-subdir", URL: pl.Repo, Path: pl.Path}
	case ownerRepo.MatchString(pl.Repo):
		s = Source{Source: "github", Repo: pl.Repo}
	default:
		s = Source{Source: "url", URL: pl.Repo}
	}
	if pl.Version != "" && !commitRE.MatchString(pl.Version) {
		s.Ref = pl.Version
	}
	s.SHA = commit
	return s
}

// Entries is every entry the packs publish, in pack order: each pack's stack
// entry, then those of its plugins not already published by an earlier pack,
// each pinned to its commit in lock. Callers check pack.Problems first: a
// name defined two ways would be published as the first definition.
func Entries(packs []pack.Pack, lock map[string]string) []Entry {
	var out []Entry
	published := map[string]bool{}
	for _, p := range packs {
		tags := []string{OwnerTag, p.Name}
		out = append(out, stack(p, tags))
		for _, pl := range p.Plugins {
			if published[pl.Name] {
				continue
			}
			published[pl.Name] = true
			e := Entry{
				Name:         pl.Name,
				Description:  pl.Description,
				Source:       SourceFor(pl, lock[pl.Upstream().Key()]),
				Category:     p.Name,
				Tags:         tags,
				Skills:       pl.Skills,
				Dependencies: pl.Dependencies,
				Pack:         p.Name,
			}
			if len(pl.Agents) > 0 {
				agents := pl.Agents
				e.Agents = &agents
			}
			out = append(out, e)
		}
	}
	return out
}

// stack is the entry for the pack itself: a dependency on every plugin it
// lists and every pack it includes.
func stack(p pack.Pack, tags []string) Entry {
	var parts []string
	if n := len(p.Plugins); n > 0 {
		parts = append(parts, plural(n, "plugin"))
	}
	if len(p.Meta.Includes) > 0 {
		parts = append(parts, "includes "+strings.Join(p.Meta.Includes, ", "))
	}
	desc := p.Meta.Description
	if desc == "" {
		desc = "The " + p.Name + " pack"
	}
	desc = fmt.Sprintf("%s (%s)", desc, strings.Join(parts, "; "))

	var deps []string
	for _, pl := range p.Plugins {
		deps = append(deps, pl.Name)
	}
	deps = append(deps, p.Meta.Includes...)

	// The pack directory holds only manifest.toml; strict:false with empty
	// component lists states that the stack itself contributes nothing.
	strict, none := false, []string{}
	return Entry{
		Name:         p.Name,
		Description:  desc,
		Source:       "./" + p.Name,
		Category:     p.Name,
		Tags:         tags,
		Strict:       &strict,
		Dependencies: deps,
		Commands:     &none,
		Agents:       &none,
		Version:      p.Meta.Version,
		Pack:         p.Name,
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// Build assembles the marketplace.
func Build(entries []Entry, packNames []string, o Options) Marketplace {
	owner := Owner{Name: o.Owner, URL: o.RepoURL}
	desc := o.Description
	if desc == "" {
		desc = "Plugins from " + strings.Join(packNames, ", ")
	}
	return Marketplace{Schema: Schema, Name: o.Name, Description: desc, Owner: owner, Plugins: entries}
}

// IsOurs reports whether the marketplace.json at path was written by ipm:
// every entry carries OwnerTag. A missing or unparseable file is not ours.
func IsOurs(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m struct {
		Plugins []struct {
			Tags []string `json:"tags"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &m) != nil || len(m.Plugins) == 0 {
		return false
	}
	for _, p := range m.Plugins {
		if !slices.Contains(p.Tags, OwnerTag) && !slices.Contains(p.Tags, legacyOwnerTag) {
			return false
		}
	}
	return true
}

// Write writes m to path atomically: a temp file beside it, then a rename, so
// an interrupted write leaves the previous file intact.
func Write(path string, m Marketplace) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Describe is a one-line summary of an entry's source for terminal output.
func Describe(src any) string {
	s, ok := src.(Source)
	if !ok {
		return fmt.Sprint(src)
	}
	where := s.Repo + s.URL
	if s.Path != "" {
		where += "//" + s.Path
	}
	if s.Ref != "" {
		where += "@" + s.Ref
	}
	if s.SHA != "" {
		where += "@" + s.SHA
	}
	return where
}
