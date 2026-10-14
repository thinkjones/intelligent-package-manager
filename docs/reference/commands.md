---
title: Commands
parent: Reference
nav_order: 1
---

# Commands
{: .no_toc }

Every `ipm` command and flag, in the order of the workflow: onboard → marketplace → catalog → search → pack → install. `ipm help <command>` prints the same from the CLI.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

Flags may come before or after positional arguments. Long flags take two dashes (`--update`); `-g` is short for `--global`. `ipm <command> --help` and `ipm help <command>` print a command's usage and flags, and a mistyped command gets a suggestion.

## Getting started

### `ipm onboard`

```text
ipm onboard [--list <path|url>] [--lists <name>]… [--keep|--fresh] [--yes]
```

Takes you from nothing to a searchable library. It first prints the four concepts, one line each (marketplace, catalog, pack, install), then picks sources from the marketplace's curated list, adds them, and runs `ipm catalog sync`, `ipm marketplace stats` and `ipm catalog serve` with `ipm catalog rebuild`, printing `✓` or `✗` for each step. Stats are skipped when `gh` is missing or not logged in; search is skipped when Docker isn't running (`start Docker, then ipm catalog serve && ipm catalog rebuild`). A failed step prints `rerun: <command>` and exits 1. It ends with `✓ library ready — N sources, M artifacts` and `try: ipm search … · ipm tui`.

If `ipm` already has sources or a catalog, it prints, for example, `found existing ipm data: 26 sources · 3,280 artifacts · synced 2026-10-09` and asks `[u]se it · [b]ack it up and start fresh · [q]uit`. Backing up renames the home and cache folders to `<folder>.backup-YYYYMMDD-HHMM` and prints how to undo it: `to restore: rm -rf <home> <cache> && mv <home>.backup-… <home> && mv <cache>.backup-… <cache> && ipm catalog serve`. If you then quit the checklist, it says `no sources added; your data was backed up to …` and prints the same line. With sources you already follow, `enter` with nothing new ticked continues with those (`0 new repos — y continue with the N you follow`), so rerunning `ipm onboard` after a failed step resumes it. While filtering, space on a list ticks only the repos shown.

The curated list is fetched each time and cached under the cache folder's `curated/`; offline, the cached copy is used. In a terminal it shows a checklist of the lists, which expand into their repos; see [Getting started](../getting-started.md#onboard) for the keys. The line above the key hints shows the highlighted repo's topics and homepage. On a fresh start "AI · Agentic Engineering Methods" and "AI · Skills, Plugins & Registries" are ticked; repos you already follow show `✓` and nothing is ticked. Rerunning `ipm onboard` adds only what is new.

