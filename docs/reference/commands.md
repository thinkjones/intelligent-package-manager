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

## Setup

### `ipm onboard`

```text
ipm onboard [--list <path|url>] [--lists <name>]… [--keep|--fresh] [--yes]
```

Takes you from nothing to a searchable library: picks sources from a curated list, adds them, then runs `ipm sync`, `ipm stats` and `ipm serve` with `ipm load`, printing `✓` or `✗` for each step. Stats are skipped when `gh` is missing or not logged in; search is skipped when Docker isn't running (`start Docker, then ipm serve && ipm load`). A failed step prints `rerun: <command>` and exits 1. It ends with `✓ library ready — N sources, M artifacts` and `try: ipm search … · ipm tui`.

If `ipm` already has sources or a catalog, it prints, for example, `found existing ipm data: 26 sources · 3,280 artifacts · synced 2026-10-09` and asks `[u]se it · [b]ack it up and start fresh · [q]uit`. Backing up renames the home and cache folders to `<folder>.backup-YYYYMMDD-HHMM` and prints how to undo it: `to restore: rm -rf <home> <cache> && mv <home>.backup-… <home> && mv <cache>.backup-… <cache> && ipm serve`. If you then quit the checklist, it says `no sources added; your data was backed up to …` and prints the same line. With sources you already follow, `enter` with nothing new ticked continues with those (`0 new repos — y continue with the N you follow`), so rerunning `ipm onboard` after a failed step resumes it. While filtering, space on a list ticks only the repos shown.

The curated list is fetched each time and cached under the cache folder's `curated/`; offline, the cached copy is used. In a terminal it shows a checklist of the lists, which expand into their repos; see [Getting started](../getting-started.md#onboard) for the keys. The line above the key hints shows the highlighted repo's topics and homepage. On a fresh start "AI · Agentic Engineering Methods" and "AI · Skills, Plugins & Registries" are ticked; repos you already follow show `✓` and nothing is ticked. Rerunning `ipm onboard` adds only what is new.

| Flag | Does |
| --- | --- |
| `--list L` | The curated list: a URL or a file; a GitHub `blob/` link is read as the raw file. Default `https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/source-packs.toml`. The format is in [Curated lists](../guides/sources.md#curated-lists). |
| `--lists NAME` | Follow every repo in this list, by exact name (repeatable). Skips the checklist. |
| `--yes` | Follow the pre-ticked lists without showing the checklist. |
| `--keep` | Use existing ipm data without asking. |
| `--fresh` | Back up existing ipm data and start fresh without asking. |

When not in a terminal, `--lists` or `--yes` is required, and so is `--keep` or `--fresh` if data exists. `--keep` and `--fresh` together are a usage error.

It then runs five steps: add the sources, `ipm sync`, `ipm stats`, `ipm serve` and `ipm load`. In a terminal, one line is redrawn while they run:

```text
[████████░░░░░░░░░░░░] 3/5 stats · 17/42 anthropics/skills · 0:48
```

The bar covers all five steps and moves inside `stats` (per repo) and `load` (per 500 documents); after the step comes the last line it printed or its count, then the time since the steps began. The steps' own output is not shown. Each finished step leaves one line, such as `✓ sync · catalog: 3280 record(s), 2 warning(s)`; run `ipm index` to read the warnings. If a step fails, everything it printed is shown, then `✗ <step>` and `rerun: <command>`. When stdout is not a terminal there is no bar and each step prints what it prints on its own.

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
| `list` | Every source under the list it belongs to in the curated list, each with its alias, repo, stars, ref and pinned commit (`not synced` until `ipm sync`), then its description, topics and homepage. See [List and remove](../guides/sources.md#list-and-remove). |
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

An artifact's fields, its repo's GitHub stats, and its neighbours: `up:` (what contains it, within its repo), `down:` (what it contains) and `used by:` (external entries in other repos that reference it — for a repo, anything in it). Reads the catalog; no search server needed.

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

### `ipm tui`

```text
ipm tui [-g]
```

Search, see each artifact in its tree, collect into packs, build and deploy, in one screen. Uses the search server when it is running and indexed, and keyword search over the catalog otherwise. Needs a terminal and a catalog. See [Use the TUI](../guides/tui.md) for the keys.

| Flag | Does |
| --- | --- |
| `-g`, `--global` | Use your user scope's packs (`$IPM_HOME/global`). |

### `ipm deploy`

```text
ipm deploy [pack] [--scope user|project|local] [--path DIR] [-g]
```

Builds the scope's packs (as `ipm install`), then installs `pack` into Claude Code. `pack` may be left out when the scope has only one. It prints each command before running it and needs no confirmation: typing the command is the decision.

| Flag | Does |
| --- | --- |
| `--scope S` | `user` (every project, just you), `project` (a folder, shared through its `.claude/settings.json`) or `local` (a folder, just you). Default `project`; `user` with `-g`. |
| `--path DIR` | The folder for project or local scope. Default: the pack's project. `~` and relative paths work. |
| `-g`, `--global` | Deploy a pack from your user scope. |

The commands it runs: `claude plugin marketplace add <project>/.ipm --scope S` (always — Claude Code accepts a repeat), `claude plugin marketplace update <name>` when Claude Code already knows the marketplace, then `claude plugin install <pack>@<name> --scope S`, or `claude plugin update …` when the pack is already installed there. Needs `claude` on your `PATH`.

## Other

### `ipm doctor`

```text
ipm doctor
```

Checks the home and cache folders, pnpm, sources, catalog, Docker, the search server and `claude`. `✓` is fine, `!` is a warning, `✗` is a problem; any problem makes it exit 1.

### `ipm uninstall`

```text
ipm uninstall [--purge] [--yes] [--force]
```

Removes the `ipm-typesense` container (when Docker is available) and the running `ipm` binary. Your data is kept, and it says where: `ipm onboard` will offer to use it or back it up. A development build (a binary in a `bin/` folder inside an `ipm` checkout) is refused without `--force`.

| Flag | Does |
| --- | --- |
| `--purge` | Also delete ipm's home and cache. You type the home path to confirm. |
| `--yes` | With `--purge`, delete without asking. Required with `--purge` when not in a terminal. |
| `--force` | Remove a development build in a git checkout too. |

### `ipm version`, `ipm help`

```text
ipm version              (or ipm --version)
ipm help [command]       (or ipm <command> --help)
```

`ipm version` prints the release `ipm` was installed from, for example `ipm 0.2.0`. A build of a checkout (`./install.sh` in a clone, or `bin/ipm`) prints `ipm dev`.

### `ipm completion`

```text
ipm completion zsh|bash|fish|powershell
```

Prints a shell completion script for commands and flags. For zsh: `ipm completion zsh > "${fpath[1]}/_ipm"`, then start a new shell; `ipm completion <shell> --help` shows the steps for each shell.

## Exit codes

| Code | Means |
| --- | --- |
| `0` | Success |
| `1` | Failure: a bad source, pnpm missing or failed, unsynced sources, an item not found or not pickable, validation failed, a failed `onboard` step, or a `doctor` problem |
| `2` | A malformed command line |

## Environment

| Variable | Does |
| --- | --- |
| `IPM_HOME` | Where your sources, the pnpm store and user-scope packs live. Must be absolute. |
| `IPM_CACHE_DIR` | Where the catalog and search data live. Must be absolute. |
| `COLUMNS` | The width `ipm search` fits its table to when stdout isn't a terminal. |
| `XDG_DATA_HOME`, `XDG_CACHE_HOME` | Honoured on every OS when the variables above are unset. |

Defaults are in [Files and paths](files.md).
