# User guide

How to set up `ipm`, choose the repos it watches, keep them current, and search what they hold. For the sources files and rules see the [sources reference](sources-reference.md); for how it works inside see the [architecture](architecture.md).

## Concepts in one minute

- A **source** is a repo of interest: a GitHub `owner/repo` or any git URL, optionally at a tag, branch or commit. Your sources are one pnpm project in your ipm home, one git dependency each.
- **`ipm sync`** has pnpm fetch and pin every source to a commit, then **`ipm index`** builds the **catalog**: one record per repo, marketplace, plugin, Copilot plugin, skill, agent, command, hook and MCP server, with its commit and links to what owns it.
- **`ipm serve`** and **`ipm load`** put the catalog into a local Typesense search server; **`ipm search`** queries it, and **`ipm show`**, **`tree`** and **`graph`** explore an artifact's relationships.
- Nothing upstream is modified or executed, and `ipm` installs nothing into Claude Code. Building your own plugins from picked items is coming in part 3 of [the design](superpowers/specs/2026-10-09-ipm-design.md).

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
| A source from a non-GitHub host is missing files | pnpm applies npm's packing rules (`files`, `.npmignore`) to non-GitHub git sources. A known limitation; use the GitHub mirror if there is one. |
