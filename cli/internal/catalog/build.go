package catalog

import (
	"slices"

	"github.com/thinkjones/intelligent-package-manager/cli/internal/pack"
)

// componentKeys are the plugin fields that list owned components.
var componentKeys = []string{"skills", "agents", "commands", "hooks", "mcp"}

// Build indexes every checkout and assembles the catalog: a repo record per
// repo of interest and per external source repo, and the reverse links —
// each component's plugins, each plugin's marketplaces. Warnings are sorted.
func Build(cos []Checkout) ([]Record, []string, error) {
	byID := map[string]*Record{}
	var order []string
	var warnings []string
	followed := map[string]bool{}
	var sources []string
	for _, c := range cos {
		followed[pack.NormalizeRepo(c.Repo)] = true
	}
	for _, c := range cos {
		res, err := IndexRepo(c)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, res.Warnings...)
		sources = append(sources, res.Sources...)
		counts := map[string]int{}
		for _, r := range res.Records {
			counts[r.Type]++
			r := r
			byID[r.ID] = &r
			order = append(order, r.ID)
		}
		repo := Record{ID: TypeRepo + ":" + c.Repo, Type: TypeRepo, Name: c.Repo, Repo: c.Repo, Commit: c.Commit,
			Fields: map[string]any{"followed": true, "counts": counts}}
		byID[repo.ID] = &repo
		order = append(order, repo.ID)
	}
	for _, s := range sources {
		id := TypeRepo + ":" + s
		if followed[s] || byID[id] != nil {
			continue
		}
		byID[id] = &Record{ID: id, Type: TypeRepo, Name: s, Repo: s, Fields: map[string]any{"followed": false}}
		order = append(order, id)
	}

	// Reverse links.
	link := func(target, key, from string) {
		r := byID[target]
		if r == nil {
			return
		}
		l, _ := r.Fields[key].([]string)
		if !slices.Contains(l, from) {
			r.Fields[key] = append(l, from)
		}
	}
	for _, id := range order {
		r := byID[id]
		switch r.Type {
		case TypePlugin, TypeCopilotPlugin:
			if r.Type == TypePlugin {
				if _, ok := r.Fields["marketplaces"]; !ok {
					r.Fields["marketplaces"] = []string{}
				}
			}
			for _, key := range componentKeys {
				ids, _ := r.Fields[key].([]string)
				for _, target := range ids {
					link(target, "plugins", id)
				}
			}
		case TypeMarketplace:
			ids, _ := r.Fields["plugins"].([]string)
			for _, target := range ids {
				link(target, "marketplaces", id)
			}
		}
	}
	out := make([]Record, 0, len(order))
	for _, id := range order {
		r := byID[id]
		for _, key := range []string{"plugins", "marketplaces"} {
			if l, ok := r.Fields[key].([]string); ok && len(l) > 0 && r.Type != TypeMarketplace {
				r.Fields[key] = slices.Sorted(slices.Values(l))
			}
		}
		if r.Type == TypeHook {
			if owners, _ := r.Fields["plugins"].([]string); len(owners) > 0 && byID[owners[0]] != nil {
				r.Name = byID[owners[0]].Name
			}
		}
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b Record) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	slices.Sort(warnings)
	return out, warnings, nil
}
