# CLAUDE.md

Guidance for Claude Code in this repository. [`AGENTS.md`](AGENTS.md) is the router (where things are, conventions); this file adds commands and the sharp edges.

`ipm` was called `aihs`. Parts 1–4 of [the ipm design](docs/superpowers/specs/2026-10-09-ipm-design.md) are built: rename, sources on pnpm, packs and `ipm pack build`, and `ipm tui` ([TUI design](docs/superpowers/specs/2026-10-09-ipm-tui-design.md)).

## What this repo is

The source code of `ipm`, a Go CLI (`cli/`) that keeps a per-user list of repos of interest as a pnpm project, has pnpm fetch and pin them, catalogs every skill, agent, command, hook, MCP server and plugin they hold, searches that catalog with a local Typesense server, and builds packs of picked items (`ipm.toml`) into Claude Code plugins under a project's `.ipm/` (or `$IPM_HOME/global/.ipm/` with `-g`). Its data lives in `IPM_HOME` and `IPM_CACHE_DIR`, not in this checkout. See [docs/architecture.md](docs/architecture.md).

## Commands

| Task | Command |
| --- | --- |
| Install `ipm` into `~/.local/bin` from this checkout (once; `$IPM_BIN_DIR` to change), then start onboarding | `./install.sh` |
| Run a development build of this checkout (rebuilt when a Go source changes) | `bin/ipm <command>` |
| Pick sources from the marketplace's curated list (after a concepts intro), then add, sync, fetch stats and load search in one go | `ipm onboard [--list <path\|url>] [--lists <name>]… [--keep\|--fresh] [--yes]` |
| Show the curated repos with ✓ on the ones you follow; show only the followed ones | `ipm marketplace [--list …] [--lists …]`, `ipm marketplace --followed` |
| Follow or unfollow repos | `ipm marketplace add <owner/repo[@ref]\|git URL[#ref]\|plugin ID>`, `ipm marketplace remove <repo>` |
| Migrate an aihs-era list of repos (once) | `ipm marketplace import <path/to/manifest.toml>` |
| Refresh GitHub stars, topics and descriptions into `github.jsonl` (needs `gh`) | `ipm marketplace stats` |
| Summarise the catalog: sources, items by type, last index, search server | `ipm catalog` |
| Fetch and pin every source with pnpm, rebuild the catalog, reload search if the server is up | `ipm catalog sync` |
| Move every source to the latest commit of its branch or tag | `ipm catalog sync --upgrade` |
| Rebuild the catalog from synced sources, then reload search if the server is up | `ipm catalog rebuild` |
| Start / stop the local search server (Typesense in Docker) | `ipm catalog serve` / `ipm catalog serve --stop` |
| Report the plugin, skills, hooks and agents folders in synced sources | `ipm catalog scan [--paths] [owner/repo...]` |
| Search every artifact (hybrid, with SCORE; `--keyword`, `--type`, `--repo`, `--tag`, `--limit`, `--page N`, `--all`, `--explain`, `--ids`, `--json`; each hit shows its ID) | `ipm search <query>` (same as `ipm catalog search`) |
| Inspect an artifact and its relationships (reads the catalog only) | `ipm show <id>`, `ipm show tree <id> [--up]`, `ipm show graph <id> --format mermaid\|dot` |
| Create a pack (creates `ipm.toml` in the working directory if no ancestor has one; `-g` for user scope) | `ipm pack new <name> [--description …]` |
| Add or remove catalog items in a pack; list packs; show a pack's items and installs | `ipm pack add <pack> <id>…`, `ipm pack remove <pack> <id>…`, `ipm pack list`, `ipm pack show <pack>` |
| Build every pack into a plugin and marketplace under `.ipm/` (writes `.ipm/build.stamp`), then validate | `ipm pack build [-g] [--update] [--no-validate]` |
| Build when stale, install a pack into Claude Code (user, project or local scope) and record the install | `ipm install claude [pack] [-s\|--scope] [-p\|--path] [-g] [--update] [--dry-run]` |
| Write a pack as a Gas City pack folder (`<dir>/<name>`), build first if stale, and print the import snippet; records the install, never edits a city | `ipm install gascity [pack] [-C <dir>] [--name <name>] [-g] [--force] [--dry-run] [--update]` |
| Reinstall every recorded install (`ipm.toml`, or `$IPM_HOME/installs.json` with `-g`) | `ipm install [-g]` |
| Four tabs in one screen (Bubble Tea): search and collect, packs (build, install), installs, marketplace (follow repos, sync) | `ipm tui [-g]` |
| Check home, cache, pnpm, sources, catalog, docker, typesense, claude | `ipm doctor` |
| Remove the search container and the binary, keeping data (`--purge` deletes home and cache; a dev build needs `--force`) | `ipm uninstall [--purge] [--yes] [--force]` |
| Test a change (the gate: format and lint checks, build, vet, unit tests) | `mise run verify` |
| Install the git hooks, once per clone | `mise run setup` |
| Fix formatting | `mise run fmt` |
| Run the tests that need real pnpm, Node and Docker | `mise run test:integration` |
| Look for new repos to add to `source-packs.toml` (downloads repos; the Source packs workflow runs it and opens a pull request) | `mise run packs:update` |
| Install from the published module | `go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest` |

The workflow is onboard → marketplace → catalog → search → pack → install; `ipm -h` lists the commands in that order, and the old names (`source`, `stats`, `sync`, `index`, `load`, `serve`, `scan`, `tree`, `graph`, `deploy`) are hidden stubs that exit 2 and name the new command. The loop: `ipm marketplace add <repo>` → `ipm catalog sync` (installs, indexes, then loads search if the server is up) → `ipm marketplace stats` when stars should be refreshed → `ipm catalog rebuild` to refresh search. Source files and refs are in [docs/reference/sources.md](docs/reference/sources.md). The pack loop: `ipm pack new` → `ipm pack add` → `ipm pack build` → `ipm install claude` (or `claude plugin marketplace add <project>/.ipm`, in a sandbox `CLAUDE_CONFIG_DIR` when testing).

