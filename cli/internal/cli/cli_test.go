package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/tui"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

// fake is a System with no real environment, git or claude. Output answers
// from canned results keyed by the program name and its first arguments.
type fake struct {
	width   int // the terminal width TermWidth reports; 0 is not a terminal
	env     map[string]string
	out     bytes.Buffer
	errOut  bytes.Buffer
	onPath  map[string]bool
	results map[string]result
	// full answers by the whole command line, before results; dirs records
	// each call's working directory, in the same order as calls.
	full  map[string]result
	dirs  []string
	mu    sync.Mutex
	calls []string
	http  *http.Client
	cwd   string
}

type result struct {
	out string
	err error
}

// cacheOf is the IPM_CACHE_DIR newFake gives a checkout root.
func cacheOf(root string) string { return filepath.Join(root, ".ipm-cache") }

func newFake(root string) *fake {
	return &fake{
		env:    map[string]string{"IPM_HOME": filepath.Join(root, ".ipm-home"), "IPM_CACHE_DIR": cacheOf(root)},
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
		Stdout:    &f.out,
		Stderr:    &f.errOut,
		Getenv:    func(k string) string { return f.env[k] },
		TermWidth: func() int { return f.width },
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
			line := name + " " + strings.Join(args, " ")
			f.calls = append(f.calls, line)
			f.dirs = append(f.dirs, dir)
			if r, ok := f.full[line]; ok {
				return r.out, r.err
			}
			r := f.results[key]
			return r.out, r.err
		},
		HTTP: f.http,
		Getwd: func() (string, error) {
			if f.cwd != "" {
				return f.cwd, nil
			}
			return filepath.Dir(f.env["IPM_HOME"]), nil
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

// checkout is an empty working directory; newFake puts IPM_HOME and
// IPM_CACHE_DIR inside it.
func checkout(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHelpAndVersion(t *testing.T) {
	f := newFake("")
	if err := f.run(); err != nil || !strings.Contains(f.out.String(), "source") {
		t.Fatalf("help: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("help", "sync"); err != nil || !strings.Contains(f.out.String(), "-upgrade") {
		t.Fatalf("help sync: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("search", "--help"); err != nil || !strings.Contains(f.out.String(), "--type stringArray") {
		t.Fatalf("search --help: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("help", "install"); err != nil || strings.Contains(f.out.String(), "-no-validate claude") {
		t.Fatalf("a backquoted usage turns into a flag argument name:\n%s", f.out.String())
	}
	f.out.Reset()
	if err := f.run("--version"); err != nil || f.out.String() != "ipm test\n" {
		t.Fatalf("version: %v %q", err, f.out.String())
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"nope"},
		{"--nope"},
		{"sync", "--bogus"},
		{"sync", "stray"},
		{"help", "nope"},
		{"help", "a", "b"},
		{"sync", "-upgrade"}, // POSIX flags: long flags take two dashes
		{"source"},
		{"source", "bogus"},
		{"source", "add"},
		{"pack", "add", "kit"},
		{"pack", "new"},
		{"show"},
		{"show", "a", "b"},
	} {
		var usage *UsageError
		if err := newFake("").run(args...); !errors.As(err, &usage) {
			t.Errorf("%v: want a UsageError, got %v", args, err)
		}
	}
}

func TestDoctorRows(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/one/SKILL.md": "x"}})
	f.run("sync")
	f.out.Reset()
	if err := f.run("doctor"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	out := f.out.String()
	for _, want := range []string{"✓ home", "✓ cache", "✓ pnpm", "12.10.1", "✓ sources", "1 synced", "✓ catalog", "! docker", "! typesense", "! claude", "0 problem(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor missing %q:\n%s", want, out)
		}
	}
	f = newFake(checkout(t))
	var exit *ExitError
	if err := f.run("doctor"); !errors.As(err, &exit) || !strings.Contains(f.out.String(), "✗ pnpm") || !strings.Contains(f.out.String(), "! sources") || !strings.Contains(f.out.String(), "! catalog") {
		t.Errorf("bare machine: %v\n%s", err, f.out.String())
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
	write(t, filepath.Join(cacheOf(root), "catalog", "repos.jsonl"), b.String())
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
	data, _ := os.ReadFile(filepath.Join(cacheOf(root), "catalog", "github.jsonl"))
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
	write(t, filepath.Join(cacheOf(root), "catalog", "github.jsonl"), prev)
	f := newFake(root)
	f.onPath["gh"] = true
	f.results["gh api"] = result{"HTTP 502", errors.New("exit 1")}
	var exit *ExitError
	if err := f.run("stats"); !errors.As(err, &exit) || exit.Code != 1 {
		t.Errorf("want exit 1, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(cacheOf(root), "catalog", "github.jsonl")); string(data) != prev {
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
	if _, err := os.Stat(filepath.Join(cacheOf(root), "catalog", "github.jsonl")); !os.IsNotExist(err) {
		t.Errorf("github.jsonl should not exist")
	}
}

func typesenseAt(f *fake, srv *httptest.Server) {
	target, _ := url.Parse(srv.URL)
	f.http = &http.Client{Transport: rewrite{target, srv.Client().Transport}}
}

type rewrite struct {
	target *url.URL
	next   http.RoundTripper
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return r.next.RoundTrip(req)
}

// healthy is a Typesense stand-in that is up and holds n documents.
func healthy(t *testing.T, n int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		fmt.Fprintf(w, `{"num_documents":%d}`, n)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dockerFake(t *testing.T, inspect result) (*fake, string) {
	t.Helper()
	root := checkout(t)
	f := newFake(root)
	f.onPath["docker"] = true
	f.results["docker info"] = result{"ok", nil}
	f.results["docker insp"] = inspect
	typesenseAt(f, healthy(t, 0))
	return f, root
}

func callWith(f *fake, prefix string) string {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return c
		}
	}
	return ""
}

func TestServeCreatesContainer(t *testing.T) {
	f, root := dockerFake(t, result{"Error: No such object", errors.New("exit 1")})
	if err := f.run("serve"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	keyFile := filepath.Join(cacheOf(root), "typesense", "api-key")
	key, err := os.ReadFile(keyFile)
	info, _ := os.Stat(keyFile)
	if err != nil || !regexpMatch(`^[0-9a-f]{32}$`, string(key)) || info.Mode().Perm() != 0o600 {
		t.Fatalf("key %q %v %v", key, err, info.Mode())
	}
	want := "docker run -d --name ipm-typesense -p 127.0.0.1:8108:8108 -v " + filepath.Join(cacheOf(root), "typesense", "data") +
		":/data typesense/typesense:30.2 --data-dir /data --api-key " + string(key)
	if got := callWith(f, "docker run"); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	f2, _ := dockerFake(t, result{"Error", errors.New("exit 1")})
	f2.env = f.env
	f2.run("serve")
	if again, _ := os.ReadFile(keyFile); string(again) != string(key) {
		t.Error("the key must be reused")
	}
}

func TestServeLeavesRunningContainer(t *testing.T) {
	f, _ := dockerFake(t, result{"true\n", nil})
	if err := f.run("serve"); err != nil {
		t.Fatal(err)
	}
	if callWith(f, "docker run") != "" || callWith(f, "docker start") != "" {
		t.Errorf("calls %v", f.calls)
	}
}

func TestServeStartsStoppedContainer(t *testing.T) {
	f, _ := dockerFake(t, result{"false\n", nil})
	if err := f.run("serve"); err != nil {
		t.Fatal(err)
	}
	if callWith(f, "docker start ipm-typesense") == "" {
		t.Errorf("calls %v", f.calls)
	}
}

func TestServeStop(t *testing.T) {
	f, _ := dockerFake(t, result{"true", nil})
	if err := f.run("serve", "--stop"); err != nil {
		t.Fatal(err)
	}
	if callWith(f, "docker stop ipm-typesense") == "" {
		t.Errorf("calls %v", f.calls)
	}
}

func TestServeWithoutDocker(t *testing.T) {
	f := newFake(checkout(t))
	if err := f.run("serve"); err == nil || !strings.Contains(err.Error(), "Docker") {
		t.Errorf("no docker: %v", err)
	}
	f.onPath["docker"] = true
	f.results["docker info"] = result{"Cannot connect", errors.New("exit 1")}
	if err := f.run("serve"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("docker down: %v", err)
	}
}

func TestServeHealthTimeout(t *testing.T) {
	defer func(e, f time.Duration) { healthEvery, healthFor = e, f }(healthEvery, healthFor)
	healthEvery, healthFor = 10*time.Millisecond, 50*time.Millisecond
	f, _ := dockerFake(t, result{"true", nil})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	typesenseAt(f, srv)
	if err := f.run("serve"); err == nil || !strings.Contains(err.Error(), "docker logs ipm-typesense") {
		t.Errorf("got %v", err)
	}
}

func TestDoctorReportsTypesense(t *testing.T) {
	f, root := dockerFake(t, result{"true", nil})
	os.MkdirAll(filepath.Join(cacheOf(root), "typesense"), 0o755)
	os.WriteFile(filepath.Join(cacheOf(root), "typesense", "api-key"), []byte("k"), 0o600)
	typesenseAt(f, healthy(t, 42))
	f.run("doctor")
	if !strings.Contains(f.out.String(), "✓ docker") || !strings.Contains(f.out.String(), "42 document(s)") {
		t.Errorf("doctor:\n%s", f.out.String())
	}
}

// withCatalog writes a catalog with n skills (and empty files for the other
// types) under root, and a search key.
func withCatalog(t *testing.T, root string, n int) {
	t.Helper()
	var skills strings.Builder
	for i := range n {
		fmt.Fprintf(&skills, "{\"id\":\"skill:o/r:s%04d\",\"type\":\"skill\",\"name\":\"s%d\",\"repo\":\"o/r\"}\n", i, i)
	}
	for _, typ := range catalog.Types {
		body := ""
		if typ == catalog.TypeSkill {
			body = skills.String()
		}
		write(t, filepath.Join(cacheOf(root), "catalog", catalog.Files[typ]), body)
	}
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
}

// tsRecorder is a fake Typesense that records requests and answers imports
// with one result line per document; fail names IDs to reject.
type tsRecorder struct {
	mu       sync.Mutex
	requests []string
	batches  []int
	status   int
	fail     map[string]bool
}

func (r *tsRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req.Method+" "+req.URL.Path)
	if r.status != 0 {
		w.WriteHeader(r.status)
		return
	}
	if strings.HasSuffix(req.URL.Path, "/import") {
		body, _ := io.ReadAll(req.Body)
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		r.batches = append(r.batches, len(lines))
		for _, l := range lines {
			var d map[string]any
			json.Unmarshal([]byte(l), &d)
			if r.fail[d["id"].(string)] {
				fmt.Fprintln(w, `{"code":400,"error":"bad","success":false}`)
			} else {
				fmt.Fprintln(w, `{"success":true}`)
			}
		}
		return
	}
	fmt.Fprint(w, `{}`)
}

func loadFake(t *testing.T, n int) (*fake, *tsRecorder) {
	t.Helper()
	root := checkout(t)
	withCatalog(t, root, n)
	rec := &tsRecorder{}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	f := newFake(root)
	typesenseAt(f, srv)
	return f, rec
}

func TestLoadRebuildsCollection(t *testing.T) {
	f, rec := loadFake(t, 1200)
	if err := f.run("load"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if len(rec.requests) < 3 || rec.requests[0] != "DELETE /collections/artifacts" || rec.requests[1] != "POST /collections" {
		t.Errorf("requests %v", rec.requests)
	}
	if !reflect.DeepEqual(rec.batches, []int{500, 500, 200}) {
		t.Errorf("batches %v", rec.batches)
	}
	if !regexpMatch(`(?m)^skill\s+1200$`, f.out.String()) || !strings.Contains(f.out.String(), "1200 imported") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestLoadReportsFailures(t *testing.T) {
	f, rec := loadFake(t, 3)
	rec.fail = map[string]bool{"skill:o/r:s0001": true}
	var exit *ExitError
	if err := f.run("load"); !errors.As(err, &exit) || exit.Code != 1 {
		t.Errorf("want exit 1, got %v", err)
	}
	if !strings.Contains(f.out.String(), "skill:o/r:s0001") || !strings.Contains(f.out.String(), "bad") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestLoadNeedsServer(t *testing.T) {
	root := checkout(t)
	withCatalog(t, root, 1)
	os.Remove(filepath.Join(cacheOf(root), "typesense", "api-key"))
	if err := newFake(root).run("load"); err == nil || !strings.Contains(err.Error(), "ipm serve") {
		t.Errorf("no key: %v", err)
	}
	f, _ := loadFake(t, 1)
	srv := httptest.NewServer(http.NotFoundHandler())
	typesenseAt(f, srv)
	srv.Close()
	if err := f.run("load"); err == nil || !strings.Contains(err.Error(), "ipm serve") {
		t.Errorf("server down: %v", err)
	}
}

func TestLoadWrongKey(t *testing.T) {
	f, rec := loadFake(t, 1)
	rec.status = 401
	if err := f.run("load"); err == nil || !strings.Contains(err.Error(), "port 8108") {
		t.Errorf("got %v", err)
	}
}

func TestLoadWithoutCatalog(t *testing.T) {
	f, rec := loadFake(t, 1)
	os.RemoveAll(filepath.Join(f.env["IPM_CACHE_DIR"], "catalog"))
	if err := f.run("load"); err == nil || !strings.Contains(err.Error(), "ipm index") {
		t.Errorf("got %v", err)
	}
	if len(rec.requests) != 0 {
		t.Errorf("no request should be sent: %v", rec.requests)
	}
}

// searchFake answers searches with two hits and records the last query.
func searchFake(t *testing.T, status int) (*fake, *url.Values) {
	t.Helper()
	root := checkout(t)
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
	var last url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.Query()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		fmt.Fprint(w, `{"found":2,"search_time_ms":3,"facet_counts":[{"field_name":"type","counts":[{"value":"skill","count":2}]}],"hits":[
			{"document":{"id":"skill:o/r:a","type":"skill","name":"tdd","repo":"o/r","stars":12,"plugin_names":["tools"],"description":"Test first"}},
			{"document":{"id":"skill:o/r:b","type":"skill","name":"pdf","repo":"o/r","stars":0,"plugin_names":[],"description":"PDFs"}}]}`)
	}))
	t.Cleanup(srv.Close)
	f := newFake(root)
	typesenseAt(f, srv)
	return f, &last
}

func TestSearchPrintsTable(t *testing.T) {
	f, last := searchFake(t, 0)
	if err := f.run("search", "--type", "skill", "c#", "testing"); err != nil {
		t.Fatal(err)
	}
	if last.Get("q") != "c# testing" || !strings.Contains(last.Get("filter_by"), "type:=[`skill`]") || !strings.HasSuffix(last.Get("query_by"), "embedding") {
		t.Errorf("query %v", *last)
	}
	if out := f.out.String(); !strings.HasPrefix(out, "SCORE  TYPE") || !strings.Contains(out, "2 match(es) · skill 2 · 3 ms") {
		t.Errorf("output:\n%s", out)
	}
}

func TestSearchKeyword(t *testing.T) {
	f, last := searchFake(t, 0)
	f.run("search", "--keyword", "tdd")
	if strings.Contains(last.Get("query_by"), "embedding") {
		t.Errorf("query_by %q", last.Get("query_by"))
	}
}

func TestSearchJSON(t *testing.T) {
	f, _ := searchFake(t, 0)
	if err := f.run("search", "--json", "tdd"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(f.out.String()), "\n")
	var hit map[string]any
	if len(lines) != 2 || json.Unmarshal([]byte(lines[0]), &hit) != nil || hit["document"] == nil {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestSearchNeedsQuery(t *testing.T) {
	f, _ := searchFake(t, 0)
	var usage *UsageError
	if err := f.run("search"); !errors.As(err, &usage) {
		t.Errorf("got %v", err)
	}
	if err := f.run("search", "--type", "skill"); !errors.As(err, &usage) {
		t.Errorf("flags only: got %v", err)
	}
}

func TestSearchNoIndex(t *testing.T) {
	f, _ := searchFake(t, 404)
	if err := f.run("search", "x"); err == nil || !strings.Contains(err.Error(), "ipm load") {
		t.Errorf("404: %v", err)
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	typesenseAt(f, srv)
	srv.Close()
	if err := f.run("search", "x"); err == nil || !strings.Contains(err.Error(), "ipm serve") {
		t.Errorf("down: %v", err)
	}
}

func TestSearchFlagsAnywhere(t *testing.T) {
	f, last := searchFake(t, 0)
	if err := f.run("search", "c#", "--type", "skill", "testing"); err != nil {
		t.Fatal(err)
	}
	if last.Get("q") != "c# testing" || !strings.Contains(last.Get("filter_by"), "`skill`") {
		t.Errorf("query %v", *last)
	}
}

// withGraphCatalog writes a small linked catalog: repo o/r (10★) → marketplace
// m → plugin p → skill s1, an external plugin e from x/c, and a chain
// deep enough to test tree depth.
func withGraphCatalog(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"repos.jsonl": `{"id":"repo:o/r","type":"repo","name":"o/r","repo":"o/r","commit":"c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1","followed":true}` + "\n" +
			`{"id":"repo:x/c","type":"repo","name":"x/c","repo":"x/c","followed":false}` + "\n",
		"marketplaces.jsonl": `{"id":"marketplace:o/r:.","type":"marketplace","name":"m","repo":"o/r","plugins":["plugin:o/r:p","plugin:o/r#e"]}` + "\n",
		"plugins.jsonl": `{"id":"plugin:o/r#e","type":"plugin","name":"e","repo":"o/r","source_repo":"x/c","entry":{"name":"e","source":{"source":"github","repo":"x/c"}}}` + "\n" +
			`{"id":"plugin:o/r:p","type":"plugin","name":"p","repo":"o/r","commit":"c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1","path":"p","skills":["skill:o/r:p/skills/s1"],"hooks":["hook:o/r:p/hooks/hooks.json"]}` + "\n",
		"skills.jsonl": `{"id":"skill:o/r:p/skills/s1","type":"skill","name":"s1","description":"Test first","tags":["testing"],"repo":"o/r","commit":"c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1","path":"p/skills/s1","plugins":["plugin:o/r:p"]}` + "\n",
		"hooks.jsonl":  `{"id":"hook:o/r:p/hooks/hooks.json","type":"hook","name":"p","repo":"o/r","plugins":["plugin:o/r:p"]}` + "\n",
		"github.jsonl": `{"repo":"o/r","stars":10,"forks":2,"license":"MIT","pushed_at":"2026-10-01T00:00:00Z","topics":["t"]}` + "\n",
	}
	for _, typ := range catalog.Types {
		if _, ok := files[catalog.Files[typ]]; !ok {
			files[catalog.Files[typ]] = ""
		}
	}
	for name, body := range files {
		write(t, filepath.Join(cacheOf(root), "catalog", name), body)
	}
}

func graphFake(t *testing.T) *fake {
	t.Helper()
	root := checkout(t)
	withGraphCatalog(t, root)
	return newFake(root)
}

func TestShow(t *testing.T) {
	f := graphFake(t)
	if err := f.run("show", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	for _, want := range []string{"name", "s1", "type", "skill", "commit", "c1", "stars", "10", "MIT", "up:", "plugin      p", "down:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestShowExternalPlugin(t *testing.T) {
	f := graphFake(t)
	f.run("show", "plugin:o/r#e")
	lines := strings.Split(strings.TrimSpace(f.out.String()), "\n")
	if lines[len(lines)-1] != "add its source: ipm source add plugin:o/r#e" {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestShowJSON(t *testing.T) {
	f := graphFake(t)
	f.run("show", "--json", "skill:o/r:p/skills/s1")
	var m map[string]any
	if err := json.Unmarshal(f.out.Bytes(), &m); err != nil || m["id"] != "skill:o/r:p/skills/s1" || m["description"] != "Test first" {
		t.Errorf("got %v %v\n%s", m, err, f.out.String())
	}
}

func TestTreeAndGraphCommands(t *testing.T) {
	f := graphFake(t)
	if err := f.run("tree", "--up", "skill:o/r:p/skills/s1"); err != nil || !strings.HasPrefix(f.out.String(), "skill       s1\n└─ plugin      p") {
		t.Errorf("tree --up: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("graph", "--format", "dot", "plugin:o/r:p"); err != nil || !strings.HasPrefix(f.out.String(), "digraph") {
		t.Errorf("graph dot: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("graph", "plugin:o/r:p"); err != nil || !strings.HasPrefix(f.out.String(), "flowchart LR") {
		t.Errorf("graph default: %v\n%s", err, f.out.String())
	}
	var usage *UsageError
	if err := f.run("graph", "--format", "svg", "plugin:o/r:p"); !errors.As(err, &usage) {
		t.Errorf("svg: %v", err)
	}
	if err := f.run("tree"); !errors.As(err, &usage) {
		t.Errorf("no id: %v", err)
	}
}

func TestShowWithoutCatalog(t *testing.T) {
	f := newFake(checkout(t))
	for _, cmd := range []string{"show", "tree", "graph"} {
		if err := f.run(cmd, "skill:o/r:x"); err == nil || !strings.Contains(err.Error(), "ipm index") {
			t.Errorf("%s: %v", cmd, err)
		}
	}
}

func TestTreeDefaultDepth(t *testing.T) {
	f := graphFake(t)
	f.run("tree", "repo:o/r")
	// repo (0) → marketplace (1) → plugin p (2) → skill/hook (3): depth 3 shows s1
	if !strings.Contains(f.out.String(), "skill       s1") {
		t.Errorf("repo tree:\n%s", f.out.String())
	}
	f.out.Reset()
	f.run("tree", "--depth", "1", "repo:o/r")
	if strings.Contains(f.out.String(), "plugin      p") {
		t.Errorf("--depth 1 shows grandchildren:\n%s", f.out.String())
	}
}

func TestServeRecreatesContainerWithStaleKey(t *testing.T) {
	f, _ := dockerFake(t, result{"true", nil})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		f.mu.Lock()
		recreated := callWithLocked(f, "docker rm -f ipm-typesense") != ""
		f.mu.Unlock()
		if !recreated {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"num_documents":3}`)
	}))
	defer srv.Close()
	typesenseAt(f, srv)
	if err := f.run("serve"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if callWith(f, "docker rm -f ipm-typesense") == "" || callWith(f, "docker run") == "" {
		t.Errorf("calls %v", f.calls)
	}
	if strings.Contains(f.out.String(), "ipm load") {
		t.Errorf("a populated index needs no load hint:\n%s", f.out.String())
	}
}

func callWithLocked(f *fake, prefix string) string {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return c
		}
	}
	return ""
}

func TestSearchDoubleDashStopsFlags(t *testing.T) {
	f, last := searchFake(t, 0)
	if err := f.run("search", "--", "-x"); err != nil || last.Get("q") != "-x" {
		t.Errorf("-- -x: %v q=%q", err, last.Get("q"))
	}
	if err := f.run("search", "--type", "skill", "foo", "--", "--type"); err != nil || last.Get("q") != "foo --type" || !strings.Contains(last.Get("filter_by"), "`skill`") {
		t.Errorf("foo -- --type: %v q=%q filter=%q", err, last.Get("q"), last.Get("filter_by"))
	}
}

func TestSearchRejectsBlankQueryAndBacktickFilters(t *testing.T) {
	f, _ := searchFake(t, 0)
	var usage *UsageError
	if err := f.run("search", "   "); !errors.As(err, &usage) {
		t.Errorf("blank: %v", err)
	}
	if err := f.run("search", "--type", "sk`ill", "x"); !errors.As(err, &usage) {
		t.Errorf("backtick: %v", err)
	}
}

func TestDoctorWarnsOnEmptyIndex(t *testing.T) {
	f, root := dockerFake(t, result{"true", nil})
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
	typesenseAt(f, healthy(t, 0))
	f.run("doctor")
	if !strings.Contains(f.out.String(), "! typesense") || !strings.Contains(f.out.String(), "ipm load") {
		t.Errorf("doctor:\n%s", f.out.String())
	}
}

// pagedFake has found results, served page by page.
func pagedFake(t *testing.T, found int) (*fake, *[]url.Values) {
	t.Helper()
	root := checkout(t)
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
	var seen []url.Values
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		seen = append(seen, q)
		mu.Unlock()
		per, _ := strconv.Atoi(q.Get("per_page"))
		page, _ := strconv.Atoi(q.Get("page"))
		page = max(page, 1)
		hybrid := ""
		if strings.Contains(q.Get("query_by"), "embedding") {
			hybrid = `,"hybrid_search_info":{"rank_fusion_score":0.5}`
		}
		var hits []string
		for i := (page - 1) * per; i < min(page*per, found); i++ {
			hits = append(hits, fmt.Sprintf(`{"document":{"id":"skill:o/r:%d","type":"skill","name":"s%d","repo":"o/r","stars":0},"text_match":%d%s}`, i, i, 1000-i, hybrid))
		}
		fmt.Fprintf(w, `{"found":%d,"search_time_ms":1,"hits":[%s]}`, found, strings.Join(hits, ","))
	}))
	t.Cleanup(srv.Close)
	f := newFake(root)
	typesenseAt(f, srv)
	return f, &seen
}

func TestSearchPage(t *testing.T) {
	f, seen := pagedFake(t, 45)
	if err := f.run("search", "--page", "2", "x"); err != nil {
		t.Fatal(err)
	}
	if (*seen)[0].Get("page") != "2" || !strings.Contains(f.out.String(), "page 2 of 3") || !strings.Contains(f.out.String(), "s20") {
		t.Errorf("params %v\n%s", *seen, f.out.String())
	}
}

func TestSearchAll(t *testing.T) {
	f, seen := pagedFake(t, 300)
	if err := f.run("search", "--all", "--json", "x"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(f.out.String(), "\n"); n != 300 || len(*seen) != 2 || (*seen)[0].Get("per_page") != "250" {
		t.Errorf("lines %d, requests %d (%v)", n, len(*seen), *seen)
	}
	var usage *UsageError
	if err := f.run("search", "--all", "--page", "2", "x"); !errors.As(err, &usage) {
		t.Errorf("--all with --page: %v", err)
	}
}

func TestSearchKeywordLaterPageScoresAgainstFirst(t *testing.T) {
	f, seen := pagedFake(t, 45)
	if err := f.run("search", "--keyword", "--page", "2", "x"); err != nil {
		t.Fatal(err)
	}
	// page 1's best text_match is 1000; page 2's first hit (i=20) has 980 → 98
	lines := strings.Split(f.out.String(), "\n")
	if len(*seen) != 2 || (*seen)[1].Get("per_page") != "1" || !strings.HasPrefix(lines[1], "98 ") {
		t.Errorf("requests %v\n%s", *seen, f.out.String())
	}
}

// withSources makes f's IPM_HOME hold synced sources: package.json, a pnpm
// lock pinning each repo to a fake commit, and node_modules/<alias> with the
// given files (rel path → body).
func withSources(t *testing.T, f *fake, repos map[string]map[string]string) {
	t.Helper()
	dir := filepath.Join(f.env["IPM_HOME"], "sources")
	var names []string
	for r := range repos {
		names = append(names, r)
	}
	sort.Strings(names)
	if _, err := sources.Add(dir, names); err != nil {
		t.Fatal(err)
	}
	var lock strings.Builder
	lock.WriteString("lockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    dependencies:\n")
	for i, r := range names {
		a := sources.Alias(r)
		fmt.Fprintf(&lock, "      %s:\n        specifier: github:%s\n        version: https://codeload.github.com/%s/tar.gz/%s\n", a, r, r, strings.Repeat(string(rune('a'+i)), 40))
		for rel, body := range repos[r] {
			write(t, filepath.Join(dir, "node_modules", a, rel), body)
		}
		os.MkdirAll(filepath.Join(dir, "node_modules", a), 0o755)
	}
	write(t, filepath.Join(dir, "pnpm-lock.yaml"), lock.String())
}

func TestSourceAddListRemove(t *testing.T) {
	f := newFake(checkout(t))
	if err := f.run("source", "add", "obra/superpowers", "anthropics/skills@v1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.out.String(), "ipm sync") {
		t.Errorf("add output:\n%s", f.out.String())
	}
	f.out.Reset()
	f.run("source", "list")
	if !regexpMatch(`anthropics--skills\s+anthropics/skills\s+v1\s+not synced`, f.out.String()) || !strings.Contains(f.out.String(), "obra/superpowers") {
		t.Errorf("list:\n%s", f.out.String())
	}
	if err := f.run("source", "remove", "obra/superpowers"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	f.run("source", "list")
	if strings.Contains(f.out.String(), "obra") {
		t.Errorf("after remove:\n%s", f.out.String())
	}
	var usage *UsageError
	if err := f.run("source", "frob"); !errors.As(err, &usage) {
		t.Errorf("unknown verb: %v", err)
	}
	if err := f.run("source"); !errors.As(err, &usage) {
		t.Errorf("no verb: %v", err)
	}
}

func TestSourceListShowsCommits(t *testing.T) {
	f := newFake(checkout(t))
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/a/SKILL.md": "---\nname: a\n---\n"}})
	f.run("source", "list")
	if !strings.Contains(f.out.String(), strings.Repeat("a", 7)) {
		t.Errorf("list:\n%s", f.out.String())
	}
}

func TestSourceAddPluginID(t *testing.T) {
	f := newFake(checkout(t))
	write(t, filepath.Join(f.env["IPM_CACHE_DIR"], "catalog", "plugins.jsonl"),
		`{"id":"plugin:o/r#c","type":"plugin","name":"c","repo":"o/r","source_repo":"x/c"}`+"\n")
	if err := f.run("source", "add", "plugin:o/r#c"); err != nil {
		t.Fatal(err)
	}
	got, _ := sources.Load(filepath.Join(f.env["IPM_HOME"], "sources"))
	if len(got) != 1 || got[0].Repo != "x/c" {
		t.Errorf("got %+v", got)
	}
	if err := f.run("source", "add", "plugin:o/r#nope"); err == nil || !strings.Contains(err.Error(), "plugin:o/r#nope") {
		t.Errorf("unknown plugin: %v", err)
	}
}

func TestSourceImport(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "old", "manifest.toml"), "[[repo]]\nrepo = \"o/r\"\n\n[[repo]]\nrepo = \"x/y\"\nversion = \"v2\"\n")
	write(t, filepath.Join(root, "old", "catalog", "github.jsonl"), `{"repo":"o/r","stars":5}`+"\n")
	f := newFake(root)
	if err := f.run("source", "import", filepath.Join(root, "old", "manifest.toml")); err != nil {
		t.Fatal(err)
	}
	got, _ := sources.Load(filepath.Join(f.env["IPM_HOME"], "sources"))
	if len(got) != 2 || got[0].Repo != "o/r" || got[1].Ref != "v2" {
		t.Errorf("got %+v", got)
	}
	if data, _ := os.ReadFile(filepath.Join(f.env["IPM_CACHE_DIR"], "catalog", "github.jsonl")); !strings.Contains(string(data), `"stars":5`) {
		t.Errorf("github.jsonl not copied: %q", data)
	}
}

func TestSyncRunsPnpmThenIndexes(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {".claude-plugin/plugin.json": `{"name":"p"}`, "skills/one/SKILL.md": "---\nname: one\n---\n"}})
	if err := f.run("sync"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	home := f.env["IPM_HOME"]
	src := filepath.Join(home, "sources")
	want := "pnpm install --no-frozen-lockfile --ignore-scripts --config.ignore-pnpmfile=false --config.pnpmfile=" + filepath.Join(src, ".pnpmfile.cjs") + " --store-dir " + filepath.Join(home, "store") + " --dir " + src
	if got := callWith(f, "pnpm install"); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	data, err := os.ReadFile(filepath.Join(f.env["IPM_CACHE_DIR"], "catalog", "skills.jsonl"))
	if err != nil || !strings.Contains(string(data), `"skill:o/r:skills/one"`) || !strings.Contains(string(data), strings.Repeat("a", 40)) {
		t.Errorf("skills.jsonl %q %v", data, err)
	}
}

func TestSyncUpgrade(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/one/SKILL.md": "x"}})
	f.run("sync", "--upgrade")
	if !strings.HasPrefix(callWith(f, "pnpm update"), "pnpm update --latest --ignore-scripts") {
		t.Errorf("calls %v", f.calls)
	}
}

func TestSyncPnpmFailureKeepsCatalog(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/one/SKILL.md": "x"}})
	cat := filepath.Join(f.env["IPM_CACHE_DIR"], "catalog", "skills.jsonl")
	write(t, cat, "old\n")
	f.results["pnpm inst"] = result{"ERR_PNPM_FETCH_404 nope", errors.New("exit 1")}
	var exit *ExitError
	if err := f.run("sync"); !errors.As(err, &exit) || exit.Code != 1 {
		t.Errorf("got %v", err)
	}
	if data, _ := os.ReadFile(cat); string(data) != "old\n" || !strings.Contains(f.out.String(), "ERR_PNPM_FETCH_404") {
		t.Errorf("catalog %q, output:\n%s", data, f.out.String())
	}
}

func TestSyncNeedsPnpmAndSources(t *testing.T) {
	f := newFake(checkout(t))
	if err := f.run("sync"); err == nil || !strings.Contains(err.Error(), "pnpm") {
		t.Errorf("no pnpm: %v", err)
	}
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	if err := f.run("sync"); err == nil || !strings.Contains(err.Error(), "ipm source add") {
		t.Errorf("no sources: %v", err)
	}
}

func TestIndexNeedsSync(t *testing.T) {
	f := newFake(checkout(t))
	sources.Add(filepath.Join(f.env["IPM_HOME"], "sources"), []string{"o/r"})
	if err := f.run("index"); err == nil || !strings.Contains(err.Error(), "ipm sync") {
		t.Errorf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.env["IPM_CACHE_DIR"], "catalog")); !os.IsNotExist(err) {
		t.Error("nothing should be written")
	}
}

func TestScanSources(t *testing.T) {
	f := newFake(checkout(t))
	withSources(t, f, map[string]map[string]string{"o/r": {".claude-plugin/plugin.json": "{}", "skills/one/SKILL.md": "x", "skills/two/SKILL.md": "x"}})
	if err := f.run("scan", "--paths"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if !strings.Contains(f.out.String(), "o/r") || !strings.Contains(f.out.String(), "1 (2)") || len(f.calls) != 0 {
		t.Errorf("output:\n%s\ncalls %v", f.out.String(), f.calls)
	}
	if err := f.run("scan", "x/y"); err == nil || !strings.Contains(err.Error(), "ipm source add") {
		t.Errorf("unknown: %v", err)
	}
}

func TestSyncWritesPnpmfile(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/one/SKILL.md": "x"}})
	if err := f.run("sync"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(f.env["IPM_HOME"], "sources", ".pnpmfile.cjs"))
	root, _ := sources.RootName(filepath.Join(f.env["IPM_HOME"], "sources"))
	for _, want := range []string{"readPackage", `"` + root + `"`, "codeload", "delegate", "/archive/"} {
		if err != nil || !strings.Contains(string(data), want) {
			t.Errorf("pnpmfile missing %q (%v):\n%s", want, err, data)
		}
	}
}

func TestSyncFallsBackToNpxForOldPnpm(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"], f.onPath["npx"] = true, true
	f.results["pnpm --ve"] = result{"10.33.2\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/one/SKILL.md": "x"}})
	if err := f.run("sync"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if !strings.HasPrefix(callWith(f, "npx "), "npx -y pnpm@12.10.1 install --no-frozen-lockfile --ignore-scripts") {
		t.Errorf("calls %v", f.calls)
	}
	f = newFake(checkout(t))
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"10.33.2\n", nil}
	withSources(t, f, map[string]map[string]string{"o/r": {"skills/one/SKILL.md": "x"}})
	if err := f.run("sync"); err == nil || !strings.Contains(err.Error(), "12") {
		t.Errorf("old pnpm and no npx: %v", err)
	}
}

func TestServeRemovesLegacyContainer(t *testing.T) {
	f, _ := dockerFake(t, result{"Error: No such object", errors.New("exit 1")})
	f.results["docker ps"] = result{"aihs-typesense\n", nil}
	if err := f.run("serve"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if callWith(f, "docker rm -f aihs-typesense") == "" || callWith(f, "docker run -d --name ipm-typesense") == "" {
		t.Errorf("calls %v", f.calls)
	}
	if !strings.Contains(f.out.String(), "ipm load") {
		t.Errorf("output:\n%s", f.out.String())
	}
}

func TestFindPnpmIgnoresWarnings(t *testing.T) {
	f := newFake(checkout(t))
	f.onPath["pnpm"], f.onPath["npx"] = true, true
	f.results["pnpm --ve"] = result{"WARN corepack is enabled\n12.3.0\n", nil}
	pn, err := findPnpm(f.system())
	if err != nil || pn.name != "pnpm" || pn.version != "12.3.0" {
		t.Errorf("got %+v %v", pn, err)
	}
	f.results["pnpm --ve"] = result{"boom", errors.New("exit 1")}
	f.onPath["npx"] = false
	if _, err := findPnpm(f.system()); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("a failing pnpm --version should be reported: %v", err)
	}
}

func TestSourceImportIsRepeatable(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(root, "old", "manifest.toml"), "[[repo]]\nrepo = \"o/r\"\n")
	f := newFake(root)
	if err := f.run("source", "import", filepath.Join(root, "old", "manifest.toml")); err != nil {
		t.Fatal(err)
	}
	if err := f.run("source", "import", filepath.Join(root, "old", "manifest.toml")); err != nil || !strings.Contains(f.out.String(), "already") {
		t.Errorf("second import: %v\n%s", err, f.out.String())
	}
}

// packFake is a fake whose working directory is a project root with the
// linked catalog from withGraphCatalog.
func packFake(t *testing.T) (*fake, string) {
	t.Helper()
	root := checkout(t)
	withGraphCatalog(t, root)
	f := newFake(root)
	return f, root
}

func TestPackNewAddShow(t *testing.T) {
	f, root := packFake(t)
	if err := f.run("pack", "new", "csharp-sdlc", "--description", "C# kit"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "add", "csharp-sdlc", "skill:o/r:p/skills/s1", "plugin:o/r#e", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	packs, _ := project.LoadPacks(root)
	if len(packs) != 1 || packs[0].Description != "C# kit" || !reflect.DeepEqual(packs[0].Items, []string{"skill:o/r:p/skills/s1", "plugin:o/r#e"}) {
		t.Errorf("packs %+v", packs)
	}
	f.out.Reset()
	f.run("pack", "show", "csharp-sdlc")
	if !strings.Contains(f.out.String(), "s1") || !strings.Contains(f.out.String(), "plugin:o/r#e") {
		t.Errorf("show:\n%s", f.out.String())
	}
	f.out.Reset()
	f.run("pack", "list")
	if !regexpMatch(`csharp-sdlc\s+2\s+C# kit`, f.out.String()) {
		t.Errorf("list:\n%s", f.out.String())
	}
	if err := f.run("pack", "new", "csharp-sdlc"); err == nil {
		t.Error("an existing pack must be refused")
	}
}

func TestPackNewRefusesReservedNames(t *testing.T) {
	f, _ := packFake(t)
	for _, n := range []string{"claude-tools", "official-claude", "bad name"} {
		if err := f.run("pack", "new", n); err == nil {
			t.Errorf("%q accepted", n)
		}
	}
}

func TestPackAddRefusesUnpickable(t *testing.T) {
	f, _ := packFake(t)
	f.run("pack", "new", "kit")
	err := f.run("pack", "add", "kit", "hook:o/r:p/hooks/hooks.json")
	if err == nil || !strings.Contains(err.Error(), "plugin:o/r:p") || !strings.Contains(err.Error(), "ipm pack add kit") {
		t.Errorf("hook: %v", err)
	}
	if err := f.run("pack", "add", "kit", "repo:o/r"); err == nil || !strings.Contains(err.Error(), "ipm tree") {
		t.Errorf("repo: %v", err)
	}
}

func TestPackAddUnknownIDAndPack(t *testing.T) {
	f, _ := packFake(t)
	f.run("pack", "new", "kit")
	if err := f.run("pack", "add", "kit", "skill:nope"); err == nil || !strings.Contains(err.Error(), "skill:nope") {
		t.Errorf("unknown id: %v", err)
	}
	if err := f.run("pack", "add", "nokit", "skill:o/r:p/skills/s1"); err == nil || !strings.Contains(err.Error(), "ipm pack new") {
		t.Errorf("unknown pack: %v", err)
	}
}

func TestPackRemove(t *testing.T) {
	f, root := packFake(t)
	f.run("pack", "new", "kit")
	f.run("pack", "add", "kit", "skill:o/r:p/skills/s1", "plugin:o/r#e")
	if err := f.run("pack", "remove", "kit", "plugin:o/r#e"); err != nil {
		t.Fatal(err)
	}
	packs, _ := project.LoadPacks(root)
	if !reflect.DeepEqual(packs[0].Items, []string{"skill:o/r:p/skills/s1"}) {
		t.Errorf("items %v", packs[0].Items)
	}
}

func TestPackGlobalScope(t *testing.T) {
	f, root := packFake(t)
	if err := f.run("pack", "-g", "new", "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.env["IPM_HOME"], "global", "ipm.toml")); err != nil {
		t.Errorf("global ipm.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ipm.toml")); !os.IsNotExist(err) {
		t.Error("-g must not write the project's ipm.toml")
	}
}

func TestPackWalksUpToProject(t *testing.T) {
	f, root := packFake(t)
	f.run("pack", "new", "kit")
	sub := filepath.Join(root, "src", "deep")
	os.MkdirAll(sub, 0o755)
	f.cwd = sub
	if err := f.run("pack", "add", "kit", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sub, "ipm.toml")); !os.IsNotExist(err) {
		t.Error("must use the project's ipm.toml, not create one in a subfolder")
	}
}

// installFake is a project with a pack holding a skill, an agent and an
// external plugin, pnpm 12 on PATH (doing nothing) and the picked repo
// already in .ipm/node_modules.
func installFake(t *testing.T) (*fake, string) {
	t.Helper()
	f, root := packFake(t)
	f.onPath["pnpm"] = true
	f.results["pnpm --ve"] = result{"12.10.1\n", nil}
	f.results["git conf"] = result{"Ada\n", nil}
	write(t, filepath.Join(cacheOf(root), "catalog", "agents.jsonl"), `{"id":"agent:o/r:p/agents/rev.md","type":"agent","name":"reviewer","repo":"o/r","commit":"c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1","path":"p/agents/rev.md","plugins":["plugin:o/r:p"]}`+"\n")
	write(t, filepath.Join(root, ".ipm", "node_modules", "o--r", "p", "skills", "s1", "SKILL.md"), "---\nname: s1\n---\n")
	write(t, filepath.Join(root, ".ipm", "node_modules", "o--r", "p", "agents", "rev.md"), "---\nname: reviewer\n---\n")
	if err := f.run("pack", "new", "kit", "--description", "My kit"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "add", "kit", "skill:o/r:p/skills/s1", "agent:o/r:p/agents/rev.md", "plugin:o/r#e"); err != nil {
		t.Fatal(err)
	}
	return f, root
}

func TestInstallBuildsPlugin(t *testing.T) {
	f, root := installFake(t)
	if err := f.run("install"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	ipm := filepath.Join(root, ".ipm")
	for _, rel := range []string{"plugins/kit/skills/s1/SKILL.md", "plugins/kit/agents/reviewer.md", "ipm.lock", ".pnpmfile.cjs", "package.json"} {
		if _, err := os.Stat(filepath.Join(ipm, rel)); err != nil {
			t.Errorf("missing %s", rel)
		}
	}
	var pj map[string]any
	data, _ := os.ReadFile(filepath.Join(ipm, "plugins", "kit", ".claude-plugin", "plugin.json"))
	json.Unmarshal(data, &pj)
	items := pj["metadata"].(map[string]any)["ipm"].(map[string]any)["items"].([]any)
	if pj["name"] != "kit" || pj["description"] != "My kit" || pj["author"].(map[string]any)["name"] != "Ada" ||
		!reflect.DeepEqual(pj["dependencies"], []any{"kit-e"}) || len(items) != 2 {
		t.Errorf("plugin.json:\n%s", data)
	}
	mp, _ := os.ReadFile(filepath.Join(ipm, ".claude-plugin", "marketplace.json"))
	if !strings.Contains(string(mp), `"./plugins/kit"`) || !strings.Contains(string(mp), `"kit-e"`) || !strings.Contains(string(mp), `"work-ipm"`) {
		t.Errorf("marketplace.json:\n%s", mp)
	}
	gi, _ := os.ReadFile(filepath.Join(ipm, ".gitignore"))
	if string(gi) != "node_modules/\nplugins/\nplugins.tmp/\n" {
		t.Errorf(".gitignore %q", gi)
	}
	if got := callWith(f, "pnpm install"); !strings.Contains(got, "--dir "+ipm) {
		t.Errorf("pnpm call %q", got)
	}
	pkg, _ := os.ReadFile(filepath.Join(ipm, "package.json"))
	if !strings.Contains(string(pkg), `"o--r": "github:o/r#c1c1`) {
		t.Errorf("package.json:\n%s", pkg)
	}
	if !strings.Contains(f.out.String(), "plugin:o/r#e is not pinned upstream") {
		t.Errorf("want the unpinned warning:\n%s", f.out.String())
	}
	if !strings.Contains(f.out.String(), "claude plugin marketplace add "+ipm) || !strings.Contains(f.out.String(), "/plugin install kit@work-ipm") {
		t.Errorf("hints:\n%s", f.out.String())
	}
}

func TestInstallKeepsPinsUnlessUpdate(t *testing.T) {
	f, root := installFake(t)
	f.run("install")
	ipm := filepath.Join(root, ".ipm")
	lock, _ := project.LoadLock(ipm)
	s1 := lock["skill:o/r:p/skills/s1"]
	s1.SHA = strings.Repeat("d", 40)
	lock[s1.ID] = s1
	project.SaveLock(ipm, lock)
	f.run("install")
	if l, _ := project.LoadLock(ipm); l[s1.ID].SHA != strings.Repeat("d", 40) {
		t.Errorf("install must keep the pin: %+v", l[s1.ID])
	}
	f.run("install", "--update")
	if l, _ := project.LoadLock(ipm); l[s1.ID].SHA != "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1" {
		t.Errorf("--update must re-pin: %+v", l[s1.ID])
	}
}

func TestInstallFromLockWithoutCatalog(t *testing.T) {
	f, root := installFake(t)
	f.run("install")
	os.RemoveAll(filepath.Join(cacheOf(root), "catalog"))
	os.RemoveAll(filepath.Join(root, ".ipm", "plugins"))
	if err := f.run("install"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".ipm", "plugins", "kit", "agents", "reviewer.md")); err != nil {
		t.Error("the lock alone must be enough to rebuild")
	}
}

func TestInstallUnknownItem(t *testing.T) {
	f, root := installFake(t)
	packs, _ := project.LoadPacks(root)
	packs[0].Items = append(packs[0].Items, "skill:gone")
	project.SavePacks(root, packs)
	if err := f.run("install"); err == nil || !strings.Contains(err.Error(), "skill:gone") {
		t.Errorf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".ipm", "ipm.lock")); !os.IsNotExist(err) {
		t.Error("nothing should be written")
	}
}

func TestInstallIsDeterministic(t *testing.T) {
	f, root := installFake(t)
	f.run("install")
	snap := func() map[string]string {
		m := map[string]string{}
		filepath.WalkDir(filepath.Join(root, ".ipm"), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !strings.Contains(p, "node_modules") {
				b, _ := os.ReadFile(p)
				m[p] = string(b)
			}
			return nil
		})
		return m
	}
	first := snap()
	packs, _ := project.LoadPacks(root)
	project.SavePacks(root, packs)
	f.run("install")
	if !reflect.DeepEqual(first, snap()) {
		t.Error("a second install changed .ipm")
	}
	packs[0].Items = []string{"agent:o/r:p/agents/rev.md"}
	project.SavePacks(root, packs)
	f.run("install")
	if _, err := os.Stat(filepath.Join(root, ".ipm", "plugins", "kit", "skills")); !os.IsNotExist(err) {
		t.Error("a removed item's files must go")
	}
}

func TestInstallValidates(t *testing.T) {
	f, root := installFake(t)
	f.onPath["claude"] = true
	if err := f.run("install"); err != nil {
		t.Fatal(err)
	}
	if callWith(f, "claude plugin validate --strict "+filepath.Join(root, ".ipm")) == "" {
		t.Errorf("calls %v", f.calls)
	}
	f.results["claude plug"] = result{"✘ Validation failed", errors.New("exit 1")}
	var exit *ExitError
	if err := f.run("install"); !errors.As(err, &exit) || !strings.Contains(f.out.String(), "Validation failed") {
		t.Errorf("got %v\n%s", err, f.out.String())
	}
}

func TestInstallGlobal(t *testing.T) {
	f, _ := installFake(t)
	f.run("pack", "-g", "new", "mine")
	f.run("pack", "-g", "add", "mine", "agent:o/r:p/agents/rev.md")
	write(t, filepath.Join(f.env["IPM_HOME"], "global", ".ipm", "node_modules", "o--r", "p", "agents", "rev.md"), "x")
	if err := f.run("install", "-g"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	mp, _ := os.ReadFile(filepath.Join(f.env["IPM_HOME"], "global", ".ipm", ".claude-plugin", "marketplace.json"))
	if !strings.Contains(string(mp), `"ipm-global"`) {
		t.Errorf("marketplace:\n%s", mp)
	}
}

func TestInstallFailureKeepsPreviousBuild(t *testing.T) {
	f, root := installFake(t)
	if err := f.run("install"); err != nil {
		t.Fatal(err)
	}
	lockBefore, _ := os.ReadFile(filepath.Join(root, ".ipm", "ipm.lock"))
	os.RemoveAll(filepath.Join(root, ".ipm", "node_modules", "o--r")) // placement will fail
	packs, _ := project.LoadPacks(root)
	packs[0].Description = "changed"
	project.SavePacks(root, packs)
	if err := f.run("install"); err == nil {
		t.Fatal("want a failure")
	}
	if _, err := os.Stat(filepath.Join(root, ".ipm", "plugins", "kit", "agents", "reviewer.md")); err != nil {
		t.Error("the previous plugins must survive a failed install")
	}
	if after, _ := os.ReadFile(filepath.Join(root, ".ipm", "ipm.lock")); string(after) != string(lockBefore) {
		t.Error("the lock must only change when the build succeeds")
	}
}

func TestSearchShowsAndListsIDs(t *testing.T) {
	f, _ := searchFake(t, 0)
	if err := f.run("search", "tdd"); err != nil {
		t.Fatal(err)
	}
	if out := f.out.String(); !strings.Contains(out, "\n       skill:o/r:a\n") || !strings.Contains(out, "\n       skill:o/r:b\n") {
		t.Errorf("each hit needs its ID beneath it:\n%s", out)
	}
	f.out.Reset()
	if err := f.run("search", "tdd", "--ids"); err != nil {
		t.Fatal(err)
	}
	if f.out.String() != "skill:o/r:a\nskill:o/r:b\n" {
		t.Errorf("--ids prints only IDs, one per line:\n%q", f.out.String())
	}
	if err := f.run("search", "tdd", "--ids", "--json"); err == nil {
		t.Error("--ids and --json don't combine")
	}
}

func TestSearchUsesTheTerminalWidth(t *testing.T) {
	longest := func(out string) int {
		n := 0
		for _, l := range strings.Split(out, "\n") {
			if !strings.HasPrefix(l, " ") {
				n = max(n, utf8.RuneCountInString(l))
			}
		}
		return n
	}
	f, _ := searchFake(t, 0)
	f.width = 50
	f.run("search", "tdd")
	if n := longest(f.out.String()); n > 50 {
		t.Errorf("a 50-column terminal got a %d-column line:\n%s", n, f.out.String())
	}
	f.out.Reset()
	f.width, f.env["COLUMNS"] = 0, "60"
	f.run("search", "tdd")
	if n := longest(f.out.String()); n > 60 {
		t.Errorf("COLUMNS=60 got a %d-column line:\n%s", n, f.out.String())
	}
	f.out.Reset()
	delete(f.env, "COLUMNS")
	f.run("search", "tdd")
	if strings.Contains(f.out.String(), "…") {
		t.Errorf("piped output (no terminal, no COLUMNS) must not be cut:\n%s", f.out.String())
	}
}

func TestCobraCommandLine(t *testing.T) {
	f := newFake("")
	err := f.run("instal")
	if err == nil || !strings.Contains(err.Error(), "did you mean install?") {
		t.Errorf("want a suggestion, got %v", err)
	}
	f.out.Reset()
	if err := f.run("help", "pack", "add"); err != nil || !strings.Contains(f.out.String(), "ipm pack add <pack> <id>...") || !strings.Contains(f.out.String(), "-g, --global") {
		t.Errorf("help pack add: %v\n%s", err, f.out.String())
	}
	f.out.Reset()
	if err := f.run("completion", "zsh"); err != nil || !strings.Contains(f.out.String(), "compdef") {
		t.Errorf("completion zsh: %v", err)
	}
	f.out.Reset()
	if err := f.run("version"); err != nil || f.out.String() != "ipm test\n" {
		t.Errorf("version: %v %q", err, f.out.String())
	}
}

func TestPackGlobalFlagEitherSide(t *testing.T) {
	f, _ := packFake(t)
	if err := f.run("pack", "-g", "new", "one"); err != nil {
		t.Fatal(err)
	}
	if err := f.run("pack", "new", "two", "--global", "--description", "Second"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := f.run("pack", "list", "-g"); err != nil || !strings.Contains(f.out.String(), "one") || !strings.Contains(f.out.String(), "Second") {
		t.Errorf("pack list -g: %v\n%s", err, f.out.String())
	}
}

func TestCommandLineHints(t *testing.T) {
	for args, want := range map[string]string{
		"serach tdd":    "did you mean search?",
		"pack ad kit":   "did you mean add?",
		"sync -upgrade": "long flags take two dashes: --upgrade",
	} {
		err := newFake("").run(strings.Fields(args)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", args, want, err)
		}
	}
}

func TestPackOperationsWithoutOutput(t *testing.T) {
	f, root := packFake(t)
	scope := project.Scope{Root: root}
	sys := f.system()
	if err := createPack(scope, "kit", "My kit"); err != nil {
		t.Fatal(err)
	}
	s1 := "skill:o/r:p/skills/s1"
	p, err := addToPack(sys, scope, "kit", []string{s1})
	if err != nil || !reflect.DeepEqual(p.Items, []string{s1}) {
		t.Fatalf("add: %v %v", p.Items, err)
	}
	if p, _ = addToPack(sys, scope, "kit", []string{s1}); len(p.Items) != 1 {
		t.Errorf("a second add duplicated: %v", p.Items)
	}
	if _, err := addToPack(sys, scope, "kit", []string{"hook:o/r:p/hooks/hooks.json"}); err == nil || !strings.Contains(err.Error(), "comes only with its plugin") {
		t.Errorf("hook: %v", err)
	}
	if packs, _ := project.LoadPacks(root); !reflect.DeepEqual(packs[0].Items, []string{s1}) {
		t.Errorf("a refused add must not write: %v", packs[0].Items)
	}
	if p, err = removeFromPack(scope, "kit", []string{s1}); err != nil || len(p.Items) != 0 {
		t.Errorf("remove: %v %v", p.Items, err)
	}
	if f.out.Len() != 0 {
		t.Errorf("pack operations must not print: %q", f.out.String())
	}
}

func rec(id, name, desc string, tags ...string) map[string]any {
	typ, _, _ := strings.Cut(id, ":")
	var tl []any
	for _, t := range tags {
		tl = append(tl, t)
	}
	return map[string]any{"id": id, "type": typ, "name": name, "description": desc, "repo": "o/r", "tags": tl}
}

func TestLocalSearchRanking(t *testing.T) {
	records := []map[string]any{
		rec("skill:o/r:pdf", "pdf", "tdd notes"),
		rec("skill:o/r:tdd-guard", "tdd-guard", "test-driven"),
		rec("skill:o/r:tdd", "tdd", "Test first"),
		rec("skill:o/r:other", "other", "nothing", "misc"),
	}
	stars := func(id string) func(string) (int, bool) {
		return func(string) (int, bool) { return 0, false }
	}
	res := localSearch(records, stars(""), func(string) int { return 0 }, "tdd", tui.Filter{})
	var names []string
	for _, h := range res.Hits {
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "tdd,tdd-guard,pdf" || res.Found != 3 {
		t.Errorf("ranking %v found %d", names, res.Found)
	}
	byStars := func(repo string) (int, bool) { return 0, true }
	records[1]["repo"], records[2]["repo"] = "big/one", "small/one"
	byStars = func(repo string) (int, bool) {
		if repo == "big/one" {
			return 900, true
		}
		return 1, true
	}
	if res := localSearch(records, byStars, func(string) int { return 0 }, "tdd", tui.Filter{}); res.Hits[0].Name != "tdd-guard" {
		t.Errorf("equal scores should rank by stars: %v", res.Hits[0].Name)
	}
	if res := localSearch(records, stars(""), func(string) int { return 0 }, "tdd", tui.Filter{Type: "agent"}); len(res.Hits) != 0 {
		t.Errorf("type filter: %v", res.Hits)
	}
	var many []map[string]any
	for i := range 60 {
		many = append(many, rec(fmt.Sprintf("skill:o/r:s%02d", i), fmt.Sprintf("tdd %d", i), ""))
	}
	if res := localSearch(many, stars(""), func(string) int { return 0 }, "tdd", tui.Filter{}); len(res.Hits) != 50 || res.Found != 60 {
		t.Errorf("cap: %d hits, found %d", len(res.Hits), res.Found)
	}
}

func TestTuiSearcherFallsBack(t *testing.T) {
	f := newFake(checkout(t))
	s := newTuiSearcher(f.system(), nil, "")
	if s.Mode() != "local" || !strings.Contains(s.Notice(), "server off") {
		t.Errorf("no server: mode %q notice %q", s.Mode(), s.Notice())
	}
	docs := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, `{"ok":true}`)
		case "/collections/artifacts":
			fmt.Fprintf(w, `{"num_documents":%d}`, docs)
		}
	}))
	t.Cleanup(srv.Close)
	root := checkout(t)
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
	f = newFake(root)
	typesenseAt(f, srv)
	s = newTuiSearcher(f.system(), nil, "")
	if s.Mode() != "local" || s.Notice() != "search index empty — run `ipm load`" {
		t.Errorf("empty index: mode %q notice %q", s.Mode(), s.Notice())
	}
	docs = 3
	s.Recheck()
	if s.Mode() != "hybrid" || s.Notice() != "" {
		t.Errorf("ready: mode %q notice %q", s.Mode(), s.Notice())
	}
}

func TestTuiHybridSearch(t *testing.T) {
	f, last := searchFake(t, 0)
	s := &tuiSearcher{sys: f.system()}
	c, _ := tsClient(f.system())
	s.client, s.mode = c, "hybrid"
	res, err := s.Search(context.Background(), "tdd", tui.Filter{Type: "skill"})
	if err != nil || len(res.Hits) != 2 || res.Found != 2 || res.Hits[0].ID != "skill:o/r:a" || res.Hits[0].Stars != 12 {
		t.Fatalf("hits %+v %v", res, err)
	}
	if !strings.Contains(last.Get("filter_by"), "type:=[`skill`]") || last.Get("per_page") != "50" {
		t.Errorf("query %v", *last)
	}
}

func TestTuiPacks(t *testing.T) {
	f, root := packFake(t)
	p := tuiPacks{sys: f.system(), scope: project.Scope{Root: root}}
	if p.Scope() != "project "+root {
		t.Errorf("scope %q", p.Scope())
	}
	if err := p.Create("kit", "My kit"); err != nil {
		t.Fatal(err)
	}
	if err := p.Add("kit", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	if err := p.Add("kit", "hook:o/r:p/hooks/hooks.json"); err == nil || !strings.Contains(err.Error(), "comes only with its plugin") {
		t.Errorf("hook: %v", err)
	}
	packs, err := p.List()
	if err != nil || len(packs) != 1 || packs[0].Description != "My kit" || !reflect.DeepEqual(packs[0].Items, []string{"skill:o/r:p/skills/s1"}) {
		t.Errorf("list %+v %v", packs, err)
	}
	if err := p.Remove("kit", "skill:o/r:p/skills/s1"); err != nil {
		t.Fatal(err)
	}
	if (tuiPacks{global: true}).Scope() != "global" {
		t.Error("global scope")
	}
}

func TestTuiInstallerStreams(t *testing.T) {
	f, _ := installFake(t)
	var out bytes.Buffer
	if err := (tuiInstaller{sys: f.system()}).Install(context.Background(), &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "✓ kit:") || strings.Contains(f.out.String(), "✓ kit:") {
		t.Errorf("install output must go to the writer, not stdout:\n%s", out.String())
	}
}

func TestTuiNeedsTerminal(t *testing.T) {
	f := newFake(checkout(t))
	if err := f.run("tui"); err == nil || err.Error() != "ipm tui needs a terminal" {
		t.Errorf("no terminal: %v", err)
	}
	f.width = 120
	if err := f.run("tui"); err == nil || err.Error() != "no catalog — run `ipm sync`" {
		t.Errorf("no catalog: %v", err)
	}
}

func TestTuiSearcherNoCollectionIsEmptyIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	t.Cleanup(srv.Close)
	root := checkout(t)
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
	f := newFake(root)
	typesenseAt(f, srv)
	if s := newTuiSearcher(f.system(), nil, ""); s.Notice() != noticeIndexEmpty {
		t.Errorf("server up, no collection: notice %q", s.Notice())
	}
}

func TestTuiSearcherConcurrentFirstSearches(t *testing.T) {
	root := checkout(t)
	withGraphCatalog(t, root)
	dir := filepath.Join(cacheOf(root), "catalog")
	want := 0
	for _, typ := range catalog.Types {
		recs, _ := catalog.Load(dir, typ)
		want += len(recs)
	}
	s := newTuiSearcher(newFake(root).system(), nil, dir)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); s.Search(context.Background(), "s1", tui.Filter{}) }()
		go func() { defer wg.Done(); s.Recheck() }()
	}
	wg.Wait()
	if len(s.records) != want {
		t.Errorf("records loaded %d times over: %d, want %d", len(s.records)/max(1, want), len(s.records), want)
	}
}

