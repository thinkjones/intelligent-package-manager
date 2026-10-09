# ipm — design

Status: approved in conversation 2026-10-09; for review as a document · supersedes the stage 3 pack plan

## Intent

`ipm` ("intelligent package manager") lets a person find agent artifacts across many upstream repos — skills, agents, commands, hooks, MCP servers, plugins — and assemble their own Claude Code plugins from them, at project or user scope, modelled on pnpm. It grows out of `aihs`: stages 1–2 (catalog, GitHub stats, hybrid search, relationship views) carry over unchanged in behaviour; what changes is where things live, who fetches upstreams, and how packs become plugins.

The generated plugin is deliberately the output (not installs into `.claude/agents`): it is the source of truth for porting to other providers later. `ipm` never installs into Claude Code; the user does.

### Why the pivot

The reference-only plugin design hit limits in Claude Code 2.1.295 (spike, recorded in `.superpowers/sdd/stage3/progress.md`): marketplace entries ignore declared `agents`/`commands` lists; symlinked agent and command files are ignored; directory symlinks load whole folders; and items referenced from many upstreams land in many plugin namespaces. Cloned or hard-linked files inside one local plugin load correctly and give one namespace per pack. pnpm already solves fetching, pinning, a shared content store and cheap linking — so `ipm` uses it.

### Success criteria

- `ipm source add <repo>` then `ipm sync` fetches through pnpm and rebuilds the catalog; `ipm search` works as today.
- In any project, `ipm pack new` + `ipm pack add` + `ipm install` builds `.ipm/plugins/<pack>/` with every picked skill, agent and command loading as `<pack>:<item>`, whole plugins as pinned dependencies, and `claude plugin validate --strict` passing.
- A teammate who clones the project and runs `ipm install` gets byte-identical plugin content (same commits, from the committed lockfile).
- No upstream file is ever executed: pnpm always runs with `--ignore-scripts`.

## Names and decisions

