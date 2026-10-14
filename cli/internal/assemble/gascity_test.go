package assemble

import (
	"reflect"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

const extSkip = "skipped plugin:o/r#stripe: external plugin — its files live in another repo; add its skills and agents directly"

func gasPlan(t *testing.T, ids ...string) GascityPlan {
	t.Helper()
	packs, items := resolved(t, ids...)
	plan, err := PlanGascity(packs[0], "kit", items)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func gasNames(p GascityPlan) []string {
	var out []string
	for _, e := range p.Entries {
		out = append(out, string(e.Kind)+" "+e.Name)
	}
	return out
}

func TestPlanGascityItems(t *testing.T) {
	plan := gasPlan(t, "skill:o/r:skills/code-review", "agent:o/r:agents/rev.md", "command:o/r:commands/ship.md")
	if want := []string{"skill code-review", "agent reviewer"}; !reflect.DeepEqual(gasNames(plan), want) {
		t.Errorf("got %v want %v", gasNames(plan), want)
	}
	if want := []string{"skipped command:o/r:commands/ship.md: Gas City has no slash commands"}; !reflect.DeepEqual(plan.Skips, want) {
		t.Errorf("skips %v", plan.Skips)
	}
	if plan.Name != "kit" || plan.Description != "My kit" || len(plan.Picked) != 3 {
		t.Errorf("header %+v", plan)
	}
	if e := plan.Entries[0]; e.Alias != "o--r" || e.From != "skills/code-review" {
		t.Errorf("entry %+v", e)
	}
}

func TestPlanGascityExpandsPlugin(t *testing.T) {
	packs, items := resolved(t, "plugin:o/r:plugins/tools")
	it := items["plugin:o/r:plugins/tools"]
	it.Parts = []string{
		"agent:o/r:plugins/tools/agents/b.md",
		"command:o/r:plugins/tools/commands/c.md",
		"hook:o/r:plugins/tools/hooks/hooks.json#PreToolUse",
		"mcp:o/r:plugins/tools/.mcp.json#My_Server",
		"skill:o/r:plugins/tools/skills/a",
	}
	items[it.ID] = it
	plan, err := PlanGascity(packs[0], "kit", items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"skill a", "agent b", "mcp my-server"}; !reflect.DeepEqual(gasNames(plan), want) {
		t.Errorf("got %v want %v", gasNames(plan), want)
	}
	m := plan.Entries[2]
	if m.From != "plugins/tools/.mcp.json" || m.Server != "My_Server" || m.Item.ID != "mcp:o/r:plugins/tools/.mcp.json#My_Server" || m.Item.SHA != shaA || m.Item.Repo != "o/r" {
		t.Errorf("mcp entry %+v", m)
	}
	want := []string{
		"skipped command:o/r:plugins/tools/commands/c.md: Gas City has no slash commands",
		"skipped hook:o/r:plugins/tools/hooks/hooks.json#PreToolUse: Gas City packs have no hooks",
	}
	if !reflect.DeepEqual(plan.Skips, want) {
		t.Errorf("skips %v", plan.Skips)
	}
}

func TestPlanGascityPluginWithoutParts(t *testing.T) {
	packs, items := resolved(t, "plugin:o/r:plugins/tools")
	it := items["plugin:o/r:plugins/tools"]
	it.Parts = nil
	items[it.ID] = it
	plan, err := PlanGascity(packs[0], "kit", items)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"skipped plugin:o/r:plugins/tools: its parts aren't recorded — run ipm pack build --update"}
	if len(plan.Entries) != 0 || !reflect.DeepEqual(plan.Skips, want) {
		t.Errorf("%+v", plan)
	}
}

func TestPlanGascityExternalPlugin(t *testing.T) {
	plan := gasPlan(t, "plugin:o/r#stripe")
	if len(plan.Entries) != 0 || !reflect.DeepEqual(plan.Skips, []string{extSkip}) {
		t.Errorf("%+v", plan)
	}
}

func TestPlanGascityCopilotPlugin(t *testing.T) {
	plan := gasPlan(t, "copilot-plugin:o/r:plugins/go")
	if want := []string{"skill go-expert"}; !reflect.DeepEqual(gasNames(plan), want) {
		t.Errorf("got %v", gasNames(plan))
	}
}

func TestPlanGascityNameClash(t *testing.T) {
	plan := gasPlan(t, "skill:o/r:skills/code-review", "skill:mattpocock/skills:skills/code-review")
	if want := []string{"skill code-review", "skill code-review-mattpocock"}; !reflect.DeepEqual(gasNames(plan), want) {
		t.Errorf("got %v", gasNames(plan))
	}
	items := map[string]project.LockItem{
		"agent:o/r:agents/a.md": {ID: "agent:o/r:agents/a.md", Type: "agent", Name: "Code.Review", Repo: "o/r", Path: "agents/a.md", SHA: shaA},
		"agent:o/r:agents/b.md": {ID: "agent:o/r:agents/b.md", Type: "agent", Name: "code-review", Repo: "o/r", Path: "agents/b.md", SHA: shaA},
	}
	plan, err := PlanGascity(project.Pack{Name: "kit", Items: []string{"agent:o/r:agents/a.md", "agent:o/r:agents/b.md"}}, "kit", items)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"agent code-review", "agent code-review-2"}; !reflect.DeepEqual(gasNames(plan), want) {
		t.Errorf("got %v", gasNames(plan))
	}
}

func TestPlanGascityRefusesOtherTypes(t *testing.T) {
	items := map[string]project.LockItem{"hook:o/r:h#x": {ID: "hook:o/r:h#x", Type: "hook", Repo: "o/r", SHA: shaA}}
	if _, err := PlanGascity(project.Pack{Name: "kit", Items: []string{"hook:o/r:h#x"}}, "kit", items); err == nil {
		t.Error("want an error")
	}
}

func TestValidGasName(t *testing.T) {
	for _, n := range []string{"kit", "Kit_2", "a-b"} {
		if !ValidGasName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range []string{"-kit", ".kit", "kit/x", "kit.x", ""} {
		if ValidGasName(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
}
