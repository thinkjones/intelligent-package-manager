package assemble

import (
	"fmt"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/catalog"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/project"
	"github.com/thinkjones/intelligent-package-manager/cli/internal/sources"
)

// MusePlugin is one whole plugin a pack installs into Muse Code on its own:
// the folder From (empty for the root) of the repo installed as Alias, which
// Item pins to a commit.
type MusePlugin struct {
	Name, Alias, From string
	Item              project.LockItem // Repo and SHA are where the plugin's files are
}

// MusePlan is a pack's whole plugins as Muse Code installs them. Muse reads
// the pack's plugin but not its dependencies, so each whole plugin is
// installed from its own folder.
type MusePlan struct {
	Plugins []MusePlugin
	Skips   []string // "skipped <id>: …", in item order
}

// Repos is every repo the plugins read: alias → pnpm spec pinned to a
// commit, as Plan.Repos.
func (mp MusePlan) Repos() map[string]string {
	out := map[string]string{}
	for _, p := range mp.Plugins {
		out[p.Alias] = PinSpec(p.Item)
	}
	return out
}

// PlanMuse plans p's whole plugins: a local or inline plugin from its repo at
// the locked commit, an external one from the repo and commit its upstream
// entry names. An external plugin that names no commit, or that is not in
// git, is skipped.
func PlanMuse(p project.Pack, items map[string]project.LockItem) (MusePlan, error) {
	var plan MusePlan
	pins := map[string]string{} // alias → spec
	for _, id := range p.Items {
		it, ok := items[id]
		if !ok {
			return MusePlan{}, fmt.Errorf("%s is not resolved", id)
		}
		if it.Type != catalog.TypePlugin {
			continue
		}
		at, from := it, it.Path
		if it.Path == "" {
			var skip string
			var err error
			if at, from, skip, err = externalAt(it); err != nil {
				return MusePlan{}, fmt.Errorf("%s: %v", id, err)
			}
			if skip != "" {
				plan.Skips = append(plan.Skips, "skipped "+id+": "+skip)
				continue
			}
		}
		if from == "." {
			from = ""
		}
		alias, spec := sources.Alias(at.Repo), PinSpec(at)
		if prev, ok := pins[alias]; ok && prev != spec {
			return MusePlan{}, fmt.Errorf("%s is pinned at two commits by this pack's plugins — Muse Code can hold only one; drop one of them", at.Repo)
		}
		pins[alias] = spec
		plan.Plugins = append(plan.Plugins, MusePlugin{Name: it.Name, Alias: alias, From: from, Item: at})
	}
	return plan, nil
}

// externalAt is where an external plugin's files are: its upstream entry's
// repo and commit, and the folder in it; skip says why ipm can't fetch it.
func externalAt(it project.LockItem) (at project.LockItem, from, skip string, err error) {
	entry, err := upstreamEntry(it)
	if err != nil {
		return it, "", "", err
	}
	src, _ := entry["source"].(map[string]any)
	kind := str(src, "source")
	repo := ""
	switch kind {
	case "github":
		repo = str(src, "repo")
	case "url":
		repo = str(src, "url")
	case "git-subdir":
		repo, from = str(src, "url"), str(src, "path")
	default:
		return it, "", fmt.Sprintf("its source is %q, which Muse Code can't get from git", kind), nil
	}
	sha := str(src, "sha")
	if !commitRE.MatchString(sha) {
		sha = str(src, "ref")
	}
	if !commitRE.MatchString(sha) {
		return it, "", "its upstream entry names no commit, so ipm can't fetch a fixed copy", nil
	}
	if repo = pack.NormalizeRepo(repo); repo == "" {
		return it, "", "its upstream entry names no repo", nil
	}
	return project.LockItem{ID: it.ID, Type: it.Type, Name: it.Name, Repo: repo, SHA: sha}, from, "", nil
}