- **Name:** `ipm`. Rejected: `agpm` (aig787/agpm is a same-domain tool), `spm` (Swift PM and several skill tools), `champs` (hard to type), `agasp`. `ipm` is free on PATH, Homebrew, npm and crates.io; InterSystems IPM (ObjectScript, runs inside IRIS) shares the name.
- **pnpm is the store and fetcher.** Verified with pnpm 10.33.2: git dependencies without a `package.json` install (as `0.0.0`); `github:owner/repo#<sha>&path:/<dir>` installs exactly one folder; content matches a git checkout except `.gitignore`; `.claude-plugin/` is kept; `pnpm-lock.yaml` records repo, commit and path; files are APFS clones of a content-addressable store.
- **Dedicated store** by default (`--store-dir $IPM_HOME/store`), configurable to share the user's normal pnpm store. pnpm's docs: a store is for mutually trusted users and processes.
- **Sources and catalog are per user** (like pnpm's registry config and metadata cache); **packs are per scope** (a project's `ipm.toml`, or the user's global one).
- **Pickable item by item:** skills, agents, commands, whole plugins (local, inline, external), Copilot plugins (as their skills). **Not individually pickable:** hooks, MCP servers, inline commands — they come only with their whole plugin. Surgical removal of hooks and MCP servers is a later design question.
- **Naming in Claude:** picked items are `<pack>:<item>`; a whole plugin keeps its own namespace.
- **Import method:** hard link, then copy (across volumes). APFS cloning was left out to avoid a new dependency; editing a file in `.ipm/plugins/` therefore also edits pnpm's copy, and `ipm install` rebuilds it.

## Layout

`$IPM_HOME` overrides; defaults follow pnpm: macOS `~/Library/ipm`, Linux `$XDG_DATA_HOME/ipm` (`~/.local/share/ipm`), Windows `%LOCALAPPDATA%\ipm`. Cache: macOS `~/Library/Caches/ipm`, Linux `$XDG_CACHE_HOME/ipm`.

```text
$IPM_HOME/
  sources/package.json      generated: one git dependency per repo of interest (alias owner--repo)
  sources/pnpm-lock.yaml    pnpm's pins
  sources/node_modules/…    installed source repos, read by `ipm index`
  store/                    pnpm --store-dir
  global/ipm.toml           user-scope packs (+ global/.ipm/ as in a project)
<cache>/
  catalog/*.jsonl, github.jsonl   the catalog (rebuildable; no longer committed)
  typesense/                       search data and API key

<project>/
  ipm.toml                  packs (committed)
  .ipm/package.json         generated: one dependency per source repo, pinned to a commit (committed)
  .ipm/pnpm-lock.yaml       generated pins (committed)
  .ipm/ipm.lock             every picked item resolved: id, type, name, repo, path, sha (committed)
  .ipm/node_modules/        pnpm output (gitignored)
  .ipm/plugins/<pack>/      built plugin: cloned items + plugin.json (gitignored)
  .ipm/.claude-plugin/marketplace.json   generated (committed)
```

`ipm.toml`:

```toml
[packs.csharp-sdlc]
description = "C# development lifecycle"
items = [
  "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns",
  "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md",
  "plugin:anthropics/claude-plugins-official#stripe",
]
```

Item IDs are the catalog's: `<type>:<repo>:<path>`, `#<name>` for items defined inside a file, `plugin:<repo>#<name>` for external plugins. They carry no commit.

## Built plugin

`.ipm/plugins/<pack>/`:

- `.claude-plugin/plugin.json` — `name` = pack, `description`, `author`, `version`, `dependencies` (one per whole-plugin pick), and `metadata.ipm.items`: `[{id, repo, path, sha, file}]` per cloned item (JSON has no comments; unknown keys fail `--strict`; `metadata` is documented as free-form and ignored by Claude Code).
- `skills/<name>/`, `agents/<name>.md`, `commands/<name>.md` — hard-linked (copied across volumes) from `.ipm/node_modules/<alias>/…`. Name clashes within a pack get `-<repo owner>` appended.

Pins are per source repo, not per picked item: part 2's fetcher hook fetches whole commits as plain tarballs and drops pnpm's `path:` subfolders, and whole repos come from the shared store anyway. `.ipm/ipm.lock` records every resolved item so `ipm install` works without the user's catalog. `ipm install` keeps existing pins; `ipm install --update` moves them to the commits in the user's current catalog. Spike (2026-10-09, Claude Code 2.1.295): a hand-built `.ipm/` with hard-linked items and a stripe dependency validated `--strict`, installed with the dependency, and loaded each item under the pack's name.

`.ipm/.claude-plugin/marketplace.json` lists each pack (`source: ./plugins/<pack>`) and each whole-plugin pick as a pinned external entry (`git-subdir` or the upstream entry's own pinned source), tagged `ipm`. Marketplace name: `<project dir>-ipm` (user scope: `ipm-global`).

## Commands

| Command | Does |
| --- | --- |
| `ipm source add/remove/list <owner/repo>[@ref]` | edit the user's sources (replaces the root `manifest.toml` and `promote`) |
| `ipm sync [--upgrade]` | `pnpm install` (`pnpm update` with `--upgrade`) in `$IPM_HOME/sources`, then `ipm index` |
| `ipm index`, `stats`, `serve`, `load`, `search`, `show`, `tree`, `graph`, `scan`, `doctor` | as in `aihs`, reading the user-level catalog; `doctor` adds pnpm, sources and scope rows |
| `ipm pack new/add/remove/list/show [-g]` | edit packs in `ipm.toml` (project) or `global/ipm.toml` (`-g`) |
| `ipm install [-g] [--update] [--no-validate]` | resolve picks (lock, then catalog) → write `.ipm/ipm.lock` and `.ipm/package.json` → pnpm install → place files in `.ipm/plugins/*` → write `plugin.json`s and `marketplace.json` → `claude plugin validate --strict` |

Installing in Claude: `claude plugin marketplace add ./.ipm`, then `/plugin install <pack>@<marketplace>`.

## Parts

Each part gets its own plan and commits; each leaves the tool working.

1. **Rename** `aihs` → `ipm`: binary, `cli/` module and package names where they say aihs, `bin/` shim, `AIHS_ROOT` → `IPM_ROOT` for the transition, `OwnerTag`, docs, help text. No behaviour change.
2. **Sources and relocation on pnpm:** a paths layer (`IPM_HOME`, cache dir, XDG/macOS defaults); `ipm source`; `ipm sync` via pnpm; the indexer reads `sources/node_modules`; catalog and Typesense data move to the cache dir; one-time import of this repo's `manifest.toml`; remove the `upstream` package, `aihs.lock`, the committed `catalog/` and the stale `.claude-plugin/marketplace.json`; the old pack flow (`pack.Load/LoadAll/Problems/…`, `marketplace.Entries/stack`, `copilot.Expand`) goes too.
3. **Packs and `ipm install`:** `ipm.toml`, `ipm pack`, `.ipm/` generation, cloning, `metadata` provenance, marketplace, validation; user scope via `-g`.
4. **TUI** over search and packs.

## Open for later

Surgical removal of hooks and MCP servers from whole plugins; non-GitHub sources through pnpm's `git+https` (untested); `ipm outdated`/`update` for packs; porting built plugins to other providers; Windows paths.
