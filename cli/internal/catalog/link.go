package catalog

// Plugin ownership follows the Claude Code plugin manifest reference
// (code.claude.com/docs/en/plugins-reference, "How each key combines with its
// default location"), confirmed 2026-10-09:
//
//   - skills     adds to the default skills/ scan; a skills root holds skill
//     dirs directly (<name>/SKILL.md), or is one itself; "." is the plugin
//     root. A plugin with SKILL.md at its root, no skills/ and no skills key
//     is one skill.
//   - agents     replaces the default agents/ (scanned at any depth); .md
//     files only.
//   - commands   replaces the default commands/*.md; paths (files or dirs) or
//     an object map of name → {source} or {content}.
//   - hooks      merges with hooks/hooks.json; a path, inline object, or array.
//   - mcpServers merges with .mcp.json; a .json path, inline map, or array;
//     .mcpb/.dxt bundles and bundle URLs are not cataloged.

import (
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/pack"
)

// RepoResult is one repo's records, the warnings raised indexing it, and the
// external repos its marketplaces reference.
type RepoResult struct {
	Records  []Record
	Warnings []string
	Sources  []string
}

// repoIndex is the state of indexing one checkout.
type repoIndex struct {
	c     Checkout
	found Found
	recs  map[string]Record
	warns []string
}

// put adds r unless its ID is taken; a different record under a taken ID is
// a warning, never a silent drop.
func (x *repoIndex) put(r Record) {
	prev, ok := x.recs[r.ID]
	if !ok {
		x.recs[r.ID] = r
		return
	}
	if prev.ContentHash != r.ContentHash || prev.Name != r.Name {
		x.warn("%s: defined more than once; kept the first", r.ID)
	}
}

func (x *repoIndex) warn(format string, a ...any) {
	x.warns = append(x.warns, fmt.Sprintf(format, a...))
}

// exists reports whether rel is inside the checkout once symlinks are
// resolved; one that leaves it is a warning.
func (x *repoIndex) exists(rel string) bool {
	_, err := x.c.path(rel)
	if errors.Is(err, errLeaves) {
		x.warn("%s:%s: %v", x.c.Repo, rel, err)
	}
	return err == nil
}

// IndexRepo walks a checkout, parses every marker, and links each plugin to
// the components it owns.
func IndexRepo(c Checkout) (RepoResult, error) {
	found, err := Walk(c.Dir)
	if err != nil {
		return RepoResult{}, err
	}
	x := &repoIndex{c: c, found: found, recs: map[string]Record{}}
	for _, d := range found.Skills {
		x.put(ParseSkill(c, d))
	}
	for _, f := range found.Agents {
		x.put(ParseAgent(c, f))
	}
	for _, f := range found.Commands {
		x.put(ParseCommand(c, f))
	}
	for _, f := range found.HookFiles {
		x.put(ParseHookFile(c, f))
	}
	for _, f := range found.MCPFiles {
		x.parseMCP(f)
	}
	for _, d := range found.CopilotPlugins {
		x.put(ParseCopilotPlugin(c, d))
	}
	for _, d := range found.Plugins {
		r, comps := ParsePlugin(c, d)
		for key, ids := range x.own(r.ID, d, comps, false) {
			r.Fields[key] = ids
		}
		x.put(r)
	}
	sources := x.marketplaces()
	out := RepoResult{Warnings: x.warns, Sources: sources}
	for _, id := range slices.Sorted(func(yield func(string) bool) {
		for k := range x.recs {
			if !yield(k) {
				return
			}
		}
	}) {
		out.Records = append(out.Records, x.recs[id])
	}
	return out, nil
}

func (x *repoIndex) parseMCP(file string) {
	recs, warns := ParseMCPFile(x.c, file)
	x.warns = append(x.warns, warns...)
	for _, r := range recs {
		x.put(r)
	}
}

// resolve joins a plugin-relative path onto the plugin's dir; false when it
// leaves the repo.
func (x *repoIndex) resolve(owner, dir, p string) (string, bool) {
	j := path.Clean(path.Join(dir, p))
	if path.IsAbs(p) || j == ".." || strings.HasPrefix(j, "../") {
		x.warn("%s: path %s leaves the repo", owner, p)
		return "", false
	}
	return j, true
}

