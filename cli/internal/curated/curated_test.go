package curated

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/lists.toml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func names(lists []List) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l.Name)
	}
	return out
}

func find(lists []List, name string) List {
	for _, l := range lists {
		if l.Name == name {
			return l
		}
	}
	return List{}
}

func TestParseOrdersListsAndRepos(t *testing.T) {
	lists, skipped, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names(lists), ","); got != "Beta,Alpha,Gamma" {
		t.Errorf("lists = %s, want Beta,Alpha,Gamma", got)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	beta := find(lists, "Beta")
	if len(beta.Repos) != 2 || beta.Repos[0].Repo != "b/high" || beta.Repos[1].Repo != "b/low" {
		t.Errorf("Beta repos = %+v, want b/high then b/low", beta.Repos)
	}
	if r := beta.Repos[0]; r.Stars != 900 || r.Language != "Go" || r.Description != "high" || strings.Join(r.Topics, ",") != "tdd,hooks" || r.List != "Beta" {
		t.Errorf("b/high = %+v", r)
	}
	for _, l := range []string{"Alpha", "Gamma"} {
		found := false
		for _, r := range find(lists, l).Repos {
			found = found || r.Repo == "shared/both"
		}
		if !found {
			t.Errorf("shared/both missing from %s", l)
		}
	}
}

func TestParseLastPushForms(t *testing.T) {
	lists, _, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	same := func(a time.Time) bool { y, m, d := a.Date(); return y == 2026 && m == 10 && d == 10 }
	for _, repo := range []string{"b/low", "b/high", "a/one"} {
		var got time.Time
		for _, l := range lists {
			for _, r := range l.Repos {
				if r.Repo == repo {
					got = r.LastPush
				}
			}
		}
		if !same(got) {
			t.Errorf("%s last push = %v, want the date of %v", repo, got, want)
		}
	}
	if g := find(lists, "Gamma").Repos; !g[len(g)-1].LastPush.IsZero() {
		t.Errorf("g/x last push = %v, want zero", g[len(g)-1].LastPush)
	}
}

func TestParseIgnoresUnknownFieldsAndMeta(t *testing.T) {
	for _, body := range []string{
		"[meta]\ncount = 1\nsource = \"x\"\n[[repos]]\nlist = \"L\"\nrepo = \"o/r\"\nforks = 3\nlicense = \"MIT\"\n",
		"[[repos]]\nlist = \"L\"\nrepo = \"o/r\"\n",
	} {
		lists, _, err := Parse([]byte(body))
		if err != nil || len(lists) != 1 || lists[0].Repos[0].Repo != "o/r" {
			t.Errorf("Parse(%q) = %+v, %v", body, lists, err)
		}
	}
}

func TestFetchCachesAndFallsBack(t *testing.T) {
	body := fixture(t)
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(500)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()
	cache := t.TempDir()
	got, cached, err := Fetch(srv.Client(), srv.URL+"/list.toml", cache)
	if err != nil || cached || !bytes.Equal(got, body) {
		t.Fatalf("first fetch: cached=%v err=%v", cached, err)
	}
	files, _ := filepath.Glob(filepath.Join(cache, "curated", "*.toml"))
	if len(files) != 1 {
		t.Fatalf("cache files = %v, want 1", files)
	}
	fail = true
	got, cached, err = Fetch(srv.Client(), srv.URL+"/list.toml", cache)
	if err != nil || !cached || !bytes.Equal(got, body) {
		t.Fatalf("fallback: cached=%v err=%v", cached, err)
	}
}

func TestFetchBadResponseKeepsCache(t *testing.T) {
	body := fixture(t)
	mode := "ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "404":
			http.NotFound(w, r)
		case "html":
			w.Write([]byte("<html><body>hello</body></html>"))
		default:
			w.Write(body)
		}
	}))
	defer srv.Close()
	cache := t.TempDir()
	url := srv.URL + "/list.toml"
	if _, _, err := Fetch(srv.Client(), url, cache); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(cache, "curated", "*.toml"))
	for _, m := range []string{"404", "html"} {
		mode = m
		got, cached, err := Fetch(srv.Client(), url, cache)
		if err != nil || !cached || !bytes.Equal(got, body) {
			t.Errorf("%s: cached=%v err=%v", m, cached, err)
		}
		now, _ := os.ReadFile(files[0])
		if !bytes.Equal(now, body) {
			t.Errorf("%s: cache file changed", m)
		}
	}
}

func TestFetchNoCacheErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	url := srv.URL + "/list.toml"
	_, _, err := Fetch(srv.Client(), url, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), url) || !strings.Contains(err.Error(), "--list") {
		t.Errorf("err = %v, want the URL and --list", err)
	}
}

func TestFetchPath(t *testing.T) {
	cache := t.TempDir()
	got, cached, err := Fetch(nil, "testdata/lists.toml", cache)
	if err != nil || cached || !bytes.Equal(got, fixture(t)) {
		t.Fatalf("cached=%v err=%v", cached, err)
	}
	if entries, _ := os.ReadDir(cache); len(entries) != 0 {
		t.Errorf("cache dir has %d entries, want none", len(entries))
	}
}

func TestRawURL(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://github.com/thinkjones/agentic-tools/blob/main/starred_repos.toml", "https://raw.githubusercontent.com/thinkjones/agentic-tools/main/starred_repos.toml"},
		{"https://github.com/o/r/blob/v1.2/lists/a.toml?plain=1", "https://raw.githubusercontent.com/o/r/v1.2/lists/a.toml"},
		{"https://example.com/blob/x.toml", "https://example.com/blob/x.toml"},
		{DefaultURL, DefaultURL},
	} {
		if got := rawURL(tc.in); got != tc.want {
			t.Errorf("rawURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseDropsControlCharacters(t *testing.T) {
	lists, _, err := Parse([]byte("[[repos]]\nrepo = \"o/r\"\nlist = \"L\\u001b[2J\"\ndescription = \"red \\u001b[31m text\\u0007 and\\na line\"\nhomepage = \"https://h.example/\\u001b]0;x\"\ntopics = [\"a\\u001bb\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := lists[0].Repos[0]
	for _, s := range []string{lists[0].Name, r.List, r.Description, r.Homepage, r.Topics[0]} {
		for _, c := range s {
			if c < 0x20 || c == 0x7f {
				t.Errorf("%q keeps a control character", s)
			}
		}
	}
	if r.Description != "red [31m text and a line" {
		t.Errorf("description = %q", r.Description)
	}
}
