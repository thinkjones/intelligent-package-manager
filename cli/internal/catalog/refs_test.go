package catalog

import (
	"encoding/json"
	"reflect"
	"testing"
)

// records builds a catalog from JSON lines, keyed by type.
func records(t *testing.T, lines ...string) map[string][]map[string]any {
	t.Helper()
	out := map[string][]map[string]any{}
	for _, l := range lines {
		var r map[string]any
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("%v: %s", err, l)
		}
		typ, _ := r["type"].(string)
		out[typ] = append(out[typ], r)
	}
	return out
}

const (
	repoAM = `{"id":"repo:a/m","type":"repo","repo":"a/m"}`
	repoBS = `{"id":"repo:b/s","type":"repo","repo":"b/s"}`
)

func entry(id, name, source string, src string) string {
	return `{"id":"` + id + `","type":"plugin","name":"` + name + `","repo":"a/m","source_repo":"` + source + `","entry":{"name":"` + name + `","source":` + src + `}}`
}

func TestRefsByPath(t *testing.T) {
	r := BuildRefs(records(t, repoAM, repoBS,
		entry("plugin:a/m#x", "x", "b/s", `{"source":"git-subdir","url":"https://github.com/b/s.git","path":"plugins/two"}`),
		`{"id":"plugin:b/s:plugins/one","type":"plugin","name":"x","repo":"b/s","path":"plugins/one"}`,
		`{"id":"plugin:b/s:plugins/two","type":"plugin","name":"two","repo":"b/s","path":"plugins/two"}`))
	if r.Target["plugin:a/m#x"] != "plugin:b/s:plugins/two" {
		t.Errorf("by path: %q", r.Target["plugin:a/m#x"])
	}
	if !reflect.DeepEqual(r.By["plugin:b/s:plugins/two"], []string{"plugin:a/m#x"}) {
		t.Errorf("by: %v", r.By)
	}
}

func TestRefsByName(t *testing.T) {
	r := BuildRefs(records(t, repoAM, repoBS,
		entry("plugin:a/m#x", "two", "b/s", `{"source":"url","url":"https://github.com/b/s.git"}`),
		`{"id":"plugin:b/s:plugins/one","type":"plugin","name":"one","repo":"b/s","path":"plugins/one"}`,
		`{"id":"plugin:b/s:plugins/two","type":"plugin","name":"two","repo":"b/s","path":"plugins/two"}`))
	if r.Target["plugin:a/m#x"] != "plugin:b/s:plugins/two" {
		t.Errorf("by name: %q", r.Target["plugin:a/m#x"])
	}
}

func TestRefsOnlyPlugin(t *testing.T) {
	r := BuildRefs(records(t, repoAM, repoBS,
		entry("plugin:a/m#cloudflare", "cloudflare", "b/s", `{"source":"url","url":"https://github.com/b/s.git"}`),
		`{"id":"plugin:b/s:.","type":"plugin","name":"skills","repo":"b/s","path":"."}`))
	if r.Target["plugin:a/m#cloudflare"] != "plugin:b/s:." {
		t.Errorf("only plugin: %q", r.Target["plugin:a/m#cloudflare"])
	}
}

func TestRefsToRepo(t *testing.T) {
	r := BuildRefs(records(t, repoAM, repoBS,
		entry("plugin:a/m#x", "x", "B/S", `{"source":"url","url":"https://github.com/B/S.git"}`)))
	if r.Target["plugin:a/m#x"] != "repo:b/s" {
		t.Errorf("to repo: %q", r.Target["plugin:a/m#x"])
	}
}

func TestRefsUnresolvedSource(t *testing.T) {
	r := BuildRefs(records(t, repoAM,
		entry("plugin:a/m#x", "x", "c/missing", `{"source":"url","url":"https://github.com/c/missing.git"}`)))
	if _, ok := r.Target["plugin:a/m#x"]; ok {
		t.Errorf("no source repo, no target: %v", r.Target)
	}
	if !r.External["plugin:a/m#x"] {
		t.Error("it is still an external entry")
	}
}

func TestRefsSelfReferenceNotExternal(t *testing.T) {
	r := BuildRefs(records(t, repoAM,
		entry("plugin:a/m#x", "x", "A/M", `{"source":"url","url":"https://github.com/A/M.git"}`),
		`{"id":"plugin:a/m:p","type":"plugin","name":"x","repo":"a/m","path":"p"}`))
	if len(r.Target) != 0 || len(r.By) != 0 || len(r.UsedBy) != 0 || r.External["plugin:a/m#x"] {
		t.Errorf("a repo listing itself is not external: %+v", r)
	}
}

func TestUsedByDistinctRepos(t *testing.T) {
	r := BuildRefs(records(t, repoAM, repoBS, `{"id":"repo:c/n","type":"repo","repo":"c/n"}`,
		entry("plugin:a/m#x", "x", "b/s", `{"source":"url","url":"u"}`),
		entry("plugin:a/m#y", "y", "b/s", `{"source":"url","url":"u"}`),
		`{"id":"plugin:c/n#z","type":"plugin","name":"z","repo":"c/n","source_repo":"b/s","entry":{"name":"z","source":{"source":"url","url":"u"}}}`))
	if !reflect.DeepEqual(r.UsedBy["b/s"], []string{"a/m", "c/n"}) {
		t.Errorf("used by %v", r.UsedBy["b/s"])
	}
}

func TestRefsPlaceholderRepoIsNotATarget(t *testing.T) {
	// catalog.Build writes a followed:false record for every external
	// source repo; that is not a source, so the entry is unresolved.
	r := BuildRefs(records(t, repoAM, `{"id":"repo:42c/plugins","type":"repo","repo":"42c/plugins","followed":false}`,
		entry("plugin:a/m#x", "x", "42c/plugins", `{"source":"url","url":"u"}`)))
	if _, ok := r.Target["plugin:a/m#x"]; ok || len(r.UsedBy) != 0 || !r.External["plugin:a/m#x"] {
		t.Errorf("a placeholder repo is not a target: %+v", r)
	}
}