// own resolves a plugin's component paths and returns the IDs it owns per
// component key: skills, agents, commands, hooks, mcp. noDefaultSkills skips
// the default skills/ scan (a marketplace entry at the marketplace root that
// lists its skills).
func (x *repoIndex) own(owner, dir string, comps Components, noDefaultSkills bool) map[string][]string {
	c := x.c
	pluginJSON := path.Join(dir, ".claude-plugin", "plugin.json")
	owned := map[string]map[string]bool{"skills": {}, "agents": {}, "commands": {}, "hooks": {}, "mcp": {}}
	add := func(key, id string) { owned[key][id] = true }

	// skills: the default skills/ plus every declared root.
	var roots []string
	if !noDefaultSkills {
		roots = append(roots, path.Join(dir, "skills"))
	}
	for _, p := range comps.Skills {
		if j, ok := x.resolve(owner, dir, p); ok {
			roots = append(roots, j)
		}
	}
	for _, s := range x.found.Skills {
		for _, root := range roots {
			if s == root || path.Dir(s) == root {
				add("skills", c.id(TypeSkill, s, ""))
			}
		}
	}
	if len(owned["skills"]) == 0 && !comps.Declared("skills") && slices.Contains(x.found.Skills, dir) {
		add("skills", c.id(TypeSkill, dir, ""))
	}

	// agents: declared .md files replace the default agents/ tree.
	if comps.Declared("agents") {
		for _, p := range comps.Agents {
			j, ok := x.resolve(owner, dir, p)
			switch {
			case !ok:
			case !strings.HasSuffix(j, ".md"):
				x.warn("%s: agents path %s is not a .md file", owner, p)
			case !x.exists(j):
				x.warn("%s: agents path not found: %s", owner, p)
			default:
				x.put(ParseAgent(c, j))
				add("agents", c.id(TypeAgent, j, ""))
			}
		}
	} else {
		for _, f := range x.found.Agents {
			if strings.HasPrefix(f, path.Join(dir, "agents")+"/") {
				add("agents", c.id(TypeAgent, f, ""))
			}
		}
	}

	// commands: declared paths or map replace the default commands/.
	if comps.Declared("commands") {
		for _, p := range comps.Commands {
			if j, ok := x.resolve(owner, dir, p); ok {
				for _, f := range x.mdFiles(owner, j, p) {
					x.put(ParseCommand(c, f))
					add("commands", c.id(TypeCommand, f, ""))
				}
			}
		}
		for name, v := range comps.CommandMap {
			m, _ := v.(map[string]any)
			if src := str(m, "source"); src != "" {
				if j, ok := x.resolve(owner, dir, src); ok && x.exists(j) {
					x.put(ParseCommand(c, j))
					add("commands", c.id(TypeCommand, j, ""))
				} else if ok {
					x.warn("%s: commands path not found: %s", owner, src)
				}
				continue
			}
			r := c.record(TypeCommand, pluginJSON, name)
			r.Name, r.Description, r.Meta = name, str(m, "description"), m
			if b, err := encode(m); err == nil {
				r.ContentHash = ContentHash(b)
			}
			r.Fields = map[string]any{"plugins": []string{}}
			x.put(r)
			add("commands", r.ID)
		}
	} else {
		for _, f := range x.found.Commands {
			if path.Dir(f) == path.Join(dir, "commands") {
				add("commands", c.id(TypeCommand, f, ""))
			}
		}
	}

	// hooks: the default file merged with every declared file and inline object.
	if def := path.Join(dir, "hooks", "hooks.json"); x.exists(def) {
		x.put(ParseHookFile(c, def))
		add("hooks", c.id(TypeHook, def, ""))
	}
	inline := map[string]any{}
	for _, v := range items(comps.Hooks) {
		switch t := v.(type) {
		case string:
			if j, ok := x.resolve(owner, dir, t); ok && x.exists(j) {
				x.put(ParseHookFile(c, j))
				add("hooks", c.id(TypeHook, j, ""))
			} else if ok {
				x.warn("%s: hooks path not found: %s", owner, t)
			}
		case map[string]any:
			for k, e := range t {
				inline[k] = e
			}
		}
	}
	if len(inline) > 0 {
		r := InlineHook(c, pluginJSON, inline)
		x.put(r)
		add("hooks", r.ID)
	}

	// mcp: the default .mcp.json merged with every declared file and map.
	if def := path.Join(dir, ".mcp.json"); x.exists(def) { // parsed by the walk
		for id := range x.recs {
			if strings.HasPrefix(id, c.id(TypeMCP, def, "")+"#") {
				add("mcp", id)
			}
		}
	}
	for _, v := range items(comps.MCP) {
		switch t := v.(type) {
		case string:
			if strings.HasSuffix(t, ".mcpb") || strings.HasSuffix(t, ".dxt") || strings.HasPrefix(t, "https://") {
				x.warn("%s: MCP bundle %s not cataloged", owner, t)
				continue
			}
			if j, ok := x.resolve(owner, dir, t); ok && x.exists(j) {
				recs, warns := ParseMCPFile(c, j)
				x.warns = append(x.warns, warns...)
				for _, r := range recs {
					x.put(r)
					add("mcp", r.ID)
				}
			} else if ok {
				x.warn("%s: mcpServers path not found: %s", owner, t)
			}
		case map[string]any:
			for _, r := range InlineMCP(c, pluginJSON, t) {
				x.put(r)
				add("mcp", r.ID)
			}
		}
	}

	out := map[string][]string{}
	for key, set := range owned {
		ids := []string{}
		for id := range set {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		out[key] = ids
	}
	return out
}

// items is v as a list: nil → none, an array → its elements, else itself.
func items(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	}
	return []any{v}
}