// deployFake is installFake with claude on PATH and empty claude listings;
// it builds the project first so .ipm/ exists.
func deployFake(t *testing.T) (*fake, string) {
	t.Helper()
	f, root := installFake(t)
	f.onPath["claude"] = true
	f.full = map[string]result{
		"claude plugin marketplace list --json": {"[]", nil},
		"claude plugin list --json":             {"[]", nil},
	}
	if err := f.run("install"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	f.calls, f.dirs = nil, nil
	f.out.Reset()
	return f, root
}

// claudeFixture is a captured claude --json listing with its paths filled in.
func claudeFixture(t *testing.T, name, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReplacer("{{ROOT}}", root, "{{CONFIG}}", "/cfg").Replace(string(data))
}

func cmdLines(cmds []claudeCmd) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, c.Dir+" | "+c.String())
	}
	return out
}

func TestPlanDeployFirstTime(t *testing.T) {
	f, root := deployFake(t)
	ipm := filepath.Join(root, ".ipm")
	cmds, err := planDeploy(f.system(), ipm, "work-ipm", "kit", deployTarget{Scope: "project", Path: root})
	want := []string{
		root + " | claude plugin marketplace add " + ipm + " --scope project",
		root + " | claude plugin install kit@work-ipm --scope project",
	}
	if err != nil || !reflect.DeepEqual(cmdLines(cmds), want) {
		t.Errorf("got %v %v\nwant %v", cmdLines(cmds), err, want)
	}
}

