---
title: Use the TUI
parent: Guides
nav_order: 3
---

# Use the TUI
{: .no_toc }

Search the catalog, collect items into packs, build and install them, and follow repos, all in one screen with four tabs.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Start it

In your project:

```bash
ipm tui            # the project's packs (the nearest ipm.toml, or the current folder)
ipm tui -g         # your user-scope packs
```

`ipm tui` needs a terminal and a catalog: run `ipm catalog sync` first. For the best ranking, run `ipm catalog serve` and `ipm catalog rebuild` too; without them the TUI still works, with keyword search.

## The screen

Top to bottom: a title line, the tab bar, a one-line stats strip for the active tab, the tab's body, a status line, and the key footer for the active tab.

```text
ipm  project /work                                                          ● hybrid
CATALOG (14,917)  PACKS (2)  INSTALLS (1)  MARKETPLACE (192)
━━━━━━━━━━━━━━━━──────────────────────────────────────────
skill 11,804 · agent 1,077 · command 838 · hook 28 · mcp 28 · plugin 643 · marketplace 28 · repo 298
╭─ search: c# testing ──────────────────────────────────────────────────────────────────╮
│   TYPE   ✓ NAME           REPO                     ↗     ★ SCORE DESCRIPTION           │
│ ▶ agent  ✓ csharp-pro     wshobson/agents              ★40.3k    71 Write modern C# …  │
╰────────────────────────────────────────────────────────────────────────────────────────╯
╭─ tree ── agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md ─────────────╮
│ …                                                                                      │
╰────────────────────────────────────────────────────────────────────────────────────────╯
─ csharp-sdlc (4) · project /work
skill dotnet-backend-patterns · agent csharp-pro · command accessibility-audit · plugin stripe
50 of 100
↑↓ results  ←→ tree  tab tree mode  a add  t tray  p packs  f type  1-4 tabs  ? help  q quit
```

- **Title line:** `ipm`, the scope (`project <folder>` or `global`), and on the right `● hybrid` (the search server) or `○ local` (keyword search over the catalog).
- **Tab bar:** `CATALOG (n)  PACKS (n)  INSTALLS (n)  MARKETPLACE (n)`, with thousands separators. A tab that hasn't loaded yet shows `(…)`: Installs and Marketplace load when you first open them. The Catalog is the tab you start on.
- **Stats strip:** one line about the active tab; each tab below says what it holds. While you filter a tab, the strip ends with `filter: <text>`.
- **Detail pane:** Packs, Installs and Marketplace show the selected row in full beside the table. Below 100 columns the pane is hidden; on Packs, `enter` then shows the pack's items across the whole body.
- **Colour:** each type has its own colour everywhere (repo blue, marketplace purple, plugin amber, skill teal, agent pink, command green, hook and MCP server grey); the panel with focus has an amber border. Set `NO_COLOR` to turn colour off.
- **A tab whose data fails to load** shows the error in its body; the other tabs keep working.

Below 60×15 the TUI asks you to enlarge the terminal; in a short terminal the Catalog's tree leaves out children.

## Keys on every tab

| Key | Does |
| --- | --- |
| `1`–`4` | go to Catalog, Packs, Installs or Marketplace |
| `[` `]` | previous / next tab (wrapping) |
| `/` | focus the active tab's filter (on the Catalog, the search box) |
| `?` | every key of the active tab; `esc` or `?` closes it |
| `q`, `ctrl+c` | quit (`ctrl+c` quits even while typing) |

While a text input has focus (the Catalog's search box, a tab's filter, a field of a form), every printable key types into it and `esc` leaves it. So in the Catalog's search box `1`, `q` and `/` are part of the query: press `esc`, `↓` or `tab` to move to the results, and the other keys work.

On Packs, Installs and Marketplace, `/` filters the rows by a case-insensitive substring of their visible column text. `enter` keeps the filter and returns to the rows; `esc` clears it.

## Catalog

Search every artifact in the catalog, see where each lives, and collect into the active pack. The stats strip is the number of items of each type.

| Column | Is |
| --- | --- |
| `TYPE` | skill, agent, cmd, plugin, hook, mcp, market, repo or copilt |
| `✓` | the item is already in the active pack |
| `NAME`, `REPO` | the artifact and the repo it lives in |
| `↗` | how many other repos use its repo (omitted when none) |
| `★` | the repo's GitHub stars |
| `SCORE` | the search score |
| `DESCRIPTION` | its description |

