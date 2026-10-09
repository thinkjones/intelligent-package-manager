package catalog

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Found is every marker in a checkout, as repo-relative slash paths, each
// list sorted. Directories for skills and plugins; files for the rest.
type Found struct {
	Skills, Plugins, CopilotPlugins, Marketplaces []string
	Agents, Commands, HookFiles, MCPFiles         []string
}

// skip is folders never walked.
var skip = map[string]bool{".git": true, "node_modules": true}

// Walk finds every marker under dir. Symlinks are not followed.
func Walk(dir string) (Found, error) {
	var f Found
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		name, parent := d.Name(), path.Dir(rel)
		switch {
		case name == "SKILL.md":
			f.Skills = append(f.Skills, parent)
		case name == "plugin.json" && path.Base(parent) == ".claude-plugin":
			f.Plugins = append(f.Plugins, path.Dir(parent))
		case name == "marketplace.json" && path.Base(parent) == ".claude-plugin":
			f.Marketplaces = append(f.Marketplaces, path.Dir(parent))
		case name == "plugin.json" && strings.HasSuffix("/"+parent, "/.github/plugin"):
			f.CopilotPlugins = append(f.CopilotPlugins, path.Dir(path.Dir(parent)))
		case name == "plugin.json" && !strings.HasPrefix(path.Base(parent), "."):
			f.CopilotPlugins = append(f.CopilotPlugins, parent)
		case name == "hooks.json" && path.Base(parent) == "hooks":
			f.HookFiles = append(f.HookFiles, rel)
		case name == ".mcp.json":
			f.MCPFiles = append(f.MCPFiles, rel)
		case strings.HasSuffix(name, ".md") && !strings.EqualFold(name, "README.md"):
			switch {
			case under(rel, "agents"):
				f.Agents = append(f.Agents, rel)
			case path.Base(parent) == "commands":
				f.Commands = append(f.Commands, rel)
			}
		}
		return nil
	})
	for _, l := range []*[]string{&f.Skills, &f.Plugins, &f.CopilotPlugins, &f.Marketplaces, &f.Agents, &f.Commands, &f.HookFiles, &f.MCPFiles} {
		*l = slices.Compact(slices.Sorted(slices.Values(*l)))
	}
	return f, err
}

// under reports whether rel lies at any depth inside a folder named dir.
func under(rel, dir string) bool {
	return slices.Contains(strings.Split(path.Dir(rel), "/"), dir)
}