func TestPlanDeployAgain(t *testing.T) {
	f, root := deployFake(t)
	f.full["claude plugin marketplace list --json"] = result{claudeFixture(t, "claude-marketplace-list.json", root), nil}
	f.full["claude plugin list --json"] = result{claudeFixture(t, "claude-plugin-list.json", root), nil}
	ipm := filepath.Join(root, ".ipm")
	cmds, err := planDeploy(f.system(), ipm, "work-ipm", "kit", deployTarget{Scope: "project", Path: root})
	want := []string{
		root + " | claude plugin marketplace add " + ipm + " --scope project",
		root + " | claude plugin marketplace update work-ipm",
		root + " | claude plugin update kit@work-ipm --scope project",
	}
	if err != nil || !reflect.DeepEqual(cmdLines(cmds), want) {
		t.Errorf("got %v %v\nwant %v", cmdLines(cmds), err, want)
	}
}

func TestPlanDeployUserScope(t *testing.T) {
	f, root := deployFake(t)
	f.full["claude plugin marketplace list --json"] = result{claudeFixture(t, "claude-marketplace-list.json", root), nil}
	f.full["claude plugin list --json"] = result{claudeFixture(t, "claude-plugin-list.json", root), nil}
	ipm := filepath.Join(root, ".ipm")
	cmds, err := planDeploy(f.system(), ipm, "work-ipm", "kit", deployTarget{Scope: "user"})
	want := []string{
		root + " | claude plugin marketplace add " + ipm + " --scope user",
		root + " | claude plugin marketplace update work-ipm",
		root + " | claude plugin install kit@work-ipm --scope user", // installed only at project scope
	}
	if err != nil || !reflect.DeepEqual(cmdLines(cmds), want) {
		t.Errorf("got %v %v\nwant %v", cmdLines(cmds), err, want)
	}
}

