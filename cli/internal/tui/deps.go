// Package tui is ipm's terminal interface for the curation loop: search the
// catalog, judge an artifact and its relationships, collect items into a
// pack, and build it. It depends only on the interfaces in this file; the
// cli package implements them over ipm's search, catalog, packs and install.
package tui

import (
	"context"
	"io"
)

// Filter narrows a search; an empty Type is every type.
type Filter struct{ Type string }

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

// Ref is an artifact related to another: a parent or a child.
type Ref struct{ ID, Type, Name, Description string }

// Node is one artifact with its parents and children.
type Node struct {
	ID, Type, Name, Repo, Path, Description string
	Stars                                   int
	Parents, Children                       []Ref
}

// Catalog describes artifacts.
type Catalog interface {
	Node(id string) (Node, error)
}

// Pack is one pack in the scope's ipm.toml.
type Pack struct {
	Name, Description string
	Items             []string
}

// Packs reads and edits the scope's packs, with the same rules as ipm pack.
type Packs interface {
	// Scope describes where the packs live: "project /path" or "global".
	Scope() string
	List() ([]Pack, error)
	Create(name, description string) error
	Add(pack string, ids ...string) error
	Remove(pack string, ids ...string) error
}

// Installer builds every pack, as ipm install does, writing its output to out.
type Installer interface {
	Install(ctx context.Context, out io.Writer) error
}

// Target is where a pack is deployed in Claude Code: Scope is "user",
// "project" or "local"; Path is the folder for project and local scope.
type Target struct{ Scope, Path string }

// Deployer installs a built pack into Claude Code.
type Deployer interface {
	// Default is the target a pack deploys to unless the user picks another.
	Default(pack string) Target
	// Plan checks t and returns it as Deploy will use it (its folder made
	// absolute), with the commands Deploy will run, for the user to confirm.
	Plan(pack string, t Target) (Target, []string, error)
	// Deploy builds and installs pack at t, writing its output to out.
	Deploy(ctx context.Context, pack string, t Target, out io.Writer) error
}

// Deps is what the TUI works with.
type Deps struct {
	Searcher  Searcher
	Catalog   Catalog
	Packs     Packs
	Installer Installer
	Deployer  Deployer
}
