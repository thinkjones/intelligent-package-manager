package catalog

import (
	"reflect"
	"strings"
	"testing"
)

func TestParsePlugin(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "p/.claude-plugin/plugin.json", `{"name":"tools","description":"d","keywords":["k"],"category":"dev","version":"1.0",
		"author":{"name":"A"},"license":"MIT","skills":"./extra","agents":["./a/x.md"],
		"commands":{"about":{"content":"hi","description":"About"}},"hooks":"./h.json","mcpServers":{"s":{"command":"c"}}}`)
	r, comps := ParsePlugin(c, "p")
	if r.ID != "plugin:o/r:p" || r.Name != "tools" || r.Description != "d" || r.Path != "p" ||
		!reflect.DeepEqual(sorted(r.Tags), []string{"dev", "k"}) || r.Fields["version"] != "1.0" || r.Fields["license"] != "MIT" ||
		!reflect.DeepEqual(r.Fields["author"], map[string]any{"name": "A"}) || r.ContentHash == "" {
		t.Errorf("record %+v", r)
	}
	if !reflect.DeepEqual(comps.Skills, []string{"./extra"}) || !reflect.DeepEqual(comps.Agents, []string{"./a/x.md"}) ||
		comps.Hooks != "./h.json" || comps.CommandMap["about"] == nil || comps.Commands != nil ||
		!reflect.DeepEqual(comps.MCP, map[string]any{"s": map[string]any{"command": "c"}}) {
		t.Errorf("components %+v", comps)
	}
	if !comps.Declared("agents") || !comps.Declared("commands") || comps.Declared("skills") == false {
		t.Errorf("Declared: %+v", comps)
	}
}

func TestParsePluginBadJSON(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "p/.claude-plugin/plugin.json", `{nope`)
	r, comps := ParsePlugin(c, "p")
	if len(r.Errors) == 0 || r.Name != "p" || comps.Declared("skills") {
		t.Errorf("got %+v %+v", r, comps)
	}
}

func TestParseCopilotPlugin(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "plugins/go/plugin.json", `{"name":"go","description":"Go","keywords":["mcp"],"extensions":{"com.github.awesome-copilot":{"skills":["./skills/go-sdk"]}}}`)
	file(t, c.Dir, "skills/go-sdk/SKILL.md", "---\nname: go-sdk\n---\n")
	r := ParseCopilotPlugin(c, "plugins/go")
	if r.ID != "copilot-plugin:o/r:plugins/go" || r.Type != TypeCopilotPlugin || r.Name != "go" ||
		!reflect.DeepEqual(r.Tags, []string{"mcp"}) || !reflect.DeepEqual(r.Fields["skills"], []string{"skill:o/r:skills/go-sdk"}) {
		t.Errorf("got %+v", r)
	}
}

func TestParseMarketplace(t *testing.T) {
	c := co(t)
	file(t, c.Dir, ".claude-plugin/marketplace.json", `{"name":"m","metadata":{"description":"Market","pluginRoot":"./plugins"},"plugins":[
		{"name":"a","source":"./plugins/a"},
		{"name":"b","source":{"source":"github","repo":"x/b"}},
		{"source":"./c"}]}`)
	r, entries, warns := ParseMarketplace(c, ".")
	if r.ID != "marketplace:o/r:." || r.Name != "m" || r.Description != "Market" || r.Fields["plugin_root"] != "./plugins" {
		t.Errorf("record %+v", r)
	}
	if len(entries) != 2 || entries[0].Name != "a" || entries[0].Source != "./plugins/a" || entries[1].Raw["name"] != "b" {
		t.Errorf("entries %+v", entries)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "entry 2 has no name") {
		t.Errorf("warnings %v", warns)
	}
}