func TestPlanDeployUnreadableClaudeJSON(t *testing.T) {
	f, root := deployFake(t)
	f.full["claude plugin list --json"] = result{"not json", nil}
	_, err := planDeploy(f.system(), filepath.Join(root, ".ipm"), "work-ipm", "kit", deployTarget{Scope: "project", Path: root})
	if err == nil || !strings.Contains(err.Error(), "claude plugin list --json") {
		t.Errorf("got %v", err)
	}
}

func TestDeployTargetPathExpanded(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "work"), 0o755)
	os.WriteFile(filepath.Join(home, "file"), []byte("x"), 0o644)
	got, err := cleanTarget(deployTarget{Scope: "project", Path: "~/work/"}, "/elsewhere", home)
	if err != nil || got.Path != filepath.Join(home, "work") {
		t.Errorf("~ expansion: %+v %v", got, err)
	}
	if got, err := cleanTarget(deployTarget{Scope: "local", Path: "work"}, home, home); err != nil || got.Path != filepath.Join(home, "work") {
		t.Errorf("relative path: %+v %v", got, err)
	}
	for _, bad := range []deployTarget{{Scope: "project", Path: "~/file"}, {Scope: "project", Path: "~/missing"}, {Scope: "team"}, {Scope: "project"}} {
		if _, err := cleanTarget(bad, home, home); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if got, err := cleanTarget(deployTarget{Scope: "user", Path: "~/work"}, home, home); err != nil || got.Path != "" {
		t.Errorf("user scope keeps no path: %+v %v", got, err)
	}
}

