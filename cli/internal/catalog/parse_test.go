package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// file writes rel under root with body.
func file(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func co(t *testing.T) Checkout { return Checkout{Repo: "o/r", Commit: "c1", Dir: t.TempDir()} }

func TestParseSkill(t *testing.T) {
	c := co(t)
	body := "---\nname: tdd\ndescription: Test first\ncategory: testing\nmetadata:\n  tags: [go, quality]\n---\nbody"
	file(t, c.Dir, "skills/tdd/SKILL.md", body)
	r := ParseSkill(c, "skills/tdd")
	if r.ID != "skill:o/r:skills/tdd" || r.Type != TypeSkill || r.Name != "tdd" || r.Description != "Test first" ||
		r.Repo != "o/r" || r.Commit != "c1" || r.Path != "skills/tdd" || r.Meta["name"] != "tdd" || len(r.Errors) != 0 {
		t.Errorf("got %+v", r)
	}
	if !reflect.DeepEqual(sorted(r.Tags), []string{"go", "quality", "testing"}) {
		t.Errorf("tags %v", r.Tags)
	}
	if r.ContentHash != ContentHash([]byte(body)) || r.ContentHash == "" {
		t.Errorf("hash %q", r.ContentHash)
	}
	if p, ok := r.Fields["plugins"].([]string); !ok || p == nil || len(p) != 0 {
		t.Errorf("plugins %#v", r.Fields["plugins"])
	}
}

func TestFrontmatterCRLF(t *testing.T) {
	meta, found, err := Frontmatter([]byte("---\r\nname: a\r\n---\r\nbody\r\n"))
	if err != nil || !found || meta["name"] != "a" {
		t.Errorf("got %v %v %v", meta, found, err)
	}
	if ContentHash([]byte("a\r\nb")) != ContentHash([]byte("a\nb")) {
		t.Error("ContentHash should normalize CRLF")
	}
}

func TestFrontmatterTimestampBecomesString(t *testing.T) {
	meta, _, err := Frontmatter([]byte("---\ndate: 2025-01-02\nk:\n  1: x\n---\n"))
	if err != nil || meta["date"] != "2025-01-02T00:00:00Z" {
		t.Errorf("date %#v %v", meta["date"], err)
	}
	if k, ok := meta["k"].(map[string]any); !ok || k["1"] != "x" {
		t.Errorf("non-string keys should become strings: %#v", meta["k"])
	}
}

func TestParseSkillBadYAML(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "skills/tdd/SKILL.md", "---\n: [\n---\nbody")
	r := ParseSkill(c, "skills/tdd")
	if len(r.Errors) == 0 || r.Meta["raw"] != ": [" || r.Name != "tdd" {
		t.Errorf("got %+v", r)
	}
}

func TestParseSkillWithoutName(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "skills/tdd/SKILL.md", "---\ndescription: d\n---\n")
	if r := ParseSkill(c, "skills/tdd"); r.Name != "tdd" || len(r.Errors) != 0 {
		t.Errorf("got %+v", r)
	}
	file(t, c.Dir, "skills/bare/SKILL.md", "no frontmatter")
	if r := ParseSkill(c, "skills/bare"); r.Name != "bare" || !reflect.DeepEqual(r.Errors, []string{"no frontmatter"}) {
		t.Errorf("got %+v", r)
	}
}

func TestParseAgentAndCommand(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "agents/rev.md", "---\nname: reviewer\ndescription: Reviews\nmodel: opus\n---\n")
	file(t, c.Dir, "commands/ship.md", "no frontmatter here")
	a := ParseAgent(c, "agents/rev.md")
	if a.ID != "agent:o/r:agents/rev.md" || a.Type != TypeAgent || a.Name != "reviewer" || a.Description != "Reviews" || a.Meta["model"] != "opus" {
		t.Errorf("agent %+v", a)
	}
	cmd := ParseCommand(c, "commands/ship.md")
	if cmd.ID != "command:o/r:commands/ship.md" || cmd.Type != TypeCommand || cmd.Name != "ship" || len(cmd.Errors) != 0 || len(cmd.Meta) != 0 {
		t.Errorf("command %+v", cmd)
	}
}

