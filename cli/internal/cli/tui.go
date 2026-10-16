package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/assemble"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/curated"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/tui"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/typesense"
)

func newTuiCmd(sys System) *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "search, judge, collect into packs and build, in one screen (-g for your user scope)",
		Args:  noArgs,
		RunE:  func(*cobra.Command, []string) error { return runTui(sys, global) },
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "edit and build your user scope's packs ($IPM_HOME/global)")
	return cmd
}

func runTui(sys System, global bool) error {
	if sys.TermWidth == nil || sys.TermWidth() == 0 {
		return errors.New("ipm tui needs a terminal")
	}
	p, err := ipmPaths(sys)
	if err != nil {
		return err
	}
	g, err := catgraph.Load(p.Catalog())
	if err != nil {
		return errors.New("no catalog — run `ipm catalog sync`")
	}
	scope, err := currentScope(sys, global)
	if err != nil {
		return err
	}
	m := tui.New(tui.Deps{
		Searcher:    newTuiSearcher(sys, g, p.Catalog()),
		Catalog:     tuiCatalog{g},
		Packs:       tuiPacks{sys: sys, scope: scope, global: global},
		Builder:     tuiBuilder{sys: sys, global: global},
		Installer:   tuiInstaller{sys: sys, scope: scope},
		Marketplace: tuiMarketplace{sys: sys},
	})
	_, err = tea.NewProgram(m).Run()
	return err
}

const (
	noticeServerOff  = "server off: keyword search only — run `ipm catalog serve` and `ipm catalog rebuild` for semantic search"
	noticeIndexEmpty = "search index empty — run `ipm catalog rebuild`"
	tuiLimit         = 50
)

// tuiSearcher searches with Typesense when it is up and indexed, and
// otherwise with keywords over the catalog.
type tuiSearcher struct {
	sys        System
	g          *catgraph.Graph
	catalogDir string

	// mu guards the fields below: searches run on Bubble Tea's command
	// goroutines while ctrl+r rechecks on its update goroutine.
	mu      sync.Mutex
	client  *typesense.Client
	mode    string
	notice  string
	records []map[string]any
}

func newTuiSearcher(sys System, g *catgraph.Graph, catalogDir string) *tuiSearcher {
	s := &tuiSearcher{sys: sys, g: g, catalogDir: catalogDir}
	s.Recheck()
	return s
}

func (s *tuiSearcher) Mode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

func (s *tuiSearcher) Notice() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notice
}

func (s *tuiSearcher) set(c *typesense.Client, mode, notice string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client, s.mode, s.notice = c, mode, notice
}

// Recheck picks hybrid mode when the server answers and its index has
// documents, local mode otherwise.
func (s *tuiSearcher) Recheck() {
	c, err := tsClient(s.sys)
	if err != nil || c.Health() != nil {
		s.set(nil, "local", noticeServerOff)
		return
	}
	n, err := c.NumDocuments(search.Collection)
	switch {
	case errors.Is(err, typesense.ErrNoCollection) || err == nil && n == 0:
		s.set(nil, "local", noticeIndexEmpty)
	case err != nil:
		s.set(nil, "local", noticeServerOff)
	default:
		s.set(c, "hybrid", "")
	}
}

func (s *tuiSearcher) Search(_ context.Context, q string, f tui.Filter) (tui.Results, error) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return s.local(q, f)
	}
	query := search.Query{Text: q, Limit: tuiLimit}
	if f.Type != "" {
		query.Types = []string{f.Type}
	}
	if f.Repo != "" {
		query.Repos = []string{f.Repo}
	}
	if strings.TrimSpace(q) == "" {
		// Typesense's "every document", narrowed by the filters.
		query.Text = "*"
	}
	res, err := client.Search(search.Collection, query.Params())
	if err != nil {
		return tui.Results{}, tsError(err)
	}
	var top int64
	for _, h := range res.Hits {
		top = max(top, h.TextMatch)
	}
	out := tui.Results{Found: res.Found}
	for _, h := range res.Hits {
		d := h.Document
		hit := tui.Hit{ID: str(d, "id"), Type: str(d, "type"), Name: str(d, "name"), Repo: str(d, "repo"),
			Description: str(d, "description"), Plugins: strList(d["plugin_names"]), Score: search.Score(h, top)}
		if n, ok := d["stars"].(float64); ok {
			hit.Stars = int(n)
		}
		if n, ok := d["used_by"].(float64); ok {
			hit.UsedBy = int(n)
		}
		hit.External, _ = d["external"].(bool)
		out.Hits = append(out.Hits, hit)
	}
	return out, nil
}

