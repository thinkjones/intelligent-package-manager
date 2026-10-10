---
title: Concepts
nav_order: 3
---

# Concepts
{: .no_toc }

The handful of ideas `ipm` is built on.
{: .fs-6 .fw-300 }

![How ipm works: source add lists the repos you follow; sync has pnpm fetch and pin each repo and ipm catalogs it; search finds artifacts on your machine; pack add picks items; install builds one validated plugin per pack.](assets/brand/workflow.svg)

1. TOC
{:toc}

## Sources

A **source** is a repo of interest: a GitHub `owner/repo`, or any git URL, optionally at a tag, branch or commit. Your sources are personal: one list per user, kept in your ipm home and usable from any project.

Under the hood your sources are a single pnpm project with one git dependency per repo. `ipm source add` edits it; `ipm sync` has pnpm fetch every repo and pin it to a commit in `pnpm-lock.yaml`. Later syncs keep those commits until you run `ipm sync --upgrade`. See the [sources guide](guides/sources.md).

## The catalog

`ipm sync` ends by building the **catalog**: one record for every artifact in your sources, with the commit it came from and links to what owns it.

| Type | What it is |
| --- | --- |
| `repo` | A source, or a repo an external plugin lives in |
| `marketplace` | A `.claude-plugin/marketplace.json` |
| `plugin` | A Claude Code plugin: local (its own folder), inline (defined in a marketplace entry) or external (in another repo) |
| `copilot-plugin` | A GitHub Copilot plugin |
| `skill` | A folder with a `SKILL.md` |
| `agent` | An agent Markdown file |
| `command` | A slash command, in its own file or inline |
| `hook` | A `hooks.json` hook |
| `mcp` | An MCP server |

The catalog is rebuilt from the pinned commits, so the same pins always give the same catalog. It lives in your cache folder and can be deleted at any time; `ipm index` rebuilds it.

## IDs

Every artifact has an ID:

```text
<type>:<repo>:<path in repo>          skill:obra/superpowers:skills/test-driven-development
<type>:<repo>:<path>#<name>           for items defined inside a file, such as an inline plugin
plugin:<marketplace repo>#<name>      an external plugin, listed by a marketplace in another repo
```

IDs carry no commit, so they survive syncs and upgrades. `ipm search` prints them; `ipm show`, `tree`, `graph` and `pack add` take them. Quote IDs containing `#` in your shell.

## Search

`ipm serve` runs [Typesense](https://typesense.org) in Docker on `127.0.0.1:8108`, and `ipm load` indexes the catalog into it. `ipm search` combines keyword matching on names, descriptions and tags with semantic matching on an embedding Typesense computes locally, so "c# testing" finds a ".NET test patterns" skill. Nothing leaves your machine. See [Search and explore](guides/search.md).

## Packs

A **pack** is a named set of picked items in an `ipm.toml`:

```toml
[packs.csharp-sdlc]
description = "C# development lifecycle"
items = [
  "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns",
  "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md",
  "plugin:anthropics/claude-plugins-official#stripe",
]
```

`ipm install` builds every pack into a Claude Code plugin of the same name:

- **Skills, agents and file-based commands** are placed in the pack's plugin, so they load as `<pack>:<item>`.
- **Copilot plugins** contribute their skills.
- **Whole plugins** become dependencies of the pack. They keep their own name, hooks and MCP servers, and Claude Code installs them with the pack.
- **Hooks, MCP servers and inline commands** can't be picked on their own; they come with their plugin. `ipm pack add` tells you which plugin to add instead.

## Scopes

Packs live in a **scope**:

- **Project scope** (the default): the nearest folder at or above where you run `ipm` that holds an `ipm.toml`. `ipm pack new` creates one in the current folder if there is none. The built plugins go in that folder's `.ipm/`.
- **User scope** (`-g`): `$IPM_HOME/global`, for packs you want in every project. Its marketplace is called `ipm-global`.

## Pins and the lock

`ipm install` resolves every picked item to a repo, path and commit and records it in `.ipm/ipm.lock`. Later installs reuse those pins, so the plugin doesn't change under you when your sources move. `ipm install --update` re-pins everything to the commits in your current catalog.

Because the lock and pnpm's pins are committed, anyone with `ipm` can rebuild the same plugin from your project without your sources or catalog. See [Share packs with a team](guides/teams.md).

## What ipm never does

- **Run upstream code.** pnpm runs with `--ignore-scripts`, and sources' npm dependencies are never installed.
- **Modify upstream repos.** Picked files are hard-linked (or copied) from pnpm's fetched copy.
- **Install into Claude Code on its own.** Building writes only `.ipm/`; `ipm deploy` (or `D` in the TUI) installs a pack, at the scope you choose, only when you ask.
