# ipm

The Go CLI. Every command is Go code in this module; it keeps its data in `IPM_HOME` and `IPM_CACHE_DIR` (plus `ipm.toml` and `.ipm/` in a project), never installs into Claude Code, and is separate from fsh and Gas City. For using it see [the documentation](https://thinkjones.github.io/intelligent-package-manager/) (source in [`docs/`](../docs/)); for the design see [how it works](../docs/architecture.md).

## Usage

```text
ipm source add <repo>…         add sources: owner/repo[@ref], a git URL[#ref], or an external plugin ID
ipm source remove <repo>…      remove sources
ipm source list                alias, repo, ref and pinned commit of every source
ipm source import <manifest>   one-time: add the [[repo]] entries of an aihs-era manifest.toml
ipm sync [--upgrade]           pnpm install (or update --latest) the sources, then ipm index
ipm index                      rebuild <cache>/catalog/*.jsonl from the synced sources
ipm stats                      fetch GitHub stats for every catalog repo into github.jsonl (needs gh)
ipm serve [--stop]             run Typesense in Docker as ipm-typesense on 127.0.0.1:8108
ipm load                       rebuild the search index from the catalog
ipm search <query> [flags]     hybrid search with a SCORE column: --type, --repo, --tag (repeatable), --keyword,
                               --limit, --page N, --all, --explain (KW/SEM columns), --ids, --json
ipm show <id> [--json]         an artifact's fields, GitHub stats and neighbours
ipm tree <id> [--up]           relationships as a tree (--depth N, --all)
ipm graph <id> [--format dot]  the neighbourhood as Mermaid (default) or Graphviz (--depth N, --all, --up)
ipm pack new <name> [--description …] [-g]   create a pack in ipm.toml (created here if no ancestor has one)
ipm pack add <pack> <id>… [-g]                add catalog items: skills, agents, file commands, plugins, Copilot plugins
ipm pack remove <pack> <id>… [-g]             remove items
ipm pack list [-g]                            PACK, ITEMS, DESCRIPTION
ipm pack show <pack> [-g]                     TYPE, NAME, REPO, ★, ID of each item
ipm install [-g] [--update] [--no-validate]   build each pack into .ipm/plugins/<pack>/ and .ipm/.claude-plugin/marketplace.json,
                               then claude plugin validate --strict; --update re-pins to the current catalog
ipm scan [--paths] [repo...]   report the plugin, skills, hooks and agents folders in synced sources
ipm doctor                     check home, cache, pnpm, sources, catalog, docker, typesense, claude
ipm version
ipm help [command]             usage, or one command's flags
```

Exit codes: `0` success; `1` failure (bad source, pnpm missing or failed, unsynced sources, an item not pickable or not found, validation failed, doctor problems); `2` malformed command line.

## Setup

```bash
direnv allow     # once: .envrc puts the repo's bin/ on PATH
ipm doctor
```

Users install the same binary with `go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest`.

`bin/ipm` is a shell shim: it builds `cli/bin/ipm` on first use and whenever a Go source in `cli/` changes, then execs the binary. The binary reads `IPM_HOME` and `IPM_CACHE_DIR` (and `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `HOME`, `LOCALAPPDATA`) to find its data, so it works from any directory.

`ipm sync` and `ipm install` need pnpm 12 or later: pnpm from PATH when it is new enough, otherwise `npx -y pnpm@12.10.1`. Search needs Docker; `ipm stats` needs `gh`; `ipm install` validates with `claude` when it is on PATH.

## Layout

```text
cmd/ipm/main.go           wires the process into cli.Run: argv, real commands, HTTP client, exit codes
internal/cli/              the commands table, help, and one file per command (packcmd.go is ipm pack); pnpm.go finds pnpm and writes .pnpmfile.cjs
internal/paths/            IPM_HOME and the cache dir: platform defaults, overrides, the folders under them
internal/sources/          sources/package.json (aliases, specifiers), pnpm-lock.yaml commits, Checkouts; WriteProject for .ipm/package.json
internal/catalog/          walk, parse and link a source into catalog records; read and write *.jsonl
internal/github/           GitHub stats through gh
internal/typesense/        a plain net/http Typesense client
internal/search/           documents, schema, search parameters, the results table
internal/catgraph/         the catalog as a graph for show, tree and graph
internal/scan/             the folder report behind ipm scan
internal/project/          the scope root (ipm.toml up the tree, or $IPM_HOME/global), ipm.toml packs, .ipm/ipm.lock
internal/assemble/         resolve picks (lock, then catalog), plan each pack's files and dependencies, place them by hard link
internal/pack/             naming rules, and reading an aihs-era manifest.toml for source import
internal/copilot/          which skills a Copilot plugin.json lists
internal/marketplace/      marketplace.json entries (including upstream entry maps) and atomic Write
```

Commands reach the outside world only through `cli.System` — stdout, stderr, environment, `LookPath`, an `Output` runner used for pnpm, docker, gh and claude, an HTTP client for Typesense, and a clock — so tests drive them in-process with a fake.

## Adding a command

Add `internal/cli/<name>.go` with a `<name>Flags()` function returning the command's `flag.FlagSet` and its run function, and a row in the `commands` table in `cli.go`. Write its tests in `internal/cli` against the fake `System`. Resolve data folders with `ipmPaths(sys)`.

## Testing

```bash
mise run cli:test      # gofmt check, go vet, go test ./... — the gate for changes here
mise run cli:build     # cli/bin/ipm (bin/ipm also does this on demand)
```

- `internal/paths` and `internal/sources` test path resolution on every OS, aliases, the `package.json` round trip, collisions and reading commits from a fixture `pnpm-lock.yaml`.
- `internal/project` and `internal/assemble` test the scope walk, the `ipm.toml` and lock round trips, resolving from lock then catalog, name clashes, Copilot plugins expanded to skills, whole-plugin dependencies, hard link falling back to copy, and stale files removed.
- `internal/catalog`, `internal/catgraph`, `internal/search` and `internal/typesense` test the catalog, the graph and search against fixtures and a fake server.
- `internal/cli` drives every command in-process with a fake `System`, `IPM_HOME` and `IPM_CACHE_DIR` in temp dirs; no real pnpm, docker or claude.
- `cmd/ipm` builds the real binary and checks exit codes; with pnpm installed it also runs `source add`, `sync` and `index` against a local git repo over `file://`.
