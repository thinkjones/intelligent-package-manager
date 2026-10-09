// Package catalog extracts deterministic metadata about every repo,
// marketplace, plugin, skill, agent, command, hook and MCP server in the
// cached repos of interest, and writes it as sorted JSONL under catalog/.
//
// The same commits always produce byte-identical files, so `git diff` of
// catalog/ is the history of what changed upstream. See
// docs/superpowers/specs/2026-10-08-artifact-catalog-design.md.
package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Artifact types, as they appear in a record's "type" and ID prefix.
const (
	TypeRepo          = "repo"
	TypeMarketplace   = "marketplace"
	TypePlugin        = "plugin"
	TypeCopilotPlugin = "copilot-plugin"
	TypeSkill         = "skill"
	TypeAgent         = "agent"
	TypeCommand       = "command"
	TypeHook          = "hook"
	TypeMCP           = "mcp"
)

// Types is every type, in report order.
var Types = []string{TypeRepo, TypeMarketplace, TypePlugin, TypeCopilotPlugin, TypeSkill, TypeAgent, TypeCommand, TypeHook, TypeMCP}

// Files is the catalog file each type is written to.
var Files = map[string]string{
	TypeRepo:          "repos.jsonl",
	TypeMarketplace:   "marketplaces.jsonl",
	TypePlugin:        "plugins.jsonl",
	TypeCopilotPlugin: "copilot-plugins.jsonl",
	TypeSkill:         "skills.jsonl",
	TypeAgent:         "agents.jsonl",
	TypeCommand:       "commands.jsonl",
	TypeHook:          "hooks.jsonl",
	TypeMCP:           "mcp.jsonl",
}

// Record is one artifact. Fields holds the type-specific fields, written at
// the top level beside the common ones.
type Record struct {
	ID, Type, Name, Description string
	Tags                        []string
	Repo                        string
	// Commit and Path are empty for external plugins and unfollowed repos.
	Commit, Path string
	Meta         map[string]any
	ContentHash  string
	Errors       []string
	Fields       map[string]any
}

// MarshalJSON writes the record as one object with sorted keys, no HTML
// escaping, and empty slices as [] rather than null.
func (r Record) MarshalJSON() ([]byte, error) {
	tags := slices.Compact(slices.Sorted(slices.Values(r.Tags)))
	if tags == nil {
		tags = []string{}
	}
	m := map[string]any{
		"id": r.ID, "type": r.Type, "name": r.Name, "description": r.Description,
		"tags": tags, "repo": r.Repo,
	}
	for k, v := range map[string]string{"commit": r.Commit, "path": r.Path, "content_hash": r.ContentHash} {
		if v != "" {
			m[k] = v
		}
	}
	if len(r.Meta) > 0 {
		m["meta"] = r.Meta
	}
	if len(r.Errors) > 0 {
		m["errors"] = r.Errors
	}
	for k, v := range r.Fields {
		if _, taken := m[k]; taken || k == "commit" || k == "path" || k == "content_hash" || k == "meta" || k == "errors" {
			panic(fmt.Sprintf("catalog: field %q of %s collides with a common field", k, r.ID))
		}
		if s, ok := v.([]string); ok && s == nil {
			v = []string{}
		}
		m[k] = v
	}
	return encode(m)
}

// encode is json.Marshal without HTML escaping or a trailing newline.
func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
