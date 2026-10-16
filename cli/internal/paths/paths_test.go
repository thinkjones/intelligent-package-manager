package paths

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolveDefaults(t *testing.T) {
	for _, c := range []struct {
		goos        string
		env         map[string]string
		home, cache string
	}{
		{"darwin", map[string]string{"HOME": "/u"}, "/u/Library/ipm", "/u/Library/Caches/ipm"},
		{"linux", map[string]string{"HOME": "/u"}, "/u/.local/share/ipm", "/u/.cache/ipm"},
		{"linux", map[string]string{"HOME": "/u", "XDG_DATA_HOME": "/d", "XDG_CACHE_HOME": "/c"}, "/d/ipm", "/c/ipm"},
		{"darwin", map[string]string{"HOME": "/u", "XDG_CACHE_HOME": "/c"}, "/u/Library/ipm", "/c/ipm"},
		{"darwin", map[string]string{"HOME": "/u", "XDG_DATA_HOME": "/d"}, "/d/ipm", "/u/Library/Caches/ipm"},
		{"windows", map[string]string{"USERPROFILE": `C:\u`, "LOCALAPPDATA": "/la"}, "/la/ipm", "/la/ipm/cache"},
	} {
		p, err := Resolve(env(c.env), c.goos)
		if err != nil || p.Home != c.home || p.Cache != c.cache {
			t.Errorf("%s %v: got %+v %v, want %s %s", c.goos, c.env, p, err, c.home, c.cache)
		}
	}
}

func TestResolveOverrides(t *testing.T) {
	p, err := Resolve(env(map[string]string{"HOME": "/u", "IPM_HOME": "/h", "IPM_CACHE_DIR": "/k"}), "darwin")
	if err != nil || p.Home != "/h" || p.Cache != "/k" {
		t.Fatalf("got %+v %v", p, err)
	}
	if p.Sources() != "/h/sources" || p.Store() != "/h/store" || p.Catalog() != "/k/catalog" || p.Typesense() != "/k/typesense" || p.Pinned() != "/h/pinned" {
		t.Errorf("subdirs: %s %s %s %s %s", p.Sources(), p.Store(), p.Catalog(), p.Typesense(), p.Pinned())
	}
}

func TestResolveRejectsRelative(t *testing.T) {
	for _, k := range []string{"IPM_HOME", "IPM_CACHE_DIR"} {
		_, err := Resolve(env(map[string]string{"HOME": "/u", k: "rel/dir"}), "darwin")
		if err == nil || !strings.Contains(err.Error(), k) || !strings.Contains(err.Error(), "absolute") {
			t.Errorf("%s: got %v", k, err)
		}
	}
}

func TestResolveNeedsHome(t *testing.T) {
	if _, err := Resolve(env(nil), "linux"); err == nil || !strings.Contains(err.Error(), "HOME") {
		t.Errorf("got %v", err)
	}
	if _, err := Resolve(env(map[string]string{"IPM_HOME": "/h", "IPM_CACHE_DIR": "/k"}), "linux"); err != nil {
		t.Errorf("overrides need no HOME: %v", err)
	}
}
