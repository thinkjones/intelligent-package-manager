---
title: Use the TUI
parent: Guides
nav_order: 3
---

# Use the TUI
{: .no_toc }

Search, judge, collect into a pack and build it, all in one screen.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Start it

In your project:

```bash
ipm tui            # the project's packs (the nearest ipm.toml, or the current folder)
ipm tui -g         # your user-scope packs
```

`ipm tui` needs a terminal and a catalog: run `ipm sync` first. For the best ranking, run `ipm serve` and `ipm load` too; without them the TUI still works, with keyword search.

## The screen

```text
ipm ─ search: c# testing                                            ● hybrid
─────────────────────────────────────────────────────────────────────────────
▶71   agent          ✓ csharp-pro             │ agent  csharp-pro
 65   copilot-plugin   csharp-dotnet-develop… │ wshobson/agents ★ 40.3k
 24   skill          ✓ dotnet-backend-patter… │ agent:wshobson/agents:plugins/…
 15   agent            backend-development-… │ Write modern C# code with …
                                              │
                                              │ ▶up    plugin   jvm-languages
─ csharp-tui (2) • not built · project /work ─────────────────────────────────
agent csharp-pro · skill dotnet-backend-patterns
50 of 100
/ search  a add  d remove  p packs  i install  f type  ? help  q quit
```

- **Header:** your query, and `● hybrid` (the search server) or `○ local` (keyword search over the catalog).
- **Results, left:** SCORE, type, `✓` when the item is already in the active pack, and name.
- **Detail, right:** the selected artifact's type, name, repo, stars, ID, the start of its description, and its relationships: what contains it (up) and what it contains (down).
- **Tray, bottom:** the active pack, its item count, `• not built` once you've changed it since the last install, and its items.
- **Status line:** how many results, or what just happened.

Below 100 columns the detail pane hides, and `enter` shows it full-width. Below 60×15 the TUI asks you to enlarge the terminal.

## Keys

| Key | Does |
| --- | --- |
| typing | search; results refresh as you type |
| `/` | focus the search box |
| `↓`, `tab` from search | focus the results |
| `↑` `↓`, `j` `k` | move through results |
| `→`, `l` | focus the detail pane's relationships |
| `enter` (detail) | open the selected relationship; `backspace` goes back |
| `enter` (results, narrow terminal) | show the detail full-width; `esc` returns |
| `a` | add the selected result or relationship to the active pack |
| `d` | remove the selected item from the active pack |
| `tab` (results) | focus the tray; `tab` again returns to search |
| `p` | switch pack, or `n` for a new one (name, then description) |
| `i` | build every pack (`ipm install`), with live output |
| `f` | cycle the type filter: all, skill, agent, command, plugin |
| `ctrl+r` | check the search server again |
| `?` | every key |
| `q`, `ctrl+c` | quit |

In the search box every letter is part of the query, so `a`, `d`, `p`, `i`, `f`, `?` and `q` work once you've moved to the results (`↓` or `tab`).

## Build a pack

1. Type what you're after, for example `c# testing`.
2. `↓` to the results. Move with `j`/`k`; the detail pane follows. `→` to walk into its plugin or repo and see what else is there; `backspace` to come back.
3. `p`, then `n`, to create a pack: type a name, `enter`, a description, `enter`. It becomes the active pack.
4. `a` on everything you want. Hooks and MCP servers can't be added on their own; the status line names the plugin to add instead.
5. `i` to build. The output streams in, ending in `✓ installed` and the `/plugin install` commands. `esc` closes it.

Everything the TUI does goes through the same code as `ipm pack` and `ipm install`, and writes the same `ipm.toml`, so you can switch between the TUI and the CLI at any time.

## Without the search server

When the server isn't running, or its index is empty, the TUI searches the catalog by keyword (names count most, then descriptions and tags) and shows a banner saying so. Start it with `ipm serve` and `ipm load` in another terminal, then press `ctrl+r`.
