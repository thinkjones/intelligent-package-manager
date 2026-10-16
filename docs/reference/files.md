---
title: Files and paths
parent: Reference
nav_order: 3
---

# Files and paths
{: .no_toc }

Where `ipm` keeps its data, and every file it reads or writes in a project.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Your data folders

`ipm` keeps its data in two folders of your own, never in the directory you run it from.

| Folder | Holds | macOS | Linux | Windows | Override |
| --- | --- | --- | --- | --- | --- |
| Home | Your sources, the pin store, the pnpm store, user-scope packs | `~/Library/ipm` | `$XDG_DATA_HOME/ipm`, else `~/.local/share/ipm` | `%LOCALAPPDATA%\ipm` | `IPM_HOME` |
| Cache | The catalog and search data | `~/Library/Caches/ipm` | `$XDG_CACHE_HOME/ipm`, else `~/.cache/ipm` | `%LOCALAPPDATA%\ipm\cache` | `IPM_CACHE_DIR` |

Overrides must be absolute paths. `XDG_DATA_HOME` and `XDG_CACHE_HOME` are honoured on every OS.

The home holds what you would lose: your list of sources and its pins, and every repo a pack build reads. The cache holds only what `ipm` can rebuild, so you can delete it at any time (`ipm catalog sync` and `ipm catalog rebuild` rebuild it).

```text
$IPM_HOME/
  sources/package.json        your sources (edit with ipm marketplace)
  sources/pnpm-lock.yaml      the commit each source is pinned to
  sources/node_modules/       each source as pnpm fetched it
  sources/.pnpmfile.cjs       pnpm hooks, rewritten by every sync
  pinned/                     the pin store: repos pack builds need at commits your sources aren't at
  pinned/package.json         one dependency per pin, <alias>--<sha12>; pins are added, never removed
  pinned/node_modules/        each pin as pnpm fetched it
  pinned/.lock                held while a build adds pins
  store/                      ipm's own pnpm content store
  global/                     user scope (-g): ipm.toml and .ipm/, as in a project
  installs.json               where user-scope packs are installed (ipm install -g)
$IPM_CACHE_DIR/
  catalog/*.jsonl             the catalog, one file per type
  catalog/github.jsonl        GitHub stats
  typesense/                  the search server's data and API key
  curated/                    the curated lists ipm onboard fetched, for offline use
```

## In a project

```text
<project>/
  ipm.toml                               your packs and their installs   commit
  .ipm/ipm.lock                          every item resolved to a commit commit
  .ipm/.gitignore                        ignores the last three          commit
  .ipm/.claude-plugin/marketplace.json   the marketplace                 commit
  .ipm/build.stamp                       what the last build used        ignored
  .ipm/plugins/<pack>/                   the built plugins               ignored
  .ipm/.gascity/<name>/                  Gas City pack folders           ignored
```

The project is the nearest folder, at or above where you run `ipm`, that holds an `ipm.toml`. It holds no pnpm project: `ipm pack build` reads each repo from your home — the synced source when it is at the pinned commit, else `$IPM_HOME/pinned` — and removes the `package.json`, `pnpm-lock.yaml`, `.pnpmfile.cjs` and `node_modules/` older builds wrote to `.ipm/`. If you committed those, commit their removal. Everything under `.ipm/` is generated: `.ipm/.gascity/` by `ipm install gascity` (which `ipm pack build` leaves alone), the rest by `ipm pack build`; don't edit it by hand.

## `ipm.toml`

```toml
# ipm packs: each becomes a Claude Code plugin under .ipm/plugins/ when you run `ipm pack build`.

[packs.csharp-sdlc]
description = "C# development lifecycle"
items = [
  "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns",
  "plugin:anthropics/claude-plugins-official#stripe",
]
```

One `[packs.<name>]` table per pack, with a `description` and a list of catalog IDs. A name that isn't a bare TOML key (one with a `.`) is quoted: `[packs."my.pack"]`. `ipm pack new` without `--description` writes `ipm pack <name>`, and `ipm pack build` uses that for a pack whose description is empty. `ipm pack` writes it with packs sorted by name and items in the order you added them. Unknown keys are an error.

`ipm install claude` adds a `[[packs.<name>.installs]]` table for each place a pack is installed, so a bare `ipm install` can replay them:

```toml
[[packs.csharp-sdlc.installs]]
target = "claude"
path = "."
scope = "project"
```

`target` is `claude` or `gascity`; for `claude`, `scope` is `user`, `project` or `local`; `path` is the folder for project or local scope, stored relative to the project when it is inside it. Installing the same pack to the same target, path and scope again replaces its table; project and local scope at one folder are two tables. A user-scope pack's installs are kept in `$IPM_HOME/installs.json` instead: a JSON array of `{pack, target, path, scope, name}` objects sorted by pack, target, path and scope, with absolute paths.

