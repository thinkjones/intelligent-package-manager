---
title: Commands
parent: Reference
nav_order: 1
---

# Commands
{: .no_toc }

Every `ipm` command and flag. `ipm help <command>` prints the same from the CLI.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

Flags may come before or after positional arguments. Long flags take two dashes (`--update`); `-g` is short for `--global`. `ipm <command> --help` and `ipm help <command>` print a command's usage and flags, and a mistyped command gets a suggestion.

## Sources

### `ipm source`

```text
ipm source add <repo>…
ipm source remove <repo>…
ipm source list
ipm source import <path/to/manifest.toml>
```

| Subcommand | Does |
| --- | --- |
| `add` | Add sources: `owner/repo[@ref]`, a git URL (`https://`, `ssh://`, `file://`, `git@`) with an optional `#ref`, or an external plugin ID (`plugin:<marketplace repo>#<name>`), which adds the repo the plugin lives in. A repo already listed, or two repos that would share an alias, is refused and nothing is written. |
| `remove` | Remove sources. An unknown repo is an error and nothing is written. |
| `list` | Alias, repo, ref and pinned commit of every source (`not synced` until `ipm sync`). |
| `import` | One-time migration from an aihs-era `manifest.toml`. |

`add`, `remove` and `import` only edit the source list; run `ipm sync` after.

### `ipm sync`

```text
ipm sync [--upgrade]
```

Has pnpm fetch and pin every source, then runs `ipm index`. If pnpm fails, the catalog is unchanged and `ipm` exits 1.

| Flag | Does |
| --- | --- |
| `--upgrade` | Move every source to the latest commit of its branch or tag first. |

### `ipm index`

```text
ipm index
```

Rebuilds the catalog from your synced sources, without fetching.

### `ipm stats`

```text
ipm stats
```

Fetches GitHub stars, forks, topics, licence, description and last push for every repo in the catalog. Needs `gh`, logged in.

### `ipm scan`

```text
ipm scan [--paths] [owner/repo…]
```

Reports each synced source's commit and its plugins, Copilot plugins, marketplaces, and `skills`, `hooks` and `agents` folders.

| Flag | Does |
| --- | --- |
| `--paths` | List every folder found, not only the counts. |

## Search

### `ipm serve`

```text
ipm serve [--stop]
```

Starts Typesense in Docker as container `ipm-typesense` on `127.0.0.1:8108`, with its data in your cache folder.

| Flag | Does |
| --- | --- |
| `--stop` | Stop the server. Its data is kept. |

### `ipm load`

```text
ipm load
```

Rebuilds the search index from the catalog and GitHub stats. Run it after `ipm sync` or `ipm stats`.

### `ipm search`

```text
ipm search [flags] <query>
```

Hybrid keyword and semantic search. Needs `ipm serve` and `ipm load`. Each hit's full ID is printed on the line under it. The table fits the terminal width (or `COLUMNS`), dropping DESCRIPTION, then PLUGINS, then REPO when narrow; piped output isn't cut.

| Flag | Does |
| --- | --- |
| `--type T` | Only this type (repeatable): `repo`, `marketplace`, `plugin`, `copilot-plugin`, `skill`, `agent`, `command`, `hook`, `mcp`. |
| `--repo R` | Only artifacts from this repo (repeatable). |
| `--tag T` | Only artifacts with this tag or topic (repeatable). |
| `--limit N` | Results to show, at most 250. Default 20. |
| `--page N` | Page of results, `--limit` per page. |
| `--all` | Every result, fetched 250 at a time. |
| `--keyword` | Keyword ranking only. |
| `--explain` | Add KW and SEM columns. |
| `--ids` | Only each hit's ID, one per line. Doesn't combine with `--json`. |
| `--json` | One JSON line per hit. |

### `ipm show`

```text
ipm show [--json] <id>
```

An artifact's fields, its repo's GitHub stats, and its neighbours. Reads the catalog; no search server needed.

| Flag | Does |
| --- | --- |
| `--json` | Print the catalog record as JSON. |

### `ipm tree`

```text
ipm tree [flags] <id>
```

An artifact's relationships as a tree. Reads the catalog.

| Flag | Does |
| --- | --- |
| `--up` | Walk to what contains the artifact, instead of what it contains. |
| `--depth N` | Levels to show. Default: 3 from a repo, otherwise all. |
| `--all` | Show every child, not the first 25. |

### `ipm graph`

```text
ipm graph [flags] <id>
```

An artifact's neighbourhood as a Mermaid or Graphviz graph. Reads the catalog.

| Flag | Does |
| --- | --- |
| `--format F` | `mermaid` (default) or `dot`. |
| `--depth N` | Levels to include. Default 2. |
| `--up` | Walk to what contains the artifact. |
| `--all` | Include every child, not the first 25. |

## Packs

### `ipm pack`

```text
ipm pack new <name> [--description …] [-g]
ipm pack add <pack> <id>… [-g]
ipm pack remove <pack> <id>… [-g]
ipm pack list [-g]
ipm pack show <pack> [-g]
```

| Subcommand | Does |
| --- | --- |
| `new` | Create a pack. Creates `ipm.toml` in the current folder if no folder above has one. |
| `add` | Add catalog items. Every ID is checked first; if any is unknown or can't be picked, nothing is added. Duplicates are ignored. |
| `remove` | Remove items. |
| `list` | Every pack in the scope: name, item count, description. |
| `show` | A pack's items: type, name, repo, stars and ID. |

| Flag | Does |
| --- | --- |
| `--description D` | The pack's description (`new`). |
| `-g` | Use your user scope (`$IPM_HOME/global`) instead of the project. |

### `ipm install`

```text
ipm install [-g] [--update] [--no-validate]
```

Builds every pack in the scope into a Claude Code plugin under `.ipm/plugins/`, writes `.ipm/.claude-plugin/marketplace.json` and `.ipm/ipm.lock`, and validates the result with `claude plugin validate --strict`. A failure leaves the previous build and lock in place.

| Flag | Does |
| --- | --- |
| `-g` | Build your user scope's packs. |
| `--update` | Re-pin every item to the commit in your current catalog, instead of keeping the pins in `.ipm/ipm.lock`. |
| `--no-validate` | Skip `claude plugin validate --strict`. |

## Other

### `ipm doctor`

```text
ipm doctor
```

Checks the home and cache folders, pnpm, sources, catalog, Docker, the search server and `claude`. `✓` is fine, `!` is a warning, `✗` is a problem; any problem makes it exit 1.

### `ipm version`, `ipm help`

```text
ipm version              (or ipm --version)
ipm help [command]       (or ipm <command> --help)
```

### `ipm completion`

```text
ipm completion zsh|bash|fish|powershell
```

Prints a shell completion script for commands and flags. For zsh: `ipm completion zsh > "${fpath[1]}/_ipm"`, then start a new shell; `ipm completion <shell> --help` shows the steps for each shell.

## Exit codes

| Code | Means |
| --- | --- |
| `0` | Success |
| `1` | Failure: a bad source, pnpm missing or failed, unsynced sources, an item not found or not pickable, validation failed, or a `doctor` problem |
| `2` | A malformed command line |

## Environment

| Variable | Does |
| --- | --- |
| `IPM_HOME` | Where your sources, the pnpm store and user-scope packs live. Must be absolute. |
| `IPM_CACHE_DIR` | Where the catalog and search data live. Must be absolute. |
| `COLUMNS` | The width `ipm search` fits its table to when stdout isn't a terminal. |
| `XDG_DATA_HOME`, `XDG_CACHE_HOME` | Honoured on every OS when the variables above are unset. |

Defaults are in [Files and paths](files.md).
