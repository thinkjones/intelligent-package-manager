# ipm

The Go CLI. Every command is Go code in this module; it keeps its data in `IPM_HOME` and `IPM_CACHE_DIR` (plus `ipm.toml` and `.ipm/` in a project), installs into Claude Code only on `ipm install claude`, and is separate from fsh and Gas City. For using it see [the documentation](https://thinkjones.github.io/intelligent-package-manager/) (source in [`docs/`](../docs/)); for the design see [how it works](../docs/architecture.md).

## Usage

```text
ipm onboard [--list <path|url>] [--lists <name>]… [--keep|--fresh] [--yes]
                               print the concepts, pick sources from the marketplace, then add, sync, stats, serve and load
ipm marketplace [--list L] [--lists NAME] [--followed]   the curated repos with ✓ on the ones you follow (--followed: your sources, with ref, pinned commit, description, topics, homepage)
ipm marketplace add <repo>…    follow repos: owner/repo[@ref], a git URL[#ref], or an external plugin ID
ipm marketplace remove <repo>… stop following repos
ipm marketplace import <manifest>   one-time: add the [[repo]] entries of an aihs-era manifest.toml
ipm marketplace stats          fetch GitHub stats for every catalog repo into github.jsonl (needs gh)
ipm catalog                    summary: sources followed, items by type, last index, search server
ipm catalog sync [--upgrade]   pnpm install (or update --latest) the sources, rebuild the catalog, reload search when the server is up
ipm catalog rebuild            rebuild <cache>/catalog/*.jsonl from the synced sources, then reload search
ipm catalog serve [--stop]     run Typesense in Docker as ipm-typesense on 127.0.0.1:8108
ipm catalog scan [--paths] [repo...]   report the plugin, skills, hooks and agents folders in synced sources
ipm search <query> [flags]     (also ipm catalog search) hybrid search with a SCORE column: --type, --repo, --tag (repeatable), --keyword,
                               --limit, --page N, --all, --explain (KW/SEM columns), --ids, --json
ipm show <id> [--json]         an artifact's fields, GitHub stats and neighbours
ipm show tree <id> [--up]      relationships as a tree (--depth N, --all)
ipm show graph <id> [--format dot]   the neighbourhood as Mermaid (default) or Graphviz (--depth N, --all, --up)
ipm pack new <name> [--description …] [-g]   create a pack in ipm.toml (created here if no ancestor has one)
ipm pack add <pack> <id>… [-g]                add catalog items: skills, agents, file commands, plugins, Copilot plugins
ipm pack remove <pack> <id>… [-g]             remove items
ipm pack list [-g]                            PACK, ITEMS, DESCRIPTION
ipm pack show <pack> [-g]                     TYPE, NAME, REPO, ★, ID of each item, then where the pack is installed
ipm pack build [-g] [--update] [--no-validate]   build each pack into .ipm/plugins/<pack>/ and .ipm/.claude-plugin/marketplace.json,
                               then claude plugin validate --strict; --update re-pins to the current catalog; writes .ipm/build.stamp
ipm install claude [pack] [-s|--scope user|project|local] [-p|--path DIR] [-g] [--update] [--dry-run]
                               build when .ipm/ is stale, then install a pack into Claude Code and record it (also ipm pack install claude)
ipm install [-g]               reinstall every recorded install (ipm.toml, or $IPM_HOME/installs.json with -g)
ipm tui [-g]                   search, see the tree, collect, build and deploy, in one screen
ipm doctor                     check home, cache, pnpm, sources, catalog, docker, typesense, claude
ipm uninstall [--purge] [--yes] [--force]   remove the search container and the binary; --purge also deletes home and cache
ipm version
ipm help [command]             usage, or one command's flags (or ipm <command> --help)
ipm completion zsh|bash|fish|powershell   shell completion script
```

The old names (`source`, `stats`, `sync`, `index`, `load`, `serve`, `scan`, `tree`, `graph`, `deploy`) are hidden stubs that exit 2 and name the new command.

Exit codes: `0` success; `1` failure (bad source, pnpm missing or failed, unsynced sources, an item not pickable or not found, validation failed, doctor problems); `2` malformed command line.

## Setup

```bash
./install.sh     # from the repo root: builds cli/ into ~/.local/bin/ipm (or $IPM_BIN_DIR), then runs ipm onboard
bin/ipm doctor   # a development build of this checkout
```

Users install the same binary with `curl -fsSL https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/install.sh | bash`, which runs `go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest`. `.envrc` doesn't put `bin/` on PATH, so `ipm` is the installed binary and `bin/ipm` the development one; `ipm uninstall` refuses to delete a development build without `--force`.

`bin/ipm` is a shell shim: it builds `cli/bin/ipm` on first use and whenever a Go source in `cli/` changes, then execs the binary. The binary reads `IPM_HOME` and `IPM_CACHE_DIR` (and `XDG_DATA_HOME`, `XDG_CACHE_HOME`, `HOME`, `LOCALAPPDATA`) to find its data, so it works from any directory.

`ipm catalog sync` and `ipm pack build` need pnpm 12 or later: pnpm from PATH when it is new enough, otherwise `npx -y pnpm@12.10.1`. Search needs Docker; `ipm marketplace stats` needs `gh`; `ipm pack build` validates with `claude` when it is on PATH, and `ipm install claude` needs it.

## Layout

```text
cmd/ipm/main.go           wires the process into cli.Run: argv, real commands, HTTP client, exit codes
internal/cli/              the Cobra command tree (cli.go: newRootCmd, usage errors, argument checks) and one file per command (packcmd.go is ipm pack); pnpm.go finds pnpm and writes .pnpmfile.cjs
internal/paths/            IPM_HOME and the cache dir: platform defaults, overrides, the folders under them
internal/sources/          sources/package.json (aliases, specifiers), pnpm-lock.yaml commits, Checkouts; WriteProject for .ipm/package.json
internal/catalog/          walk, parse and link a source into catalog records; read and write *.jsonl
internal/github/           GitHub stats through gh
internal/typesense/        a plain net/http Typesense client
internal/search/           documents, schema, search parameters, the results table
internal/catgraph/         the catalog as a graph for show, show tree and show graph
internal/scan/             the folder report behind ipm catalog scan
internal/project/          the scope root (ipm.toml up the tree, or $IPM_HOME/global), ipm.toml packs, .ipm/ipm.lock
internal/assemble/         resolve picks (lock, then catalog), plan each pack's files and dependencies, place them by hard link
internal/pack/             naming rules, and reading an aihs-era manifest.toml for marketplace import
internal/copilot/          which skills a Copilot plugin.json lists
internal/marketplace/      marketplace.json entries (including upstream entry maps) and atomic Write
internal/curated/          curated source lists for ipm onboard and ipm marketplace: parse and write [[repos]] TOML, fetch with a cached copy for offline
internal/sourcepacks/      maintain source-packs.toml: find the repos of a wider list that hold something ipm can catalog (cmd/sourcepacks runs it; not part of the installed ipm)
internal/tui/              the Bubble Tea model behind ipm tui: search, detail, tray, picker, install modal; depends only on its four interfaces (deps.go); checklist.go is ipm onboard's picker
```

Commands reach the outside world only through `cli.System` — stdout, stderr, environment, `LookPath`, an `Output` runner used for pnpm, docker, gh and claude, an HTTP client for Typesense, and a clock — so tests drive them in-process with a fake.

## Adding a command

Add `internal/cli/<name>.go` with a `new<Name>Cmd(sys System) *cobra.Command` and register it in `newRootCmd` in `cli.go`. Check positional arguments with `noArgs`, `exactArgs` or `minArgs` (a parent with subcommands uses `verbs`), so a malformed command line is a `UsageError` and exits 2. Write its tests in `internal/cli` against the fake `System` (`f.run("name", "args"…)`). Resolve data folders with `ipmPaths(sys)`.

## Testing

```bash
mise run verify            # gofmt, shfmt and golangci-lint checks, go build, go vet, go test ./... — the gate
mise run test:integration  # real pnpm, Node and Typesense in Docker (-tags integration)
mise run cli:build         # cli/bin/ipm (bin/ipm also does this on demand)
```

- `internal/paths` and `internal/sources` test path resolution on every OS, aliases, the `package.json` round trip, collisions and reading commits from a fixture `pnpm-lock.yaml`.
- `internal/project` and `internal/assemble` test the scope walk, the `ipm.toml` and lock round trips, resolving from lock then catalog, name clashes, Copilot plugins expanded to skills, whole-plugin dependencies, hard link falling back to copy, and stale files removed.
- `internal/catalog`, `internal/catgraph`, `internal/search` and `internal/typesense` test the catalog, the graph and search against fixtures and a fake server.
- `internal/tui` drives the model with key presses against fake interfaces, and compares rendered screens with `testdata/*.golden` (`go test ./internal/tui -run Golden -update` rewrites them).
- `internal/cli` drives every command in-process with a fake `System`, `IPM_HOME` and `IPM_CACHE_DIR` in temp dirs; no real pnpm, docker or claude.
- `cmd/ipm` builds the real binary and checks exit codes; with pnpm installed it also runs `source add`, `sync` and `index` against a local git repo over `file://`.
