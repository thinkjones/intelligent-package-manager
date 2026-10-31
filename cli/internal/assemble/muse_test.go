package assemble

import (
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
)

func musePlan(t *testing.T, items ...project.LockItem) (MusePlan, error) {
	t.Helper()
	p := project.Pack{Name: "kit"}
	byID := map[string]project.LockItem{}
	for _, it := range items {
		p.Items = append(p.Items, it.ID)
		byID[it.ID] = it
	}
	return PlanMuse(p, byID)
}

func TestPlanMuseWholePlugins(t *testing.T) {
	plan, err := musePlan(t,
		project.LockItem{ID: "skill:o/r:skills/s", Type: "skill", Name: "s", Repo: "o/r", Path: "skills/s", SHA: shaA},
		project.LockItem{ID: "plugin:o/r:.", Type: "plugin", Name: "root", Repo: "o/r", Path: ".", SHA: shaA},
		project.LockItem{ID: "plugin:o/r:plugins/p", Type: "plugin", Name: "p", Repo: "o/r", Path: "plugins/p", SHA: shaA},
		project.LockItem{ID: "plugin:o/r#gh", Type: "plugin", Name: "gh", Repo: "o/r", Source: `{"name":"gh","source":{"source":"github","repo":"x/gh","sha":"` + shaB + `"}}`},
		project.LockItem{ID: "plugin:o/r#url", Type: "plugin", Name: "url", Repo: "o/r", Source: `{"name":"url","source":{"source":"url","url":"https://github.com/Twilio/ai.git","sha":"` + shaB + `"}}`},
		project.LockItem{ID: "plugin:o/r#sub", Type: "plugin", Name: "sub", Repo: "o/r", Source: `{"name":"sub","source":{"source":"git-subdir","url":"https://gitlab.com/g/m.git","path":"plugins/sub","ref":"` + shaB + `"}}`},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []MusePlugin{
		{Name: "root", Alias: "o--r", From: "", Item: plan.Plugins[0].Item},
		{Name: "p", Alias: "o--r", From: "plugins/p", Item: plan.Plugins[1].Item},
		{Name: "gh", Alias: "x--gh", From: "", Item: plan.Plugins[2].Item},
		{Name: "url", Alias: "twilio--ai", From: "", Item: plan.Plugins[3].Item},
		{Name: "sub", Alias: "gitlab.com--g--m", From: "plugins/sub", Item: plan.Plugins[4].Item},
	}
	if len(plan.Plugins) != len(want) {
		t.Fatalf("plugins %+v", plan.Plugins)
	}
	for i := range want {
		if got := plan.Plugins[i]; got.Name != want[i].Name || got.Alias != want[i].Alias || got.From != want[i].From {
			t.Errorf("plugin %d: got %+v want %+v", i, got, want[i])
		}
	}
	repos := map[string]string{
		"o--r":             "github:o/r#" + shaA,
		"x--gh":            "github:x/gh#" + shaB,
		"twilio--ai":       "github:Twilio/ai#" + shaB,
		"gitlab.com--g--m": "git+https://gitlab.com/g/m.git#" + shaB,
	}
	if !reflect.DeepEqual(plan.Repos(), repos) {
		t.Errorf("repos %v", plan.Repos())
	}
	if len(plan.Skips) != 0 {
		t.Errorf("skips %v", plan.Skips)
	}
}

func TestPlanMuseSkipsWhatItCannotFetch(t *testing.T) {
	plan, err := musePlan(t,
		project.LockItem{ID: "plugin:o/r#loose", Type: "plugin", Name: "loose", Repo: "o/r", Source: `{"name":"loose","source":{"source":"github","repo":"x/c"}}`},
		project.LockItem{ID: "plugin:o/r#npm", Type: "plugin", Name: "npm", Repo: "o/r", Source: `{"name":"npm","source":{"source":"npm","package":"@x/y"}}`},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Plugins) != 0 || len(plan.Skips) != 2 {
		t.Fatalf("plan %+v", plan)
	}
	if !strings.Contains(plan.Skips[0], "plugin:o/r#loose") || !strings.Contains(plan.Skips[0], "no commit") {
		t.Errorf("skip %q", plan.Skips[0])
	}
	if !strings.Contains(plan.Skips[1], "plugin:o/r#npm") || !strings.Contains(plan.Skips[1], `"npm"`) {
		t.Errorf("skip %q", plan.Skips[1])
	}
}

func TestPlanMuseRefusesTwoCommitsOfOneRepo(t *testing.T) {
	_, err := musePlan(t,
		project.LockItem{ID: "plugin:o/r:p", Type: "plugin", Name: "p", Repo: "o/r", Path: "p", SHA: shaA},
		project.LockItem{ID: "plugin:m/m#q", Type: "plugin", Name: "q", Repo: "m/m", Source: `{"name":"q","source":{"source":"github","repo":"o/r","sha":"` + shaB + `"}}`},
	)
	if err == nil || !strings.Contains(err.Error(), "o/r") {
		t.Errorf("got %v", err)
	}
}

func TestPlanMuseUnresolved(t *testing.T) {
	if _, err := PlanMuse(project.Pack{Name: "kit", Items: []string{"plugin:o/r:p"}}, nil); err == nil {
		t.Error("an unresolved item is an error")
	}
}
