# User guide

How to set up `ipm`, choose the repos it watches, keep them current, search what they hold, and build your own Claude Code plugin from what you find. For the sources files and rules see the [sources reference](sources-reference.md); for how it works inside see the [architecture](architecture.md).

## Concepts in one minute

- A **source** is a repo of interest: a GitHub `owner/repo` or any git URL, optionally at a tag, branch or commit. Your sources are one pnpm project in your ipm home, one git dependency each.
- **`ipm sync`** has pnpm fetch and pin every source to a commit, then **`ipm index`** builds the **catalog**: one record per repo, marketplace, plugin, Copilot plugin, skill, agent, command, hook and MCP server, with its commit and links to what owns it.
- **`ipm serve`** and **`ipm load`** put the catalog into a local Typesense search server; **`ipm search`** queries it, and **`ipm show`**, **`tree`** and **`graph`** explore an artifact's relationships.
- A **pack** is a named set of picked items — skills, agents, commands and whole plugins — in a project's `ipm.toml`. **`ipm install`** builds each pack into a Claude Code plugin and a local marketplace under `.ipm/`, pinned to the commits you picked from.
- Nothing upstream is modified or executed, and `ipm` installs nothing into Claude Code: you add the marketplace it builds yourself.

## Set up

You need Go and [direnv](https://direnv.net) to build `ipm`, pnpm 12 or later or Node (then `ipm` runs `npx -y pnpm@12.10.1`), Docker for search, and the GitHub CLI `gh` for stats.

```bash
cd ai-harness-skills
direnv allow          # once: .envrc puts bin/ on PATH; bin/ipm builds the CLI on demand
ipm doctor            # home, cache, pnpm, sources, catalog, docker, typesense, claude
```

`ipm` keeps its data in your user folders, not in the checkout, so commands work from any directory:

- **Home** (`IPM_HOME`): your sources and ipm's pnpm store. macOS `~/Library/ipm`; Linux `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm`; Windows `%LOCALAPPDATA%\ipm`.
- **Cache** (`IPM_CACHE_DIR`): the catalog and search data, all rebuildable. macOS `~/Library/Caches/ipm`; Linux `$XDG_CACHE_HOME/ipm` or `~/.cache/ipm`.

Set either variable to an absolute path to move it.

## Add sources

```bash
ipm source add obra/superpowers                 # a GitHub repo, default branch
ipm source add anthropics/skills@main           # at a branch, tag or commit
ipm source add https://gitlab.com/me/skills.git#v1
ipm source add 'plugin:anthropics/claude-plugins-official#stripe'   # the repo an external plugin lives in
ipm source list                                 # alias, repo, ref, pinned commit
ipm source remove anthropics/skills
```

`add`, `remove` and `import` only edit `$IPM_HOME/sources/package.json`; run `ipm sync` afterwards. An external plugin ID only works once that plugin is in your catalog (find it with `ipm search --type plugin`).

### Coming from aihs

If you used `ipm` when it was `aihs`, import the old list of repos of interest once:

```bash
ipm source import path/to/old/checkout/manifest.toml
ipm sync
```

It adds each `[[repo]]` entry (a `version` becomes the ref) and copies the old `catalog/github.jsonl` beside it into your cache, so GitHub stats need not be refetched. The old `.aihs/` folder in a checkout is no longer used and can be deleted.

## Sync and index

```bash
ipm sync              # pnpm install, then ipm index
ipm sync --upgrade    # pnpm update --latest: every source to the latest commit of its branch or tag
ipm index             # rebuild the catalog without fetching
ipm stats             # refresh GitHub stars, topics and descriptions (needs gh auth login)
```

`ipm sync` prints the pnpm command it runs, always with `--ignore-scripts`. If pnpm fails, its output is shown, `ipm` exits 1, and the catalog is left as it was. `ipm index` writes `catalog/*.jsonl` in the cache; `ipm stats` writes `catalog/github.jsonl` beside it.

To see what a source holds without searching, `ipm scan` reports each synced source's commit and its plugins, Copilot plugins, marketplaces, and `skills`, `hooks` and `agents` folders; `--paths` lists every folder, and `ipm scan owner/repo` limits it to named sources.

## Search

```bash
ipm serve                         # start Typesense in Docker (ipm-typesense, 127.0.0.1:8108)
ipm load                          # rebuild the index from the catalog; rerun after every sync or stats
ipm search c# testing             # hybrid keyword + semantic search, with a SCORE column
ipm search tdd --type skill --repo obra/superpowers --limit 10
ipm search review --explain       # KW and SEM columns: why each hit matched
ipm search hooks --keyword --json # keyword ranking only, one JSON line per hit
ipm serve --stop                  # stop the server; its data is kept
```

Filters: `--type` (repo, marketplace, plugin, copilot-plugin, skill, agent, command, hook, mcp), `--repo` and `--tag`, each repeatable. Paging: `--limit` (max 250), `--page N`, or `--all`. The SCORE is relative within one query; see [Search](architecture.md#search) for how it is computed.

## Explore an artifact

```bash
ipm show 'skill:obra/superpowers:skills/test-driven-development'   # fields, GitHub stats, neighbours (--json for the record)
ipm tree 'repo:obra/superpowers'                                   # what it contains (--depth N, --all)
ipm tree 'skill:obra/superpowers:skills/test-driven-development' --up   # what contains it
ipm graph 'repo:obra/superpowers' --format dot                     # Mermaid by default, or Graphviz
```

IDs are `<type>:<repo>:<path>`, with `#<name>` for items defined inside a file and `plugin:<repo>#<name>` for external plugins. `ipm search` shows them. These commands read only the catalog; they need no search server.

## Build your own plugin

Pick items from your catalog into a pack, and `ipm install` builds the pack into a plugin. This walks through a C# pack made of parts from two repos. It assumes `wshobson/agents` and `anthropics/claude-plugins-official` are among your sources and synced.

### 1. Find the items

Every item is named by its catalog ID. Find them with `ipm search` (the ID is in the results) and check one with `ipm show`:

```bash
ipm search c# --type skill --type agent
ipm search stripe --type plugin
ipm show 'agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md'
```

### 2. Create a pack and add items

Run these in your project. `ipm pack new` creates `ipm.toml` in the working directory unless an ancestor directory already has one, in which case that directory is the project.

```bash
ipm pack new csharp-sdlc --description "C# development lifecycle"
ipm pack add csharp-sdlc \
  skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns \
  agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md \
  command:wshobson/agents:plugins/accessibility-compliance/commands/accessibility-audit.md \
  'plugin:anthropics/claude-plugins-official#stripe'
ipm pack show csharp-sdlc     # TYPE, NAME, REPO, ★ and ID of each item
```

`ipm.toml` now holds:

```toml
[packs.csharp-sdlc]
description = "C# development lifecycle"
items = [
  "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns",
  "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md",
  "command:wshobson/agents:plugins/accessibility-compliance/commands/accessibility-audit.md",
  "plugin:anthropics/claude-plugins-official#stripe",
]
```

What you can pick:

- **Skills, agents and commands** (commands defined in their own file) are placed in your pack's plugin.
- **Copilot plugins** add their skills.
- **Whole plugins** — local, inline or external — become dependencies of your pack and keep their own name, hooks and MCP servers.
- **Hooks, MCP servers and inline commands** can't be picked on their own; `ipm pack add` refuses and prints the `ipm pack add <pack> <plugin id>` that adds the plugin owning them. Repos and marketplaces are refused too; `ipm tree <id>` shows what they hold.

Pack names use letters, digits, `.`, `_` and `-`; names Claude Code reserves (`claude-…`, `anthropic-…`, `official` beside claude or anthropic) are refused. `ipm pack remove <pack> <id>…` takes items out and `ipm pack list` lists the packs.

### 3. Build it

```bash
ipm install
```

`ipm install` resolves each item to a repo, path and commit and records it in `.ipm/ipm.lock`, has pnpm fetch those commits into `.ipm/node_modules`, places the picked files in `.ipm/plugins/csharp-sdlc/` with a `plugin.json`, writes the marketplace `.ipm/.claude-plugin/marketplace.json`, and runs `claude plugin validate --strict` (skipped with a note when `claude` is not on PATH, or with `--no-validate`). It ends by printing the commands to install it.

The marketplace is named after the project directory, `<project dir>-ipm`. It lists `csharp-sdlc` and, as its dependency, `csharp-sdlc-stripe`, pinned to the commit you picked from.

### 4. Install it in Claude Code

```bash
claude plugin marketplace add <project>/.ipm
```

Then in Claude Code run `/plugin install csharp-sdlc@<project dir>-ipm`. Claude Code installs `csharp-sdlc-stripe` with it. Your picks appear under the pack's name: the skill and the command (commands show as skills) as `csharp-sdlc:<name>`, alongside the `csharp-pro` agent. The stripe plugin keeps its own namespace.

### Keep it current and share it

- `ipm install` keeps every item at the commit in `.ipm/ipm.lock`. `ipm install --update` re-pins every item to the commit in your current catalog; run `ipm sync --upgrade` first to move your sources forward.
- After `ipm pack add` or `remove`, rerun `ipm install`. Items you remove leave no files behind, and running it twice gives identical output.
- Commit `ipm.toml`, `.ipm/ipm.lock`, `.ipm/package.json`, `.ipm/pnpm-lock.yaml`, `.ipm/.pnpmfile.cjs` and `.ipm/.claude-plugin/marketplace.json`. `ipm install` writes `.ipm/.gitignore` so `.ipm/node_modules/` and `.ipm/plugins/` stay out. A teammate with `ipm` runs `ipm install` from those files; they don't need your sources or catalog.
- Don't edit files in `.ipm/plugins/`: they are hard links to pnpm's copies, so an edit changes pnpm's copy too, and `ipm install` rebuilds the folder. Change the item upstream or in a repo of your own and pick it from there.

### Packs for every project

Add `-g` to any `ipm pack` command or `ipm install` to use your user scope, `$IPM_HOME/global` (`~/Library/ipm/global` on macOS), instead of a project. Its marketplace is `ipm-global`, at `$IPM_HOME/global/.ipm`.

## Troubleshooting

| Message | Cause and fix |
| --- | --- |
| `ipm fetches sources with pnpm 12 or later …` | No pnpm and no `npx`. Install pnpm 12+ or Node. |
| `ipm needs pnpm 12 or later (found …)` | pnpm on PATH is too old and `npx` is missing. Upgrade pnpm, or install Node. |
| `pnpm failed — the catalog is unchanged` | pnpm's output above says why (often network, or a ref that does not exist). Fix it and rerun `ipm sync`. |
| `no sources yet — add some with ipm source add owner/repo` | Add a source first. |
| `sources are not synced yet`, `… has no pinned commit in pnpm-lock.yaml`, `… is not installed` | Run `ipm sync`. |
| `… is already a source` | It is listed already; `ipm source list` shows it. |
| `… and … would both install as "<alias>" — remove one first` | Two repos differ only in case or punctuation; keep one. |
| `… is not owner/repo or a git URL` | Use `owner/repo[@ref]` or a URL starting `https://`, `ssh://`, `file://` or `git@`. |
| `no plugin plugin:… in the catalog — run ipm index` | The plugin ID is not in your catalog; check it with `ipm search`. |
| `IPM_HOME must be an absolute path` | Use an absolute path (likewise `IPM_CACHE_DIR`). |
| `the search server is not running — run ipm serve` / `no search server set up` | Start it with `ipm serve`. |
| `Docker is not running` / `ipm serve runs Typesense in Docker` | Start or install Docker (or OrbStack). |
| `the search index is empty — run ipm load` | Load the catalog into the server. |
| `the search server rejected the API key` | Another Typesense holds port 8108; check `docker ps`. |
| `gh is not logged in` | Run `gh auth login`, then `ipm stats`. |
| `no packs in … — create one with ipm pack new <name>` | There is no `ipm.toml` here or above (or it has no packs). Run `ipm pack new`, or `cd` into the project, or add `-g`. |
| `no item … in the catalog — find IDs with ipm search` | The ID is mistyped, or its repo is not one of your sources; `ipm source add` it and `ipm sync`. |
| `… comes only with its plugin: ipm pack add …` | Hooks, MCP servers and inline commands come with their plugin; run the command it prints. |
| `… is a repo` / `… is a marketplace` | Pick the items it holds; `ipm tree <id>` lists them. |
| `… can't be a pack name` | Use letters, digits, `.`, `_` and `-`, and avoid `claude-…`, `anthropic-…` and `official`. |
| `… is not in the catalog or ipm.lock — drop it with ipm pack remove …` | An item in `ipm.toml` is in neither (perhaps deleted upstream). Remove it as shown, or `ipm sync` if it is new upstream. |
| `can't resolve … without the catalog` | An item is not in `.ipm/ipm.lock` and your catalog can't be read. Run `ipm sync`. |
| `claude plugin validate --strict failed` | Its output is shown above. Rerun with `--no-validate` to inspect `.ipm/` yourself. |
| A source from a non-GitHub host is missing files | pnpm applies npm's packing rules (`files`, `.npmignore`) to non-GitHub git sources. A known limitation; use the GitHub mirror if there is one. |
