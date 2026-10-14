---
title: Share packs with a team
parent: Guides
nav_order: 5
---

# Share packs with a team
{: .no_toc }

Commit a pack so everyone on the project builds the same plugin, at the same commits.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## What to commit

| File | Commit? | Why |
| --- | --- | --- |
| `ipm.toml` | Yes | The packs, the items in them and where each pack is installed (`[[packs.<name>.installs]]`) |
| `.ipm/ipm.lock` | Yes | Every item resolved to a repo, path and commit |
| `.ipm/package.json`, `.ipm/pnpm-lock.yaml`, `.ipm/.pnpmfile.cjs` | Yes | pnpm's pins for the repos the items come from |
| `.ipm/.claude-plugin/marketplace.json` | Yes | The marketplace Claude Code reads |
| `.ipm/build.stamp` | Yes | A hash of the packs, the lock and the ipm version; lets `ipm install` skip a build that is already current |
| `.ipm/.gitignore` | Yes | Keeps the next two rows out |
| `.ipm/node_modules/`, `.ipm/plugins/` | No | Built by `ipm pack build` |

`ipm pack build` writes `.ipm/.gitignore` for you.

## What a teammate does

```bash
git pull
ipm install claude my-pack
```

Or, to build without installing, `ipm pack build`; a bare `ipm install` replays the installs recorded in `ipm.toml`.

They need `ipm` and pnpm 12+ (or Node), but not your sources or your catalog: `ipm pack build` builds from `ipm.toml` and `.ipm/ipm.lock` alone, so they get the same items at exactly the commits in the lock.

{: .note }
Two things in the build come from the person running it: the plugin and marketplace owner (their `git config user.name`) and the marketplace name (`<project folder>-ipm`). A teammate's build can therefore show a small diff in `.ipm/.claude-plugin/marketplace.json`; the plugin content is the same. Cloning into a folder with the same name keeps the `/plugin install` command the same for everyone.

## Changing the pack

Whoever changes the pack runs `ipm pack add` or `remove`, then `ipm pack build`, and commits the files above together. New items are resolved from that person's catalog, so they need the item's repo among their sources.

To move every item to newer commits, one person runs:

```bash
ipm catalog sync --upgrade
ipm pack build --update
```

and commits the new lock. Review the diff of `.ipm/ipm.lock` like any dependency bump: it shows each item's old and new commit.

## Trust

`ipm.toml` and `.ipm/ipm.lock` come from whoever committed them, so `ipm pack build` treats them as untrusted:

- pack names must be plain names, so a pack can't write outside `.ipm/plugins/`;
- every item path must stay inside its repo, symlinks included;
- commits must be full 40-character SHAs;
- nothing upstream is executed: pnpm always runs with `--ignore-scripts`.

What the items *say* is another matter. A skill or agent is instructions for Claude, and a whole plugin can bring hooks and MCP servers that run on your machine. Review new items and lock bumps as you would any third-party code.
