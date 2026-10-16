package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/tui"
)

// scopeOf is the project scope the fake's working directory is in.
func scopeOf(t *testing.T, f *fake, global bool) project.Scope {
	t.Helper()
	s, err := currentScope(f.system(), global)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTuiPacksListBuiltState(t *testing.T) {
	f, root := installFake(t)
	f.onPath["claude"] = true
	f.full = map[string]result{
		"claude plugin marketplace list --json": {"[]", nil},
		"claude plugin list --json":             {"[]", nil},
	}
	p := tuiPacks{sys: f.system(), scope: scopeOf(t, f, false)}
	built := func() string {
		t.Helper()
		packs, err := p.List(false)
		if err != nil || len(packs) != 1 {
			t.Fatalf("list %+v %v", packs, err)
		}
		return packs[0].Built
	}
	if got := built(); got != "never" {
		t.Errorf("before build: %q", got)
	}
	if err := f.run("pack", "build"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	if got := built(); got != "fresh" {
		t.Errorf("after build: %q", got)
	}
	if err := f.run("pack", "remove", "kit", "agent:o/r:p/agents/rev.md"); err != nil {
		t.Fatal(err)
	}
	if got := built(); got != "stale" {
		t.Errorf("after pack remove: %q", got)
	}
	if err := f.run("install", "claude", "kit"); err != nil {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	packs, _ := p.List(false)
	want := []tui.Install{{Pack: "kit", Target: tui.Target{Kind: "claude", Scope: "project", Path: root}}}
	if !reflect.DeepEqual(packs[0].Installs, want) {
		t.Errorf("installs %+v\nwant %+v", packs[0].Installs, want)
	}
}

func TestTuiPacksListGlobal(t *testing.T) {
	f, _ := installFake(t)
	if err := f.run("pack", "-g", "new", "mine"); err != nil {
		t.Fatal(err)
	}
	p := tuiPacks{sys: f.system(), scope: scopeOf(t, f, false)}
	local, err := p.List(false)
	if err != nil || len(local) != 1 || local[0].Name != "kit" || local[0].Global {
		t.Errorf("List(false) %+v %v", local, err)
	}
	all, err := p.List(true)
	if err != nil || len(all) != 2 || all[0].Name != "kit" || all[0].Global || all[1].Name != "mine" || !all[1].Global || all[1].Built != "never" {
		t.Errorf("List(true) %+v %v", all, err)
	}
}

func TestTuiInstallerListAndInstall(t *testing.T) {
	f, root, tmp := gascityFake(t)
	f.onPath["claude"] = true
	f.full = map[string]result{
		"claude plugin marketplace list --json": {"[]", nil},
		"claude plugin list --json":             {"[]", nil},
	}
	in := tuiInstaller{sys: f.system(), scope: scopeOf(t, f, false)}
	var out bytes.Buffer
	gas := tui.Target{Kind: "gascity", Path: tmp, Name: "kit"}
	if err := in.Install(context.Background(), "kit", gas, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "kit", "pack.toml")); err != nil {
		t.Errorf("no pack folder: %v", err)
	}
	if strings.Contains(f.out.String(), "Import it") {
		t.Errorf("output must go to the writer, not stdout:\n%s", f.out.String())
	}
	list, err := in.List(false)
	if want := []tui.Install{{Pack: "kit", Target: gas}}; err != nil || !reflect.DeepEqual(list, want) {
		t.Errorf("list %+v %v", list, err)
	}

	f.calls = nil
	if err := in.Install(context.Background(), "kit", tui.Target{Kind: "claude", Scope: "project", Path: root}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "claude plugin install kit@work-ipm --scope project") {
		t.Errorf("calls %v", f.calls)
	}
	if list, _ = in.List(false); len(list) != 2 {
		t.Errorf("list after claude %+v", list)
	}

	if err := in.Install(context.Background(), "kit", tui.Target{Kind: "zip"}, io.Discard); err == nil || !strings.Contains(err.Error(), "zip") {
		t.Errorf("unknown kind: %v", err)
	}
	if got := in.Targets(); !reflect.DeepEqual(got, []string{"claude", "gascity"}) {
		t.Errorf("targets %v", got)
	}
}

func TestTuiInstallerListGlobal(t *testing.T) {
	f, _, tmp := gascityFake(t)
	if err := f.run("pack", "-g", "new", "mine"); err != nil {
		t.Fatal(err)
	}
	gscope := scopeOf(t, f, true)
	if err := recordInstall(f.system(), gscope, "mine", project.Install{Target: "gascity", Path: tmp, Name: "mine"}); err != nil {
		t.Fatal(err)
	}
	in := tuiInstaller{sys: f.system(), scope: scopeOf(t, f, false)}
	if list, err := in.List(false); err != nil || len(list) != 0 {
		t.Errorf("List(false) %+v %v", list, err)
	}
	want := []tui.Install{{Pack: "mine", Target: tui.Target{Kind: "gascity", Path: tmp, Name: "mine", Global: true}}}
	if list, err := in.List(true); err != nil || !reflect.DeepEqual(list, want) {
		t.Errorf("List(true) %+v %v", list, err)
	}
}

func TestTuiInstallerPlanGascity(t *testing.T) {
	f, _, tmp := gascityFake(t)
	in := tuiInstaller{sys: f.system(), scope: scopeOf(t, f, false)}
	got, plan, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: tmp + "/", Name: "kit"})
	want := []string{"ipm install gascity kit -C " + tmp + " --name kit"}
	if err != nil || !reflect.DeepEqual(plan, want) || got.Path != tmp {
		t.Errorf("plan %v %+v %v\nwant %v", plan, got, err, want)
	}
	_, plan, err = in.Plan("kit", tui.Target{Kind: "gascity", Path: tmp, Name: "kit", Global: true})
	if err != nil || !reflect.DeepEqual(plan, []string{want[0] + " -g"}) {
		t.Errorf("global plan %v %v", plan, err)
	}
	if _, _, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: tmp, Name: "bad name"}); err == nil {
		t.Error("a bad folder name should be refused")
	}
	if _, _, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: filepath.Join(tmp, "missing"), Name: "kit"}); err == nil {
		t.Error("a missing folder should be refused")
	}
	spaced := filepath.Join(tmp, "my dir")
	os.Mkdir(spaced, 0o755)
	_, plan, err = in.Plan("kit", tui.Target{Kind: "gascity", Path: spaced, Name: "kit"})
	if err != nil || !reflect.DeepEqual(plan, []string{"ipm install gascity kit -C '" + spaced + "' --name kit"}) {
		t.Errorf("spaced plan %v %v", plan, err)
	}
	root := scopeOf(t, f, false).Root
	if _, _, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: filepath.Dir(root), Name: filepath.Base(root)}); err == nil || !strings.Contains(err.Error(), "refusing to replace") {
		t.Errorf("a folder holding the project should be refused at the form: %v", err)
	}
	foreign := filepath.Join(tmp, "foreign")
	os.Mkdir(foreign, 0o755)
	os.WriteFile(filepath.Join(foreign, "keep.txt"), []byte("x"), 0o644)
	if _, _, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: tmp, Name: "foreign"}); err == nil {
		t.Error("a folder ipm didn't write should be refused at the form")
	}
	// A folder ipm wrote earlier is a reinstall, not a refusal.
	if err := in.Install(context.Background(), "kit", tui.Target{Kind: "gascity", Path: tmp, Name: "kit"}, io.Discard); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, _, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: tmp, Name: "kit"}); err != nil {
		t.Errorf("planning over a folder ipm wrote should pass: %v", err)
	}
}