| Flag | Does |
| --- | --- |
| `--list L` | The curated list: a URL or a file; a GitHub `blob/` link is read as the raw file. Default `https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/source-packs.toml`. The format is in [Curated lists](../guides/sources.md#curated-lists). |
| `--lists NAME` | Follow every repo in this list, by exact name (repeatable). Skips the checklist. |
| `--yes` | Follow the pre-ticked lists without showing the checklist. |
| `--keep` | Use existing ipm data without asking. |
| `--fresh` | Back up existing ipm data and start fresh without asking. |

When not in a terminal, `--lists` or `--yes` is required, and so is `--keep` or `--fresh` if data exists. `--keep` and `--fresh` together are a usage error.

It then runs five steps: add the sources, `ipm catalog sync`, `ipm marketplace stats`, `ipm catalog serve` and `ipm catalog rebuild`'s load. In a terminal, one line is redrawn while they run:

```text
[████████░░░░░░░░░░░░] 3/5 stats · 17/42 anthropics/skills · 0:48
```

The bar covers all five steps and moves inside `stats` (per repo) and `load` (per 500 documents); after the step comes the last line it printed or its count, then the time since the steps began. The steps' own output is not shown. Each finished step leaves one line, such as `✓ sync · catalog: 3280 record(s), 2 warning(s)`; run `ipm catalog rebuild` to read the warnings. If a step fails, everything it printed is shown, then `✗ <step>` and `rerun: <command>`. When stdout is not a terminal there is no bar and each step prints what it prints on its own.

## Marketplace

The marketplace is the curated list of repos worth following (by default this repo's `source-packs.toml`). Following a repo makes it a source: `ipm` fetches, pins and catalogs it.

### `ipm marketplace`

```text
ipm marketplace [--list <path|url>] [--lists <name>]… [--followed]
ipm marketplace add <repo>…
ipm marketplace remove <repo>…
ipm marketplace import <path/to/manifest.toml>
ipm marketplace stats
```

Run alone, it shows the curated repos grouped by list, each with a `✓` when you follow it, its stars and its description, then the repos you follow that no list holds under `Other`, and ends with `✓ follows N of the repos shown`. The curated list is fetched each time and cached under the cache folder's `curated/`; offline, the cached copy is used.

| Flag | Does |
| --- | --- |
| `--list L` | The curated list: a URL or a file. Default `https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/source-packs.toml`. |
| `--lists NAME` | Show only this list, by exact name (repeatable). |
| `--followed` | Show only the repos you follow, grouped by list, each with its alias, repo, stars, ref and pinned commit (`not synced` until `ipm catalog sync`), then its description, topics and homepage. See [List and remove](../guides/sources.md#list-and-remove). |

| Subcommand | Does |
| --- | --- |
| `add` | Follow repos: `owner/repo[@ref]`, a git URL (`https://`, `ssh://`, `file://`, `git@`) with an optional `#ref`, or an external plugin ID (`plugin:<marketplace repo>#<name>`), which follows the repo the plugin lives in. A repo already followed, or two repos that would share an alias, is refused and nothing is written. |
| `remove` | Stop following repos. An unknown repo is an error and nothing is written. |
| `import` | One-time migration from an aihs-era `manifest.toml`. |
| `stats` | Fetch GitHub stars, forks, topics, licence, description and last push for every repo in the catalog into `github.jsonl`. Needs `gh`, logged in. |

`add`, `remove` and `import` only edit the source list; run `ipm catalog sync` after.

## Catalog

### `ipm catalog`

```text
ipm catalog
ipm catalog sync [--upgrade]
ipm catalog rebuild
ipm catalog serve [--stop]
ipm catalog scan [--paths] [owner/repo…]
ipm catalog search [flags] <query>
```

Run alone, it prints a summary: how many sources you follow and how many are synced, the items by type, when the catalog was last built, and the search server's state (`running, N document(s)`, or how to set it up).

```text
sources     0 followed, 0 synced
items       none
last index  never — run `ipm catalog sync`
search      no search server set up — run `ipm catalog serve`
```

### `ipm catalog sync`

```text
ipm catalog sync [--upgrade]
```

Has pnpm fetch and pin every source, then rebuilds the catalog and, when the search server is up, reloads the search index. With the server off it prints `- search not loaded: run ipm catalog serve then ipm catalog rebuild`. If pnpm fails, the catalog is unchanged and `ipm` exits 1.

| Flag | Does |
| --- | --- |
| `--upgrade` | Move every source to the latest commit of its branch or tag first. |

### `ipm catalog rebuild`

```text
ipm catalog rebuild
```

Rebuilds the catalog from your synced sources, without fetching, then reloads the search index when the server is up (the same hint otherwise). Use it after `ipm marketplace stats` to refresh the stars in search.

### `ipm catalog serve`

```text
ipm catalog serve [--stop]
```

Starts Typesense in Docker as container `ipm-typesense` on `127.0.0.1:8108`, with its data in your cache folder.

| Flag | Does |
| --- | --- |
| `--stop` | Stop the server. Its data is kept. |

### `ipm catalog scan`

```text
ipm catalog scan [--paths] [owner/repo…]
```

Reports each synced source's commit and its plugins, Copilot plugins, marketplaces, and `skills`, `hooks` and `agents` folders.

| Flag | Does |
| --- | --- |
| `--paths` | List every folder found, not only the counts. |

## Search

### `ipm search`

```text
ipm search [flags] <query>
ipm catalog search [flags] <query>
```

Hybrid keyword and semantic search; the two spellings are the same command. Needs `ipm catalog serve` and a loaded index (`ipm catalog sync` and `ipm catalog rebuild` load it). Each hit's full ID is printed on the line under it. The table fits the terminal width (or `COLUMNS`), dropping DESCRIPTION, then PLUGINS, then REPO when narrow; piped output isn't cut.

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
ipm show tree [flags] <id>
ipm show graph [flags] <id>
```

An artifact's fields, its repo's GitHub stats, and its neighbours: `up:` (what contains it, within its repo), `down:` (what it contains) and `used by:` (external entries in other repos that reference it — for a repo, anything in it). Reads the catalog; no search server needed.

| Flag | Does |
| --- | --- |
| `--json` | Print the catalog record as JSON. |

### `ipm show tree`

```text
ipm show tree [flags] <id>
```

An artifact's relationships as a tree. Reads the catalog.

| Flag | Does |
| --- | --- |
| `--up` | Walk to what contains the artifact, instead of what it contains. |
| `--depth N` | Levels to show. Default: 3 from a repo, otherwise all. |
| `--all` | Show every child, not the first 25. |

### `ipm show graph`

```text
ipm show graph [flags] <id>
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
ipm pack build [-g] [--update] [--no-validate]
ipm pack install claude|gascity [pack] …      (same as ipm install claude|gascity)
```

| Subcommand | Does |
| --- | --- |
| `new` | Create a pack. Creates `ipm.toml` in the current folder if no folder above has one. |
| `add` | Add catalog items. Every ID is checked first; if any is unknown or can't be picked, nothing is added. Duplicates are ignored. |
| `remove` | Remove items. |
| `list` | Every pack in the scope: name, item count, description. |
| `show` | A pack's items: type, name, repo, stars and ID, then one `installed:` line for each place the pack is installed. |
| `build` | Build the scope's packs into plugins; see [`ipm pack build`](#ipm-pack-build). |
| `install` | The same as [`ipm install`](#ipm-install). |

| Flag | Does |
| --- | --- |
| `--description D` | The pack's description (`new`). |
| `-g` | Use your user scope (`$IPM_HOME/global`) instead of the project. |

### `ipm pack build`

```text
ipm pack build [-g] [--update] [--no-validate]
```

Builds every pack in the scope into a Claude Code plugin under `.ipm/plugins/`, fetches the repos of any whole plugins you picked (so `ipm install gascity` can read their parts), writes `.ipm/.claude-plugin/marketplace.json` and `.ipm/ipm.lock` (which records each whole plugin's `parts`), and validates the result with `claude plugin validate --strict`. Last, it writes `.ipm/build.stamp`, a hash of the packs, the lock and the ipm version, which lets `ipm install` skip a build that is already current. A failure leaves the previous build and lock in place. It only builds: it never touches your Claude Code configuration.

| Flag | Does |
| --- | --- |
| `-g` | Build your user scope's packs. |
| `--update` | Re-pin every item to the commit in your current catalog, instead of keeping the pins in `.ipm/ipm.lock`. |
| `--no-validate` | Skip `claude plugin validate --strict`. |

### `ipm install`

```text
ipm install claude [pack] [--scope user|project|local] [--path DIR] [-g] [--update] [--dry-run]
ipm install gascity [pack] [-C DIR] [--name NAME] [-g] [--force] [--update] [--dry-run]
ipm install [-g]
```

#### `ipm install claude`

`ipm install claude` installs `pack` into Claude Code. `pack` may be left out when the scope has only one. It builds first when `.ipm/` is out of date (the build stamp no longer matches), printing `build is current` when it is not needed, then prints each `claude` command before running it and needs no confirmation: typing the command is the decision. Afterwards it records the install: in the pack's `[[packs.<name>.installs]]` table in `ipm.toml` for a project pack (a path inside the project is stored relative to it), or in `$IPM_HOME/installs.json` for a user-scope pack. Installing the same pack to the same target, path and scope again replaces its record; project and local scope at one folder are two records. `ipm pack install claude` is the same command.

| Flag | Does |
| --- | --- |
| `-s`, `--scope S` | `user` (every project, just you), `project` (a folder, shared through its `.claude/settings.json`) or `local` (a folder, just you). Default `project`; `user` with `-g`. |
| `-p`, `--path DIR` | The folder for project or local scope. Default: the pack's project. `~` and relative paths work. |
| `-g`, `--global` | Install a pack from your user scope. |
| `--update` | Re-pin every item to your current catalog and rebuild first. |
| `--dry-run` | Print the `claude` commands without building, running or recording anything. Can't be combined with `--update` (usage error, exit 2: `install claude: --dry-run and --update can't be used together`). |

The commands it runs: `claude plugin marketplace add <project>/.ipm --scope S` (always — Claude Code accepts a repeat), `claude plugin marketplace update <name>` when Claude Code already knows the marketplace, then `claude plugin install <pack>@<name> --scope S`, or `claude plugin update …` when the pack is already installed there. Needs `claude` on your `PATH`.

#### `ipm install gascity`

`ipm install gascity` writes `pack` as a [Gas City](https://github.com/gastownhall/gascity) pack folder, `<DIR>/<NAME>`, that a city can import. `pack` may be left out when the scope has only one. Like `install claude` it builds first unless the build is current. It reads no Gas City config and edits no city: it writes the folder, records the install and prints how to import it. `ipm pack install gascity` is the same command.

| Flag | Does |
| --- | --- |
| `-C`, `--dir DIR` | The folder to write the pack folder into; it must exist. Default: the working directory. `~` and relative paths work. |
| `--name NAME` | The pack folder's name. Default: the pack's name. It must start with a letter or digit and use only letters, digits, `_` and `-`, else a usage error (exit 2). |
| `-g`, `--global` | Install a pack from your user scope. |
| `--force` | Replace a folder that ipm did not write. |
| `--update` | Re-pin every item to your current catalog and rebuild first. |
| `--dry-run` | List the files that would be written, without building, writing or recording anything. Needs a current build: `--dry-run needs a current build — run ipm pack build first`. If the folder would be refused, it prints `would refuse: <message>` and exits 1. Can't be combined with `--update` (usage error, exit 2: `install gascity: --dry-run and --update can't be used together`). |

The folder holds `pack.toml`, `skills/<name>/` (each skill folder copied whole), `agents/<name>/prompt.md` with `agents/<name>/agent.toml`, and `mcp/<name>.toml`. Files are copied, not hard-linked, and two runs give byte-identical folders. Names are lowercased. For skills and agents `_` is kept and other characters become `-`; MCP server names also turn `_` into `-` (they use only `a-z 0-9 -`). Clashes within a kind get `-<owner>`, then `-2`, `-3`. An agent's prompt is its body without the frontmatter, as plain markdown (Gas City does not template it).

Gas City has no equivalent for some items, so ipm skips them and prints each as a warning:

| Message | Meaning |
| --- | --- |
| `skipped <id>: Gas City has no slash commands` | Commands, from a pack or from a whole plugin, are not written. |
| `skipped <id>: Gas City packs have no hooks` | A plugin's hooks are not written. |
| `skipped <id>: external plugin — its files live in another repo; add its skills and agents directly` | A plugin whose files are in another repo. Add its skills and agents to the pack instead. |
| `skipped <id>: its parts aren't recorded — run ipm pack build --update` | A whole plugin that the lock has no parts for; `ipm pack build --update` records them. |
| `skipped <id>: nothing Gas City can hold` | A whole plugin with no recorded parts (`parts = []` in `ipm.lock`). A plugin whose parts are all commands or hooks gets one skip line per part instead. |
| `agents/<name>: frontmatter is not valid YAML — wrote the body only` | The agent's frontmatter doesn't parse. ipm still writes `prompt.md` with the body and an `agent.toml` holding only `provider = "claude"`. |
| `agents/<name>: dropped tools, color — Gas City agents have no equivalent` | Agent frontmatter keys other than `name`, `description` and `model` are dropped, one warning per agent. |
| `mcp/<name>: dropped <keys>` | MCP server keys Gas City doesn't know (Claude's `type` is dropped silently). |
| `mcp/<name>: uses ${CLAUDE_PLUGIN_ROOT}, which Gas City does not set` | The server's command, arguments, environment or URL use Claude Code's plugin path, which won't resolve in a city. |

ipm writes into `<DIR>/.<NAME>.ipm-tmp` and swaps the result in, replacing a folder it wrote earlier completely, so items removed from a pack leave no files. A folder that exists, isn't empty and wasn't written by ipm (its `pack.toml` lacks ipm's header line) is refused with exit 1: `<path> exists and was not written by ipm — use --force to replace it`. The check runs before any build. Even with `--force`, ipm refuses a folder that is, or holds, the project (or `$IPM_HOME/global` with `-g`) or `$IPM_HOME`: `refusing to replace <path>: it holds the project` or `… it holds ipm's home`.

Afterwards it records the install (`target = "gascity"`, `path`, `name`), as for Claude Code, and prints:

```text
Import it into a city's pack.toml or city.toml:
  [imports.<name>]
  source = "<folder>"
```

#### Replay recorded installs

Bare `ipm install` replays every install recorded for the scope (`-g` for your user scope). With none recorded it is a usage error that names the targets. A target other than `claude` or `gascity` is a usage error. Before running anything it checks every record's target: a record of a target this ipm doesn't know is reported as `<pack> → <target> …: unknown target`, and nothing is installed. A record that fails (a stale one whose folder was deleted, say) doesn't stop the others: its error is printed as `✗ <pack> → <target> <scope or name> <path>: <error>`, and the replay ends with exit 1 and `✗ N of M recorded installs failed; if a record is stale, remove it from <file>`, where the file is `ipm.toml` for a project or `$IPM_HOME/installs.json` with `-g`, each with its path. There is no command to forget a record: delete it from that file.

## Tools

### `ipm tui`

```text
ipm tui [-g]
```

Search, see each artifact in its tree, collect into packs, build and deploy, in one screen. Uses the search server when it is running and indexed, and keyword search over the catalog otherwise. Needs a terminal and a catalog. See [Use the TUI](../guides/tui.md) for the keys.

| Flag | Does |
| --- | --- |
| `-g`, `--global` | Use your user scope's packs (`$IPM_HOME/global`). |

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

## Renamed commands

The old command names are removed. Typing one is a usage error (exit 2) that names its replacement.

| Old | New |
| --- | --- |
| `ipm source add`, `remove`, `import` | `ipm marketplace add`, `remove`, `import` |
| `ipm source list` | `ipm marketplace` (the curated list with `✓`), or `ipm marketplace --followed` (your sources) |
| `ipm stats` | `ipm marketplace stats` |
| `ipm sync [--upgrade]` | `ipm catalog sync [--upgrade]` |
| `ipm index`, `ipm load` | `ipm catalog rebuild` |
| `ipm serve [--stop]` | `ipm catalog serve [--stop]` |
| `ipm scan` | `ipm catalog scan` |
| `ipm tree`, `ipm graph` | `ipm show tree`, `ipm show graph` |
| `ipm install` (the build) | `ipm pack build` |
| `ipm deploy` | `ipm install claude` |

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
