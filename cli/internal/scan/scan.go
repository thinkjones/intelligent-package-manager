// Package scan reports what a cached upstream checkout holds that a pack
// could publish: Claude Code plugins, Copilot plugins, marketplaces, and
// skills, hooks and agents folders. It reads the checkout only.
package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind is what a found folder is.
type Kind string

const (
	Plugin        Kind = "plugin"         // .claude-plugin/plugin.json
	CopilotPlugin Kind = "copilot-plugin" // plugin.json at the folder's root
	Marketplace   Kind = "marketplace"    // .claude-plugin/marketplace.json
	Skills        Kind = "skills"
	Hooks         Kind = "hooks"
	Agents        Kind = "agents"
)

// Kinds is every kind, in report order.
var Kinds = []Kind{Plugin, CopilotPlugin, Marketplace, Skills, Hooks, Agents}

// Find is one folder of interest.
type Find struct {
	Kind Kind
	// Path is relative to the checkout, slash separated; "." is its root.
	Path string
	// Items counts what the folder holds: SKILL.md files at any depth under
	// skills, agent .md files under agents, files under hooks. Zero for
	// plugins and marketplaces. A skills, hooks or agents folder holding none
	// is not reported.
	Items int
}

// skip is folders never worth walking.
var skip = map[string]bool{".git": true, "node_modules": true}

// Dir scans the checkout at dir, sorted by kind order then path.
func Dir(dir string) ([]Find, error) {
	var finds []Find
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if !d.IsDir() {
			switch {
			case strings.HasSuffix("/"+rel, "/.claude-plugin/plugin.json"):
				finds = append(finds, Find{Kind: Plugin, Path: parent(parent(rel))})
			case strings.HasSuffix("/"+rel, "/.claude-plugin/marketplace.json"):
				finds = append(finds, Find{Kind: Marketplace, Path: parent(parent(rel))})
			case d.Name() == "plugin.json" && !strings.HasPrefix(filepath.Base(filepath.Dir(path)), "."):
				finds = append(finds, Find{Kind: CopilotPlugin, Path: parent(rel)})
			}
			return nil
		}
		if skip[d.Name()] {
			return filepath.SkipDir
		}
		var items int
		switch d.Name() {
		case "skills":
			items = skillCount(path)
		case "hooks":
			items = count(path, func(e fs.DirEntry) bool { return !e.IsDir() })
		case "agents":
			items = count(path, func(e fs.DirEntry) bool {
				return !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !strings.EqualFold(e.Name(), "README.md")
			})
		default:
			return nil
		}
		// An empty folder of the name holds nothing of its kind — an agents/
		// folder of Codex config inside a skill, say.
		if items > 0 {
			finds = append(finds, Find{Kind: Kind(d.Name()), Path: rel, Items: items})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	order := map[Kind]int{}
	for i, k := range Kinds {
		order[k] = i
	}
	sort.Slice(finds, func(i, j int) bool {
		if finds[i].Kind != finds[j].Kind {
			return order[finds[i].Kind] < order[finds[j].Kind]
		}
		return finds[i].Path < finds[j].Path
	})
	return finds, nil
}

// parent is rel's folder; "." for the checkout root.
func parent(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return "."
}

// skillCount is how many SKILL.md files are under dir at any depth — skills
// may be grouped in category folders — leaving out nested skills folders,
// which are reported on their own.
func skillCount(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case d.IsDir() && path != dir && (skip[d.Name()] || d.Name() == "skills"):
			return filepath.SkipDir
		case !d.IsDir() && d.Name() == "SKILL.md":
			n++
		}
		return nil
	})
	return n
}

// count is how many of dir's entries match.
func count(dir string, match func(fs.DirEntry) bool) int {
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if match(e) {
			n++
		}
	}
	return n
}

// Count is how many finds there are of each kind.
func Count(finds []Find) map[Kind]int {
	n := map[Kind]int{}
	for _, f := range finds {
		n[f.Kind]++
	}
	return n
}
