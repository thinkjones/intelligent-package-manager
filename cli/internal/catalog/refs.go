package catalog

import (
	"slices"
	"sort"
	"strings"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
)

// Refs is how repos reference each other: a marketplace entry whose plugin
// lives in another repo (an external entry) points at a target there.
type Refs struct {
	// External marks every external entry, whether or not its target is known.
	External map[string]bool
	// Target is what an external entry references: a plugin in the source
	// repo, or the repo itself. Entries whose source repo isn't in the
	// catalog have none.
	Target map[string]string
	// By lists the external entries that target an ID, sorted.
	By map[string][]string
	// UsedBy lists, for a normalized repo, the other repos (normalized,
	// sorted) whose external entries target anything in it.
	UsedBy map[string][]string
}

// BuildRefs finds every external entry in records (keyed by type) and what it
// references: the source repo's plugin at the entry's source path, else the
// one with the entry's name, else the repo's only plugin, else the repo.
func BuildRefs(records map[string][]map[string]any) Refs {
	refs := Refs{External: map[string]bool{}, Target: map[string]string{}, By: map[string][]string{}, UsedBy: map[string][]string{}}
	repos := map[string]string{} // normalized repo → repo record ID
	for _, r := range records[TypeRepo] {
		// catalog.Build writes a followed:false record for each external
		// source repo; it isn't a source, so it can't be a target.
		if f, ok := r["followed"].(bool); ok && !f {
			continue
		}
		repos[NormRepo(str(r, "repo"))] = str(r, "id")
	}
	plugins := map[string][]map[string]any{} // normalized repo → its plugins
	for _, typ := range []string{TypePlugin, TypeCopilotPlugin} {
		for _, p := range records[typ] {
			if str(p, "source_repo") == "" {
				repo := NormRepo(str(p, "repo"))
				plugins[repo] = append(plugins[repo], p)
			}
		}
	}
	usedBy := map[string]map[string]bool{}
	for _, e := range records[TypePlugin] {
		src, own := NormRepo(str(e, "source_repo")), NormRepo(str(e, "repo"))
		if src == "" || src == own {
			continue
		}
		id := str(e, "id")
		refs.External[id] = true
		target := resolve(e, plugins[src], repos[src])
		if target == "" {
			continue
		}
		refs.Target[id] = target
		refs.By[target] = append(refs.By[target], id)
		if usedBy[src] == nil {
			usedBy[src] = map[string]bool{}
		}
		usedBy[src][own] = true
	}
	for id := range refs.By {
		sort.Strings(refs.By[id])
	}
	for repo, set := range usedBy {
		for other := range set {
			refs.UsedBy[repo] = append(refs.UsedBy[repo], other)
		}
		slices.Sort(refs.UsedBy[repo])
	}
	return refs
}

// NormRepo is a repo as owner/name, lowercased: GitHub names ignore case.
func NormRepo(s string) string { return strings.ToLower(pack.NormalizeRepo(s)) }

// resolve picks an external entry's target among its source repo's plugins.
func resolve(entry map[string]any, plugins []map[string]any, repoID string) string {
	var path string
	if e, ok := entry["entry"].(map[string]any); ok {
		if s, ok := e["source"].(map[string]any); ok {
			path = str(s, "path")
		}
	}
	if path != "" {
		for _, p := range plugins {
			if str(p, "path") == path {
				return str(p, "id")
			}
		}
	}
	for _, p := range plugins {
		if str(p, "name") == str(entry, "name") {
			return str(p, "id")
		}
	}
	if len(plugins) == 1 {
		return str(plugins[0], "id")
	}
	return repoID
}