func (s *tuiSearcher) local(q string, f tui.Filter) (tui.Results, error) {
	s.mu.Lock()
	if s.records == nil {
		var all []map[string]any
		for _, typ := range catalog.Types {
			recs, err := catalog.Load(s.catalogDir, typ)
			if err != nil {
				s.mu.Unlock()
				return tui.Results{}, err
			}
			all = append(all, recs...)
		}
		s.records = all
	}
	records := s.records
	s.mu.Unlock()
	stars := func(string) (int, bool) { return 0, false }
	usedBy := func(string) int { return 0 }
	if s.g != nil {
		stars = s.g.Stars
		usedBy = func(repo string) int { return len(s.g.UsedBy(repo)) }
	}
	return localSearch(records, stars, usedBy, q, f), nil
}

// localSearch ranks catalog records by the query's words, ignoring case: 4
// for each word in the name, 3 in the repo, 3 in an owning plugin's name, 2
// in the description and 2 in the tags. Ties go to the repo more other repos
// use, then to more stars, then to the ID. It returns the best 50. A Repo
// filter keeps that repo's records, ignoring case; with it, an empty query
// matches all of them.
func localSearch(records []map[string]any, stars func(repo string) (int, bool), usedBy func(repo string) int, q string, f tui.Filter) tui.Results {
	terms := strings.Fields(strings.ToLower(q))
	pluginNames := map[string]string{}
	for _, r := range records {
		if t := str(r, "type"); t == catalog.TypePlugin || t == catalog.TypeCopilotPlugin {
			pluginNames[str(r, "id")] = strings.ToLower(str(r, "name"))
		}
	}
	type scored struct {
		hit   tui.Hit
		score int
	}
	var all []scored
	for _, r := range records {
		if f.Type != "" && str(r, "type") != f.Type {
			continue
		}
		name, desc := strings.ToLower(str(r, "name")), strings.ToLower(str(r, "description"))
		repo := strings.ToLower(str(r, "repo"))
		if f.Repo != "" && repo != strings.ToLower(f.Repo) {
			continue
		}
		tags := strings.ToLower(strings.Join(strList(r["tags"]), " "))
		var owners []string
		for _, p := range strList(r["plugins"]) {
			owners = append(owners, pluginNames[p])
		}
		plugins := strings.Join(owners, " ")
		score := 0
		for _, t := range terms {
			for _, field := range []struct {
				text   string
				weight int
			}{{name, 4}, {repo, 3}, {plugins, 3}, {desc, 2}, {tags, 2}} {
				if strings.Contains(field.text, t) {
					score += field.weight
				}
			}
		}
		if score == 0 && (len(terms) > 0 || f.Repo == "") {
			continue
		}
		hit := tui.Hit{ID: str(r, "id"), Type: str(r, "type"), Name: str(r, "name"), Repo: str(r, "repo"), Description: str(r, "description")}
		hit.Stars, _ = stars(hit.Repo)
		hit.UsedBy = usedBy(hit.Repo)
		all = append(all, scored{hit, score})
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.hit.UsedBy != b.hit.UsedBy {
			return a.hit.UsedBy > b.hit.UsedBy
		}
		if a.hit.Stars != b.hit.Stars {
			return a.hit.Stars > b.hit.Stars
		}
		return a.hit.ID < b.hit.ID
	})
	out := tui.Results{Found: len(all)}
	for i := range min(tuiLimit, len(all)) {
		out.Hits = append(out.Hits, all[i].hit)
	}
	return out
}

// tuiCatalog describes artifacts from the catalog graph.
type tuiCatalog struct{ g *catgraph.Graph }

