# ipm workflow commands, Gas City install target and TUI tabs — design

Date: 2026-10-14. Brief: `AgenticHarness/plans/intelligent-package-manager/active/2026-10-14-deploy-to-gas-city/brief.md` (Obsidian vault).

## Intent

`ipm -h` lists twenty flat commands, and nothing in that list teaches the workflow. A new user should read the help and learn the path: **onboard → marketplace → catalog → search → pack → install**. `ipm deploy` only knows Claude Code, and the user wants to install the same pack into Gas City as a Gas City pack. The TUI is one search screen; it should show the workflow as tabs with tidy tables and a master-detail view.

Success:
- `ipm -h` groups commands by workflow stage, and every old capability is reachable under its new name.
- `ipm install claude <pack>` does what `ipm deploy <pack>` did; `ipm install gascity <pack>` writes a loadable Gas City schema-2 pack folder.
- `ipm tui` opens on tabs Catalog / Packs / Installs / Marketplace, and `/` filters the active tab.

Decisions taken with the user:
- Old command names are removed, not aliased; typing one is a usage error naming the new command.
- `ipm install <target>` builds and installs, skipping the build when `.ipm/` is already current. `ipm pack build` is build-only.
- Gas City: skills and agents are written, whole plugins expand to their skills, agents and MCP servers; commands and hooks are skipped with a warning.
- The marketplace list stays this repo's `source-packs.toml` (`curated.DefaultURL`), not `thinkjones/agentic-tools`.
- Installs are recorded: project-pack installs in `ipm.toml`, global-pack installs in `$IPM_HOME/installs.json`.
- Tab order: Catalog, Packs, Installs, Marketplace (no All tab).
- Delivery: one spec, three PRs in order — command re-org, Gas City target, TUI tabs.

## Part 1 — Command tree (PR 1)

### Help

The root uses Cobra command groups and a help header that states the workflow:

```
ipm — intelligent package manager for agent skills, agents, commands and plugins

Workflow: onboard → marketplace → catalog → search → pack → install

Getting started
  onboard      learn the concepts, pick repos from the marketplace, build your catalog
Marketplace — curated repos you can follow
  marketplace  the curated repos and which you follow (add, remove, import, stats)
Catalog — the repos you follow, fetched and searchable
  catalog      what's in your catalog (sync, rebuild, serve, scan, search)
  search       search your catalog (same as catalog search)
  show         an item's fields and neighbours (tree, graph)
Packs — your picks, built and installed
  pack         create, edit and build packs (new, add, remove, list, show, build, install)
  install      install a pack into claude or gascity (same as pack install)
Tools
  tui  doctor  uninstall  version  completion  help
```

The Config block (IPM_HOME, IPM_CACHE_DIR) stays.

### Mapping

| Old | New | Behaviour change |
| --- | --- | --- |
| `source add/remove/import` | `marketplace add/remove/import` | none |
| `source list` | bare `marketplace` | Shows the curated list with ✓ on followed repos, grouped by list, then followed repos that are not curated. `--list`, `--lists`, `--followed` filter. |
| `stats` | `marketplace stats` | none |
| `sync [--upgrade]` | `catalog sync [--upgrade]` | After indexing, loads the search index when the search server is up; otherwise prints a hint to run `ipm catalog serve`. |
| `index`, `load` | `catalog rebuild` | Indexes, then loads search when the server is up (hint otherwise). |
| `serve [--stop]` | `catalog serve [--stop]` | none |
| `scan` | `catalog scan` | none |
| `search` | `catalog search`, and top-level `search` | Same command registered twice. |
| — | bare `catalog` | Summary: sources followed, items by type, last sync time, search server state. |
| `tree`, `graph` | `show tree <id>`, `show graph <id>` | `show <id>` unchanged. Catalog IDs always contain `/` or `:`, so they never collide with `tree` or `graph`. |
| `install` (build) | `pack build [-g] [--update] [--no-validate]` | none |
| `deploy` | `install claude [pack] [--scope] [--path] [-g] [--dry-run]` and `pack install claude …` | Builds only when stale; records the install; `--dry-run` prints the `claude` commands without running them or building. |
| — | `install gascity [pack] [-C dir] [--name] [-g] [--force] [--dry-run]` | Part 2. |
| bare `install` | — | Reinstalls every recorded install for the scope; with none recorded, a usage error listing the targets. |