## Sharp edges

- **Never hand-edit generated files**: `$IPM_HOME/sources/package.json` (use `ipm marketplace`), `pnpm-lock.yaml`, `.pnpmfile.cjs` (rewritten on every sync), the catalog, `$IPM_HOME/installs.json`, and everything in a project's `.ipm/` including `build.stamp` (`ipm pack build` rewrites it; `ipm.toml` is edited with `ipm pack`, which also holds the `[[packs.<name>.installs]]` records `ipm install` writes).
- **`source-packs.toml` and `source-packs.skipped.txt` are generated too**, by `mise run packs:update`; see [AGENTS.md](AGENTS.md#source-packs).
- **`.ipm/plugins/` files are hard links into pnpm's store.** Editing one edits pnpm's copy too, and `ipm pack build` rebuilds the folder anyway. Edit upstream or in your own repo.
- **`ipm pack build` keeps existing pins** from `.ipm/ipm.lock`; `--update` re-pins to the current catalog. It must stay deterministic: two runs give byte-identical `.ipm/` (including `build.stamp`, a hash of the packs, the lock and the ipm version), and items removed from a pack leave no files. `ipm install <target>` skips the build while the stamp matches.
- **Hooks, MCP servers and inline commands are not pickable**; `ipm pack add` refuses them and names the owning plugin to add instead. Pack names Claude Code reserves (`claude-…`, `anthropic-…`, `official` beside claude/anthropic) are refused.
- **pnpm 12 or later is required.** `ipm` uses pnpm from PATH when it is 12+, else `npx -y pnpm@12.10.1`. pnpm always runs with `--ignore-scripts --store-dir <home>/store` and `--dir <home>/sources` (`catalog sync`) or `--dir <root>/.ipm` (`pack build`); never run it there without those flags.
- **The `.pnpmfile.cjs` hook is load-bearing.** Without it pnpm re-packs GitHub repos with npm's packing rules and drops files (`nizos/tdd-guard`: 429 files → 3). Non-GitHub sources still go through pnpm's git fetcher and can lose files the same way.
- **A failed `ipm catalog sync` leaves the catalog unchanged**; indexing only runs after pnpm succeeds.
- **Tests never touch your real home**: set `IPM_HOME` and `IPM_CACHE_DIR` to absolute temp paths (relative ones are refused).
- **Verify upstream facts from the installed sources**, not from memory: `$IPM_HOME/sources/node_modules/<owner--repo>/` is the repo at its pinned commit. Run `ipm catalog sync` first if it is missing.
- **The search server is container `ipm-typesense` on `127.0.0.1:8108`.** `ipm catalog serve` removes an old `aihs-typesense` container that would hold the port. `mise run test:integration` starts a real one, so it refuses to run while that container exists or the port is taken; its green run comes from CI.
- **A test that needs pnpm, Node or Docker uses `testtool.Need`**: it skips under a bare `go test` and fails under mise or CI (`IPM_TEST_STRICT=1`). Tests of real pnpm or Docker carry the `integration` build tag.
- **Commits go through a pull request.** The hooks and CI are described in [AGENTS.md](AGENTS.md#quality-gate); `main` refuses direct pushes.
- **`ipm install claude` (and the TUI's installs, from the Packs and Installs tabs) changes the user's Claude config** through `claude plugin marketplace add|update` and `claude plugin install|update`; every `claude` call goes through `System.Output`, so tests fake it. `internal/cli/testdata/claude-*.json` are real `claude` 2.1.296 listings. **Installing plugins changes the user's Claude config.** Test installs with `CLAUDE_CONFIG_DIR` pointing at a temporary directory.
- **The docs site is `docs/`**, published by GitHub Pages from `main` (`docs/_config.yml`, Just the Docs). Commands, flags, messages and file formats are documented there; update [the commands reference](docs/reference/commands.md), the relevant guide and [troubleshooting](docs/troubleshooting.md) when they change. Verify claims against the code or a live run, not memory.
- **`internal/tui` depends only on its six interfaces** (`Searcher`, `Catalog`, `Packs`, `Builder`, `Installer`, `Marketplace`); never import ipm packages or `System` there. The adapters in `internal/cli/tui.go` reuse the CLI's functions (`search.Query`, `addToPack` and `removeFromPack`, `runBuild`, `planDeploy` with the `installers` table, `tuiInstalls`, `sourceAdd` and `sourceRemove`, `runSync`, `curated.Fetch`), so the TUI adds no behaviour of its own. Global packs and installs listed from a project are read-only (`ipm tui -g` changes them), and the TUI never reloads the catalog after a sync from the Marketplace tab: it tells you to restart. Screen changes update `internal/tui/testdata/*.golden` with `go test ./internal/tui -run Golden -update`; read the diff before committing.
- **`docs/superpowers/` is history.** Specs and plans describe what was intended at the time; don't edit them to match the code.
- **`ipm` stays self-contained**: no global config beyond its own home and cache, nothing shared with fsh; ipm writes Gas City pack folders on `ipm install gascity` but reads and edits no Gas City config. Commands are Go only, built with Cobra: a `new<Name>Cmd(sys System)` in `cli/internal/cli`, registered in `newRootCmd` in `cli.go`. Malformed command lines return a `UsageError` (exit 2); use `noArgs`, `exactArgs`, `minArgs` and `verbs` from `cli.go` rather than Cobra's own argument checks, which return plain errors.