func TestRunDeployBuildsThenRuns(t *testing.T) {
	f, root := deployFake(t)
	if err := f.run("deploy", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	var order []string
	for _, c := range f.calls {
		switch {
		case strings.HasPrefix(c, "pnpm install"), strings.HasPrefix(c, "claude plugin marketplace add"), strings.HasPrefix(c, "claude plugin install"):
			order = append(order, strings.Join(strings.Fields(c)[:3], " "))
		}
	}
	if strings.Join(order, ";") != "pnpm install --no-frozen-lockfile;claude plugin marketplace;claude plugin install" {
		t.Errorf("order %v", order)
	}
	out := f.out.String()
	if !strings.Contains(out, "$ claude plugin install kit@work-ipm --scope project") || !strings.Contains(out, "✓ deployed kit (project "+root+")") {
		t.Errorf("output:\n%s", out)
	}
}

func TestRunDeployStopsOnFailure(t *testing.T) {
	f, root := deployFake(t)
	f.full["claude plugin install kit@work-ipm --scope project"] = result{"boom", errors.New("exit status 1")}
	err := f.run("deploy", "kit", "--path", root)
	if err == nil || !strings.Contains(err.Error(), "claude plugin install kit@work-ipm") || strings.Contains(f.out.String(), "✓ deployed") {
		t.Errorf("err %v\n%s", err, f.out.String())
	}
}

func TestDeployNeedsClaude(t *testing.T) {
	f, _ := deployFake(t)
	f.onPath["claude"] = false
	if err := f.run("deploy", "kit"); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Errorf("got %v", err)
	}
}

