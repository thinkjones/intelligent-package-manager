# ipm

`ipm` ("intelligent package manager") finds agent artifacts across many repos. You list the repos you care about; `ipm` has pnpm fetch and pin them, catalogs every skill, agent, command, hook, MCP server and plugin they hold, and gives you hybrid keyword and semantic search over the lot. Then you pick items into **packs** and `ipm install` builds each pack into a Claude Code plugin, pinned to the commits you picked from.

**Documentation: [thinkjones.github.io/intelligent-package-manager](https://thinkjones.github.io/intelligent-package-manager/)**

## Why it is different

- **pnpm does the fetching.** Your repos of interest are git dependencies in one pnpm project, so pinning (`pnpm-lock.yaml`), a content-addressable store and cheap upgrades come for free.
- **Pinned and reproducible.** Every repo is pinned to a commit; the catalog records the commit each artifact came from. Upgrading is one deliberate command.
- **No upstream code runs.** pnpm always runs with `--ignore-scripts`, and repos' own npm dependencies are never installed: sources are content, not code.
- **Relationships, not just files.** The catalog links plugins to the skills, agents, commands, hooks and MCP servers they own, and marketplaces to the plugins they list, so you can see where anything comes from.
- **Your own plugins from anyone's parts.** A pack in `ipm.toml` names skills, agents, commands and whole plugins from any of your sources; `ipm install` builds it into a plugin and a local marketplace under `.ipm/`, at project scope or (`-g`) user scope.
- **Works from anywhere.** Data lives in your user folders, not a checkout.

## Quick start

Requirements: Go 1.26+, pnpm 12+ or Node (ipm then runs pnpm 12 through `npx`), and Docker for search. `gh` for GitHub stats, `jq` to list IDs from search.

```bash
go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest

ipm source add obra/superpowers anthropics/skills
ipm sync                              # pnpm fetches and pins them, then ipm indexes
ipm stats                             # GitHub stars, topics, descriptions (needs gh)
ipm serve && ipm load                 # start Typesense in Docker and index the catalog
ipm search tdd
ipm search tdd --type skill --json | jq -r .document.id   # the IDs you pick with
ipm doctor

# in a project: build a plugin from picked items
ipm pack new my-pack --description "My picks"
ipm pack add my-pack 'skill:obra/superpowers:skills/test-driven-development'
ipm install                           # builds .ipm/plugins/my-pack/ and .ipm/.claude-plugin/marketplace.json
claude plugin marketplace add ./.ipm  # then /plugin install my-pack@<project dir>-ipm in Claude Code
```

See [Getting started](https://thinkjones.github.io/intelligent-package-manager/getting-started.html) for the full walkthrough.

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
| `ipm pack new\|add\|remove\|list\|show [-g]` | Create and edit packs of catalog items in `ipm.toml` (project) or your user scope (`-g`) |
| `ipm install [-g] [--update] [--no-validate]` | Build each pack into a Claude Code plugin and marketplace under `.ipm/`, then `claude plugin validate --strict` |
| `ipm scan [--paths] [repo…]` | Report the plugin, skills, hooks and agents folders in synced sources |
| `ipm doctor` | Check home, cache, pnpm, sources, catalog, Docker, Typesense and `claude` |
| `ipm help [command]` | Usage, or one command's flags |

## Where data lives

| | macOS | Linux | Override |
| --- | --- | --- | --- |
| Home: sources, the pnpm store and user-scope packs (`global/`) | `~/Library/ipm` | `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm` | `IPM_HOME` |
| Cache: catalog and search data | `~/Library/Caches/ipm` | `$XDG_CACHE_HOME/ipm` or `~/.cache/ipm` | `IPM_CACHE_DIR` |

On Windows the home is `%LOCALAPPDATA%\ipm`.

## Limits

- **`ipm` never installs into Claude Code.** `ipm install` builds the plugin and marketplace; you add the marketplace and install the plugin in Claude Code yourself.
- **Hooks and MCP servers come only with their whole plugin.** They (and inline commands) can't be picked on their own.
- **No TUI yet.** Part 4 of [the design](docs/superpowers/specs/2026-10-09-ipm-design.md), a TUI over search and packs, is next.
- **Non-GitHub sources may lose files.** GitHub repos are fetched as plain tarballs; other git sources go through pnpm's git fetcher, which applies npm's packing rules (`files`, `.npmignore`).
- **No packaged release yet.** Install with `go install`, or build from a clone.
- **Search results don't show IDs.** Use `--json` (or `ipm pack show` for items already picked).
- **macOS and Linux.** Windows paths are defined but untested.

## Documentation

The [documentation site](https://thinkjones.github.io/intelligent-package-manager/) is built from [`docs/`](docs/):

- [Getting started](docs/getting-started.md) and [Concepts](docs/concepts.md).
- Guides: [sources](docs/guides/sources.md), [search](docs/guides/search.md), [build your own plugin](docs/guides/build-a-plugin.md), [share packs with a team](docs/guides/teams.md).
- Reference: [commands](docs/reference/commands.md), [sources](docs/reference/sources.md), [files and paths](docs/reference/files.md).
- [Troubleshooting](docs/troubleshooting.md) and [how it works](docs/architecture.md).
- [`cli/README.md`](cli/README.md): working on `ipm` itself.

## Contributing

```bash
git clone https://github.com/thinkjones/intelligent-package-manager.git
cd intelligent-package-manager
direnv allow          # puts bin/ on PATH; bin/ipm rebuilds the CLI when its source changes
mise run cli:test     # gofmt, go vet, go test: the gate for every change
```

`ipm` was previously called `aihs`. Design history is in [`docs/superpowers/`](docs/superpowers/).

## License

[MIT](LICENSE)

<p align="center"><sub>Maintained by Dr Gene Conroy-Jones.</sub></p>