// mdFiles is rel itself when it is a file, else its direct *.md children.
func (x *repoIndex) mdFiles(owner, rel, declared string) []string {
	if !x.exists(rel) {
		x.warn("%s: path not found: %s", owner, declared)
		return nil
	}
	real, _ := x.c.path(rel)
	info, err := os.Stat(real)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []string{rel}
	}
	entries, _ := os.ReadDir(real)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !strings.EqualFold(e.Name(), "README.md") {
			out = append(out, path.Join(rel, e.Name()))
		}
	}
	return out
}

// set replaces a record already indexed.
func (x *repoIndex) set(r Record) { x.recs[r.ID] = r }

// marketplaces resolves every marketplace's entries — to a local plugin, an
// inline plugin, or an external plugin — and returns the external repos
// referenced, normalized, sorted.
func (x *repoIndex) marketplaces() []string {
	c := x.c
	sources := map[string]bool{}
	for _, d := range x.found.Marketplaces {
		m, entries, warns := ParseMarketplace(c, d)
		x.warns = append(x.warns, warns...)
		base := d
		if root, _ := m.Fields["plugin_root"].(string); root != "" {
			if j, ok := x.resolve(m.ID, d, root); ok {
				base = j
			}
		}
		var listed []string
		for _, e := range entries {
			id := ""
			switch src := e.Source.(type) {
			case string:
				id = x.localEntry(m.ID, d, base, src, e)
			case map[string]any:
				id = x.externalEntry(m.ID, e, src, sources)
			default:
				x.warn("%s: entry %q has a source aihs cannot read", m.ID, e.Name)
			}
			if id != "" && !slices.Contains(listed, id) {
				listed = append(listed, id)
			}
		}
		if listed != nil {
			m.Fields["plugins"] = listed
		}
		x.put(m)
	}
	return slices.Sorted(func(yield func(string) bool) {
		for s := range sources {
			if !yield(s) {
				return
			}
		}
	})
}

// localEntry links an entry whose source is a path in this repo: to the
// plugin there, or to an inline plugin the entry defines.
func (x *repoIndex) localEntry(mid, mdir, base, src string, e Entry) string {
	c := x.c
	dir, ok := x.resolve(mid, base, src)
	if !ok {
		return ""
	}
	comps := components(e.Raw)
	if x.exists(path.Join(dir, ".claude-plugin", "plugin.json")) {
		id := c.id(TypePlugin, dir, "")
		r, found := x.recs[id]
		if !found {
			return ""
		}
		r.Fields["entry"] = e.Raw
		if strict, set := e.Raw["strict"].(bool); !set || strict {
			// A strict entry appends what it declares; marking every path
			// key declared keeps own from rescanning the defaults.
			only := Components{Skills: comps.Skills, Agents: comps.Agents, Commands: comps.Commands,
				CommandMap: comps.CommandMap, Hooks: inlineOnly(comps.Hooks),
				declared: map[string]bool{"skills": true, "agents": true, "commands": true}}
			for key, ids := range x.own(id, dir, only, true) {
				prev, _ := r.Fields[key].([]string)
				r.Fields[key] = slices.Compact(slices.Sorted(slices.Values(append(prev, ids...))))
			}
		}
		x.set(r)
		return id
	}
	if !x.exists(dir) {
		x.warn("%s: entry %q: source %s not found", mid, e.Name, src)
		return ""
	}
	// The entry name is part of the ID: several entries may share one
	// source folder (e.g. "./" with different skills lists).
	r := c.record(TypePlugin, dir, e.Name)
	r.Name = e.Name
	r.Fields = map[string]any{}
	describe(&r, e.Raw)
	r.Fields["entry"] = e.Raw
	if b, err := encode(e.Raw); err == nil {
		r.ContentHash = ContentHash(b)
	}
	comps.Hooks = inlineOnly(comps.Hooks)
	for key, ids := range x.own(r.ID, dir, comps, dir == mdir && comps.Declared("skills")) {
		r.Fields[key] = ids
	}
	x.put(r)
	return r.ID
}

// inlineOnly keeps the inline objects of a hooks value: a marketplace entry
// may declare hooks only inline.
func inlineOnly(v any) any {
	var out []any
	for _, h := range items(v) {
		if m, ok := h.(map[string]any); ok {
			out = append(out, m)
		}
	}
	if out == nil {
		return nil
	}
	return out
}

// externalEntry records a plugin that lives in another repo.
func (x *repoIndex) externalEntry(mid string, e Entry, src map[string]any, sources map[string]bool) string {
	c := x.c
	r := Record{ID: TypePlugin + ":" + c.Repo + "#" + e.Name, Type: TypePlugin, Name: e.Name, Repo: c.Repo, Fields: map[string]any{}}
	describe(&r, e.Raw)
	r.Name = e.Name
	r.Meta = nil
	r.Fields["entry"] = e.Raw
	if b, err := encode(e.Raw); err == nil {
		r.ContentHash = ContentHash(b)
	}
	repo := str(src, "repo")
	if repo == "" {
		repo = str(src, "url")
	}
	if repo != "" {
		repo = pack.NormalizeRepo(repo)
		r.Fields["source_repo"] = repo
		sources[repo] = true
	} else {
		x.warn("%s: entry %q: external source names no repo or url", mid, e.Name)
	}
	x.put(r)
	return r.ID
}
