// Package tui is ipm's terminal interface for the curation loop: search the
// catalog, judge an artifact and its relationships, collect items into a
// pack, and build it. It depends only on the interfaces in this file; the
// cli package implements them over ipm's search, catalog, packs and install.
package tui

import (
	"context"
	"io"
)

// Filter narrows a search; an empty Type is every type, an empty Repo every
// repo. A search with no text and a Repo is every item of that repo.
type Filter struct{ Type, Repo string }

// Hit is one search result.
type Hit struct {
	ID, Type, Name, Repo, Description string
	Stars                             int
	Plugins                           []string
	// Score is the SCORE ipm search prints; "" in local mode.
	Score string
	// UsedBy is how many other repos reference this artifact's repo.
	UsedBy int
	// External marks a marketplace entry for a plugin in another repo.
	External bool
}

// Results is a page of hits and how many matched in all.
type Results struct {
	Hits  []Hit
	Found int
}

// Searcher finds artifacts: with the search server (hybrid) or without it
// (local keyword matching over the catalog).
type Searcher interface {
	Search(ctx context.Context, query string, f Filter) (Results, error)
	// Mode is "hybrid" or "local".
	Mode() string
	// Notice is the banner to show, "" when nothing needs saying.
	Notice() string
	// Recheck repeats the search server check (ctrl+r).
	Recheck()
}

// Ref is an artifact related to another: a parent, a child, or a reference.
type Ref struct {
	ID, Type, Name, Description string
	// Repo is the repo the artifact lives in.
	Repo string
	// External marks a marketplace entry for a plugin in another repo.
	External bool
	// UsedBy is how many other repos reference this artifact directly.
	UsedBy int
}

// Node is one artifact with its parents and children.
type Node struct {
	ID, Type, Name, Repo, Path, Description string
	Stars                                   int
	Parents, Children                       []Ref
	// External is what an external entry references; Unresolved is its
	// source repo when that repo isn't among the user's sources.
	External   *Ref
	Unresolved string
	// UsedBy is the external entries in other repos that reference this
	// node; UsedByRepos is how many other repos reference its repo.
	UsedBy      []Ref
	UsedByRepos int
}

// Catalog describes artifacts.
type Catalog interface {
	Node(id string) (Node, error)
	// Counts is how many items the catalog holds of each type.
	Counts() map[string]int
}

// Install is one place a pack is installed.
type Install struct {
	Pack string
	Target
}

// Pack is one pack in the scope's ipm.toml.
type Pack struct {
	Name, Description string
	Items             []string
	// Global marks a pack of the user scope, listed from a project.
	Global bool
	// Built is "fresh", "stale" or "never".
	Built    string
	Installs []Install
}

// Packs reads and edits the scope's packs, with the same rules as ipm pack.
type Packs interface {
	// Scope describes where the packs live: "project /path" or "global".
	Scope() string
	// List is the scope's packs, and the user scope's too when global is set.
	List(global bool) ([]Pack, error)
	Create(name, description string) error
	Add(pack string, ids ...string) error
	Remove(pack string, ids ...string) error
}

// Builder builds every pack, as ipm pack build does, writing its output to out.
type Builder interface {
	Build(ctx context.Context, out io.Writer) error
}

// Target is where a pack is installed: Kind "claude" (Scope user, project or
// local; Path the folder for project and local) or "gascity" (Path the parent
// folder, Name the pack folder). Global installs a pack of the user scope.
type Target struct {
	Kind, Scope, Path, Name string
	Global                  bool
}

// Installer installs built packs into Claude Code or as a Gas City pack folder.
type Installer interface {
	// Targets are the kinds offered: "claude" and "gascity".
	Targets() []string
	// Default is the target a pack installs to unless the user picks another.
	Default(pack, kind string) Target
	// Plan checks t and returns it as Install will use it (its folder made
	// absolute), with the commands Install will run, for the user to confirm.
	Plan(pack string, t Target) (Target, []string, error)
	// Install builds if needed and installs pack at t, writing its output to out.
	Install(ctx context.Context, pack string, t Target, out io.Writer) error
	// List is the recorded installs of the project, and of the user scope when global is set.
	List(global bool) ([]Install, error)
}

// Repo is a repo of a curated list, or one followed outside every list.
type Repo struct {
	Repo, List, Description string
	Topics                  []string
	Stars, Items            int
	Followed                bool
}

// Marketplace is the curated repos and which of them the user follows.
type Marketplace interface {
	// List is the curated repos in list order, then the followed repos no
	// list holds, under List "Other".
	List() ([]Repo, error)
	Follow(repo string) error
	Unfollow(repo string) error
	// Sync runs ipm catalog sync, writing its output to out.
	Sync(ctx context.Context, out io.Writer) error
}

// Deps is what the TUI works with.
type Deps struct {
	Searcher    Searcher
	Catalog     Catalog
	Packs       Packs
	Builder     Builder
	Installer   Installer
	Marketplace Marketplace
}
