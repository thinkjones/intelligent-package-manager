package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/catgraph"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/search"
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
		return errors.New("no catalog — run `ipm sync`")
	}
	scope, err := currentScope(sys, global)
	if err != nil {
		return err
	}
	m := tui.New(tui.Deps{
		Searcher:  newTuiSearcher(sys, g, p.Catalog()),
		Catalog:   tuiCatalog{g},
		Packs:     tuiPacks{sys: sys, scope: scope, global: global},
		Installer: tuiInstaller{sys: sys, global: global},
		Deployer:  tuiDeployer{sys: sys, scope: scope, global: global},
	})
	_, err = tea.NewProgram(m).Run()
	return err
}

const (
	noticeServerOff  = "server off: keyword search only — run `ipm serve` and `ipm load` for semantic search"
	noticeIndexEmpty = "search index empty — run `ipm load`"
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
// use, then to more stars, then to the ID. It returns the best 50.
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
		if score == 0 {
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
	out.Parents, out.Children = c.refs(id, true), c.refs(id, false)
	return out, nil
}

func (c tuiCatalog) refs(id string, up bool) []tui.Ref {
	var out []tui.Ref
	for _, nid := range c.g.Children(id, up) {
		n, _ := c.g.Node(nid)
		out = append(out, tui.Ref{ID: nid, Type: str(n, "type"), Name: str(n, "name"), Description: str(n, "description")})
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

func (p tuiPacks) List() ([]tui.Pack, error) {
	packs, err := project.LoadPacks(p.scope.Root)
	if err != nil {
		return nil, err
	}
	var out []tui.Pack
	for _, pk := range packs {
		out = append(out, tui.Pack{Name: pk.Name, Description: pk.Description, Items: pk.Items})
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

// tuiInstaller runs ipm install with its output going to the TUI.
type tuiInstaller struct {
	sys    System
	global bool
}

func (in tuiInstaller) Install(_ context.Context, out io.Writer) error {
	sys := in.sys
	sys.Stdout = out
	err := runInstall(sys, in.global, false, false)
	var exit *ExitError
	if errors.As(err, &exit) {
		return errors.New("install failed — see the output above")
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

// tuiDeployer deploys the scope's packs with the code behind ipm deploy.
type tuiDeployer struct {
	sys    System
	scope  project.Scope
	global bool
}

func (d tuiDeployer) Default(string) tui.Target {
	t := deployDefaults(d.scope, deployTarget{})
	return tui.Target{Scope: t.Scope, Path: t.Path}
}

func (d tuiDeployer) Plan(pack string, t tui.Target) (tui.Target, []string, error) {
	if err := needClaude(d.sys); err != nil {
		return t, nil, err
	}
	target, err := cleanTarget(deployTarget{Scope: t.Scope, Path: t.Path}, workDir(d.sys, d.scope), d.sys.Getenv("HOME"))
	if err != nil {
		return t, nil, err
	}
	cmds, err := planDeploy(d.sys, d.scope.IPMDir(), marketName(d.scope), pack, target)
	if err != nil {
		return t, nil, err
	}
	build := "ipm install"
	if d.global {
		build += " -g"
	}
	out := []string{build}
	for _, c := range cmds {
		out = append(out, c.String())
	}
	return tui.Target{Scope: target.Scope, Path: target.Path}, out, nil
}

func (d tuiDeployer) Deploy(_ context.Context, pack string, t tui.Target, out io.Writer) error {
	_, err := runDeploy(d.sys, d.global, pack, deployTarget{Scope: t.Scope, Path: t.Path}, out)
	return err
}
