# CLAUDE.md

Guidance for Claude Code in this repository. [`AGENTS.md`](AGENTS.md) is the router (where things are, conventions); this file adds commands and the sharp edges.

`ipm` was called `aihs`. Parts 1–4 of [the ipm design](docs/superpowers/specs/2026-10-09-ipm-design.md) are built: rename, sources on pnpm, packs and `ipm install`, and `ipm tui` ([TUI design](docs/superpowers/specs/2026-10-09-ipm-tui-design.md)).

## What this repo is

The source code of `ipm`, a Go CLI (`cli/`) that keeps a per-user list of repos of interest as a pnpm project, has pnpm fetch and pin them, catalogs every skill, agent, command, hook, MCP server and plugin they hold, searches that catalog with a local Typesense server, and builds packs of picked items (`ipm.toml`) into Claude Code plugins under a project's `.ipm/` (or `$IPM_HOME/global/.ipm/` with `-g`). Its data lives in `IPM_HOME` and `IPM_CACHE_DIR`, not in this checkout. See [docs/architecture.md](docs/architecture.md).

## Commands

| Task | Command |
| --- | --- |
| Put `ipm` on PATH (once) | `direnv allow` |
| Add, remove or list repos of interest | `ipm source add <owner/repo[@ref]\|git URL[#ref]\|plugin ID>`, `ipm source remove <repo>`, `ipm source list` |
| Migrate an aihs-era list of repos (once) | `ipm source import <path/to/manifest.toml>` |
| Fetch and pin every source with pnpm, then rebuild the catalog | `ipm sync` |
| Move every source to the latest commit of its branch or tag | `ipm sync --upgrade` |
| Rebuild the catalog from synced sources | `ipm index` |
| Refresh GitHub stars, topics and descriptions into `github.jsonl` (needs `gh`) | `ipm stats` |
| Start / stop the local search server (Typesense in Docker) | `ipm serve` / `ipm serve --stop` |
| Rebuild the search index from the catalog | `ipm load` |
| Search every artifact (hybrid, with SCORE; `--keyword`, `--type`, `--repo`, `--tag`, `--limit`, `--page N`, `--all`, `--explain`, `--ids`, `--json`; each hit shows its ID) | `ipm search <query>` |
| Inspect an artifact and its relationships (reads the catalog only) | `ipm show <id>`, `ipm tree <id> [--up]`, `ipm graph <id> --format mermaid\|dot` |
| Create a pack (creates `ipm.toml` in the working directory if no ancestor has one; `-g` for user scope) | `ipm pack new <name> [--description …]` |
| Add or remove catalog items in a pack; list packs; show a pack's items | `ipm pack add <pack> <id>…`, `ipm pack remove <pack> <id>…`, `ipm pack list`, `ipm pack show <pack>` |
| Build every pack into a plugin and marketplace under `.ipm/`, then validate | `ipm install [-g] [--update] [--no-validate]` |
| Search, see the tree, collect, build and deploy in one screen (Bubble Tea) | `ipm tui [-g]` |
| Build, then install a pack into Claude Code (user, project or local scope) | `ipm deploy [pack] [--scope] [--path] [-g]` |
| Report the plugin, skills, hooks and agents folders in synced sources | `ipm scan [--paths] [owner/repo...]` |
| Check home, cache, pnpm, sources, catalog, docker, typesense, claude | `ipm doctor` |
| Test changes to `cli/` (the gate) | `mise run cli:test` |
| Install from the published module | `go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest` |

The loop: `ipm source add <repo>` → `ipm sync` (installs, then indexes) → `ipm stats` when stars should be refreshed → `ipm load` to refresh search. Source files and refs are in [docs/reference/sources.md](docs/reference/sources.md). The pack loop: `ipm pack new` → `ipm pack add` → `ipm install` → `claude plugin marketplace add <project>/.ipm` (in a sandbox `CLAUDE_CONFIG_DIR` when testing).

## Sharp edges