Removed names (`source`, `stats`, `sync`, `index`, `load`, `serve`, `scan`, `tree`, `graph`, `deploy`) are kept as hidden stub commands that return a `UsageError` such as `ipm deploy was renamed: use ipm install claude`. Exit code 2. They take arbitrary arguments and flags so the message always shows.

`onboard` gains a short concepts intro (marketplace, catalog, pack, install, one line each) printed before the checklist, and its pipeline calls the renamed functions; its behaviour is otherwise unchanged. Messages and hints across the CLI that name old commands are updated.

### Build staleness

`pack build` writes `.ipm/build.stamp` last: a SHA-256 over the scope's `ipm.toml` bytes, the `ipm.lock` bytes written by that build, and the ipm version. `install <target>` computes the stamp from the current `ipm.toml` and existing `ipm.lock`; when it matches and `.ipm/plugins/` exists, the build is skipped and `build is current` is printed. `--update` always rebuilds. The stamp is part of `.ipm/`, which `ipm install` already rewrites deterministically; it must stay byte-identical across two runs.

### Install records

A record is `{pack, target, path, scope (claude only), name (gascity only)}`. Re-installing the same pack to the same target and path replaces its record.

- Project packs: `ipm.toml` gains `[[packs.<name>.installs]]` entries. Paths inside the project root are stored relative to it. Written through the same TOML code as `ipm pack`, so formatting stays stable.
- Global packs: `$IPM_HOME/installs.json`, a sorted JSON array with absolute paths.

`ipm pack show <pack>` lists the pack's installs.

### Docs

`docs/reference/commands.md` is reorganised into the same groups. `getting-started.md`, `concepts.md`, `troubleshooting.md`, `architecture.md`, `index.md`, `guides/tui.md`, `guides/build-a-plugin.md`, `README.md`, `cli/README.md`, `AGENTS.md` and `CLAUDE.md` use the new names. `docs/superpowers/` is history and is not edited.

## Part 2 — Gas City target (PR 2)

`ipm install gascity [pack] [-C dir] [--name name] [-g] [--force] [--dry-run]`

- Output folder: `<dir>/<name>`. `-C` defaults to `.`; `--name` defaults to the pack name and must match `^[A-Za-z0-9][A-Za-z0-9_-]*$`.
- Resolution and fetch are shared with the Claude build (`assemble.Resolve`, pnpm into `.ipm/node_modules`, pins from `ipm.lock`), skipped when the stamp is current.
- A new `assemble.PlaceGascity(plan, root, out)` writes the layout:

```
<name>/
  pack.toml            # [pack] name, schema = 2, version = "0.0.0-ipm", description
  skills/<skill>/...   # the skill directory, copied whole
  agents/<agent>/
    prompt.md          # agent body without frontmatter
    agent.toml         # description, provider = "claude", option_defaults.model when set
  mcp/<server>.toml    # from a whole plugin's .mcp.json
```

