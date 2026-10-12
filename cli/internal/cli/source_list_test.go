package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
)

const sourcePacksList = `[[repos]]
list = "AI · Methods"
repo = "obra/superpowers"
stars = 297229
description = "An agentic skills framework."
homepage = "https://superpowers.example"
topics = ["skills", "sdlc"]

[[repos]]
list = "AI · Skills"
repo = "Anthropics/Skills"
stars = 41200
description = "Agent Skills."

[[repos]]
list = "AI · Skills"
repo = "obra/superpowers"
stars = 297229

[[repos]]
list = "AI · Unused"
repo = "o/not-followed"
`

// savePacks writes list where ipm keeps its copy of the source packs.
func savePacks(t *testing.T, f *fake, list string) {
	t.Helper()
	sum := sha256.Sum256([]byte(curated.DefaultURL))
	write(t, filepath.Join(f.env["IPM_CACHE_DIR"], "curated", hex.EncodeToString(sum[:])[:16]+".toml"), list)
}

func TestSourceListGroupsByPack(t *testing.T) {
	f := newFake(checkout(t))
	savePacks(t, f, sourcePacksList)
	write(t, filepath.Join(f.env["IPM_CACHE_DIR"], "catalog", "github.jsonl"),
		`{"repo":"me/private-skills","stars":7,"description":"Mine.","topics":["mine"]}`+"\n")
	if err := f.run("source", "add", "obra/superpowers", "anthropics/skills@v1", "me/private-skills", "me/unknown"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := f.run("source", "list"); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	want := `AI · Methods (1)
  obra--superpowers   obra/superpowers   ★ 297k   —   not synced
      An agentic skills framework.
      topics: skills, sdlc · https://superpowers.example

AI · Skills (1)
  anthropics--skills  anthropics/skills  ★ 41.2k  v1  not synced
      Agent Skills.

Other (2)
  me--private-skills  me/private-skills  ★ 7      —   not synced
      Mine.
      topics: mine
  me--unknown         me/unknown         ★ —      —   not synced

4 source(s) in `
	if !strings.HasPrefix(out, want) {
		t.Errorf("list =\n%s\nwant it to start\n%s", out, want)
	}
}

func TestSourceListWithoutPacksIsOneGroup(t *testing.T) {
	f := newFake(checkout(t))
	if err := f.run("source", "add", "obra/superpowers"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	f.run("source", "list")
	out := f.out.String()
	if !strings.HasPrefix(out, "Sources (1)\n  obra--superpowers  obra/superpowers  ★ —  —  not synced\n\n1 source(s) in ") {
		t.Errorf("list:\n%s", out)
	}
	if !strings.Contains(out, "- not grouped: no copy of the source packs yet — `ipm onboard` fetches one\n") {
		t.Errorf("want a line saying why there are no groups:\n%s", out)
	}
}

func TestSourceListCutsToTheTerminal(t *testing.T) {
	f := newFake(checkout(t))
	savePacks(t, f, strings.Replace(sourcePacksList, "An agentic skills framework.", strings.Repeat("long words ", 30), 1))
	f.width = 60
	f.run("source", "add", "obra/superpowers")
	f.out.Reset()
	f.run("source", "list")
	for _, l := range strings.Split(f.out.String(), "\n") {
		if n := len([]rune(l)); n > 60 && !strings.Contains(l, "source(s) in") {
			t.Errorf("%d columns: %q", n, l)
		}
	}
	if !strings.Contains(f.out.String(), "long words long…") && !strings.Contains(f.out.String(), "…\n") {
		t.Errorf("want the description cut with …:\n%s", f.out.String())
	}
}

func TestSourceListFetchesMissingPacksOnce(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(sourcePacksList))
	}))
	defer srv.Close()
	f := newFake(checkout(t))
	typesenseAt(f, srv)
	f.run("source", "add", "obra/superpowers")
	for range 2 {
		f.out.Reset()
		f.run("source", "list")
		if !strings.HasPrefix(f.out.String(), "AI · Methods (1)\n") || strings.Contains(f.out.String(), "not grouped") {
			t.Fatalf("list:\n%s", f.out.String())
		}
	}
	if hits != 1 {
		t.Errorf("the list was fetched %d times, want once and then the saved copy", hits)
	}
}