`ipm install gascity` records the folder it wrote into and the pack folder's name, with no scope:

```toml
[[packs.csharp-sdlc.installs]]
target = "gascity"
path = ".ipm/.gascity"
name = "csharp-sdlc"
```

The pack folder is `<path>/<name>`. A `gascity` record in `installs.json` has the same `target`, `path` and `name`, with an absolute `path`: the project's `.ipm/.gascity` by default, since `ipm install gascity -g` writes user-scope packs into the project it runs in. Installing the same pack to the same folder and name again replaces its record.

## `.ipm/ipm.lock`

```toml
# Generated by `ipm pack build` — commit it. Every picked item, resolved to a repo, path and commit.

[[item]]
id = "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns"
type = "skill"
name = "dotnet-backend-patterns"
repo = "wshobson/agents"
path = "plugins/dotnet-contribution/skills/dotnet-backend-patterns"
sha = "…40 hex characters…"
```

One `[[item]]` per picked item, sorted by ID. Whole plugins defined in a marketplace (IDs with `#`) also carry `source`: their upstream marketplace entry, as JSON. A Copilot plugin's `source` lists its skills' IDs. A whole plugin also carries `parts`: the IDs of the skills, agents, commands, hooks and MCP servers it holds (an empty `parts = []` means the plugin holds none), which `ipm install gascity` reads to write them; `ipm pack build --update` records them for a lock written before ipm did.

## `.ipm/build.stamp`

One line: the hex SHA-256 of the packs in `ipm.toml` (name, description and items; install records are left out), the `.ipm/ipm.lock` that the build wrote and the ipm version. `ipm pack build` writes it last. `ipm install claude` recomputes it and skips the build while it matches and `.ipm/plugins/` and the marketplace file exist. Generated and git-ignored: it describes the ignored `.ipm/plugins/`, and because it hashes the ipm version it would change between teammates.

## The built plugin

```text
.ipm/plugins/<pack>/
  .claude-plugin/plugin.json
  skills/<name>/…
  agents/<name>.md
  commands/<name>.md
```

Files are named after the item, lowercased, with anything other than `a-z 0-9 . _ -` turned into `-` ("Go Expert" becomes `go-expert`). Two items with the same name in one pack get `-<repo owner>` appended, then `-2`, `-3`.

`plugin.json` records where every file came from under `metadata.ipm.items`:

```json
{
  "name": "csharp-sdlc",
  "description": "C# development lifecycle",
  "version": "0.0.0-ipm",
  "author": { "name": "Your Name" },
  "dependencies": ["csharp-sdlc-stripe"],
  "metadata": {
    "ipm": {
      "items": [
        {
          "file": "skills/dotnet-backend-patterns",
          "id": "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns",
          "name": "dotnet-backend-patterns",
          "path": "plugins/dotnet-contribution/skills/dotnet-backend-patterns",
          "repo": "wshobson/agents",
          "sha": "…",
          "type": "skill"
        }
      ]
    }
  }
}
```

The author is your `git config user.name`, or `ipm` if it isn't set.

## The Gas City pack folder

`ipm install gascity` writes this folder in `.ipm/.gascity/`, or wherever you point it with `-C`:

```text
<dir>/<name>/
  pack.toml
  skills/<name>/…
  agents/<name>/prompt.md
  agents/<name>/agent.toml
  mcp/<name>.toml
```

`pack.toml` starts with the line `# generated by ipm — rewritten by ipm install gascity; do not edit`, then one `# <item id> @ <commit>` line per picked item (`@ -` when the item has no commit), then the `[pack]` table: `name`, `schema = 2`, `version = "0.0.0-ipm"` and, when the pack has one, `description`. That first line is how ipm knows it wrote the folder; delete it and ipm refuses to replace the folder without `--force`. Everything in it is rewritten on every `ipm install gascity`: don't edit it, change the pack or the source instead.

An `agent.toml` holds `description` when the agent has one, `provider = "claude"` and, when the agent's frontmatter sets a `model` other than `inherit`, `[option_defaults]` with `model`. An `mcp/<name>.toml` holds `name` (the file's name), `description`, and one of `command` (with `args` and `env`) or `url` (with `headers`).

## The marketplace

`.ipm/.claude-plugin/marketplace.json` is named `<project folder>-ipm` (user scope: `ipm-global`). It lists:

- each pack, with source `./plugins/<pack>`;
- each whole plugin a pack depends on, named `<pack>-<plugin name>` and pinned to its commit. A plugin with its own folder is a `git-subdir` source at the locked commit; one defined in a marketplace entry keeps that entry, renamed.

Every entry is tagged `ipm` and with its pack's name.
