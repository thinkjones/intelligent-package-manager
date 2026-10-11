---
title: Add and sync sources
parent: Guides
nav_order: 1
---

# Add and sync sources
{: .no_toc }

Choose the repos `ipm` follows, fetch and pin them, and keep them current.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Add sources

```bash
ipm source add obra/superpowers                  # a GitHub repo, default branch
ipm source add anthropics/skills@main            # at a branch, tag or commit
ipm source add https://gitlab.com/me/skills.git#v1
ipm source add 'plugin:anthropics/claude-plugins-official#stripe'   # the repo an external plugin lives in
```

A source is `owner/repo[@ref]` for GitHub, or a git URL (`https://…`, `ssh://…`, `file://…`, `git@…`) with an optional `#ref`. Without a ref, a source follows the repo's default branch.

An external plugin ID adds the repo that plugin lives in. It only works once the plugin is in your catalog, because some other source's marketplace lists it; find such plugins with `ipm search --type plugin`.

`ipm source add`, `remove` and `import` only edit your source list. Run `ipm sync` afterwards.

## Curated lists

`ipm onboard` offers repos from a curated list: a TOML file that groups GitHub repos into named lists. The default is [thinkjones/agentic-tools](https://github.com/thinkjones/agentic-tools)'s [`starred_repos.toml`](https://github.com/thinkjones/agentic-tools/blob/main/starred_repos.toml). Anyone can publish one; pass it as a URL or a file (a GitHub `blob/` link is read as the raw file):

```bash
ipm onboard --list https://example.com/my-list.toml
ipm onboard --list ./my-list.toml --lists "My favourites"   # no checklist: every repo in that list
```

Each repo is one `[[repos]]` table:

```toml
[[repos]]
repo = "obra/superpowers"                   # owner/repo on GitHub, required
list = "AI · Agentic Engineering Methods"   # the list it appears under, required
stars = 297229                              # optional, like everything below
description = "An agentic skills framework & software development methodology that works."
language = "Shell"
last_push = 2026-10-10                      # a TOML date or date-time, or a "YYYY-MM-DD" string
topics = ["skills", "sdlc", "superpowers"]
```

Lists appear in the order they first occur, with their repos sorted by stars. A repo can be in several lists by repeating its table with another `list`. Rows without a valid `owner/repo` or a `list` are skipped and counted. Other fields, and a `[meta]` table, are ignored.

A list fetched from a URL is cached under the cache folder's `curated/`. When the URL can't be reached, `ipm onboard` uses that copy and says so; with no copy, pass a local file with `--list <path>`.

## Sync

```bash
ipm sync
```

`ipm sync` prints the pnpm command it runs, then has pnpm fetch every source and pin it to a commit, and finally rebuilds the catalog. Later syncs keep the pinned commits, so the catalog doesn't change unless you ask it to.

If pnpm fails, its output is shown, `ipm` exits 1, and your catalog is left as it was.

## Upgrade

```bash
ipm sync --upgrade
```

`--upgrade` moves every source to the latest commit of its branch or tag, then rebuilds the catalog. A source pinned to a commit stays there.

Packs you have already built keep their own pins; see [Keep it current](build-a-plugin.md#keep-it-current).

## List and remove

```bash
ipm source list                   # alias, repo, ref and pinned commit
ipm source remove anthropics/skills
ipm sync
```

`list` shows `not synced` for a source added since the last sync.

## See what a source holds

```bash
ipm scan                          # every source: commit, plugins, marketplaces, skills/hooks/agents folders
ipm scan obra/superpowers --paths # one source, listing every folder found
ipm tree 'repo:obra/superpowers'  # the catalog's view: marketplaces, plugins and what they own
```

## GitHub stats

```bash
gh auth login        # once
ipm stats
ipm load             # refresh search so results rank by the new stars
```

`ipm stats` fetches stars, forks, topics, licence, description and last push for every repo in the catalog through `gh`. Stats are kept apart from the catalog, so `ipm sync` doesn't refetch them.

## Coming from aihs

`ipm` used to be called `aihs`. To bring across an aihs-era list of repos once:

```bash
ipm source import path/to/old/checkout/manifest.toml
ipm sync
```

It adds each `[[repo]]` entry (a `version` becomes the ref) and copies the old `catalog/github.jsonl` into your cache, so stats needn't be refetched.

## Good to know

- **Nothing runs.** pnpm always runs with `--ignore-scripts`, and sources' own npm dependencies are never installed.
- **Every file is kept.** For GitHub sources `ipm` fetches the commit as a plain tarball, so npm packing rules (`files`, `.npmignore`) don't strip a repo that is also an npm package.
- **Non-GitHub sources** still go through pnpm's git fetcher, which applies npm's packing rules and can drop files. Use a GitHub mirror if there is one.

The files behind all this are in the [sources reference](../reference/sources.md).
