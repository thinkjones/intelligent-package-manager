package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Checkout is one repo of interest at its locked commit, cached in Dir.
type Checkout struct{ Repo, Commit, Dir string }

// errLeaves is a path that, once symlinks are resolved, is outside the
// checkout. Upstreams are third-party: nothing outside may reach the catalog.
var errLeaves = errors.New("leaves the checkout")

// path is rel's real location, symlinks resolved, and an error wrapping
// errLeaves when that is outside the checkout.
func (c Checkout) path(rel string) (string, error) {
	real, err := filepath.EvalSymlinks(filepath.Join(c.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	base, err := filepath.EvalSymlinks(c.Dir)
	if err != nil {
		return "", err
	}
	if r, err := filepath.Rel(base, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %w", rel, errLeaves)
	}
	return real, nil
}

func (c Checkout) read(rel string) ([]byte, error) {
	real, err := c.path(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(real)
}

// id is "<type>:<repo>:<path>", with "#<name>" appended when name is set.
func (c Checkout) id(typ, rel, name string) string {
	id := typ + ":" + c.Repo + ":" + rel
	if name != "" {
		id += "#" + name
	}
	return id
}

func (c Checkout) record(typ, rel, name string) Record {
	return Record{ID: c.id(typ, rel, name), Type: typ, Repo: c.Repo, Commit: c.Commit, Path: rel}
}

// ContentHash is the sha256 of data with CRLF normalized to LF.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	return hex.EncodeToString(sum[:])
}

// splitFrontmatter returns the YAML between a leading "---" line and the
// next "---" line, CRLF normalized.
func splitFrontmatter(data []byte) (string, bool) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", false
	}
	rest := text[len("---\n"):]
	if strings.HasPrefix(rest, "---\n") || rest == "---" {
		return "", true
	}
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if !strings.HasSuffix(rest, "\n---") {
			return "", false
		}
		end = len(rest) - len("\n---")
	}
	return rest[:end], true
}

// Frontmatter parses a Markdown file's YAML frontmatter into plain JSON
// types: maps with string keys, timestamps as RFC 3339 strings.
func Frontmatter(data []byte) (map[string]any, bool, error) {
	raw, found := splitFrontmatter(data)
	if !found {
		return nil, false, nil
	}
	var v any
	if err := yaml.Unmarshal([]byte(raw), &v); err != nil {
		return nil, true, err
	}
	switch m := plain(v).(type) {
	case nil:
		return map[string]any{}, true, nil
	case map[string]any:
		return m, true, nil
	}
	return nil, true, fmt.Errorf("frontmatter is not a mapping")
}

// plain converts YAML-decoded values to what encoding/json writes stably.
func plain(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = plain(x)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[fmt.Sprint(k)] = plain(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = plain(x)
		}
		return out
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return fmt.Sprint(t) // JSON has no NaN or Inf
		}
	}
	return v
}

