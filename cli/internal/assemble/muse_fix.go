package assemble

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// MuseFix is how ipm changes a Claude Code plugin folder so Muse Code
// installs it: Muse checks manifests more strictly than Claude Code, and
// runs only stdio MCP servers.
type MuseFix struct {
	Files   map[string][]byte // slash path → new content
	Drop    []string          // slash paths of folders left out, sorted
	Fixed   []string          // changes that keep what the plugin does
	Dropped []string          // parts left out, which the plugin loses in Muse
}

const claudeManifest = ".claude-plugin/plugin.json"

// FixForMuse reads the plugin folder dir and says what its Muse copy changes;
// it changes nothing. pack is the pack's own plugin, whose dependencies are
// left out, since ipm installs each whole plugin itself. A plugin with its own
// .muse-plugin manifest keeps its manifest and MCP files.
func FixForMuse(dir string, pack bool) (MuseFix, error) {
	fix := MuseFix{Files: map[string][]byte{}}
	_, err := os.Stat(filepath.Join(dir, ".muse-plugin", "plugin.json"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := fix.manifest(dir, pack); err != nil {
			return MuseFix{}, err
		}
	case err != nil:
		return MuseFix{}, err
	}
	if err := fix.markdown(dir); err != nil {
		return MuseFix{}, err
	}
	sort.Strings(fix.Drop)
	return fix, nil
}

// manifest fixes .claude-plugin/plugin.json and the MCP file it names.
func (fix *MuseFix) manifest(dir string, pack bool) error {
	m, err := readJSON(filepath.Join(dir, filepath.FromSlash(claudeManifest)))
	if m == nil {
		return err
	}
	changed := false
	if _, ok := m["dependencies"]; ok && pack {
		delete(m, "dependencies")
		changed = true
	}
	for _, k := range []string{"agents", "commands", "skills"} {
		if s, ok := m[k].(string); ok {
			m[k] = []any{s}
			changed = true
			fix.Fixed = append(fix.Fixed, fmt.Sprintf("`%s` in %s is now a list", k, claudeManifest))
		}
	}
	mcpFile := ".mcp.json"
	switch v := m["mcpServers"].(type) {
	case map[string]any:
		changed = fix.stdioOnly(v, claudeManifest) || changed
	case string:
		mcpFile = path.Clean(strings.TrimPrefix(v, "./"))
		if !filepath.IsLocal(filepath.FromSlash(mcpFile)) {
			mcpFile = ""
		}
	}
	if changed {
		if fix.Files[claudeManifest], err = encodeJSON(m); err != nil {
			return err
		}
	}
	if mcpFile == "" {
		return nil
	}
	return fix.mcp(dir, mcpFile)
}

// mcp fixes an MCP file: wraps a bare map of servers in mcpServers, as
// Claude Code accepts and Muse doesn't, and leaves out remote servers.
func (fix *MuseFix) mcp(dir, rel string) error {
	m, err := readJSON(filepath.Join(dir, filepath.FromSlash(rel)))
	if m == nil {
		return err
	}
	changed := false
	servers, ok := m["mcpServers"].(map[string]any)
	if !ok {
		if _, has := m["mcpServers"]; has || !looksLikeServers(m) {
			return nil
		}
		servers, m, changed = m, map[string]any{"mcpServers": m}, true
		fix.Fixed = append(fix.Fixed, "wrapped "+rel+" in `mcpServers`")
	}
	if fix.stdioOnly(servers, rel) || changed {
		fix.Files[rel], err = encodeJSON(m)
	}
	return err
}

// stdioOnly deletes the servers Muse Code can't run, reporting whether any
// were.
func (fix *MuseFix) stdioOnly(servers map[string]any, file string) bool {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	changed := false
	for _, name := range names {
		s, _ := servers[name].(map[string]any)
		kind := str(s, "type")
		if kind == "" && str(s, "command") == "" && str(s, "url") != "" {
			kind = "http"
		}
		if kind == "" || kind == "stdio" {
			continue
		}
		delete(servers, name)
		changed = true
		fix.Dropped = append(fix.Dropped, fmt.Sprintf("left out MCP server %q (%s): it is %s, and Muse Code runs only stdio servers", name, file, kind))
	}
	return changed
}

// looksLikeServers reports whether every value of m is a server: an object
// with a command or a url.
func looksLikeServers(m map[string]any) bool {
	for _, v := range m {
		s, ok := v.(map[string]any)
		if !ok || str(s, "command") == "" && str(s, "url") == "" {
			return false
		}
	}
	return len(m) > 0
}

// markdown fixes the frontmatter of the plugin's skills, commands and agents
// (see loadRoots), and leaves out a skill with none, or with the name of a
// skill before it: the default folder's first, then each manifest path's.
func (fix *MuseFix) markdown(dir string) error {
	roots, err := loadRoots(dir)
	if err != nil {
		return err
	}
	names := map[string]string{} // skill name → its folder
	seen := map[string]bool{}
	for _, kind := range []string{"skills", "commands", "agents"} {
		for _, root := range roots[kind] {
			err := filepath.WalkDir(filepath.Join(dir, filepath.FromSlash(root)), func(p string, d fs.DirEntry, err error) error {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				if err != nil {
					return err
				}
				if d.IsDir() {
					if n := d.Name(); n == "node_modules" || n == ".git" {
						return filepath.SkipDir
					}
					return nil
				}
				rel, _ := filepath.Rel(dir, p)
				rel = filepath.ToSlash(rel)
				skill := kind == "skills" && d.Name() == "SKILL.md"
				if seen[rel] || !d.Type().IsRegular() || !skill && (kind == "skills" || !strings.HasSuffix(rel, ".md")) {
					return nil
				}
				seen[rel] = true
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				fm, ok := fix.frontmatter(rel, data)
				if skill {
					fix.skill(path.Dir(rel), fm, ok, names)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// skill leaves out the skill in folder when its SKILL.md has no frontmatter
// (hasFM false) or names a skill already in names.
func (fix *MuseFix) skill(folder string, fm map[string]any, hasFM bool, names map[string]string) {
	if !hasFM {
		fix.Drop = append(fix.Drop, folder)
		fix.Dropped = append(fix.Dropped, "left out "+folder+": its SKILL.md has no frontmatter")
		return
	}
	name, _ := fm["name"].(string)
	if name == "" {
		return
	}
	if first, dup := names[name]; dup {
		fix.Drop = append(fix.Drop, folder)
		fix.Dropped = append(fix.Dropped, fmt.Sprintf("left out %s: %s already has the skill name %q", folder, first, name))
		return
	}
	names[name] = folder
}

// loadRoots is where the plugin in dir loads skills, commands and agents
// from: the default folder of each, and the paths its manifest (the Muse one,
// else the Claude one) adds, as slash paths relative to dir.
func loadRoots(dir string) (map[string][]string, error) {
	m, err := readJSON(filepath.Join(dir, ".muse-plugin", "plugin.json"))
	if m == nil && err == nil {
		m, err = readJSON(filepath.Join(dir, filepath.FromSlash(claudeManifest)))
	}
	if err != nil {
		return nil, err
	}
	roots := map[string][]string{}
	for _, k := range []string{"skills", "commands", "agents"} {
		roots[k] = []string{k}
		paths := strs(m[k])
		if s, ok := m[k].(string); ok {
			paths = []string{s}
		}
		for _, p := range paths {
			if p = path.Clean(strings.TrimPrefix(p, "./")); filepath.IsLocal(filepath.FromSlash(p)) {
				roots[k] = append(roots[k], p)
			}
		}
	}
	return roots, nil
}

var fmLine = regexp.MustCompile(`^([A-Za-z0-9_-]+):[ \t]+(.*[^ \t\r])([ \t]*\r?)$`)

// frontmatter parses data's YAML frontmatter, quoting top-level values that
// aren't valid YAML on their own (`argument-hint: [a] [b]`, which Claude Code
// reads as text); ok is false when data has none. A block it can't repair is
// left for Muse to report, and parses as empty.
func (fix *MuseFix) frontmatter(rel string, data []byte) (map[string]any, bool) {
	s := string(data)
	nl := "\n"
	if strings.HasPrefix(s, "---\r\n") {
		nl = "\r\n"
	} else if !strings.HasPrefix(s, "---\n") {
		return nil, false
	}
	start := 3 + len(nl)
	end := start
	if rest := s[start:]; !strings.HasPrefix(rest, "---") {
		i := strings.Index(rest, nl+"---")
		if i < 0 {
			return nil, false
		}
		end += i
	}
	block := s[start:end]
	var m map[string]any
	if yaml.Unmarshal([]byte(block), &m) == nil {
		return m, true
	}
	lines := strings.Split(block, "\n")
	var keys []string
	for i, line := range lines {
		if g := fmLine.FindStringSubmatch(line); g != nil && yaml.Unmarshal([]byte("k: "+g[2]), new(any)) != nil {
			lines[i] = g[1] + ": " + strconv.Quote(g[2]) + g[3]
			keys = append(keys, g[1])
		}
	}
	fixed := strings.Join(lines, "\n")
	if len(keys) == 0 || yaml.Unmarshal([]byte(fixed), &m) != nil {
		return map[string]any{}, true
	}
	fix.Files[rel] = []byte(s[:start] + fixed + s[end:])
	fix.Fixed = append(fix.Fixed, "quoted "+strings.Join(keys, ", ")+" in "+rel)
	return m, true
}

// WriteMuseCopy makes dst the Muse copy of the plugin folder src: every
// regular file hard linked (copied across volumes), except fix's rewritten
// files, written fresh, and its left-out folders. src is never changed.
func WriteMuseCopy(src, dst string, fix MuseFix) error {
	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	drop := map[string]bool{}
	for _, d := range fix.Drop {
		drop[d] = true
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && drop[rel] {
			return filepath.SkipDir
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		to := filepath.Join(dst, filepath.FromSlash(rel))
		data, ok := fix.Files[rel]
		if !ok {
			return put(p, to, os.Link)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		return os.WriteFile(to, data, info.Mode().Perm())
	})
	return err
}

// readJSON reads a JSON object; it is nil, with no error, when the file is
// missing or not an object, which Muse reports itself.
func readJSON(p string) (map[string]any, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil, nil
	}
	return m, nil
}

// encodeJSON is m as indented JSON with sorted keys and <, >, & unescaped.
func encodeJSON(m map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(m)
	return buf.Bytes(), err
}
