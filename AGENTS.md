# AGENTS.md

Router for agents working in [thinkjones/intelligent-package-manager](https://github.com/thinkjones/intelligent-package-manager): the source code of `ipm` ("intelligent package manager"), a local Go CLI that fetches repos of agent artifacts with pnpm, catalogs every skill, agent, command, hook, MCP server and plugin in them, searches the catalog, and builds packs of picked items into Claude Code plugins. Read the linked file for detail; this page is only where things are.

`ipm` was called `aihs`. The design and its parts are in [the ipm design](docs/superpowers/specs/2026-10-09-ipm-design.md); parts 1 (rename), 2 (sources on pnpm) and 3 (packs built into plugins with `ipm install`) are built; part 4 (a TUI) is not.

## Where things are

| Path | What it is |
| --- | --- |
| [`cli/`](cli/README.md) | `ipm`, the Go CLI. Every command is in `cli/internal/cli`. |
| `bin/ipm` | Shim that builds `cli/bin/ipm` when a Go source changes and runs it; `direnv allow` puts `bin/` on PATH. |
| [`docs/`](docs/) | The user documentation, published to GitHub Pages at <https://thinkjones.github.io/intelligent-package-manager/> (Jekyll, Just the Docs theme, `docs/_config.yml`): [getting started](docs/getting-started.md), [concepts](docs/concepts.md), `guides/`, `reference/` ([commands](docs/reference/commands.md), [sources](docs/reference/sources.md), [files](docs/reference/files.md)), [troubleshooting](docs/troubleshooting.md), [how it works](docs/architecture.md). |
| `docs/assets/brand/` | The hero, workflow and logo SVGs, shared by the README and the site. The palette (`#11151C` background, `#F3EFE6` text, `#F0A847` pins, `#5CC8A8` verified) is also in `docs/_sass/custom/setup.scss`; change both together. |
| `docs/plans/` | Early notes. Historical, and excluded from the site. |
| `docs/superpowers/` | Design specs and implementation plans. Historical: never rewrite them to match the code. Excluded from the site. |
| `LICENSE` | MIT. |

`ipm`'s data is outside this repo, per user:

| Path | What it is |
| --- | --- |
| `$IPM_HOME/sources/` | Your repos of interest as a pnpm project: `package.json` (one git dependency per repo, alias `owner--repo`), `pnpm-lock.yaml` (pinned commits), `node_modules/<alias>/` (the installed repos), `.pnpmfile.cjs` (generated). |
| `$IPM_HOME/store/` | ipm's dedicated pnpm store. |
| `$IPM_HOME/global/` | User scope (`-g`): `ipm.toml` and `.ipm/`, laid out as in a project. |
| `$IPM_CACHE_DIR/catalog/*.jsonl` | The catalog, written by `ipm index` (`github.jsonl` by `ipm stats`). Rebuildable; never committed. See [the catalog spec](docs/superpowers/specs/2026-10-08-artifact-catalog-design.md). |
| `$IPM_CACHE_DIR/typesense/` | The local search server's data and API key. Disposable; `ipm load` rebuilds the index. |

In a project (the nearest ancestor of the working directory holding `ipm.toml`):

| Path | What it is |
| --- | --- |
| `ipm.toml` | Packs: `[packs.<name>]` with `description` and `items` (catalog IDs). Edited with `ipm pack`. Committed. |
| `.ipm/ipm.lock` | Every picked item resolved to repo, path and commit. Generated, committed. |
| `.ipm/package.json`, `.ipm/pnpm-lock.yaml`, `.ipm/.pnpmfile.cjs` | One pnpm dependency per source repo, pinned to a commit. Generated, committed. |
| `.ipm/.claude-plugin/marketplace.json` | The marketplace listing each pack and its whole-plugin dependencies. Generated, committed. |
| `.ipm/plugins/<pack>/`, `.ipm/node_modules/` | Built plugins (hard links into pnpm's files) and pnpm's output. Gitignored by the generated `.ipm/.gitignore`. |

Defaults: `IPM_HOME` is `~/Library/ipm` on macOS, `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm` on Linux, `%LOCALAPPDATA%\ipm` on Windows; `IPM_CACHE_DIR` is `~/Library/Caches/ipm` on macOS, `$XDG_CACHE_HOME/ipm` or `~/.cache/ipm` on Linux.

## Conventions

- **Generated files are regenerated, not edited.** Change sources with `ipm source`, then `ipm sync`; change packs with `ipm pack`, then `ipm install`.
- **No upstream code runs.** pnpm always runs with `--ignore-scripts`, and sources' own npm dependencies are never installed.
- **`ipm` is self-contained.** It works from any directory, keeps its data only in `IPM_HOME` and `IPM_CACHE_DIR` (plus `ipm.toml` and `.ipm/` in a project), never installs into Claude Code, and shares no configuration with fsh or Gas City.
- **All CLI commands are Go**, in `cli/internal/cli`. There are no script commands.
- **Prose in Markdown is not hard-wrapped**: one line per paragraph or list item.
- **User docs change with the code.** A change to a command, flag, message or file format updates `docs/reference/commands.md`, the guide that uses it and `docs/troubleshooting.md` in the same commit. Each site page has Just the Docs front matter (`title`, `nav_order`, `parent` under `guides/` and `reference/`); link between pages with relative `.md` links, which work on GitHub and on the site. Don't link to `docs/superpowers/` with a relative link from a site page (it isn't published); use its GitHub URL.

[`CLAUDE.md`](CLAUDE.md) has the commands and the sharp edges.
