package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thinkjones/fs-skills/cli/internal/marketplace"
	"github.com/thinkjones/fs-skills/cli/internal/upstream"
)

// fake is a System with no real environment, git or claude. Output answers
// from canned results keyed by the program name and its first arguments.
type fake struct {
	env     map[string]string
	out     bytes.Buffer
	errOut  bytes.Buffer
	onPath  map[string]bool
	results map[string]result
	mu      sync.Mutex
	calls   []string
}

type result struct {
	out string
	err error
}

func newFake(root string) *fake {
	return &fake{
		env:    map[string]string{"AIHS_ROOT": root},
		onPath: map[string]bool{},
		results: map[string]result{
			"git conf":    {"Ada Lovelace\n", nil},
			"git remo":    {"git@github.com:ada/skills.git\n", nil},
			"claude plug": {"ok", nil},
		},
	}
}

func (f *fake) system() System {
	return System{
		Stdout: &f.out,
		Stderr: &f.errOut,
		Getenv: func(k string) string { return f.env[k] },
		LookPath: func(p string) (string, error) {
			if f.onPath[p] {
				return "/bin/" + p, nil
			}
			return "", errors.New("not found")
		},
		Output: func(dir, name string, args ...string) (string, error) {
			key := name
			// skip git's "-C <dir>"
			for _, a := range args {
				if a == "-C" || a == dir {
					continue
				}
				key += " " + a[:min(4, len(a))]
				break
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.calls = append(f.calls, name+" "+strings.Join(args, " "))
			r := f.results[key]
			return r.out, r.err
		},
		Now:     func() time.Time { return time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC) },
		Version: "test",
	}
}

func (f *fake) run(args ...string) error { return Run(args, f.system()) }

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkout is a root with two packs: one upstream plugin each.
func checkout(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "skills-repo")
	write(t, filepath.Join(root, "eng", "manifest.toml"), "[[plugin]]\nname = \"cflr\"\nrepo = \"cloudflare/skills\"\npath = \"skills\"\nskills = [\"./\"]\n")
	write(t, filepath.Join(root, "notes", "manifest.toml"), "[[plugin]]\nname = \"obsidian\"\nrepo = \"kepano/obsidian-skills\"\n")
	write(t, filepath.Join(root, "idle", "manifest.toml"), "")
	write(t, filepath.Join(root, "aihs.lock"), lockFor("cloudflare/skills", "kepano/obsidian-skills"))
	return root
}

// lockFor pins each repo to a fake commit: "a…a", "b…b", ….
func lockFor(repos ...string) string {
	var b strings.Builder
	for i, r := range repos {
		fmt.Fprintf(&b, "[[repo]]\nrepo = %q\ncommit = %q\n", r, strings.Repeat(string(rune('a'+i)), 40))
	}
	return b.String()
}