- `pack.toml` starts with a comment header: `generated by ipm — rewritten by ipm install gascity; do not edit`, then one `# <item id> @ <commit>` line per item. Gas City rejects unknown keys, so provenance stays in comments.
- Agents: the Claude frontmatter `description` and `model` map to `agent.toml`; `tools`, `color` and other keys are dropped with a warning naming the agent. The prompt file is `prompt.md` rather than `prompt.template.md` so `{{` in an agent body is not read as a Go template; confirm from Gas City's loader during implementation and record the finding in the PR.
- Whole plugins expand: their skills and agents as above, and each `.mcp.json` server becomes `mcp/<server>.toml`. Field names follow Gas City's MCP schema (`name`, `command`, `args`, `env`, and the URL form); confirm against `gastownhall/gascity` `schemas/mcp` during implementation.
- Commands (standalone or inside plugins) and hooks are skipped. Each skip prints `skipped <id>: Gas City has no slash commands` or `…: Gas City packs have no hooks`.
- Name clashes use the existing rules (`-<owner>`, then `-2`, `-3`); agent and server names are sanitised to Gas City's identifier pattern.
- Files are copied, never hard-linked: the folder lives outside `.ipm/`, and a hard link would let edits reach pnpm's store.
- Write to `<dir>/.<name>.ipm-tmp`, then swap it in, so the result replaces the folder completely ("deploying overrides what's there"). If the target exists, is non-empty, and its `pack.toml` lacks the ipm header, refuse with exit 1 unless `--force`.
- After writing: record the install and print the import snippet for the user to paste (`[imports.<name>]` / `source = "<path>"`). ipm does not edit a city.
- `--dry-run` prints the files, skips and warnings and writes nothing.
- `CLAUDE.md`, `AGENTS.md` and `cli/README.md` say ipm shares nothing with Gas City; reword to: ipm writes Gas City pack folders on `ipm install gascity` but reads and edits no Gas City config.
- Output must be deterministic: two runs give byte-identical folders.

Tests: golden folder trees for a pack holding a skill, an agent with frontmatter, a command, and a whole plugin with hooks and an MCP server; the refusal and `--force` paths; name validation; determinism. An integration test (`integration` tag, `testtool.Need`) loads the output with a real `gc` when it is on PATH.

## Part 3 — TUI tabs (PR 3)

### Layout

Top to bottom: title bar (`IPM`, scope, ● hybrid / ○ local); tab bar with counts (`CATALOG (2,314)  PACKS (3)  INSTALLS (2)  MARKETPLACE (192)`); one-line stats strip for the active tab; a table with aligned columns and `…` truncation; a detail pane for the selected row; a key-binding footer for the active tab. The palette in `style.go` is kept; narrow terminals hide the detail pane as today.

### Tabs

| Tab | Rows | Keys |
| --- | --- | --- |
| Catalog (default) | Search results: TYPE · NAME · REPO · ★ · SCORE. Detail: the item. `enter` opens today's tree mode. | `a`/`d`, `p`/`n`, `t`, `f` as today |
| Packs | NAME · SCOPE · ITEMS · BUILT (fresh/stale) · INSTALLED TO. Detail: the pack's items. | `g` include global, `x` remove selected item, `b` build, `i` install |
| Installs | PACK · TARGET · PATH · SCOPE · WHEN. | `enter`/`i` reinstall, `a` new install (claude or gascity form; replaces the `D` deploy modal), `g` include global |
| Marketplace | ✓ · REPO · LIST · ★ · ITEMS, grouped by curated list. Detail: description and topics. | `space` follow/unfollow, `s` catalog sync in the output modal |

Global: `/` filters the active tab (Catalog runs the search; other tabs filter rows by substring); `1`–`4` and `[`/`]` switch tabs; `←/→` stay with Catalog's panes; `?` help; `q` quit. A tab whose data fails to load shows the error in its body; other tabs keep working.

### Code

`internal/tui` keeps depending only on its interfaces:
- `Installer` (build) is renamed `Builder`.
- `Deployer` becomes `Installer`: `Targets() []string`, `Default(pack, target) Target`, `Plan`, `Install(ctx, pack, Target, out)`, `List(global bool) ([]Install, error)`.
- `Packs.List(global bool)`.
- New `Marketplace`: `List() ([]Repo, error)`, `Follow(repo)`, `Unfollow(repo)`, `Sync(ctx, out)`.

Adapters in `internal/cli/tui.go` call the CLI's functions. Each tab is its own file (`tab_catalog.go`, `tab_packs.go`, `tab_installs.go`, `tab_marketplace.go`) with a shared `table.go`. Tests: goldens for each tab, narrow and no-color variants; model tests for tab switching, per-tab filtering, global toggles and the install form. `docs/guides/tui.md` is rewritten.

## Out of scope

- Editing a Gas City `city.toml` or `pack.toml` to import the generated pack.
- Translating Claude commands or hooks into Gas City concepts.
- Moving the marketplace list to `thinkjones/agentic-tools`.
- A combined All tab.
