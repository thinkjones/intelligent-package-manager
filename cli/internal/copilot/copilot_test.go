package copilot

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSkillPaths(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "plugins/go/plugin.json"), `{"name":"go","extensions":{"com.github.awesome-copilot":{"skills":["./skills/go-sdk"],"agents":["./agents/x.md"]}}}`)
	write(t, filepath.Join(dir, "skills/go-sdk/SKILL.md"), "---\nname: go-sdk\n---\n")
	got, err := SkillPaths(dir, "plugins/go")
	if err != nil || !reflect.DeepEqual(got, []string{"skills/go-sdk"}) {
		t.Errorf("got %v %v", got, err)
	}
	if _, err := SkillPaths(dir, "nowhere"); err == nil {
		t.Error("want an error for a missing plugin.json")
	}
}
