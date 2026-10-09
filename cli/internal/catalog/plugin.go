package catalog

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/thinkjones/fs-skills/cli/internal/copilot"
)

// Components is what a plugin.json, or a marketplace entry acting as one,
// declares. Paths are plugin-relative, as written; a string becomes a
// one-element list. Hooks and MCP are nil, a path, an inline object, or an
// array mixing them.
type Components struct {
	Skills, Agents, Commands []string
	// CommandMap is commands given as an object map: name → {source|content}.
	CommandMap map[string]any
	Hooks, MCP any
	declared   map[string]bool
}

// Declared reports whether key ("skills", "agents", "commands", "hooks",
// "mcpServers") was set.
func (c Components) Declared(key string) bool { return c.declared[key] }

func components(m map[string]any) Components {
	c := Components{declared: map[string]bool{}}
	for _, k := range []string{"skills", "agents", "commands", "hooks", "mcpServers"} {
		if _, ok := m[k]; ok {
			c.declared[k] = true
		}
	}
	c.Skills, c.Agents = strs(m["skills"]), strs(m["agents"])
	if cm, ok := m["commands"].(map[string]any); ok {
		c.CommandMap = cm
	} else {
		c.Commands = strs(m["commands"])
	}
	c.Hooks, c.MCP = m["hooks"], m["mcpServers"]
	return c
}

// ParsePlugin reads dir/.claude-plugin/plugin.json.
func ParsePlugin(c Checkout, dir string) (Record, Components) {
	r := c.record(TypePlugin, dir, "")
	r.Name = stem(c, dir)
	r.Fields = map[string]any{}
	data, err := c.read(path.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		r.Errors = []string{err.Error()}
		return r, components(nil)
	}
	r.ContentHash = ContentHash(data)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		r.Meta = map[string]any{"raw": string(data)}
		r.Errors = []string{fmt.Sprintf("json: %v", err)}
		return r, components(nil)
	}
	describe(&r, m)
	return r, components(m)
}

// describe fills a plugin record from a plugin.json or marketplace entry.
func describe(r *Record, m map[string]any) {
	r.Meta = m
	if n := str(m, "name"); n != "" {
		r.Name = n
	}
	r.Description = str(m, "description")
	r.Tags = append(append(strs(m["keywords"]), strs(m["category"])...), strs(m["tags"])...)
	for _, k := range []string{"version", "author", "license"} {
		if v, ok := m[k]; ok {
			r.Fields[k] = v
		}
	}
}

// ParseCopilotPlugin reads a Copilot plugin's plugin.json and the skills its
// extension lists.
func ParseCopilotPlugin(c Checkout, dir string) Record {
	r := c.record(TypeCopilotPlugin, dir, "")
	r.Name = stem(c, dir)
	r.Fields = map[string]any{"skills": []string{}}
	file := path.Join(dir, "plugin.json")
	data, err := c.read(file)
	if err != nil {
		file = path.Join(dir, ".github", "plugin", "plugin.json")
		data, err = c.read(file)
	}
	if err != nil {
		r.Errors = []string{err.Error()}
		return r
	}
	r.ContentHash = ContentHash(data)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		r.Meta = map[string]any{"raw": string(data)}
		r.Errors = []string{fmt.Sprintf("json: %v", err)}
		return r
	}
	r.Meta = m
	if n := str(m, "name"); n != "" {
		r.Name = n
	}
	r.Description = str(m, "description")
	r.Tags = strs(m["keywords"])
	paths, err := copilot.SkillPaths(c.Dir, dir)
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	var ids []string
	for _, p := range paths {
		ids = append(ids, c.id(TypeSkill, p, ""))
	}
	if ids != nil {
		r.Fields["skills"] = ids
	}
	return r
}

// Entry is one plugin listed in a marketplace.json.
type Entry struct {
	Name   string
	Source any
	Raw    map[string]any
}

// ParseMarketplace reads dir/.claude-plugin/marketplace.json. Its record's
// meta leaves out the plugins array: each entry is kept on the plugin record
// it resolves to.
func ParseMarketplace(c Checkout, dir string) (Record, []Entry, []string) {
	r := c.record(TypeMarketplace, dir, "")
	r.Name = stem(c, dir)
	r.Fields = map[string]any{"plugins": []string{}}
	data, err := c.read(path.Join(dir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		r.Errors = []string{err.Error()}
		return r, nil, nil
	}
	r.ContentHash = ContentHash(data)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		r.Meta = map[string]any{"raw": string(data)}
		r.Errors = []string{fmt.Sprintf("json: %v", err)}
		return r, nil, nil
	}
	if n := str(m, "name"); n != "" {
		r.Name = n
	}
	meta, _ := m["metadata"].(map[string]any)
	r.Description = str(meta, "description")
	if r.Description == "" {
		r.Description = str(m, "description")
	}
	if root := str(meta, "pluginRoot"); root != "" {
		r.Fields["plugin_root"] = root
	}
	r.Meta = map[string]any{}
	for k, v := range m {
		if k != "plugins" {
			r.Meta[k] = v
		}
	}
	var entries []Entry
	var warns []string
	list, _ := m["plugins"].([]any)
	for i, x := range list {
		e, _ := x.(map[string]any)
		name, source := str(e, "name"), e["source"]
		switch {
		case name == "":
			warns = append(warns, fmt.Sprintf("%s: entry %d has no name", r.ID, i))
		case source == nil:
			warns = append(warns, fmt.Sprintf("%s: entry %q has no source", r.ID, name))
		default:
			entries = append(entries, Entry{Name: name, Source: source, Raw: e})
		}
	}
	return r, entries, warns
}