In a narrow terminal columns drop out: the repo, `↗N`, stars and description go first. A name starting `↗` is an external entry (see below). The panel below the results is the tree: the artifact in context, from its repo down to its parent, its siblings and its children. Every node shows a one-line description, lined up in one column; the current node has an arrow to its description, which runs to three lines. When an artifact belongs to more than one plugin, `also in:` lists the others. Long lists scroll, with `↑ N more` / `↓ N more` rows. The line under the tree is the active pack: its name, item count, `• not built` once you've changed it since the last build, and its items.

| Key | Does |
| --- | --- |
| typing | search; results refresh as you type |
| `↓`, `tab`, `esc` (search box) | focus the results |
| `/` | focus the search box |
| `↑` `↓`, `j` `k` | move through results; the tree follows |
| `←` `→`, `h` `l` | move the tree to the parent / first child, without changing the result |
| `tab`, `enter` | tree mode |
| `a` / `d` | add / remove the selected result |
| `t` | the pack tray (`←` `→` items, `d` removes, `esc` back) |
| `f` | cycle the type filter: all, skill, agent, command, plugin |
| `p` | switch the active pack, or `n` for a new one |
| `e` | external references: the other repos that use this repo |
| `ctrl+r` | check the search server again |

Building and installing are not on the Catalog: they live on Packs and Installs.

### Tree mode

| Key | Does |
| --- | --- |
| `↑` `↓`, `k` `j` | move between siblings, and onto `also in` rows |
| `←` `→`, `h` `l` | parent / first child |
| `enter` | on an `also in` row: follow that plugin instead |
| `a` / `d` | add / remove the node, for example a whole plugin |
| `tab`, `esc` | back to search mode; the tree returns to the selected result |

## External references

A marketplace can list a plugin that lives in another repo: `anthropics/claude-plugins-official` lists `cloudflare`, which lives in `cloudflare/skills`. ipm calls that an **external entry**, and keeps repos as the roots of their own trees:

- **In the marketplace's tree**, the entry is drawn `↗cloudflare`, and its description starts with where it points: `↗ cloudflare/skills — Skills for the Cloudflare…`.
- **`→` on it jumps** into `cloudflare/skills`' own tree, on the plugin it points at. A second root sits above the tree, `↩ anthropics/claude-plugins-official — back to cloudflare`: `↑` to it and `enter` takes you back. Jumps stack (up to 20); moving to another search result forgets them.
- **In `cloudflare/skills`' own tree**, the root is `cloudflare/skills`. Who uses it is extra information, not ancestry: the current node says `↗ used by N` (other repos using its repo), and nodes that other repos point at directly say it too.
- **`e`** lists every external reference to the current repo, grouped by the repo it comes from, with stars and marketplace. `enter` jumps there, with the back root to return.
- An external entry whose repo isn't one of your sources says `↗ <repo> (not a source — ipm marketplace add <id>)`; add it and `ipm catalog sync` to follow it.

Being used by other repos is a strong sign a repo is useful, so search also prefers it when results are otherwise equal.

## Packs

Every pack of the scope, with what has been built and installed.

| Column | Is |
| --- | --- |
| `NAME` | the pack |
| `SCOPE` | `project` or `global` |
| `ITEMS` | how many items it holds |
| `BUILT` | `fresh` (`.ipm/` matches the pack), `stale` (the pack changed since the last build) or `never` |
| `INSTALLED TO` | where its recorded installs went |

The stats strip counts packs, how many are fresh and stale, and how many installs are recorded. The detail pane lists the pack's description and items.

| Key | Does |
| --- | --- |
| `↑` `↓`, `j` `k` | move through the packs, or through the items |
| `enter`, `→` | focus the selected pack's items |
| `esc`, `←` | back to the packs |
| `x` | remove the selected item from the pack (with the items focused) |
| `b` | build every pack (`ipm pack build`); the output streams in a modal and ends with `✓ built` |
| `i` | install the selected pack: opens the install form |
| `g` | include your global packs, when you started in a project (in `ipm tui -g` it says `already in global scope`) |

From a project, `g` lists your user-scope packs too, marked `global`. They are read-only there: `x`, `b` and `i` on one say `global pack — open ipm tui -g to change it`. Start `ipm tui -g` to change them.

## Installs

The installs ipm has recorded for the scope: the `[[packs.<name>.installs]]` entries of `ipm.toml`, or `$IPM_HOME/installs.json` for your user scope.

