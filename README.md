# ipm

`ipm` ("intelligent package manager") finds agent artifacts across many repos. You list the repos you care about; `ipm` has pnpm fetch and pin them, catalogs every skill, agent, command, hook, MCP server and plugin they hold, and gives you hybrid keyword and semantic search over the lot.

This repository is `ipm`'s source code. `ipm` was previously called `aihs`.

## Why it is different

- **pnpm does the fetching.** Your repos of interest are git dependencies in one pnpm project, so pinning (`pnpm-lock.yaml`), a content-addressable store and cheap upgrades come for free.
- **Pinned and reproducible.** Every repo is pinned to a commit; the catalog records the commit each artifact came from. Upgrading is one deliberate command.
- **No upstream code runs.** pnpm always runs with `--ignore-scripts`, and repos' own npm dependencies are never installed: sources are content, not code.
- **Relationships, not just files.** The catalog links plugins to the skills, agents, commands, hooks and MCP servers they own, and marketplaces to the plugins they list, so you can see where anything comes from.
- **Works from anywhere.** Data lives in your user folders, not a checkout.

## Quick start

Requirements: Go, [direnv](https://direnv.net), pnpm 12+ or Node (ipm then runs pnpm 12 through `npx`), and Docker for search. `gh` for GitHub stats.

```bash
direnv allow                          # in this checkout: puts bin/ipm on PATH
ipm source add obra/superpowers anthropics/skills
ipm sync                              # pnpm fetches and pins them, then ipm indexes
ipm stats                             # GitHub stars, topics, descriptions (needs gh)
ipm serve && ipm load                 # start Typesense in Docker and index the catalog
ipm search tdd
ipm doctor
```

## Commands

| Command | Does |
| --- | --- |
| `ipm source add\|remove\|list\|import` | Edit your repos of interest (`owner/repo[@ref]`, a git URL, or an external plugin ID); `import` migrates an aihs-era `manifest.toml` |
| `ipm sync [--upgrade]` | Fetch and pin every source with pnpm, then rebuild the catalog; `--upgrade` moves every source to the latest commit of its branch or tag |
| `ipm index` | Rebuild the catalog from synced sources |
| `ipm stats` | Fetch GitHub stats for every repo in the catalog |
| `ipm serve [--stop]` / `ipm load` | Run the local Typesense search server; rebuild its index from the catalog |
| `ipm search <query>` | Hybrid search with a SCORE column |
| `ipm show <id>`, `ipm tree <id>`, `ipm graph <id>` | An artifact, its relationships as a tree, its neighbourhood as Mermaid or Graphviz |
| `ipm scan [--paths] [repo…]` | Report the plugin, skills, hooks and agents folders in synced sources |
| `ipm doctor` | Check home, cache, pnpm, sources, catalog, Docker, Typesense and `claude` |
| `ipm help [command]` | Usage, or one command's flags |

## Where data lives

| | macOS | Linux | Override |
| --- | --- | --- | --- |
| Home: sources and the pnpm store | `~/Library/ipm` | `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm` | `IPM_HOME` |
| Cache: catalog and search data | `~/Library/Caches/ipm` | `$XDG_CACHE_HOME/ipm` or `~/.cache/ipm` | `IPM_CACHE_DIR` |

On Windows the home is `%LOCALAPPDATA%\ipm`.

## Limits

- **Building plugins is not here yet.** Part 3 of [the design](docs/superpowers/specs/2026-10-09-ipm-design.md) adds packs (`ipm.toml`) and `ipm install`, which builds Claude Code plugins from picked items at project or user scope.
- **Non-GitHub sources may lose files.** GitHub repos are fetched as plain tarballs; other git sources go through pnpm's git fetcher, which applies npm's packing rules (`files`, `.npmignore`).
- **Local build only.** `ipm` runs from this checkout through `bin/ipm`; there is no packaged release.

## Documentation

- [User guide](docs/user-guide.md) — set up, add sources, sync, search, troubleshoot.
- [Sources reference](docs/sources-reference.md) — the sources `package.json`, aliases, refs and `ipm source`.
- [Architecture](docs/architecture.md) — how `ipm` is built and why.
- [`cli/README.md`](cli/README.md) — working on `ipm` itself.

<p align="center"><sub>Maintained by Dr Gene Conroy-Jones.</sub></p>