// lenient reads top-level "key: value" lines, values as plain strings.
func lenient(raw string) map[string]any {
	m := map[string]any{}
	for line := range strings.SplitSeq(raw, "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok && !strings.ContainsAny(k, " \t") {
			m[k] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return m
}

// stem is a fallback name: the last path segment without ".md", or the
// repo's name for the repo root.
func stem(c Checkout, rel string) string {
	if rel == "." {
		return path.Base(c.Repo)
	}
	return strings.TrimSuffix(path.Base(rel), ".md")
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func strs(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// markdown parses a frontmatter-described file into r: meta, name,
// description, hash. A missing file or bad YAML lands in r.Errors.
func markdown(c Checkout, r *Record, file string, fallback string) bool {
	r.Name = fallback
	data, err := c.read(file)
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
		return false
	}
	r.ContentHash = ContentHash(data)
	meta, found, err := Frontmatter(data)
	switch {
	case err != nil:
		// Strict YAML failed (often an unquoted colon in a description), but
		// Claude Code still loads such files, so read plain key: value lines
		// and keep the error visible.
		raw, _ := splitFrontmatter(data)
		meta = lenient(raw)
		meta["raw"] = raw
		r.Errors = append(r.Errors, fmt.Sprintf("frontmatter: %v", err))
	case !found:
		return false
	}
	r.Meta = meta
	if n := str(meta, "name"); n != "" {
		r.Name = n
	}
	r.Description = str(meta, "description")
	return true
}

// ParseSkill reads dir/SKILL.md.
func ParseSkill(c Checkout, dir string) Record {
	r := c.record(TypeSkill, dir, "")
	if !markdown(c, &r, path.Join(dir, "SKILL.md"), stem(c, dir)) && len(r.Errors) == 0 {
		r.Errors = []string{"no frontmatter"}
	}
	r.Tags = append(strs(r.Meta["category"]), tagsOf(r.Meta["metadata"])...)
	r.Fields = map[string]any{"plugins": []string{}}
	return r
}

func tagsOf(metadata any) []string {
	m, _ := metadata.(map[string]any)
	return strs(m["tags"])
}

// ParseAgent reads an agent's .md file.
func ParseAgent(c Checkout, file string) Record {
	r := c.record(TypeAgent, file, "")
	markdown(c, &r, file, stem(c, file))
	r.Fields = map[string]any{"plugins": []string{}}
	return r
}

// ParseCommand reads a command's .md file.
func ParseCommand(c Checkout, file string) Record {
	r := c.record(TypeCommand, file, "")
	markdown(c, &r, file, stem(c, file))
	r.Fields = map[string]any{"plugins": []string{}}
	return r
}

// ParseHookFile reads a hooks file: {"hooks": {<event>: [...]}}.
func ParseHookFile(c Checkout, file string) Record {
	r := c.record(TypeHook, file, "")
	data, err := c.read(file)
	if err != nil {
		r.Errors = []string{err.Error()}
		return hook(r, stem(c, path.Dir(file)), nil)
	}
	r.ContentHash = ContentHash(data)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		r.Meta = map[string]any{"raw": string(data)}
		r.Errors = []string{fmt.Sprintf("json: %v", err)}
		return hook(r, stem(c, path.Dir(file)), nil)
	}
	r.Meta = doc
	events, ok := doc["hooks"].(map[string]any)
	if !ok {
		r.Errors = []string{`no "hooks" object`}
	}
	return hook(r, stem(c, path.Dir(file)), events)
}

// InlineHook is the hooks declared inline in a plugin.json.
func InlineHook(c Checkout, pluginJSON string, hooks map[string]any) Record {
	r := c.record(TypeHook, pluginJSON, "")
	r.Meta = hooks
	if b, err := encode(hooks); err == nil {
		r.ContentHash = ContentHash(b)
	}
	return hook(r, stem(c, path.Dir(path.Dir(pluginJSON))), hooks)
}

func hook(r Record, name string, events map[string]any) Record {
	r.Name = name
	names := slices.Sorted(func(yield func(string) bool) {
		for k := range events {
			if !yield(k) {
				return
			}
		}
	})
	if names == nil {
		names = []string{}
	}
	r.Tags = names
	r.Description = "events: " + strings.Join(names, ", ")
	r.Fields = map[string]any{"events": names, "plugins": []string{}}
	return r
}

// ParseMCPFile reads a .mcp.json: servers under "mcpServers", or a
// top-level map of servers. One record per server.
func ParseMCPFile(c Checkout, file string) ([]Record, []string) {
	data, err := c.read(file)
	if err != nil {
		return nil, []string{fmt.Sprintf("%s:%s: %v", c.Repo, file, err)}
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, []string{fmt.Sprintf("%s:%s: %v", c.Repo, file, err)}
	}
	if servers, ok := doc["mcpServers"].(map[string]any); ok {
		doc = servers
	}
	return mcp(c, file, doc), nil
}

// InlineMCP is the MCP servers declared inline in a plugin.json.
func InlineMCP(c Checkout, pluginJSON string, servers map[string]any) []Record {
	return mcp(c, pluginJSON, servers)
}

func mcp(c Checkout, file string, servers map[string]any) []Record {
	var out []Record
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for k := range servers {
			if !yield(k) {
				return
			}
		}
	}) {
		cfg, ok := servers[name].(map[string]any)
		if !ok {
			continue
		}
		r := c.record(TypeMCP, file, name)
		r.Name, r.Meta = name, cfg
		if b, err := encode(cfg); err == nil {
			r.ContentHash = ContentHash(b)
		}
		transport := str(cfg, "type")
		switch {
		case transport != "":
		case cfg["command"] != nil:
			transport = "stdio"
		case cfg["url"] != nil:
			transport = "http"
		}
		r.Fields = map[string]any{"transport": transport, "plugins": []string{}}
		if cmd := str(cfg, "command"); cmd != "" {
			r.Fields["command"] = cmd
			args, _ := cfg["args"].([]any)
			if args == nil {
				args = []any{}
			}
			r.Fields["args"] = args
			r.Description = strings.TrimSpace(transport + ": " + strings.Join(append([]string{cmd}, strs(cfg["args"])...), " "))
		} else if u := str(cfg, "url"); u != "" {
			r.Fields["url"] = u
			r.Description = transport + ": " + u
		}
		out = append(out, r)
	}
	return out
}