func TestDeployPackRequired(t *testing.T) {
	f, _ := deployFake(t)
	f.run("pack", "new", "second")
	var usage *UsageError
	if err := f.run("deploy"); !errors.As(err, &usage) || !strings.Contains(err.Error(), "name a pack") {
		t.Errorf("got %v", err)
	}
}

func TestTuiCatalogNode(t *testing.T) {
	root := checkout(t)
	withGraphCatalog(t, root)
	g, err := catgraph.Load(filepath.Join(cacheOf(root), "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := tuiCatalog{g}.Node("skill:o/r:p/skills/s1")
	if err != nil || n.Name != "s1" || n.Description != "Test first" {
		t.Fatalf("%+v %v", n, err)
	}
	var parents []string
	for _, p := range n.Parents {
		parents = append(parents, p.ID+"="+p.Type)
	}
	if !slices.Contains(parents, "plugin:o/r:p=plugin") {
		t.Errorf("parents %v", parents)
	}
	if _, err := (tuiCatalog{g}).Node("skill:gone"); err == nil {
		t.Error("an unknown ID is an error")
	}
}

func TestTuiDeployerPlan(t *testing.T) {
	f, root := installFake(t) // not built yet: the plan must not need .ipm/
	f.onPath["claude"] = true
	f.full = map[string]result{
		"claude plugin marketplace list --json": {"[]", nil},
		"claude plugin list --json":             {"[]", nil},
	}
	d := tuiDeployer{sys: f.system(), scope: project.Scope{Root: root}}
	if got := d.Default("kit"); got.Scope != "project" || got.Path != root {
		t.Errorf("default %+v", got)
	}
	cleaned, plan, err := d.Plan("kit", tui.Target{Scope: "project", Path: root + "/"})
	if cleaned.Path != root {
		t.Errorf("Plan should return the cleaned target: %+v", cleaned)
	}
	want := []string{"ipm install", "claude plugin marketplace add " + filepath.Join(root, ".ipm") + " --scope project", "claude plugin install kit@work-ipm --scope project"}
	if err != nil || !reflect.DeepEqual(plan, want) {
		t.Errorf("plan %v %v\nwant %v", plan, err, want)
	}
	f.onPath["claude"] = false
	if _, _, err := d.Plan("kit", tui.Target{Scope: "user"}); err == nil || !strings.Contains(err.Error(), "claude") {
		t.Errorf("no claude: %v", err)
	}
}

func TestDeployOutputSaysEachThingOnce(t *testing.T) {
	f, root := deployFake(t)
	var out bytes.Buffer
	if err := (tuiDeployer{sys: f.system(), scope: project.Scope{Root: root}}).Deploy(context.Background(), "kit", tui.Target{Scope: "project", Path: root}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "✓ deployed") || strings.Contains(out.String(), "Install in Claude Code") {
		t.Errorf("the TUI shows its own result, and deploy installs it itself:\n%s", out.String())
	}
	f.out.Reset()
	if err := f.run("deploy", "kit"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(f.out.String(), "✓ deployed kit"); n != 1 || strings.Contains(f.out.String(), "Install in Claude Code") {
		t.Errorf("ipm deploy prints its result once (%d) and no install hints:\n%s", n, f.out.String())
	}
}

func TestPlanDeployRefusesOtherProjectsMarketplace(t *testing.T) {
	f, root := deployFake(t)
	other := filepath.Join(t.TempDir(), "work")
	f.full["claude plugin marketplace list --json"] = result{claudeFixture(t, "claude-marketplace-list.json", other), nil}
	_, err := planDeploy(f.system(), filepath.Join(root, ".ipm"), "work-ipm", "kit", deployTarget{Scope: "project", Path: root})
	if err == nil || !strings.Contains(err.Error(), "work-ipm") || !strings.Contains(err.Error(), other) {
		t.Errorf("a same-named marketplace from another folder must stop the deploy: %v", err)
	}
}

func TestTuiDeployerPlanGlobal(t *testing.T) {
	f, _ := installFake(t)
	f.onPath["claude"] = true
	f.full = map[string]result{
		"claude plugin marketplace list --json": {"[]", nil},
		"claude plugin list --json":             {"[]", nil},
	}
	home := filepath.Join(t.TempDir(), "home")
	os.MkdirAll(filepath.Join(home, "global"), 0o755)
	d := tuiDeployer{sys: f.system(), scope: project.Scope{Root: filepath.Join(home, "global"), Global: true}, global: true}
	_, plan, err := d.Plan("kit", tui.Target{Scope: "user"})
	if err != nil || len(plan) == 0 || plan[0] != "ipm install -g" {
		t.Errorf("plan %v %v", plan, err)
	}
}

func TestShowUsedBy(t *testing.T) {
	root := checkout(t)
	withGraphCatalog(t, root)
	f := newFake(root)
	if err := f.run("show", "repo:x/c"); err != nil {
		t.Fatal(err)
	}
	// x/c is only a placeholder (not a source), so nothing references it.
	if strings.Contains(f.out.String(), "used by:") {
		t.Errorf("a repo that isn't a source has no used-by:\n%s", f.out.String())
	}
}

func TestSearchOldIndex(t *testing.T) {
	root := checkout(t)
	write(t, filepath.Join(cacheOf(root), "typesense", "api-key"), "k")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "{\"message\":\"Could not find a field named `used_by` in the schema for sorting.\"}")
	}))
	t.Cleanup(srv.Close)
	f := newFake(root)
	typesenseAt(f, srv)
	if err := f.run("search", "x"); err == nil || err.Error() != "the search index is from an older ipm — run ipm load" {
		t.Errorf("got %v", err)
	}
}