- **Never hand-edit generated files**: `$IPM_HOME/sources/package.json` (use `ipm source`), `pnpm-lock.yaml`, `.pnpmfile.cjs` (rewritten on every sync), the catalog, and everything in a project's `.ipm/` (`ipm install` rewrites it; `ipm.toml` is edited with `ipm pack`).
- **`.ipm/plugins/` files are hard links into pnpm's store.** Editing one edits pnpm's copy too, and `ipm install` rebuilds the folder anyway. Edit upstream or in your own repo.
- **`ipm install` keeps existing pins** from `.ipm/ipm.lock`; `--update` re-pins to the current catalog. It must stay deterministic: two runs give byte-identical `.ipm/`, and items removed from a pack leave no files.
- **Hooks, MCP servers and inline commands are not pickable**; `ipm pack add` refuses them and names the owning plugin to add instead. Pack names Claude Code reserves (`claude-…`, `anthropic-…`, `official` beside claude/anthropic) are refused.
- **pnpm 12 or later is required.** `ipm` uses pnpm from PATH when it is 12+, else `npx -y pnpm@12.10.1`. pnpm always runs with `--ignore-scripts --store-dir <home>/store` and `--dir <home>/sources` (sync) or `--dir <root>/.ipm` (install); never run it there without those flags.
- **The `.pnpmfile.cjs` hook is load-bearing.** Without it pnpm re-packs GitHub repos with npm's packing rules and drops files (`nizos/tdd-guard`: 429 files → 3). Non-GitHub sources still go through pnpm's git fetcher and can lose files the same way.
- **A failed `ipm sync` leaves the catalog unchanged**; indexing only runs after pnpm succeeds.
- **Tests never touch your real home**: set `IPM_HOME` and `IPM_CACHE_DIR` to absolute temp paths (relative ones are refused).
- **Verify upstream facts from the installed sources**, not from memory: `$IPM_HOME/sources/node_modules/<owner--repo>/` is the repo at its pinned commit. Run `ipm sync` first if it is missing.
- **The search server is container `ipm-typesense` on `127.0.0.1:8108`.** `ipm serve` removes an old `aihs-typesense` container that would hold the port.
- **`ipm deploy` (and `D` in the TUI) changes the user's Claude config** through `claude plugin marketplace add|update` and `claude plugin install|update`; every `claude` call goes through `System.Output`, so tests fake it. `internal/cli/testdata/claude-*.json` are real `claude` 2.1.296 listings. **Installing plugins changes the user's Claude config.** Test installs with `CLAUDE_CONFIG_DIR` pointing at a temporary directory.
- **The docs site is `docs/`**, published by GitHub Pages from `main` (`docs/_config.yml`, Just the Docs). Commands, flags, messages and file formats are documented there; update [the commands reference](docs/reference/commands.md), the relevant guide and [troubleshooting](docs/troubleshooting.md) when they change. Verify claims against the code or a live run, not memory.
- **`internal/tui` depends only on its four interfaces** (`Searcher`, `Catalog`, `Packs`, `Installer`); never import ipm packages or `System` there. The adapters in `internal/cli/tui.go` reuse the CLI's functions (`addToPack`, `runInstall`, `search.Query`), so the TUI adds no behaviour of its own. Screen changes update `internal/tui/testdata/*.golden` with `go test ./internal/tui -run Golden -update`; read the diff before committing.
- **`docs/superpowers/` is history.** Specs and plans describe what was intended at the time; don't edit them to match the code.
- **`ipm` stays self-contained**: no global config beyond its own home and cache, nothing shared with fsh or Gas City. Commands are Go only, built with Cobra: a `new<Name>Cmd(sys System)` in `cli/internal/cli`, registered in `newRootCmd` in `cli.go`. Malformed command lines return a `UsageError` (exit 2); use `noArgs`, `exactArgs`, `minArgs` and `verbs` from `cli.go` rather than Cobra's own argument checks, which return plain errors.
