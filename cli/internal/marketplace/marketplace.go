// Package marketplace builds and writes a Claude Code marketplace.json:
// entries whose source points at an upstream repo — `github` for a whole
// repo, `git-subdir` (a sparse checkout) for a path, `url` for a non-GitHub
// git URL — pinned to a commit. Every entry carries OwnerTag, which is how an
// existing file is recognised as ipm's to overwrite. Part 3 of the ipm design
// (packs and `ipm pack build`) builds on it.
package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
	Schema      string `json:"$schema"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       Owner  `json:"owner"`
	// Plugins are Entry values, or upstream entries as maps.
	Plugins []any `json:"plugins"`
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
func SourceFor(repo, path, version, commit string) Source {
	var src Source
	switch {
	case path != "":
		src = Source{Source: "git-subdir", URL: repo, Path: path}
	case ownerRepo.MatchString(repo):
		src = Source{Source: "github", Repo: repo}
	default:
		src = Source{Source: "url", URL: repo}
	}
	if version != "" && !commitRE.MatchString(version) {
		src.Ref = version
	}
	src.SHA = commit
	return src
}

// Build assembles the marketplace.
func Build(entries []any, packNames []string, o Options) Marketplace {
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
		_ = os.Remove(tmp)
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