func TestLocalSearchNameBoost(t *testing.T) {
	other := rec("skill:x/y:other", "other", "cloudflare tips")
	other["repo"] = "x/y"
	sdk := rec("skill:cloudflare/skills:sdk", "agents-sdk", "Build agents")
	sdk["repo"] = "cloudflare/skills"
	none := func(string) (int, bool) { return 0, false }
	res := localSearch([]map[string]any{other, sdk}, none, func(string) int { return 0 }, "cloudflare", tui.Filter{})
	if len(res.Hits) != 2 || res.Hits[0].Name != "agents-sdk" {
		t.Errorf("a repo-name match outranks a description match: %v", res.Hits)
	}
	a, b := rec("skill:p/q:a", "tdd", ""), rec("skill:r/s:b", "tdd", "")
	a["repo"], b["repo"] = "p/q", "r/s"
	usedBy := func(repo string) int {
		if repo == "r/s" {
			return 5
		}
		return 0
	}
	if res := localSearch([]map[string]any{a, b}, none, usedBy, "tdd", tui.Filter{}); res.Hits[0].Repo != "r/s" || res.Hits[0].UsedBy != 5 {
		t.Errorf("equal scores go to the more-used repo: %+v", res.Hits)
	}
}

func TestTuiCatalogNodeReferences(t *testing.T) {
	root := checkout(t)
	withGraphCatalog(t, root)
	g, err := catgraph.Load(filepath.Join(cacheOf(root), "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	c := tuiCatalog{g}
	e, _ := c.Node("plugin:o/r#e")
	if e.External != nil || e.Unresolved != "x/c" {
		t.Errorf("x/c isn't a source: External %+v Unresolved %q", e.External, e.Unresolved)
	}
	m, _ := c.Node("marketplace:o/r:.")
	var ext bool
	for _, ch := range m.Children {
		if ch.ID == "plugin:o/r#e" {
			ext = ch.External
		}
	}
	if !ext {
		t.Error("the marketplace's child ref should be marked external")
	}
	x, _ := c.Node("repo:x/c")
	if len(x.UsedBy) != 0 || x.UsedByRepos != 0 || len(x.Parents) != 0 {
		t.Errorf("repo:x/c %+v", x)
	}
}

func TestShowUsedByCountsAnythingInARepo(t *testing.T) {
	root := checkout(t)
	dir := filepath.Join(cacheOf(root), "catalog")
	files := map[string]string{
		"repos.jsonl": `{"id":"repo:a/m","type":"repo","name":"a/m","repo":"a/m"}` + "\n" + `{"id":"repo:b/s","type":"repo","name":"b/s","repo":"b/s"}` + "\n",
		"plugins.jsonl": `{"id":"plugin:a/m#cf","type":"plugin","name":"cf","repo":"a/m","source_repo":"b/s","entry":{"name":"cf","source":{"source":"url","url":"u"}}}` + "\n" +
			`{"id":"plugin:b/s:.","type":"plugin","name":"skills","repo":"b/s","path":"."}` + "\n",
	}
	for _, typ := range catalog.Types {
		body := files[catalog.Files[typ]]
		write(t, filepath.Join(dir, catalog.Files[typ]), body)
	}
	f := newFake(root)
	if err := f.run("show", "repo:b/s"); err != nil {
		t.Fatal(err)
	}
	if out := f.out.String(); !strings.Contains(out, "used by:") || !strings.Contains(out[strings.Index(out, "used by:"):], "plugin:a/m#cf") {
		t.Errorf("a repo's used-by includes references to its plugins:\n%s", out)
	}
}

func TestShowSourceHintOnlyWhenUnresolved(t *testing.T) {
	root := checkout(t)
	dir := filepath.Join(cacheOf(root), "catalog")
	files := map[string]string{
		"repos.jsonl": `{"id":"repo:a/m","type":"repo","name":"a/m","repo":"a/m","followed":true}` + "\n" +
			`{"id":"repo:b/s","type":"repo","name":"b/s","repo":"b/s","followed":true}` + "\n" +
			`{"id":"repo:c/x","type":"repo","name":"c/x","repo":"c/x","followed":false}` + "\n",
		"plugins.jsonl": `{"id":"plugin:a/m#cf","type":"plugin","name":"cf","repo":"a/m","source_repo":"b/s","entry":{"name":"cf","source":{"source":"url","url":"u"}}}` + "\n" +
			`{"id":"plugin:a/m#other","type":"plugin","name":"other","repo":"a/m","source_repo":"c/x","entry":{"name":"other","source":{"source":"url","url":"u"}}}` + "\n" +
			`{"id":"plugin:b/s:.","type":"plugin","name":"cf","repo":"b/s","path":"."}` + "\n",
	}
	for _, typ := range catalog.Types {
		write(t, filepath.Join(dir, catalog.Files[typ]), files[catalog.Files[typ]])
	}
	f := newFake(root)
	f.run("show", "plugin:a/m#cf")
	if strings.Contains(f.out.String(), "ipm source add") {
		t.Errorf("b/s is already a source:\n%s", f.out.String())
	}
	f.out.Reset()
	f.run("show", "plugin:a/m#other")
	if !strings.Contains(f.out.String(), "ipm source add plugin:a/m#other") {
		t.Errorf("c/x isn't a source:\n%s", f.out.String())
	}
}