func (c tuiCatalog) Node(id string) (tui.Node, error) {
	n, ok := c.g.Node(id)
	if !ok {
		return tui.Node{}, fmt.Errorf("no artifact %s in the catalog", id)
	}
	out := tui.Node{ID: id, Type: str(n, "type"), Name: str(n, "name"), Repo: str(n, "repo"), Path: str(n, "path"),
		Description: str(n, "description")}
	out.Stars, _ = c.g.Stars(out.Repo)
	out.Parents, out.Children = c.refs(c.g.Children(id, true)), c.refs(c.g.Children(id, false))
	if target, ok := c.g.RefTarget(id); ok {
		r := c.refs([]string{target})[0]
		out.External = &r
	} else if c.g.External(id) {
		out.Unresolved = str(n, "source_repo")
	}
	out.UsedBy = c.refs(c.g.RefsTo(id))
	if out.Type == catalog.TypeRepo {
		out.UsedBy = c.refs(c.g.RefsInto(out.Repo))
	}
	out.UsedByRepos = len(c.g.UsedBy(out.Repo))
	return out, nil
}

// Counts is how many items the catalog holds of each type.
func (c tuiCatalog) Counts() map[string]int { return c.g.Counts() }

// refs describes ids as tree rows: name, description, repo, whether each is
// an external entry, and how many repos reference it directly.
func (c tuiCatalog) refs(ids []string) []tui.Ref {
	var out []tui.Ref
	for _, nid := range ids {
		n, _ := c.g.Node(nid)
		repos := map[string]bool{}
		for _, e := range c.g.RefsTo(nid) {
			en, _ := c.g.Node(e)
			repos[str(en, "repo")] = true
		}
		out = append(out, tui.Ref{ID: nid, Type: str(n, "type"), Name: str(n, "name"), Description: str(n, "description"),
			Repo: str(n, "repo"), External: c.g.External(nid), UsedBy: len(repos)})
	}
	return out
}

// tuiPacks edits the scope's packs with the functions ipm pack uses.
type tuiPacks struct {
	sys    System
	scope  project.Scope
	global bool
}

func (p tuiPacks) Scope() string {
	if p.global {
		return "global"
	}
	return "project " + p.scope.Root
}

// List is the scope's packs, and the user scope's too with global when the
// scope is a project.
func (p tuiPacks) List(global bool) ([]tui.Pack, error) {
	out, err := p.scopePacks(p.scope)
	if err != nil || !global || p.scope.Global {
		return out, err
	}
	gscope, err := currentScope(p.sys, true)
	if err != nil {
		return nil, err
	}
	more, err := p.scopePacks(gscope)
	return append(out, more...), err
}

// scopePacks lists scope's packs with their build state and installs.
func (p tuiPacks) scopePacks(scope project.Scope) ([]tui.Pack, error) {
	packs, err := project.LoadPacks(scope.Root)
	if err != nil {
		return nil, err
	}
	installs, err := tuiInstalls(p.sys, scope)
	if err != nil {
		return nil, err
	}
	current := buildCurrent(p.sys, scope)
	var out []tui.Pack
	for _, pk := range packs {
		built := "never"
		if _, err := os.Stat(filepath.Join(scope.IPMDir(), "plugins", pk.Name)); err == nil {
			built = "stale"
			if current {
				built = "fresh"
			}
		}
		tp := tui.Pack{Name: pk.Name, Description: pk.Description, Items: pk.Items, Global: scope.Global, Built: built}
		for _, in := range installs {
			if in.Pack == pk.Name {
				tp.Installs = append(tp.Installs, in)
			}
		}
		out = append(out, tp)
	}
	return out, nil
}

// tuiInstalls is scope's recorded installs.
func tuiInstalls(sys System, scope project.Scope) ([]tui.Install, error) {
	recs, err := loadInstalls(sys, scope)
	if err != nil {
		return nil, err
	}
	var out []tui.Install
	for _, r := range recs {
		out = append(out, tui.Install{Pack: r.Pack,
			Target: tui.Target{Kind: r.Target, Scope: r.Scope, Path: r.Path, Name: r.Name, Global: scope.Global}})
	}
	return out, nil
}

func (p tuiPacks) Create(name, description string) error {
	return createPack(p.scope, name, description)
}

func (p tuiPacks) Add(pack string, ids ...string) error {
	_, err := addToPack(p.sys, p.scope, pack, ids)
	return err
}

