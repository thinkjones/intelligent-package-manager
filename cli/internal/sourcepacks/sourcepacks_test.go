package sourcepacks

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
)

func names(lists []curated.List) []string {
	var out []string
	for _, l := range lists {
		for _, r := range l.Repos {
			out = append(out, l.Name+": "+r.Repo)
		}
	}
	return out
}

func TestUpdate(t *testing.T) {
	starred := []curated.List{
		{Name: "Skills", Repos: []curated.Repo{
			{Repo: "o/have", Stars: 900, Description: "new words"},
			{Repo: "o/new", Stars: 50, Homepage: "https://new.example", Topics: []string{"skills"}},
			{Repo: "o/empty", Stars: 40},
			{Repo: "o/mcp-only", Stars: 35},
			{Repo: "o/skipped", Stars: 30},
			{Repo: "o/broken", Stars: 20},
		}},
		{Name: "Fresh pack", Repos: []curated.Repo{
			{Repo: "o/new", Stars: 50},   // in two lists: scanned once
			{Repo: "O/Have", Stars: 900}, // already in a pack: joins this one unscanned
			{Repo: "o/big", Stars: 5000},
		}},
	}
	packs := []curated.List{
		{Name: "Kept first", Repos: []curated.Repo{{Repo: "o/gone-upstream", Stars: 1, Artifacts: map[string]int{"skills": 1}}}},
		{Name: "Skills", Repos: []curated.Repo{{Repo: "o/have", Stars: 100, Description: "old words", Artifacts: map[string]int{"skills": 2}}}},
	}
	var mu sync.Mutex
	var scanned []string
	scan := func(repo string) (map[string]int, error) {
		mu.Lock()
		scanned = append(scanned, repo)
		mu.Unlock()
		switch repo {
		case "o/new":
			return map[string]int{"skills": 3, "plugins": 1}, nil
		case "o/big":
			return map[string]int{"agents": 1}, nil
		case "o/mcp-only":
			return map[string]int{"mcp": 2}, nil
		case "o/broken":
			return nil, errors.New("HTTP 404 Not Found")
		}
		return map[string]int{}, nil
	}
	res := Update(starred, packs, []string{"o/skipped", "o/old-skip"}, false, scan, 3)

	sort.Strings(scanned)
	if want := []string{"o/big", "o/broken", "o/empty", "o/mcp-only", "o/new"}; !reflect.DeepEqual(scanned, want) {
		t.Errorf("scanned %v, want %v", scanned, want)
	}
	if res.Scanned != 5 {
		t.Errorf("Scanned = %d, want 5", res.Scanned)
	}
	wantPacks := []string{"Kept first: o/gone-upstream", "Skills: o/have", "Skills: o/new", "Fresh pack: o/big", "Fresh pack: O/Have", "Fresh pack: o/new"}
	if got := names(res.Packs); !reflect.DeepEqual(got, wantPacks) {
		t.Errorf("packs =\n%v, want\n%v", got, wantPacks)
	}
	wantAdded := []string{"Skills: o/new", "Fresh pack: o/big", "Fresh pack: O/Have", "Fresh pack: o/new"}
	if got := names(res.Added); !reflect.DeepEqual(got, wantAdded) {
		t.Errorf("added =\n%v, want\n%v", got, wantAdded)
	}
	if want := []string{"o/empty", "o/mcp-only", "o/old-skip", "o/skipped"}; !reflect.DeepEqual(res.Skipped, want) {
		t.Errorf("skipped = %v, want %v", res.Skipped, want)
	}
	if len(res.Failed) != 1 || res.Failed[0].Repo != "o/broken" {
		t.Errorf("failed = %v, want o/broken only", res.Failed)
	}
	have := res.Packs[1].Repos[0]
	if have.Stars != 900 || have.Description != "new words" || have.Artifacts["skills"] != 2 {
		t.Errorf("a repo still upstream should take its new details and keep its counts: %+v", have)
	}
	if n := res.Packs[1].Repos[1]; n.Homepage != "https://new.example" || n.Artifacts["skills"] != 3 {
		t.Errorf("a new repo should carry its details and counts: %+v", n)
	}
	if j := res.Packs[2].Repos[1]; j.Artifacts["skills"] != 2 {
		t.Errorf("a repo joining a second pack should carry the counts it has: %+v", j)
	}

	// a second run over its own output finds nothing and scans only the failure
	scanned = nil
	again := Update(starred, res.Packs, res.Skipped, false, scan, 3)
	if !reflect.DeepEqual(scanned, []string{"o/broken"}) || Count(again.Added) != 0 || !reflect.DeepEqual(names(again.Packs), wantPacks) {
		t.Errorf("second run: scanned %v, added %v, packs %v", scanned, names(again.Added), names(again.Packs))
	}
}

func TestUpdateRecheck(t *testing.T) {
	starred := []curated.List{{Name: "Skills", Repos: []curated.Repo{{Repo: "o/late"}, {Repo: "o/still-empty"}}}}
	scan := func(repo string) (map[string]int, error) {
		if repo == "o/late" {
			return map[string]int{"skills": 1}, nil
		}
		return nil, nil
	}
	skipped := []string{"o/late", "o/still-empty"}
	if res := Update(starred, nil, skipped, false, scan, 1); res.Scanned != 0 || Count(res.Added) != 0 {
		t.Errorf("without recheck nothing skipped is scanned: %+v", res)
	}
	res := Update(starred, nil, skipped, true, scan, 1)
	if !reflect.DeepEqual(names(res.Added), []string{"Skills: o/late"}) || !reflect.DeepEqual(res.Skipped, []string{"o/still-empty"}) {
		t.Errorf("recheck: added %v, skipped %v", names(res.Added), res.Skipped)
	}
	// a recheck that can't fetch a repo forgets it was skipped, so the next run tries it
	failing := func(string) (map[string]int, error) { return nil, errors.New("timeout") }
	if res := Update(starred, nil, skipped, true, failing, 1); len(res.Skipped) != 0 || len(res.Failed) != 2 {
		t.Errorf("failed recheck: skipped %v, failed %v", res.Skipped, res.Failed)
	}
}