func TestHelpAndVersion(t *testing.T) {
	f := newFake("")
	if err := f.run(); err != nil || !strings.Contains(f.out.String(), "create-plugin") {
		t.Fatalf("help: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("help", "create-plugin"); err != nil || !strings.Contains(f.out.String(), "-dry-run") {
		t.Fatalf("help create-plugin: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("create-plugin", "--help"); err != nil || !strings.Contains(f.out.String(), "-pack NAME") {
		t.Fatalf("create-plugin --help: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("--version"); err != nil || f.out.String() != "aihs test\n" {
		t.Fatalf("version: %v %q", err, f.out.String())
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"nope"},
		{"--nope"},
		{"create-plugin", "--bogus"},
		{"create-plugin", "stray"},
		{"help", "nope"},
		{"help", "a", "b"},
	} {
		var usage *UsageError
		if err := newFake("").run(args...); !errors.As(err, &usage) {
			t.Errorf("%v: want a UsageError, got %v", args, err)
		}
	}
}

func TestCreatePluginWritesAndRerunsOverItsOwnFile(t *testing.T) {
	root := checkout(t)
	f := newFake(root)
	if err := f.run("create-plugin"); err != nil {
		t.Fatalf("%v\n%s", err, f.errOut.String())
	}
	data, err := os.ReadFile(marketplace.Path(root))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`"name": "skills-repo-marketplace"`,
		`"description": "Plugins from skills-repo (eng, notes)"`,
		`"name": "Ada Lovelace"`,
		`"url": "https://github.com/ada/skills"`,
		`"source": "git-subdir"`,
		`"sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`"repo": "kepano/obsidian-skills"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("marketplace.json missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "idle") {
		t.Error("a pack that publishes nothing must be left out")
	}
	if !strings.Contains(got, `"dependencies": [
        "cflr"
      ]`) {
		t.Errorf("the eng stack entry should depend on cflr:\n%s", got)
	}
	if !strings.Contains(f.out.String(), "claude not on PATH") {
		t.Errorf("validation should be skipped without claude:\n%s", f.out.String())
	}

	// Rerun, narrowed: overwrites the file it wrote, validates when claude exists.
	f = newFake(root)
	f.onPath["claude"] = true
	if err := f.run("create-plugin", "--pack", "notes"); err != nil {
		t.Fatalf("rerun: %v\n%s", err, f.errOut.String())
	}
	data, _ = os.ReadFile(marketplace.Path(root))
	if strings.Contains(string(data), "cflr") || !strings.Contains(f.out.String(), "claude plugin validate: ok") {
		t.Fatalf("narrowed rerun:\n%s\n%s", data, f.out.String())
	}
}

func TestCreatePluginDryRunWritesNothing(t *testing.T) {
	root := checkout(t)
	f := newFake(root)
	f.onPath["claude"] = true
	if err := f.run("create-plugin", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marketplace.Path(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("--dry-run must not write")
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "claude") {
			t.Fatalf("--dry-run must not validate: %v", f.calls)
		}
	}
	if !strings.Contains(f.out.String(), "would write") || !strings.Contains(f.out.String(), "cloudflare/skills//skills") {
		t.Fatalf("output:\n%s", f.out.String())
	}
}

func TestCreatePluginRefusals(t *testing.T) {
	t.Run("foreign marketplace", func(t *testing.T) {
		root := checkout(t)
		write(t, marketplace.Path(root), `{"plugins":[{"name":"hand-written"}]}`)
		if err := newFake(root).run("create-plugin"); err == nil || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("got %v", err)
		}
		if err := newFake(root).run("create-plugin", "--force"); err != nil {
			t.Fatalf("--force: %v", err)
		}
	})
	t.Run("conflicting definitions", func(t *testing.T) {
		root := checkout(t)
		write(t, filepath.Join(root, "dup", "manifest.toml"), "[[plugin]]\nname = \"cflr\"\nrepo = \"o/r\"\n")
		if err := newFake(root).run("create-plugin"); err == nil || !strings.Contains(err.Error(), `plugin "cflr" is defined differently in dup and eng`) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("include outside --pack", func(t *testing.T) {
		root := checkout(t)
		write(t, filepath.Join(root, "notes", "manifest.toml"), "[pack]\nincludes = [\"eng\"]\n")
		if err := newFake(root).run("create-plugin", "--pack", "notes"); err == nil || !strings.Contains(err.Error(), `notes includes "eng"`) {
			t.Fatalf("got %v", err)
		}
		if err := newFake(root).run("create-plugin", "--pack", "notes", "--pack", "eng"); err != nil {
			t.Fatalf("with the included pack: %v", err)
		}
	})
	t.Run("nothing to publish", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "idle", "manifest.toml"), "")
		if err := newFake(root).run("create-plugin"); err == nil || !strings.Contains(err.Error(), "nothing to publish") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("validation failure exits 1", func(t *testing.T) {
		f := newFake(checkout(t))
		f.onPath["claude"] = true
		f.results["claude plug"] = result{"bad source", errors.New("exit 1")}
		var exit *ExitError
		if err := f.run("create-plugin"); !errors.As(err, &exit) || exit.Code != 1 {
			t.Fatalf("got %v", err)
		}
		if !strings.Contains(f.errOut.String(), "bad source") {
			t.Fatalf("stderr: %s", f.errOut.String())
		}
	})
	t.Run("upstream not in the lock", func(t *testing.T) {
		root := checkout(t)
		write(t, filepath.Join(root, "aihs.lock"), lockFor("cloudflare/skills"))
		if err := newFake(root).run("create-plugin"); err == nil || !strings.Contains(err.Error(), "does not pin kepano/obsidian-skills — run `aihs sync`") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("no root", func(t *testing.T) {
		if err := newFake("").run("create-plugin"); err == nil || !strings.Contains(err.Error(), "AIHS_ROOT is not set") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestDoctor(t *testing.T) {
	root := checkout(t)
	f := newFake(root)
	if err := f.run("doctor"); err != nil {
		t.Fatalf("healthy checkout: %v\n%s", err, f.out.String())
	}
	for _, want := range []string{"✓ eng", "! idle", "✓ aihs.lock", "2 upstream(s) pinned", "! marketplace", "! claude", "0 problem(s), 3 warning(s)"} {
		if !strings.Contains(f.out.String(), want) {
			t.Errorf("doctor missing %q:\n%s", want, f.out.String())
		}
	}

	write(t, filepath.Join(root, "broken", "manifest.toml"), "[[plugin]]\nname = \"x\"\n")
	f = newFake(root)
	var exit *ExitError
	if err := f.run("doctor"); !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("broken manifest: got %v", err)
	}
	if !strings.Contains(f.out.String(), "✗ broken") || !strings.Contains(f.out.String(), "`repo` is required") {
		t.Fatalf("doctor:\n%s", f.out.String())
	}
}

func TestGithubURL(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:ada/skills.git":      "https://github.com/ada/skills",
		"https://github.com/ada/skills":      "https://github.com/ada/skills",
		"https://github.com/ada/skills.git/": "https://github.com/ada/skills",
		"https://gitlab.com/ada/skills.git":  "",
		"":                                   "",
	} {
		if got := githubURL(in); got != want {
			t.Errorf("githubURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCreatePluginPublishesCopilotItemsUpstream(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	write(t, filepath.Join(root, "squad", "manifest.toml"), "[[copilot-plugin]]\nrepo = \"github/awesome-copilot\"\npath = \"plugins/team\"\n")
	write(t, filepath.Join(root, "aihs.lock"), lockFor("github/awesome-copilot"))
	cache := upstream.Dir(root, "github/awesome-copilot", strings.Repeat("a", 40))
	for p, body := range map[string]string{
		".git/HEAD":                strings.Repeat("a", 40) + "\n",
		"plugins/team/plugin.json": `{"name": "team", "extensions": {"com.github.awesome-copilot": {"skills": ["./skills/s/"], "agents": ["./agents/x.md"], "hooks": ["./h"]}}}`,
		"skills/s/SKILL.md":        "---\nname: s\n---\n",
	} {
		write(t, filepath.Join(cache, p), body)
	}
	f := newFake(root)
	if err := f.run("create-plugin"); err != nil {
		t.Fatalf("%v\n%s", err, f.errOut.String())
	}
	if !strings.Contains(f.errOut.String(), "skipped agents, hooks — only Copilot skills are published") {
		t.Errorf("skipped components must be reported:\n%s", f.errOut.String())
	}
	data, _ := os.ReadFile(marketplace.Path(root))
	for _, want := range []string{`"path": "skills"`, `"./s"`, `"sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("marketplace.json missing %s:\n%s", want, data)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "squad")); len(entries) != 1 {
		t.Errorf("nothing may be written into a pack: %v", entries)
	}
}

func cached(t *testing.T, root, repo, commit string, rels ...string) {
	t.Helper()
	dir := upstream.Dir(root, repo, commit)
	write(t, filepath.Join(dir, ".git", "HEAD"), commit+"\n")
	for _, rel := range rels {
		write(t, filepath.Join(dir, rel), "")
	}
}

func TestScanReposOfInterest(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "manifest.toml"), "[[repo]]\nrepo = \"cloudflare/skills\"\n")
	cached(t, root, "cloudflare/skills", strings.Repeat("a", 40),
		".claude-plugin/plugin.json", "skills/one/SKILL.md", "skills/two/SKILL.md", "hooks/hooks.json")
	f := newFake(root)
	if err := f.run("scan", "--paths"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	for _, want := range []string{"cloudflare/skills  aaaaaaa", "1 (2)", "skills  skills  2", "1 repo(s) scanned, 0 failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a locked, cached repo needs no git, ran %v", f.calls)
	}
}

func TestScanNamedRepoFailsWhenItCannotResolve(t *testing.T) {
	f := newFake(checkout(t))
	err := f.run("scan", "nobody/nothing")
	var exit *ExitError
	if !errors.As(err, &exit) || !strings.Contains(f.out.String(), "nobody/nothing") {
		t.Errorf("want exit 1 naming the repo, got %v\n%s", err, f.out.String())
	}
}

func TestScanWithNothingToScan(t *testing.T) {
	if err := newFake(checkout(t)).run("scan"); err == nil || !strings.Contains(err.Error(), "no repos to scan") {
		t.Errorf("got %v", err)
	}
}

func TestIndexWritesCatalog(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "manifest.toml"), "[[repo]]\nrepo = \"cloudflare/skills\"\n")
	cached(t, root, "cloudflare/skills", strings.Repeat("a", 40), ".claude-plugin/plugin.json", "skills/one/SKILL.md")
	f := newFake(root)
	if err := f.run("index"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "catalog", "skills.jsonl"))
	if err != nil || strings.Count(string(data), "\n") != 1 || !strings.Contains(string(data), `"skill:cloudflare/skills:skills/one"`) {
		t.Fatalf("skills.jsonl %q %v", data, err)
	}
	out := f.out.String()
	if !regexpMatch(`(?m)^skill\s+1$`, out) || !strings.Contains(out, "warning(s)") {
		t.Errorf("output:\n%s", out)
	}
	if len(f.calls) != 0 {
		t.Errorf("index must not run git: %v", f.calls)
	}
}

func TestIndexRefusesUnpinnedRepoAndWritesNothing(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "manifest.toml"), "[[repo]]\nrepo = \"a/b\"\n")
	err := newFake(root).run("index")
	if err == nil || !strings.Contains(err.Error(), "aihs sync") {
		t.Errorf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "catalog")); !os.IsNotExist(err) {
		t.Errorf("catalog/ should not exist: %v", err)
	}
}

func TestIndexWithNoReposOfInterest(t *testing.T) {
	if err := newFake(checkout(t)).run("index"); err == nil || !strings.Contains(err.Error(), "manifest.toml") {
		t.Errorf("got %v", err)
	}
}

func regexpMatch(pattern, s string) bool { return regexp.MustCompile(pattern).MatchString(s) }

// withRepos writes catalog/repos.jsonl holding the given repo IDs' records.
func withRepos(t *testing.T, root string, repos ...string) {
	t.Helper()
	var b strings.Builder
	for _, r := range repos {
		fmt.Fprintf(&b, "{\"id\":\"repo:%s\",\"type\":\"repo\",\"repo\":%q}\n", r, r)
	}
	write(t, filepath.Join(root, "catalog", "repos.jsonl"), b.String())
}

const ghRepoJSON = `{"stargazers_count":7,"forks_count":1,"description":"d","topics":[],"license":{"spdx_id":"MIT"},"pushed_at":"2026-10-01T00:00:00Z","archived":false,"default_branch":"main"}`

func TestStatsCommand(t *testing.T) {
	root := checkout(t)
	withRepos(t, root, "o/r", "https://gitlab.com/g/p")
	f := newFake(root)
	f.onPath["gh"] = true
	f.results["gh auth"] = result{"Logged in", nil}
	f.results["gh api"] = result{ghRepoJSON, nil}
	if err := f.run("stats"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	data, _ := os.ReadFile(filepath.Join(root, "catalog", "github.jsonl"))
	if strings.Count(string(data), "\n") != 1 || !strings.Contains(string(data), `"repo":"o/r","stars":7`) ||
		!strings.Contains(string(data), `"fetched_at":"2026-10-09T00:00:00Z"`) {
		t.Errorf("github.jsonl:\n%s", data)
	}
	if !strings.Contains(f.out.String(), "gitlab.com/g/p") || !strings.Contains(f.out.String(), "1 fetched, 1 skipped, 0 failed") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestStatsKeepsPreviousRecordOnFailure(t *testing.T) {
	root := checkout(t)
	withRepos(t, root, "o/r")
	prev := `{"archived":false,"default_branch":"main","description":"old","fetched_at":"2026-01-01T00:00:00Z","forks":0,"license":"","pushed_at":"","repo":"o/r","stars":1,"topics":[]}` + "\n"
	write(t, filepath.Join(root, "catalog", "github.jsonl"), prev)
	f := newFake(root)
	f.onPath["gh"] = true
	f.results["gh api"] = result{"HTTP 502", errors.New("exit 1")}
	var exit *ExitError
	if err := f.run("stats"); !errors.As(err, &exit) || exit.Code != 1 {
		t.Errorf("want exit 1, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "catalog", "github.jsonl")); string(data) != prev {
		t.Errorf("previous record changed:\n%s", data)
	}
}

func TestStatsNeedsGh(t *testing.T) {
	root := checkout(t)
	withRepos(t, root, "o/r")
	err := newFake(root).run("stats")
	if err == nil || !strings.Contains(err.Error(), "gh") {
		t.Errorf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "catalog", "github.jsonl")); !os.IsNotExist(err) {
		t.Errorf("github.jsonl should not exist")
	}
}

func TestPromoteExternalPlugin(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "manifest.toml"), "# repos of interest\n[[repo]]\nrepo = \"o/r\"\n")
	write(t, filepath.Join(root, "catalog", "plugins.jsonl"),
		`{"id":"plugin:o/r#c","type":"plugin","name":"c","repo":"o/r","source_repo":"x/c"}`+"\n"+
			`{"id":"plugin:o/r:p","type":"plugin","name":"p","repo":"o/r"}`+"\n")
	f := newFake(root)
	if err := f.run("promote", "plugin:o/r#c", "https://github.com/y/z.git"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "manifest.toml"))
	if !strings.HasPrefix(string(data), "# repos of interest") || !strings.Contains(string(data), `repo = "x/c"`) || !strings.Contains(string(data), `repo = "y/z"`) {
		t.Errorf("manifest:\n%s", data)
	}
	if !strings.Contains(f.out.String(), "aihs sync && aihs index") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestPromoteUnknownOrLocalPluginID(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "catalog", "plugins.jsonl"), `{"id":"plugin:o/r:p","type":"plugin","name":"p","repo":"o/r"}`+"\n")
	for _, id := range []string{"plugin:o/r#nope", "plugin:o/r:p"} {
		if err := newFake(root).run("promote", id); err == nil || !strings.Contains(err.Error(), id) {
			t.Errorf("%s: got %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "manifest.toml")); !os.IsNotExist(err) {
		t.Error("manifest.toml should not be written")
	}
}

func TestPromoteNeedsArgs(t *testing.T) {
	var usage *UsageError
	if err := newFake(checkout(t)).run("promote"); !errors.As(err, &usage) {
		t.Errorf("got %v", err)
	}
}

func TestPromoteRefusesWhatIsNotARepo(t *testing.T) {
	root := checkout(t)
	for _, arg := range []string{"antropics", "skill:o/r:x", "a/b/c"} {
		var usage *UsageError
		if err := newFake(root).run("promote", arg); !errors.As(err, &usage) {
			t.Errorf("%s: want a UsageError, got %v", arg, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "manifest.toml")); !os.IsNotExist(err) {
		t.Error("manifest.toml should not be written")
	}
}
