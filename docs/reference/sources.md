---
title: Sources
parent: Reference
nav_order: 2
---

# Sources reference

Your **sources** are the repos of interest `ipm` fetches, pins and catalogs. They live in one pnpm project in your ipm home, which `ipm marketplace` edits and `ipm catalog sync` installs. This page covers the files, the `ipm marketplace` commands and the naming rules.

## Where things live

| Path | What it is | Edit by hand? |
| --- | --- | --- |
| `$IPM_HOME/sources/package.json` | One git dependency per source | No — use `ipm marketplace` |
| `$IPM_HOME/sources/pnpm-lock.yaml` | pnpm's pins: the commit each source resolved to | No — written by `ipm catalog sync` |
| `$IPM_HOME/sources/node_modules/<alias>/` | Each source as pnpm installed it, read by `ipm catalog rebuild` and `ipm catalog scan` | No |
| `$IPM_HOME/sources/.pnpmfile.cjs` | pnpm hooks, rewritten by `ipm catalog sync` on every run | No |
| `$IPM_HOME/store/` | ipm's own pnpm content-addressable store (`--store-dir`) | No |
| `$IPM_CACHE_DIR/catalog/*.jsonl` | The catalog, written by `ipm catalog rebuild`; `github.jsonl` by `ipm marketplace stats` | No — rebuildable |
| `$IPM_CACHE_DIR/typesense/` | Search server data and API key (`ipm catalog serve`) | No — rebuildable |

`IPM_HOME` defaults to `~/Library/ipm` on macOS, `$XDG_DATA_HOME/ipm` (else `~/.local/share/ipm`) on Linux and `%LOCALAPPDATA%\ipm` on Windows. `IPM_CACHE_DIR` defaults to `~/Library/Caches/ipm` on macOS, `$XDG_CACHE_HOME/ipm` (else `~/.cache/ipm`) on Linux and `%LOCALAPPDATA%\ipm\cache` on Windows. `XDG_DATA_HOME` and `XDG_CACHE_HOME` are honoured on every OS. Both overrides must be absolute paths.

The home holds what you would lose (your list of sources and its pins); the cache holds only what `ipm` can rebuild.

`$IPM_HOME/pinned/` (`package.json`, `pnpm-lock.yaml`, `.pnpmfile.cjs`, `node_modules/`) is a second pnpm project of the same shape, the pin store: `ipm pack build` reads a repo from your sources when they are at the commit in `.ipm/ipm.lock`, and otherwise adds `<alias>--<first 12 characters of the commit>` here and has pnpm fetch it, once for every project. Pins are never removed. See [Build your own plugin](../guides/build-a-plugin.md).

## `package.json`

```json
{
  "name": "ipm-sources",
  "private": true,
  "dependencies": {
    "anthropics--skills": "github:anthropics/skills",
    "obra--superpowers": "github:obra/superpowers#v5.0.0"
  }
}
```

Each key is a source's **alias**; each value is its pnpm specifier: `github:<owner>/<repo>[#<ref>]` for GitHub, or `git+<url>[#<ref>]` for any other git URL (`git@host:path` is written as `git+ssh://git@host/path`). `ipm` writes the keys sorted, with a two-space indent and a trailing newline.

## Aliases

An alias is the source's key in `package.json` and its folder in `node_modules`: the repo lowercased, with `/` (and `:`) turned into `--`, any URL scheme or `git@` and a trailing `.git` dropped, and anything outside `a-z 0-9 . _ -` replaced by `-`. `obra/superpowers` becomes `obra--superpowers`; `ComposioHQ/awesome-claude-skills` becomes `composiohq--awesome-claude-skills`.

Two repos whose aliases collide (for example `Foo/bar` and `foo/bar`) cannot both be sources: `ipm marketplace add` refuses and names both.

## Refs and pins

A source may name a ref — a tag, branch or commit. Without one it follows the repo's default branch. `ipm catalog sync` runs `pnpm install`, which pins each source to a commit in `pnpm-lock.yaml`; later syncs keep that commit. `ipm catalog sync --upgrade` runs `pnpm update --latest`, which moves every source to the latest commit of its branch or tag. `ipm catalog rebuild` and `ipm catalog scan` read the commit from the lock; a source with no pinned 40-character commit, or not installed in `node_modules`, is an error that says to run `ipm catalog sync`.

## `ipm marketplace`

| Command | Does |
| --- | --- |
| `ipm marketplace add <repo>…` | Add sources. Each is `owner/repo[@ref]`, a git URL (`https://…`, `ssh://…`, `file://…`, `git@…`) with an optional `#ref`, or an external plugin's catalog ID (`plugin:<marketplace repo>#<name>`), which adds the repo that plugin lives in. A repo already listed, or an alias collision, is refused and nothing is written. |
| `ipm marketplace remove <repo>…` | Remove sources, matched by alias. An unknown repo is an error and nothing is written. |
| `ipm marketplace --followed` | Your sources grouped by the list each belongs to in the default [curated list](../guides/sources.md#curated-lists): alias, repo, stars, ref and pinned commit (`not synced` until `ipm catalog sync` has run), then description, topics and homepage. Sources in no list come last, under `Other`. |
| `ipm marketplace import <path/to/manifest.toml>` | One-time migration from the aihs-era list of repos of interest: adds each `[[repo]]` entry (`repo`, with `version` as its ref), and copies a sibling `catalog/github.jsonl` into the cache if the cache has none, so GitHub stats need not be refetched. |

Run `ipm catalog sync` after any change; `add`, `remove` and `import` only edit `package.json`.

## What pnpm is allowed to do

pnpm always runs with `--ignore-scripts --store-dir $IPM_HOME/store --dir $IPM_HOME/sources` (or `--dir $IPM_HOME/pinned` for the pin store), so no upstream file is ever executed. The generated `.pnpmfile.cjs`, written to both, adds two hooks:

- **No npm dependencies.** Sources are content, not code: their own `dependencies`, `optionalDependencies` and `peerDependencies` are emptied before pnpm resolves them.
- **Every file kept.** pnpm treats a GitHub repo as a git-hosted package and re-packs it with npm's packing rules (`package.json` `files`, `.npmignore`), which strips repos that are also npm packages — `nizos/tdd-guard` went from 429 files to 3. The hook uses pnpm 12's custom fetcher (`{ delegate }`) to fetch the same commit as a plain GitHub tarball instead.

This is why `ipm` needs pnpm 12 or later. It uses pnpm from PATH when that is 12+, and otherwise runs `npx -y pnpm@12.10.1`, so Node alone is enough.

**Known limitation:** sources that are not on GitHub (`git+https`, `git+ssh`, `git+file`) still go through pnpm's git fetcher, so npm's packing rules can drop files from them.