func TestReport(t *testing.T) {
	res := Result{
		Scanned: 3,
		Skipped: []string{"o/empty"},
		Added: []curated.List{{Name: "AI · Skills", Repos: []curated.Repo{
			{Repo: "obra/superpowers", Stars: 297229, Description: "A skills\nframework.", Topics: []string{"skills", "sdlc"},
				Homepage: "https://sp.example", Artifacts: map[string]int{"skills": 14, "plugins": 1, "hooks": 1}},
			{Repo: "o/plain", Artifacts: map[string]int{"agents": 2}},
		}}},
		Failed: []Failure{{"o/broken", errors.New("HTTP 404 Not Found")}},
	}
	want := `## 2 new repos in 1 source pack

Scanned 3 repos; 1 hold nothing ipm can catalog so far.

### AI · Skills (2)

- **[obra/superpowers](https://github.com/obra/superpowers)** ★ 297k — A skills framework.
  - topics: skills, sdlc
  - homepage: <https://sp.example>
  - holds: 1 plugin, 14 skills, 1 hooks file
- **[o/plain](https://github.com/o/plain)** ★ 0
  - holds: 2 agents

### Couldn't be scanned (1)

These are tried again on the next run.

` + "- `o/broken`: HTTP 404 Not Found\n"
	if got := Report(res); got != want {
		t.Errorf("report =\n%s\nwant\n%s", got, want)
	}
	hostile := Report(Result{Scanned: 1, Added: []curated.List{{Name: "L", Repos: []curated.Repo{{
		Repo: "o/x", Description: "Fixes #12, thanks @octocat ![x](https://t.example/p.png) <img src=x> `a` *b* [c](d)",
		Topics:   []string{"ok", "bad\n\n# heading"},
		Homepage: "https://h.example/>\n\n## injected", Artifacts: map[string]int{"skills": 1},
	}}}}})
	for _, bad := range []string{"#12", "@octocat", "![x]", " <img", "\n# heading", "\n## injected", "`a`", " *b* ", "[c](d)"} {
		if strings.Contains(hostile, bad) {
			t.Errorf("the report carries %q from a repo's own text:\n%s", bad, hostile)
		}
	}
	if !strings.Contains(hostile, "Fixes #\u200b12, thanks @\u200boctocat") {
		t.Errorf("want the words kept, readable:\n%s", hostile)
	}
	if got := Report(Result{Scanned: 0}); !strings.HasPrefix(got, "## No new repos for the source packs\n") {
		t.Errorf("empty report = %q", got)
	}
}

// archive is a gzipped tar like GitHub's: every path under one top folder.
func archive(t *testing.T, files map[string]string, links ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range links {
		if err := tw.WriteHeader(&tar.Header{Name: l, Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink}); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestFetcherScan(t *testing.T) {
	tmp := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/o/skills/archive/HEAD.tar.gz":
			w.Write(archive(t, map[string]string{
				"skills-abc123/README.md":                          "hello",
				"skills-abc123/skills/tdd/SKILL.md":                "---\nname: tdd\n---",
				"skills-abc123/skills/debug/SKILL.md":              "x",
				"skills-abc123/.claude-plugin/plugin.json":         "{}",
				"skills-abc123/agents/reviewer.md":                 "x",
				"skills-abc123/commands/go.md":                     "x",
				"skills-abc123/hooks/hooks.json":                   "{}",
				"skills-abc123/node_modules/dep/skills/x/SKILL.md": "x",
				"skills-abc123/../../escape/SKILL.md":              "x",
				"/abs/SKILL.md":                                    "x",
			}, "skills-abc123/skills/linked/SKILL.md"))
		case "/o/webapp/archive/HEAD.tar.gz":
			w.Write(archive(t, map[string]string{"webapp-1/src/hooks/useThing.ts": "x", "webapp-1/README.md": "x"}))
		case "/o/notgzip/archive/HEAD.tar.gz":
			w.Write([]byte("<html>rate limited</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := Fetcher{Client: srv.Client(), Base: srv.URL, Tmp: tmp, Timeout: 10 * time.Second}

	got, err := f.Scan("o/skills")
	want := map[string]int{"skills": 2, "plugins": 1, "agents": 1, "commands": 1, "hooks": 1}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Scan = %v, %v; want %v", got, err, want)
	}
	if got, err := f.Scan("o/webapp"); err != nil || len(got) != 0 {
		t.Errorf("a repo with only a src/hooks folder holds nothing: %v, %v", got, err)
	}
	for _, repo := range []string{"o/missing", "o/notgzip"} {
		if _, err := f.Scan(repo); err == nil {
			t.Errorf("%s: want an error, so it is tried again rather than recorded as empty", repo)
		}
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("unpacked folders should be removed, found %d", len(left))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(tmp), "escape")); err == nil {
		t.Error("a path leaving the archive was written")
	}
}