| Column | Is |
| --- | --- |
| `PACK` | the pack installed |
| `TARGET` | `claude` or `gascity` |
| `AS` | the Claude scope (`user`, `project`, `local`), or the Gas City folder name |
| `PATH` | the folder it was installed to |
| `FROM` | the scope whose `ipm.toml` records it, `project` or `global` |

The stats strip counts installs and, per target, how many. The detail pane shows the install in full and the `ipm install` command that repeats it. There is no WHEN column: ipm doesn't record when an install ran.

| Key | Does |
| --- | --- |
| `↑` `↓`, `j` `k` | move through the installs |
| `enter`, `i` | reinstall the selected install: it runs at once in the output modal, ending with `✓ installed <pack> (<target> …)` |
| `a` | a new install of the selected row's pack; with no row selected, of the only pack, or the form asks which pack when there are several |
| `g` | include your global installs, when you started in a project |

Like global packs, global installs listed from a project are read-only: `enter`, `i` and `a` on one say `global pack — open ipm tui -g to change it`.

## The install form

`i` on Packs, or `a` on Installs, opens the form in place of the tab. Each step shows its keys at the bottom; `enter` goes on, `esc` cancels.

1. **Pack** (only from `a` with several packs and no row selected): pick one of the scope's packs.
2. **Target:** `claude` (Claude Code, as a plugin) or `gascity` (a Gas City pack folder).
3. **For `claude`, the scope:** **user** (every project, just you), **project** (a folder, shared through its `.claude/settings.json`) or **local** (a folder, just you). For project and local, then the folder (`~` works). The defaults are those of `ipm install claude`.
4. **For `gascity`, the parent folder and the folder name** (the name defaults to the pack's). A name or folder `ipm install gascity` refuses is shown in red and the form returns to the folder.
5. **Confirm:** the exact commands it will run, then `y` to install:

   ```text
   Install csharp-sdlc to Claude Code — project scope in /work
     ipm pack build
     claude plugin marketplace add /work/.ipm --scope project
     claude plugin install csharp-sdlc@work-ipm --scope project
   y install · esc cancel
   ```

The output streams into the modal and ends with `✓ installed csharp-sdlc (claude project /work)`, or an error. While a build or install runs, other build and install keys say `busy — wait for the current run to finish`; the modal closes with `esc` or `enter` when it is done. The Packs and Installs tabs reload as the run finishes.

An install builds first when `.ipm/` is out of date, so Claude Code gets the current pack. Installing again adds the marketplace again (Claude Code accepts that), refreshes it, and updates the plugin instead of installing it. If Claude Code already has a marketplace of the same name from another folder, two projects in folders with the same name, the install stops rather than repoint it. Claude Code reads an ipm pack straight from `.ipm/plugins/`, so a later build takes effect at the next session or after `/reload-plugins`.

The same works from the command line with [`ipm install claude`](../reference/commands.md#ipm-install) and `ipm install gascity`, and every install is recorded either way, so a bare `ipm install` replays what the Installs tab lists.

## Marketplace

The curated lists of repos, with a `✓` on the ones you follow. Repos you follow that no list names are grouped under `Other`.

| Column | Is |
| --- | --- |
| `✓` | you follow the repo |
| `REPO` | `owner/repo`, under a divider row for each list |
| `LIST` | the list |
| `★` | GitHub stars (from `ipm marketplace stats`) |
| `ITEMS` | how many artifacts of the repo are in the catalog (0 until it is synced) |

The stats strip is `N repos · F followed · L lists`: the same counts as `ipm marketplace` prints, with a repo of two lists counted once. The detail pane shows the lists, stars, items, topics and description of the selected repo.

| Key | Does |
| --- | --- |
| `↑` `↓`, `j` `k` | move through the repos; the lists' divider rows are skipped |
| `space` | follow or unfollow the selected repo (`ipm marketplace add` / `remove`); it changes the sources list at once, the catalog only after a sync |
| `s` | run `ipm catalog sync` in the output modal: fetch the followed repos with pnpm and rebuild the catalog |

After a successful sync the status says `catalog synced — restart ipm tui to search the new items`. The TUI doesn't reload the catalog or the search index while it runs, so restart it to see new items on the Catalog and in the stats.

## Without the search server

When the server isn't running, or its index is empty, the TUI searches the catalog by keyword (names count most, then descriptions and tags) and shows a banner saying so. Start it with `ipm catalog serve` and `ipm catalog rebuild` in another terminal, then press `ctrl+r`.