func TestTuiInstallerGascityDefault(t *testing.T) {
	f, _, _ := gascityFake(t)
	scope := scopeOf(t, f, false)
	in := tuiInstaller{sys: f.system(), scope: scope}
	def := filepath.Join(scope.Root, ".ipm", ".gascity")
	d := in.Default("kit", "gascity")
	if want := (tui.Target{Kind: "gascity", Path: def, Name: "kit"}); d != want {
		t.Errorf("default %+v, want %+v", d, want)
	}
	for _, path := range []string{"", def} {
		got, plan, err := in.Plan("kit", tui.Target{Kind: "gascity", Path: path, Name: "kit"})
		if err != nil || !reflect.DeepEqual(plan, []string{"ipm install gascity kit"}) || got.Path != def || got.Name != "kit" {
			t.Errorf("%q: plan %v %+v %v", path, plan, got, err)
		}
	}
	if _, err := os.Stat(def); err == nil {
		t.Error("Plan created the default folder")
	}
	_, plan, err := in.Plan("kit", tui.Target{Kind: "gascity", Name: "team", Global: false})
	if err != nil || !reflect.DeepEqual(plan, []string{"ipm install gascity kit --name team"}) {
		t.Errorf("named plan %v %v", plan, err)
	}

	// in the user scope
	if err := f.run("pack", "-g", "new", "gk"); err != nil {
		t.Fatal(err)
	}
	gdef := filepath.Join(f.env["IPM_HOME"], "global", ".ipm", ".gascity")
	got, plan, err := in.Plan("gk", tui.Target{Kind: "gascity", Name: "gk", Global: true})
	if err != nil || !reflect.DeepEqual(plan, []string{"ipm install gascity gk -g"}) || got.Path != gdef {
		t.Errorf("global plan %v %+v %v", plan, got, err)
	}
	gin := tuiInstaller{sys: f.system(), scope: scopeOf(t, f, true)}
	if d := gin.Default("gk", "gascity"); d.Path != gdef || !d.Global {
		t.Errorf("global default %+v", d)
	}
	if err := in.Install(context.Background(), "kit", tui.Target{Kind: "gascity", Path: def, Name: "kit"}, io.Discard); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(def, "kit", "pack.toml")); err != nil {
		t.Error(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (r roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }

// tuiMarketFake follows obra/superpowers and me/mine, with a catalog holding
// two items of the first and one of the second, and sourcePacksList served
// as the curated list.
func tuiMarketFake(t *testing.T) (*fake, tuiMarketplace) {
	t.Helper()
	root := checkout(t)
	f := newFake(root)
	f.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sourcePacksList)), Header: http.Header{}}, nil
	})}
	for _, typ := range catalog.Types {
		write(t, filepath.Join(cacheOf(root), "catalog", catalog.Files[typ]), "")
	}
	write(t, filepath.Join(cacheOf(root), "catalog", "skills.jsonl"),
		`{"id":"skill:obra/superpowers:a","type":"skill","name":"a","repo":"obra/superpowers"}`+"\n"+
			`{"id":"skill:Obra/Superpowers:b","type":"skill","name":"b","repo":"Obra/Superpowers"}`+"\n"+
			`{"id":"skill:me/mine:c","type":"skill","name":"c","repo":"me/mine"}`+"\n")
	if err := f.run("marketplace", "add", "obra/superpowers", "me/mine"); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	return f, tuiMarketplace{sys: f.system()}
}

