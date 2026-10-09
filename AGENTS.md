# AGENTS.md

Router for agents working in ai-harness-skills: the source code of `ipm` ("intelligent package manager"), a local Go CLI that fetches repos of agent artifacts with pnpm, catalogs every skill, agent, command, hook, MCP server and plugin in them, and searches the catalog. Read the linked file for detail; this page is only where things are.

`ipm` was called `aihs`. The design and its parts are in [the ipm design](docs/superpowers/specs/2026-10-09-ipm-design.md); parts 1 (rename) and 2 (sources on pnpm) are built, part 3 (packs built into plugins with `ipm install`) is not.

## Where things are

| Path | What it is |
| --- | --- |
| [`cli/`](cli/README.md) | `ipm`, the Go CLI. Every command is in `cli/internal/cli`. |
| `bin/ipm` | Shim that builds `cli/bin/ipm` when a Go source changes and runs it; `direnv allow` puts `bin/` on PATH. |
| [`docs/`](docs/) | [User guide](docs/user-guide.md), [sources reference](docs/sources-reference.md), [architecture](docs/architecture.md). |
| `docs/superpowers/` | Design specs and implementation plans. Historical: never rewrite them to match the code. |
| [`assets/readme/`](assets/readme/) | SVGs from the earlier marketplace design; not referenced by the README. |

`ipm`'s data is outside this repo, per user:

| Path | What it is |
| --- | --- |
| `$IPM_HOME/sources/` | Your repos of interest as a pnpm project: `package.json` (one git dependency per repo, alias `owner--repo`), `pnpm-lock.yaml` (pinned commits), `node_modules/<alias>/` (the installed repos), `.pnpmfile.cjs` (generated). |
| `$IPM_HOME/store/` | ipm's dedicated pnpm store. |
| `$IPM_CACHE_DIR/catalog/*.jsonl` | The catalog, written by `ipm index` (`github.jsonl` by `ipm stats`). Rebuildable; never committed. See [the catalog spec](docs/superpowers/specs/2026-10-08-artifact-catalog-design.md). |
| `$IPM_CACHE_DIR/typesense/` | The local search server's data and API key. Disposable; `ipm load` rebuilds the index. |

Defaults: `IPM_HOME` is `~/Library/ipm` on macOS, `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm` on Linux, `%LOCALAPPDATA%\ipm` on Windows; `IPM_CACHE_DIR` is `~/Library/Caches/ipm` on macOS, `$XDG_CACHE_HOME/ipm` or `~/.cache/ipm` on Linux.

## Conventions

- **Generated files are regenerated, not edited.** Change sources with `ipm source`, then `ipm sync`.
- **No upstream code runs.** pnpm always runs with `--ignore-scripts`, and sources' own npm dependencies are never installed.
- **`ipm` is self-contained.** It works from any directory, keeps its data only in `IPM_HOME` and `IPM_CACHE_DIR`, never installs into Claude Code, and shares no configuration with fsh or Gas City.
- **All CLI commands are Go**, in `cli/internal/cli`. There are no script commands.
- **Prose in Markdown is not hard-wrapped**: one line per paragraph or list item.

[`CLAUDE.md`](CLAUDE.md) has the commands and the sharp edges.
