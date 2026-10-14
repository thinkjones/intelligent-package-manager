package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	if err := f.run("marketplace", "add", "obra/superpowers", "anthropics/skills@v1", "me/private-skills", "me/unknown"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := f.run("marketplace", "--followed"); err != nil {
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
	if err := f.run("marketplace", "add", "obra/superpowers"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	f.run("marketplace", "--followed")
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
	f.run("marketplace", "add", "obra/superpowers")
	f.out.Reset()
	f.run("marketplace", "--followed")
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
	f.run("marketplace", "add", "obra/superpowers")
	for range 2 {
		f.out.Reset()
		f.run("marketplace", "--followed")
		if !strings.HasPrefix(f.out.String(), "AI · Methods (1)\n") || strings.Contains(f.out.String(), "not grouped") {
			t.Fatalf("list:\n%s", f.out.String())
		}
	}
	if hits != 1 {
		t.Errorf("the list was fetched %d times, want once and then the saved copy", hits)
	}
}

const marketList = `[[repos]]
list = "A"
repo = "ada/skills"
stars = 1200
description = "Ada's skills."

[[repos]]
list = "A"
repo = "bob/agents"
description = "Bob's agents."

[[repos]]
list = "B"
repo = "cy/tools"
stars = 5
`

// marketFake follows ada/skills and has marketList saved as a local file.
func marketFake(t *testing.T) (f *fake, list string) {
	t.Helper()
	root := checkout(t)
	list = filepath.Join(root, "list.toml")
	write(t, list, marketList)
	f = newFake(root)
	if err := f.run("marketplace", "add", "ada/skills"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	return f, list
}

func TestMarketplaceListsCuratedWithFollowed(t *testing.T) {
	f, list := marketFake(t)
	if err := f.run("marketplace", "--list", list); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	a, b := strings.Index(out, "A (2)\n"), strings.Index(out, "B (1)\n")
	if a < 0 || b < a {
		t.Fatalf("want list headers A then B:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(l, "ada/skills") && !strings.Contains(l, "✓"):
			t.Errorf("followed repo has no ✓: %q", l)
		case strings.Contains(l, "bob/agents") && strings.Contains(l, "✓"):
			t.Errorf("repo not followed has a ✓: %q", l)
		}
	}
	if !strings.Contains(out, "★ 1.2k") || !strings.Contains(out, "Bob's agents.") {
		t.Errorf("want stars and description:\n%s", out)
	}
	if strings.Contains(out, "Other") {
		t.Errorf("nothing is followed outside the lists:\n%s", out)
	}
}

func TestMarketplaceFollowedOnly(t *testing.T) {
	f, list := marketFake(t)
	f.run("marketplace", "--followed")
	want := f.out.String()
	f.out.Reset()
	if err := f.run("marketplace", "--followed", "--list", list); err != nil {
		t.Fatal(err)
	}
	if f.out.String() != want || !strings.Contains(want, "ada--skills") || strings.Contains(want, "bob/agents") {
		t.Errorf("--followed =\n%s\nwant the followed listing\n%s", f.out.String(), want)
	}
}

func TestMarketplaceListsFilter(t *testing.T) {
	f, list := marketFake(t)
	if err := f.run("marketplace", "--list", list, "--lists", "B"); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	if !strings.Contains(out, "cy/tools") || strings.Contains(out, "ada/skills") || strings.Contains(out, "bob/agents") {
		t.Errorf("--lists B:\n%s", out)
	}
	var usage *UsageError
	err := f.run("marketplace", "--list", list, "--lists", "Z")
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "B") {
		t.Errorf("unknown list: want a usage error naming A and B, got %v", err)
	}
}

func TestMarketplaceFollowedNotCurated(t *testing.T) {
	f, list := marketFake(t)
	if err := f.run("marketplace", "add", "me/mine"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	f.run("marketplace", "--list", list)
	out := f.out.String()
	i := strings.Index(out, "Other")
	if i < 0 || !strings.Contains(out[i:], "✓ me/mine") {
		t.Errorf("want me/mine under Other:\n%s", out)
	}
}

func TestMarketplaceUnknownWordIsUsageError(t *testing.T) {
	f, _ := marketFake(t)
	var usage *UsageError
	err := f.run("marketplace", "frob")
	if !errors.As(err, &usage) || !strings.Contains(err.Error(), "add, remove, import or stats") {
		t.Errorf("got %v", err)
	}
}