func TestTuiMarketplaceList(t *testing.T) {
	_, m := tuiMarketFake(t)
	repos, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Repo, List string
		Followed   bool
		Items      int
	}
	var got []row
	for _, r := range repos {
		got = append(got, row{r.Repo, r.List, r.Followed, r.Items})
	}
	want := []row{
		{"obra/superpowers", "AI · Methods", true, 2},
		{"obra/superpowers", "AI · Skills", true, 2},
		{"Anthropics/Skills", "AI · Skills", false, 0},
		{"o/not-followed", "AI · Unused", false, 0},
		{"me/mine", "Other", true, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows %+v\nwant %+v", got, want)
	}
	if r := repos[0]; r.Stars != 297229 || r.Description != "An agentic skills framework." || !reflect.DeepEqual(r.Topics, []string{"skills", "sdlc"}) {
		t.Errorf("details %+v", r)
	}
}

func TestTuiMarketplaceFollow(t *testing.T) {
	f, m := tuiMarketFake(t)
	if err := m.Follow("o/not-followed"); err != nil {
		t.Fatal(err)
	}
	if f.out.Len() != 0 {
		t.Errorf("follow must not write to stdout:\n%s", f.out.String())
	}
	repos, _ := m.List()
	followed := map[string]bool{}
	for _, r := range repos {
		followed[r.Repo] = r.Followed
	}
	if !followed["o/not-followed"] {
		t.Errorf("not followed: %+v", followed)
	}
	if err := m.Unfollow("o/not-followed"); err != nil {
		t.Fatal(err)
	}
	repos, _ = m.List()
	for _, r := range repos {
		if r.Repo == "o/not-followed" && r.Followed {
			t.Errorf("still followed: %+v", r)
		}
	}
	if f.out.Len() != 0 {
		t.Errorf("unfollow must not write to stdout:\n%s", f.out.String())
	}
}

func TestTuiMarketplaceSyncGoesToWriter(t *testing.T) {
	f, m := tuiMarketFake(t)
	var out bytes.Buffer
	err := m.Sync(context.Background(), &out)
	if err == nil || !strings.Contains(err.Error(), "sync") && !strings.Contains(err.Error(), "pnpm") {
		t.Errorf("no pnpm: %v", err)
	}
	if f.out.Len() != 0 {
		t.Errorf("sync output must go to the writer:\n%s", f.out.String())
	}
}

func TestTuiCatalogCounts(t *testing.T) {
	f, root := packFake(t)
	_ = f
	g, err := catgraph.Load(filepath.Join(cacheOf(root), "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	got := tuiCatalog{g}.Counts()
	if got["skill"] != 1 || got["plugin"] != 2 || got["repo"] != 2 || got["hook"] != 1 {
		t.Errorf("counts %v", got)
	}
}
