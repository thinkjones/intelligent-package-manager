// Package assemble turns packs of picked items into plugins: it resolves each
// item (from ipm.lock, else the catalog), plans where every file goes and
// which whole plugins become marketplace dependencies, and places the files
// by hard link (copy across volumes).
package assemble

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/thinkjones/fs-skills/cli/internal/catalog"
	"github.com/thinkjones/fs-skills/cli/internal/marketplace"
	"github.com/thinkjones/fs-skills/cli/internal/pack"
	"github.com/thinkjones/fs-skills/cli/internal/project"
	"github.com/thinkjones/fs-skills/cli/internal/sources"
)

var ownerRepo = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// Resolve finds every item the packs name — in lock first, then the catalog
// — keyed by ID. A copilot plugin's skills are resolved too; its Source
// lists their IDs (JSON). External and inline plugins carry their upstream
// marketplace entry as Source (JSON).
func Resolve(packs []project.Pack, lock map[string]project.LockItem, catalogDir string) (map[string]project.LockItem, error) {
	out := map[string]project.LockItem{}
	var records map[string]map[string]any
	lookup := func(id string) (map[string]any, error) {
		if records == nil {
			records = map[string]map[string]any{}
			for _, typ := range catalog.Types {
				recs, err := catalog.Load(catalogDir, typ)
				if err != nil {
					return nil, err
				}
				for _, r := range recs {
					records[str(r, "id")] = r
				}
			}
		}
		return records[id], nil
	}
	var resolve func(packName, id string) error
	resolve = func(packName, id string) error {
		if _, done := out[id]; done {
			return nil
		}
		if l, ok := lock[id]; ok {
			out[id] = l
			if l.Type == catalog.TypeCopilotPlugin {
				var skills []string
				json.Unmarshal([]byte(l.Source), &skills)
				for _, s := range skills {
					if err := resolve(packName, s); err != nil {
						return err
					}
				}
			}
			return nil
		}
		r, err := lookup(id)
		if err != nil {
			return fmt.Errorf("can't resolve %s without the catalog: %v", id, err)
		}
		if r == nil {
			return fmt.Errorf("%s is not in the catalog or ipm.lock — drop it with `ipm pack remove %s %s`, or run `ipm sync` if it's new upstream", id, packName, id)
		}
		it := project.LockItem{ID: id, Type: str(r, "type"), Name: str(r, "name"), Repo: str(r, "repo"), Path: str(r, "path"), SHA: str(r, "commit")}
		switch it.Type {
		case catalog.TypePlugin:
			if entry, ok := r["entry"].(map[string]any); ok && strings.Contains(id, "#") {
				b, _ := json.Marshal(entry)
				it.Source = string(b)
			}
		case catalog.TypeCopilotPlugin:
			skills := strs(r["skills"])
			b, _ := json.Marshal(skills)
			it.Source = string(b)
			out[id] = it
			for _, s := range skills {
				if err := resolve(packName, s); err != nil {
					return err
				}
			}
			return nil
		}
		out[id] = it
		return nil
	}
	for _, p := range packs {
		for _, id := range p.Items {
			if err := resolve(p.Name, id); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// Placed is one file or folder copied into a plugin: From is repo-relative in
// the repo installed as Alias; To is plugin-relative.
type Placed struct {
	Alias, From, To string
	Dir             bool
	Item            project.LockItem
}

// PackPlan is one pack's plugin: files to place, and whole-plugin
// marketplace entries (marketplace.Entry or a map for upstream entries).
type PackPlan struct {
	Name, Description string
	Files             []Placed
	Deps              []any
}

// DepNames are the dependency entries' names, in order.
func (p PackPlan) DepNames() []string {
	var out []string
	for _, d := range p.Deps {
		switch e := d.(type) {
		case marketplace.Entry:
			out = append(out, e.Name)
		case map[string]any:
			out = append(out, fmt.Sprint(e["name"]))
		}
	}
	return out
}

// Plan is every pack's plugin plus the repos to fetch: alias → pnpm spec
// pinned to a commit.
type Plan struct {
	Repos map[string]string
	Packs []PackPlan
}

var unsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// slug is a name as a safe file or entry name.
func slug(s string) string {
	return strings.Trim(unsafe.ReplaceAllString(strings.ToLower(s), "-"), "-.")
}

// Build plans each pack. File names are the items' names; a clash within a
// type gets -<repo owner>, then -2, -3.
func Build(packs []project.Pack, items map[string]project.LockItem) (Plan, error) {
	plan := Plan{Repos: map[string]string{}}
	pin := func(it project.LockItem) error {
		spec := "git+" + it.Repo + "#" + it.SHA
		if ownerRepo.MatchString(it.Repo) {
			spec = "github:" + it.Repo + "#" + it.SHA
		}
		a := sources.Alias(it.Repo)
		if prev, ok := plan.Repos[a]; ok && prev != spec {
			return fmt.Errorf("%s is pinned at two commits in ipm.lock — run `ipm install --update`", it.Repo)
		}
		plan.Repos[a] = spec
		return nil
	}
	for _, p := range packs {
		pp := PackPlan{Name: p.Name, Description: p.Description}
		used := map[string]bool{}
		place := func(it project.LockItem) error {
			if err := pin(it); err != nil {
				return err
			}
			dir, ext := map[string]string{catalog.TypeSkill: "skills", catalog.TypeAgent: "agents", catalog.TypeCommand: "commands"}[it.Type], ".md"
			if it.Type == catalog.TypeSkill {
				ext = ""
			}
			base := slug(it.Name)
			if base == "" {
				base = slug(strings.TrimSuffix(path.Base(it.Path), ".md"))
			}
			name := base
			if used[dir+"/"+name] {
				owner, _, _ := strings.Cut(pack.NormalizeRepo(it.Repo), "/")
				name = base + "-" + slug(owner)
				for n := 2; used[dir+"/"+name]; n++ {
					name = fmt.Sprintf("%s-%d", base, n)
				}
			}
			used[dir+"/"+name] = true
			pp.Files = append(pp.Files, Placed{Alias: sources.Alias(it.Repo), From: it.Path, To: dir + "/" + name + ext, Dir: it.Type == catalog.TypeSkill, Item: it})
			return nil
		}
		for _, id := range p.Items {
			it := items[id]
			switch it.Type {
			case catalog.TypeSkill, catalog.TypeAgent, catalog.TypeCommand:
				if err := place(it); err != nil {
					return Plan{}, err
				}
			case catalog.TypeCopilotPlugin:
				var skills []string
				json.Unmarshal([]byte(it.Source), &skills)
				for _, s := range skills {
					if err := place(items[s]); err != nil {
						return Plan{}, err
					}
				}
			case catalog.TypePlugin:
				pp.Deps = append(pp.Deps, wholePlugin(p.Name, it))
			}
		}
		plan.Packs = append(plan.Packs, pp)
	}
	return plan, nil
}

// wholePlugin is a whole-plugin pick as a marketplace entry named
// <pack>-<slug>: a local plugin pinned by git-subdir; an inline or external
// one as its upstream entry, with an inline one's relative source pinned.
func wholePlugin(packName string, it project.LockItem) any {
	name := packName + "-" + slug(it.Name)
	tags := []string{marketplace.OwnerTag, packName}
	if it.Source == "" {
		p := it.Path
		if p == "." {
			p = ""
		}
		return marketplace.Entry{Name: name, Source: marketplace.SourceFor(it.Repo, p, "", it.SHA), Tags: tags}
	}
	var entry map[string]any
	json.Unmarshal([]byte(it.Source), &entry)
	entry["name"], entry["tags"] = name, tags
	if it.Path != "" {
		entry["source"] = marketplace.SourceFor(it.Repo, it.Path, "", it.SHA)
	}
	return entry
}

// Place clears pluginDir and puts each planned file there, linking with link
// (os.Link in production) and copying when linking fails.
func Place(p PackPlan, nodeModules, pluginDir string, link func(oldname, newname string) error) error {
	if err := os.RemoveAll(pluginDir); err != nil {
		return err
	}
	for _, f := range p.Files {
		root, err := filepath.EvalSymlinks(filepath.Join(nodeModules, f.Alias))
		if err != nil {
			return fmt.Errorf("%s is not installed in %s: %v", f.Item.Repo, nodeModules, err)
		}
		src := filepath.Join(root, filepath.FromSlash(f.From))
		dst := filepath.Join(pluginDir, filepath.FromSlash(f.To))
		if !f.Dir {
			if err := put(src, dst, link); err != nil {
				return err
			}
			continue
		}
		err = filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			return put(p, filepath.Join(dst, rel), link)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func put(src, dst string, link func(string, string) error) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if link(src, dst) == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Items is every placed file's item with its plugin-relative file, sorted by
// ID — the provenance written to plugin.json.
func (p PackPlan) Items() []map[string]string {
	var out []map[string]string
	for _, f := range p.Files {
		out = append(out, map[string]string{"id": f.Item.ID, "type": f.Item.Type, "name": f.Item.Name, "repo": f.Item.Repo, "path": f.Item.Path, "sha": f.Item.SHA, "file": f.To})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"] < out[j]["id"] })
	return out
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func strs(v any) []string {
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