func (p tuiPacks) Remove(pack string, ids ...string) error {
	_, err := removeFromPack(p.scope, pack, ids)
	return err
}

// tuiBuilder runs ipm pack build with its output going to the TUI.
type tuiBuilder struct {
	sys    System
	global bool
}

func (b tuiBuilder) Build(_ context.Context, out io.Writer) error {
	sys := b.sys
	sys.Stdout = out
	err := runBuild(sys, b.global, false, false)
	var exit *ExitError
	if errors.As(err, &exit) {
		return errors.New("build failed — see the output above")
	}
	return err
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func strList(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// tuiInstaller installs the scope's packs with the code behind ipm install
// claude and ipm install gascity.
type tuiInstaller struct {
	sys   System
	scope project.Scope
}

func (in tuiInstaller) Targets() []string { return slices.Sorted(maps.Keys(installers)) }

// scopeFor is the scope a pack to install lives in: the user scope for a
// global target, even from a project.
func (in tuiInstaller) scopeFor(global bool) (project.Scope, error) {
	if global && !in.scope.Global {
		return currentScope(in.sys, true)
	}
	return in.scope, nil
}

func (in tuiInstaller) Default(pack, kind string) tui.Target {
	if kind == "gascity" {
		return tui.Target{Kind: kind, Path: gascityDefault(in.sys, in.scope), Name: pack, Global: in.scope.Global}
	}
	t := claudeDefaults(in.scope, claudeTarget{})
	return tui.Target{Kind: "claude", Scope: t.Scope, Path: t.Path, Global: in.scope.Global}
}

func (in tuiInstaller) Plan(pack string, t tui.Target) (tui.Target, []string, error) {
	scope, err := in.scopeFor(t.Global)
	if err != nil {
		return t, nil, err
	}
	switch t.Kind {
	case "claude":
		if err := needClaude(in.sys); err != nil {
			return t, nil, err
		}
		target, err := cleanTarget(claudeTarget{Scope: t.Scope, Path: t.Path}, workDir(in.sys, scope), in.sys.Getenv("HOME"))
		if err != nil {
			return t, nil, err
		}
		cmds, err := planDeploy(in.sys, scope.IPMDir(), marketName(scope), pack, target)
		if err != nil {
			return t, nil, err
		}
		build := "ipm pack build"
		if scope.Global {
			build += " -g"
		}
		out := []string{build}
		for _, c := range cmds {
			out = append(out, c.String())
		}
		return tui.Target{Kind: "claude", Scope: target.Scope, Path: target.Path, Global: t.Global}, out, nil
	case "gascity":
		name := t.Name
		if name == "" {
			name = pack
		}
		if !assemble.ValidGasName(name) {
			return t, nil, fmt.Errorf("folder name %q must start with a letter or digit and use only letters, digits, '_' and '-'", name)
		}
		dir, _, err := gascityFolder(in.sys, scope, t.Path)
		if err != nil {
			return t, nil, err
		}
		isDefault := dir == gascityDefault(in.sys, scope)
		folder := filepath.Join(dir, name)
		if err := gascityRefusal(in.sys, scope, folder); err != nil {
			return t, nil, err
		}
		clashes, err := gascityClashes(in.sys, scope, pack, folder)
		if err != nil {
			return t, nil, err
		}
		if len(clashes) > 0 {
			return t, nil, fmt.Errorf("%s holds pack %s — run ipm install gascity with --name or --force to change it", folder, clashes[0])
		}
		if err := assemble.CheckGascityTarget(dir, name, false); err != nil {
			return t, nil, err
		}
		// The command line names the folder only when it is not the default,
		// and the name only when it differs from the pack's or -C is given.
		line := "ipm install gascity " + shellQuote(pack)
		if !isDefault {
			line += " -C " + shellQuote(dir) + " --name " + shellQuote(name)
		} else if name != pack {
			line += " --name " + shellQuote(name)
		}
		if t.Global {
			line += " -g"
		}
		return tui.Target{Kind: "gascity", Path: dir, Name: name, Global: t.Global}, []string{line}, nil
	}
	return t, nil, fmt.Errorf("can't install to %q — use %s", t.Kind, targetNames())
}

func (in tuiInstaller) Install(_ context.Context, pack string, t tui.Target, out io.Writer) error {
	install, ok := installers[t.Kind]
	if !ok {
		return fmt.Errorf("can't install to %q — use %s", t.Kind, targetNames())
	}
	scope, err := in.scopeFor(t.Global)
	if err != nil {
		return err
	}
	_, err = install(in.sys, scope, pack, project.Install{Target: t.Kind, Scope: t.Scope, Path: t.Path, Name: t.Name}, installOpts{Global: scope.Global}, out)
	return err
}

func (in tuiInstaller) List(global bool) ([]tui.Install, error) {
	out, err := tuiInstalls(in.sys, in.scope)
	if err != nil || !global || in.scope.Global {
		return out, err
	}
	gscope, err := currentScope(in.sys, true)
	if err != nil {
		return nil, err
	}
	more, err := tuiInstalls(in.sys, gscope)
	return append(out, more...), err
}

// tuiMarketplace is the curated repos and the sources, with the code behind
// ipm marketplace and ipm catalog sync. Its output never goes to stdout,
// which the TUI owns.
type tuiMarketplace struct{ sys System }

// quiet is the System with the output of the CLI functions dropped.
func (m tuiMarketplace) quiet() System {
	sys := m.sys
	sys.Stdout = io.Discard
	return sys
}

func (m tuiMarketplace) List() ([]tui.Repo, error) {
	p, err := ipmPaths(m.sys)
	if err != nil {
		return nil, err
	}
	data, _, err := curated.Fetch(m.sys.HTTP, curated.DefaultURL, p.Cache)
	if err != nil {
		return nil, err
	}
	lists, _, err := curated.Parse(data)
	if err != nil {
		return nil, err
	}
	have, err := sources.Load(p.Sources())
	if err != nil {
		return nil, err
	}
	followed := map[string]bool{}
	for _, s := range have {
		followed[strings.ToLower(s.Repo)] = true
	}
	inList := map[string]bool{}
	for _, l := range lists {
		for _, r := range l.Repos {
			inList[strings.ToLower(r.Repo)] = true
		}
	}
	var other []curated.Repo
	for _, s := range have {
		if !inList[strings.ToLower(s.Repo)] {
			other = append(other, curated.Repo{Repo: s.Repo})
		}
	}
	if len(other) > 0 {
		lists = append(lists[:len(lists):len(lists)], curated.List{Name: "Other", Repos: other})
	}
	items, err := itemsByRepo(p.Catalog())
	if err != nil {
		return nil, err
	}
	var out []tui.Repo
	for _, l := range lists {
		for _, r := range l.Repos {
			out = append(out, tui.Repo{Repo: r.Repo, List: l.Name, Description: r.Description, Topics: r.Topics, Stars: r.Stars,
				Items: items[strings.ToLower(r.Repo)], Followed: followed[strings.ToLower(r.Repo)]})
		}
	}
	return out, nil
}

// itemsByRepo counts the catalog's items per repo, by lower-case repo; a
// repo's own record is not an item.
func itemsByRepo(dir string) (map[string]int, error) {
	out := map[string]int{}
	for _, typ := range catalog.Types {
		if typ == catalog.TypeRepo {
			continue
		}
		recs, err := catalog.Load(dir, typ)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			out[strings.ToLower(str(r, "repo"))]++
		}
	}
	return out, nil
}

func (m tuiMarketplace) Follow(repo string) error {
	p, err := ipmPaths(m.sys)
	if err != nil {
		return err
	}
	return sourceAdd(m.quiet(), p, []string{repo})
}

func (m tuiMarketplace) Unfollow(repo string) error {
	p, err := ipmPaths(m.sys)
	if err != nil {
		return err
	}
	return sourceRemove(m.quiet(), p, []string{repo})
}

func (m tuiMarketplace) Sync(_ context.Context, out io.Writer) error {
	sys := m.sys
	sys.Stdout = out
	err := runSync(sys, false)
	var exit *ExitError
	if errors.As(err, &exit) {
		return errors.New("sync failed — see the output above")
	}
	return err
}

// shellQuote is s as one shell word: unchanged when it holds only safe
// characters, else in single quotes.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./:=@%+,~") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
