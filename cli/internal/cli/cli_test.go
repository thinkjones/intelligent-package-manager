package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/sources"
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
	http    *http.Client
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
		HTTP:    f.http,
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
	if err := f.run("search", "--help"); err != nil || !strings.Contains(f.out.String(), "-type value") {
		t.Fatalf("search --help: %v\n%s", err, f.out.String())
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
		"repos.jsonl": `{"id":"repo:o/r","type":"repo","name":"o/r","repo":"o/r","commit":"c1","followed":true}` + "\n" +
			`{"id":"repo:x/c","type":"repo","name":"x/c","repo":"x/c","followed":false}` + "\n",
		"marketplaces.jsonl": `{"id":"marketplace:o/r:.","type":"marketplace","name":"m","repo":"o/r","plugins":["plugin:o/r:p","plugin:o/r#e"]}` + "\n",
		"plugins.jsonl": `{"id":"plugin:o/r#e","type":"plugin","name":"e","repo":"o/r","source_repo":"x/c"}` + "\n" +
			`{"id":"plugin:o/r:p","type":"plugin","name":"p","repo":"o/r","commit":"c1","path":"p","skills":["skill:o/r:p/skills/s1"],"hooks":["hook:o/r:p/hooks/hooks.json"]}` + "\n",
		"skills.jsonl": `{"id":"skill:o/r:p/skills/s1","type":"skill","name":"s1","description":"Test first","tags":["testing"],"repo":"o/r","commit":"c1","path":"p/skills/s1","plugins":["plugin:o/r:p"]}` + "\n",
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
	want := "pnpm install --ignore-scripts --store-dir " + filepath.Join(home, "store") + " --dir " + filepath.Join(home, "sources")
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
	for _, want := range []string{"readPackage", `"ipm-sources"`, "codeload", "delegate", "/archive/"} {
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
	if !strings.HasPrefix(callWith(f, "npx "), "npx -y pnpm@12.10.1 install --ignore-scripts") {
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
