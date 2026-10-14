---
title: Use the TUI
parent: Guides
nav_order: 3
---

# Use the TUI
{: .no_toc }

Search, see where each artifact lives, collect it into a pack, build the pack and deploy it to Claude Code, all in one screen.
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

```text
ipm  wshobson/agents › claude-code-workflows › jvm-languages › csharp-pro          ● hybrid
╭─ search: c# testing ──────────────────────────────────────────────────────────────────╮
│ ▶71   agent  ✓ csharp-pro                    wshobson/agents          ★40.3k           │
│  65   copilt   csharp-dotnet-development     github/awesome-copilot   ★39.9k           │
│  24   skill  ✓ dotnet-backend-patterns       wshobson/agents          ★40.3k           │
╰────────────────────────────────────────────────────────────────────────────────────────╯
╭─ tree ── agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md ─────────────╮
│ repo    wshobson/agents                                                                │
│ └ market  claude-code-workflows        Production-ready workflow orchestration with…   │
│   └ plugin  jvm-languages              JVM language development including Java, Sca…   │
│     ├ agent  ▶csharp-pro ────────────▶ Write modern C# code with advanced features      │
│                                        like records, pattern matching, and async/…      │
│     ├ agent   java-pro                 Master Java 21+ with modern features like vir…   │
│     └ agent   scala-pro                Master enterprise-grade Scala development wit…   │
╰────────────────────────────────────────────────────────────────────────────────────────╯
─ csharp-sdlc (4) · project /work
skill dotnet-backend-patterns · agent csharp-pro · command accessibility-audit · plugin stripe
50 of 100
↑↓ results  ←→ tree  tab tree mode  a add  t tray  p packs  b build  D deploy  ? help  q quit
```

- **Header:** a breadcrumb of where the tree is (repo › marketplace › plugin › artifact), and `● hybrid` (the search server) or `○ local` (keyword search over the catalog).
- **Results panel, top:** score, type, `✓` when the item is already in the active pack, name, repo, `↗N` (how many other repos use its repo) and stars. Under 100 columns, repo, `↗N` and stars drop out. A name starting `↗` is an external entry (see below).
- **Tree panel, bottom:** the artifact in context — the path from its repo down to its parent, its siblings, and its children. Every node shows a one-line description, lined up in one column; the current node has an arrow to its description, which runs to three lines. When an artifact belongs to more than one plugin, `also in:` lists the others. Long lists scroll, with `↑ N more` / `↓ N more` rows.
- **Tray:** the active pack, its item count, `• not built` once you've changed it since the last build, and its items.
- **Colour:** each type has its own colour everywhere (repo blue, marketplace purple, plugin amber, skill teal, agent pink, command green, hook and MCP server grey); the panel with focus has an amber border. Set `NO_COLOR` to turn colour off.

Below 60×15 the TUI asks you to enlarge the terminal; in a short terminal the tree leaves out children.

## Search mode

| Key | Does |
| --- | --- |
| typing | search; results refresh as you type |
| `↓`, `tab` (search box) | focus the results |
| `/` | focus the search box |
| `↑` `↓`, `j` `k` | move through results; the tree follows |
| `←` `→`, `h` `l` | move the tree to the parent / first child, without changing the result |
| `tab`, `enter` | tree mode |
| `a` / `d` | add / remove the selected result |
| `t` | the pack tray (`←` `→` items, `d` removes, `esc` back) |
| `f` | cycle the type filter: all, skill, agent, command, plugin |
| `p` | switch pack, or `n` for a new one |
| `b` (or `i`) | build every pack (`ipm pack build`) |
| `D` | deploy the active pack to Claude Code (`ipm install claude`) |
| `e` | external references: the other repos that use this repo |
| `ctrl+r` | check the search server again |
| `?` | every key |
| `q`, `ctrl+c` | quit |

In the search box every letter is part of the query, so the other keys work once you've moved to the results (`↓` or `tab`).

## Tree mode

| Key | Does |
| --- | --- |
| `↑` `↓`, `k` `j` | move between siblings, and onto `also in` rows |
| `←` `→`, `h` `l` | parent / first child |
| `enter` | on an `also in` row: follow that plugin instead |
| `a` / `d` | add / remove the node — for example a whole plugin |
| `tab`, `esc` | back to search mode; the tree returns to the selected result |

## External references

A marketplace can list a plugin that lives in another repo: `anthropics/claude-plugins-official` lists `cloudflare`, which lives in `cloudflare/skills`. ipm calls that an **external entry**, and keeps repos as the roots of their own trees:

- **In the marketplace's tree**, the entry is drawn `↗cloudflare`, and its description starts with where it points: `↗ cloudflare/skills — Skills for the Cloudflare…`.
- **`→` on it jumps** into `cloudflare/skills`' own tree, on the plugin it points at. A second root sits above the tree, `↩ anthropics/claude-plugins-official — back to cloudflare`: `↑` to it and `enter` takes you back. Jumps stack (up to 20); moving to another search result forgets them.
- **In `cloudflare/skills`' own tree**, the root is `cloudflare/skills`. Who uses it is extra information, not ancestry: the current node says `↗ used by N` (other repos using its repo), and nodes that other repos point at directly say it too.
- **`e`** lists every external reference to the current repo, grouped by the repo it comes from, with stars and marketplace. `enter` jumps there, with the back root to return.
- An external entry whose repo isn't one of your sources says `↗ <repo> (not a source — ipm marketplace add <id>)`; add it and `ipm catalog sync` to follow it.

Being used by other repos is a strong sign a repo is useful, so search also prefers it when results are otherwise equal.

## Build and deploy

1. **Collect.** Search, look at the tree, and `a` what you want. `p`, then `n`, creates a pack.
2. **Build.** `b` runs `ipm pack build`; its output streams in and ends with `✓ installed`.
3. **Deploy.** `D` installs the active pack into Claude Code:
   1. Pick where: **user** (every project, just you), **project** (a folder, shared through its `.claude/settings.json`) or **local** (a folder, just you). The default is project scope in the pack's own project.
   2. For project and local scope, confirm or type the folder (`~` works).
   3. Read the exact commands it will run, then `y` to deploy:
      ```text
      Deploy csharp-sdlc to Claude Code — project scope in /work
        ipm pack build
        claude plugin marketplace add /work/.ipm --scope project
        claude plugin install csharp-sdlc@work-ipm --scope project
      y deploy · esc cancel
      ```
   4. The output streams in and ends with `✓ deployed csharp-sdlc (project /work)`.

Deploy builds first when `.ipm/` is out of date, so Claude Code gets the current pack. Deploying again adds the marketplace again (Claude Code accepts that), refreshes it, and updates the plugin instead of installing it. If Claude Code already has a marketplace of the same name from another folder — two projects in folders with the same name — deploy stops rather than repoint it. Claude Code reads an ipm pack straight from `.ipm/plugins/`, so a later build takes effect at the next session or after `/reload-plugins`.

The same works from the command line with [`ipm install claude`](../reference/commands.md#ipm-install). Like the command line, the TUI records each install, so a bare `ipm install` replays it.

## Without the search server

When the server isn't running, or its index is empty, the TUI searches the catalog by keyword (names count most, then descriptions and tags) and shows a banner saying so. Start it with `ipm catalog serve` and `ipm catalog rebuild` in another terminal, then press `ctrl+r`.