func TestParseHookFile(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "hooks/hooks.json", `{"hooks":{"PreToolUse":[{"hooks":[]}],"preToolUse":[]}}`)
	r := ParseHookFile(c, "hooks/hooks.json")
	events := []string{"PreToolUse", "preToolUse"}
	if r.ID != "hook:o/r:hooks/hooks.json" || r.Type != TypeHook || r.Name != "hooks" ||
		!reflect.DeepEqual(r.Fields["events"], events) || !reflect.DeepEqual(r.Tags, events) ||
		r.Description != "events: PreToolUse, preToolUse" {
		t.Errorf("got %+v", r)
	}
	in := InlineHook(c, "p/.claude-plugin/plugin.json", map[string]any{"Stop": []any{}})
	if in.ID != "hook:o/r:p/.claude-plugin/plugin.json" || !reflect.DeepEqual(in.Fields["events"], []string{"Stop"}) {
		t.Errorf("inline %+v", in)
	}
}

func TestParseMCPFileBothShapes(t *testing.T) {
	c := co(t)
	file(t, c.Dir, ".mcp.json", `{"mcpServers":{"gh":{"command":"npx","args":["x"]}}}`)
	file(t, c.Dir, "b/.mcp.json", `{"web":{"type":"http","url":"https://u"}}`)
	recs, warns := ParseMCPFile(c, ".mcp.json")
	if len(warns) != 0 || len(recs) != 1 {
		t.Fatalf("%v %v", recs, warns)
	}
	gh := recs[0]
	if gh.ID != "mcp:o/r:.mcp.json#gh" || gh.Name != "gh" || gh.Fields["transport"] != "stdio" || gh.Fields["command"] != "npx" ||
		!reflect.DeepEqual(gh.Fields["args"], []any{"x"}) || gh.Description != "stdio: npx x" {
		t.Errorf("gh %+v", gh)
	}
	recs, _ = ParseMCPFile(c, "b/.mcp.json")
	if len(recs) != 1 || recs[0].ID != "mcp:o/r:b/.mcp.json#web" || recs[0].Fields["transport"] != "http" ||
		recs[0].Fields["url"] != "https://u" || recs[0].Description != "http: https://u" {
		t.Errorf("web %+v", recs)
	}
	in := InlineMCP(c, "p/.claude-plugin/plugin.json", map[string]any{"s": map[string]any{"command": "c"}})
	if len(in) != 1 || in[0].ID != "mcp:o/r:p/.claude-plugin/plugin.json#s" {
		t.Errorf("inline %+v", in)
	}
}

func TestParseMCPFileBadJSON(t *testing.T) {
	c := co(t)
	file(t, c.Dir, ".mcp.json", `{nope`)
	recs, warns := ParseMCPFile(c, ".mcp.json")
	if len(recs) != 0 || len(warns) != 1 || !strings.Contains(warns[0], ".mcp.json") {
		t.Errorf("%v %v", recs, warns)
	}
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestParseLenientFallback(t *testing.T) {
	c := co(t)
	file(t, c.Dir, "agents/hunter.md", "---\nname: hunter\ndescription: Use this: finds silent failures\nmodel: inherit\n---\nbody")
	r := ParseAgent(c, "agents/hunter.md")
	if r.Name != "hunter" || r.Description != "Use this: finds silent failures" || len(r.Errors) != 1 ||
		r.Meta["model"] != "inherit" || r.Meta["raw"] == nil {
		t.Errorf("got %+v", r)
	}
}

func TestFrontmatterNonFiniteFloatsBecomeStrings(t *testing.T) {
	meta, _, err := Frontmatter([]byte("---\nx: .nan\ny: .inf\n---\n"))
	if err != nil || meta["x"] != "NaN" || meta["y"] != "+Inf" {
		t.Errorf("got %#v %v", meta, err)
	}
}

func TestBody(t *testing.T) {
	for in, want := range map[string]string{
		"---\nname: x\n---\nHello\n":         "Hello\n",
		"Hello\n":                            "Hello\n",
		"---\r\nname: x\r\n---\r\nHello\r\n": "Hello\n",
		"---\n---\nHello\n":                  "Hello\n",
		"---\nname: x\n---":                  "",
	} {
		if got := string(Body([]byte(in))); got != want {
			t.Errorf("Body(%q) = %q, want %q", in, got, want)
		}
	}
}
