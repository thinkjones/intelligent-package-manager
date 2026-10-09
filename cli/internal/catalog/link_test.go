package catalog

import (
	"reflect"
	"strings"
	"testing"
)

func byID(t *testing.T, res RepoResult) map[string]Record {
	t.Helper()
	m := map[string]Record{}
	for _, r := range res.Records {
		if _, dup := m[r.ID]; dup {
			t.Fatalf("duplicate record %s", r.ID)
		}
		m[r.ID] = r
	}
	return m
}

func list(r Record, key string) []string {
	l, _ := r.Fields[key].([]string)
	return l
}

func TestOwnershipDefaults(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "p/.claude-plugin/plugin.json", `{"name":"tools"}`)
	file(t, c.Dir, "p/skills/a/SKILL.md", "---\nname: a\n---\n")
	file(t, c.Dir, "p/skills/group/b/SKILL.md", "---\nname: b\n---\n")
	file(t, c.Dir, "p/agents/r.md", "---\nname: r\n---\n")
	file(t, c.Dir, "p/agents/sub/x.md", "---\nname: x\n---\n")
	file(t, c.Dir, "p/commands/c.md", "c")
	file(t, c.Dir, "p/hooks/hooks.json", `{"hooks":{"Stop":[]}}`)
	file(t, c.Dir, "p/.mcp.json", `{"mcpServers":{"s":{"command":"x"}}}`)
	file(t, c.Dir, "p/docs/z/SKILL.md", "---\nname: z\n---\n")
	res, err := IndexRepo(c)
	if err != nil {
		t.Fatal(err)
	}
	recs := byID(t, res)
	p := recs["plugin:o/r:p"]
	for key, want := range map[string][]string{
		"skills":   {"skill:o/r:p/skills/a"},
		"agents":   {"agent:o/r:p/agents/r.md", "agent:o/r:p/agents/sub/x.md"},
		"commands": {"command:o/r:p/commands/c.md"},
		"hooks":    {"hook:o/r:p/hooks/hooks.json"},
		"mcp":      {"mcp:o/r:p/.mcp.json#s"},
	} {
		if got := list(p, key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", key, got, want)
		}
	}
	for _, id := range []string{"skill:o/r:p/docs/z", "skill:o/r:p/skills/group/b"} {
		if _, ok := recs[id]; !ok {
			t.Errorf("%s should still be cataloged", id)
		}
	}
}

func TestOwnershipDeclaredPaths(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "p/.claude-plugin/plugin.json", `{"name":"tools","skills":"./more","agents":["./custom/x.md"],
		"commands":{"about":{"content":"hi","description":"About"},"ship":{"source":"./cmds/ship.md"}},
		"hooks":["./cfg/h.json",{"Stop":[]}],"mcpServers":[{"i":{"url":"https://u"}},"./b.mcpb"]}`)
	file(t, c.Dir, "p/skills/a/SKILL.md", "---\nname: a\n---\n")
	file(t, c.Dir, "p/more/m/SKILL.md", "---\nname: m\n---\n")
	file(t, c.Dir, "p/agents/r.md", "---\nname: r\n---\n")
	file(t, c.Dir, "p/custom/x.md", "---\nname: x\n---\n")
	file(t, c.Dir, "p/cmds/ship.md", "ship")
	file(t, c.Dir, "p/commands/old.md", "old")
	file(t, c.Dir, "p/hooks/hooks.json", `{"hooks":{"Start":[]}}`)
	file(t, c.Dir, "p/cfg/h.json", `{"hooks":{"PreToolUse":[]}}`)
	file(t, c.Dir, "p/.mcp.json", `{"mcpServers":{"s":{"command":"x"}}}`)
	res, err := IndexRepo(c)
	if err != nil {
		t.Fatal(err)
	}
	recs := byID(t, res)
	p := recs["plugin:o/r:p"]
	for key, want := range map[string][]string{
		"skills":   {"skill:o/r:p/more/m", "skill:o/r:p/skills/a"},                                                    // adds
		"agents":   {"agent:o/r:p/custom/x.md"},                                                                       // replaces
		"commands": {"command:o/r:p/.claude-plugin/plugin.json#about", "command:o/r:p/cmds/ship.md"},                  // replaces
		"hooks":    {"hook:o/r:p/.claude-plugin/plugin.json", "hook:o/r:p/cfg/h.json", "hook:o/r:p/hooks/hooks.json"}, // merges
		"mcp":      {"mcp:o/r:p/.claude-plugin/plugin.json#i", "mcp:o/r:p/.mcp.json#s"},                               // merges
	} {
		if got := list(p, key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", key, got, want)
		}
	}
	if about := recs["command:o/r:p/.claude-plugin/plugin.json#about"]; about.Name != "about" || about.Description != "About" {
		t.Errorf("inline command %+v", about)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "b.mcpb") {
		t.Errorf("want a warning for the bundle, got %v", res.Warnings)
	}
}

func TestPluginRootSkill(t *testing.T) {
	c := co(t)
	file(t, c.Dir, ".claude-plugin/plugin.json", `{"name":"solo"}`)
	file(t, c.Dir, "SKILL.md", "---\nname: solo\n---\n")
	res, _ := IndexRepo(c)
	if got := list(byID(t, res)["plugin:o/r:."], "skills"); !reflect.DeepEqual(got, []string{"skill:o/r:."}) {
		t.Errorf("got %v", got)
	}
}

func TestSkillOwnedByTwoPlugins(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "p1/.claude-plugin/plugin.json", `{"name":"one","skills":"../shared/skills"}`)
	file(t, c.Dir, "p2/.claude-plugin/plugin.json", `{"name":"two","skills":["../shared/skills"]}`)
	file(t, c.Dir, "shared/skills/s/SKILL.md", "---\nname: s\n---\n")
	res, _ := IndexRepo(c)
	recs := byID(t, res)
	for _, id := range []string{"plugin:o/r:p1", "plugin:o/r:p2"} {
		if got := list(recs[id], "skills"); !reflect.DeepEqual(got, []string{"skill:o/r:shared/skills/s"}) {
			t.Errorf("%s: %v", id, got)
		}
	}
}

func TestDeclaredPathEscapingRepo(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "p/.claude-plugin/plugin.json", `{"name":"x","skills":"../../../etc","agents":"/etc/passwd"}`)
	res, err := IndexRepo(c)
	if err != nil {
		t.Fatal(err)
	}
	w := strings.Join(res.Warnings, "\n")
	if !strings.Contains(w, "plugin:o/r:p") || !strings.Contains(w, "leaves the repo") || strings.Count(w, "leaves the repo") != 2 {
		t.Errorf("warnings %v", res.Warnings)
	}
	if got := list(byID(t, res)["plugin:o/r:p"], "skills"); len(got) != 0 {
		t.Errorf("skills %v", got)
	}
}

func TestCopilotPluginSkillsLinked(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "plugins/go/plugin.json", `{"name":"go","extensions":{"com.github.awesome-copilot":{"skills":["./skills/go-sdk"]}}}`)
	file(t, c.Dir, "skills/go-sdk/SKILL.md", "---\nname: go-sdk\n---\n")
	res, _ := IndexRepo(c)
	recs := byID(t, res)
	if got := list(recs["copilot-plugin:o/r:plugins/go"], "skills"); !reflect.DeepEqual(got, []string{"skill:o/r:skills/go-sdk"}) {
		t.Errorf("got %v", got)
	}
	if _, ok := recs["skill:o/r:skills/go-sdk"]; !ok {
		t.Error("skill not cataloged")
	}
}
