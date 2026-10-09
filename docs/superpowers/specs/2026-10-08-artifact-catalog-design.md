# Artifact catalog — stage 1 design

Status: draft for review · 2026-10-08

## Intent

A local catalog of every repo, marketplace, plugin, skill, agent, command, hook and MCP server in the upstreams this repo watches, so a user can find artifacts, see where each came from and how it relates to others, and later assemble a bespoke plugin from them — by reference, never by copying folders.

The work is three stages, each with its own spec and plan:

1. **Catalog** (this spec): deterministic extraction from cached upstreams into committed JSONL files, GitHub stats, and promoting external repos into the root manifest.
2. **Search**: `aihs search` loads the catalog into a local Typesense index (built-in local embedding models, hybrid keyword + vector search, facets, joins). Typesense is a disposable index; the committed files are the record.
3. **Dashboard and packs**: a local dashboard to search, view relationships and stars, and save artifacts into named packs (`packs/<name>.toml`, committed), each listing artifact IDs at their commits.

Decisions already made: Dolt is not used — the catalog is rebuildable from the cache and `aihs.lock`, history is `git diff` of the committed JSONL, and packs are committed files. Marketplace entries that point at other repos are cataloged but not cloned; any such repo can be promoted into the root `manifest.toml` to be scanned in full.

### Success criteria

- `aihs index` on an unchanged `aihs.lock` produces byte-identical `catalog/*.jsonl`.
- Every artifact records its repo, commit and path, its source metadata verbatim, and its plugin relationships in both directions.
- Plugin ownership matches what Claude Code would load from that plugin.
- External marketplace plugins are searchable by name and description and promotable with one command.

## Inputs

- The root `manifest.toml`: `[[repo]]` entries (the repos of interest), pinned in `aihs.lock` and cached under `.aihs/cache/<owner>/<repo>/<commit>/` by `aihs sync`.
- GitHub's API, through `gh api`, for repo stats (`aihs stats` only).

`aihs index` reads only the lock and the cache. A repo of interest the lock does not pin, or the cache does not hold at its locked commit, is an error naming the repo and saying to run `aihs sync` (as `upstream.Checkout` already reports).

## Data model

Every record is one JSON object. IDs are stable across commits, so a changed artifact shows as a changed line in `git diff`.

| Type | ID | `name`, `description` from | `tags` from |
| --- | --- | --- | --- |
| `repo` | `repo:<owner/repo>` | repo name; description lives in `github.jsonl` | — (topics live in `github.jsonl`) |
| `marketplace` | `marketplace:<repo>:<dir>` | `marketplace.json` `name`, `metadata.description` / `description` | — |
| `plugin` | `plugin:<repo>:<dir>` | `.claude-plugin/plugin.json` | `keywords`, `category` |
| `plugin` (inline) | `plugin:<repo>:<dir>#<entry name>` — the name is part of the ID because several entries may share one source folder | the marketplace entry | entry `category`, `keywords`, `tags` |
| `plugin` (external) | `plugin:<marketplace repo>#<entry name>` | the marketplace entry | entry `category`, `keywords`, `tags` |
| `copilot-plugin` | `copilot-plugin:<repo>:<dir>` | `plugin.json` | `keywords` |
| `skill` | `skill:<repo>:<dir>` | `SKILL.md` frontmatter | `category`, `metadata.tags` |
| `agent` | `agent:<repo>:<file>` | frontmatter | — |
| `command` | `command:<repo>:<file>` | frontmatter `description`; name is the file stem | — |
| `hook` | `hook:<repo>:<file>` | owning plugin's name (else the file's dir); description lists the events | event names, as written |
| `mcp` | `mcp:<repo>:<file>#<server>` | server name; description is transport plus command or URL | — |

`<dir>` and `<file>` are paths relative to the repo root, `/`-separated; `.` is the root. `<repo>` is the `[[repo]]` value as written (GitHub `owner/repo` or a URL).

### Common fields

| Field | Meaning |
| --- | --- |
| `id`, `type`, `name`, `description`, `tags` | as in the table; `tags` sorted, deduplicated |
| `repo`, `commit`, `path` | where it lives; `commit` from `aihs.lock` |
| `meta` | the frontmatter or JSON object verbatim, converted to plain JSON types (YAML timestamps become strings) |
| `content_hash` | `sha256` of the source file's bytes (LF-normalized); for MCP servers and inline plugins, of their canonical JSON |
| `plugins` | on skill, agent, command, hook and mcp records: IDs of every plugin that owns it, sorted |
| `errors` | present only when parsing failed: messages; `meta` then holds `{"raw": "<text>"}` |

### Type-specific fields

| Type | Fields |
| --- | --- |
| `repo` | `followed` (true for repos of interest, false for external sources); for followed repos, `commit` and `counts` (artifacts per type) |
| `marketplace` | `plugins`: IDs of the plugin records its entries resolve to, in entry order |
| `plugin` | `skills`, `agents`, `commands`, `hooks`, `mcp`: owned component IDs, sorted; `marketplaces`: IDs listing it; `version`, `author`, `license` from its JSON; `entry`: the marketplace entry verbatim, when one lists it (for inline and external plugins, the entry is their only source); external plugins add `source_repo` (owner/repo or URL) and no `commit` or `path` |
| `copilot-plugin` | `skills`: owned skill IDs |
| `hook` | `events`: event names, sorted |
| `mcp` | `transport` (`stdio`, `http`, `sse`), `command` and `args`, or `url` |

## Extraction

`internal/catalog` walks each repo of interest's cached checkout, skipping `.git` and `node_modules`, and finds:

| Type | Marker |
| --- | --- |
| skill | a directory holding `SKILL.md`, anywhere |
| agent | a `*.md` file (not `README.md`) at any depth under a directory named `agents` |
| command | a `*.md` file (not `README.md`) directly in a directory named `commands` |
| hook | a `hooks/hooks.json` file; or a `hooks` path or inline object in a `plugin.json` |
| mcp | a `.mcp.json` file (servers under `mcpServers`, or a top-level server map); or `mcpServers` in a `plugin.json` (a path or inline object); one record per server |
| plugin | `.claude-plugin/plugin.json`; the plugin's dir is the parent of `.claude-plugin` |
| copilot-plugin | a `plugin.json` whose parent directory name does not start with `.`; skills from its `com.github.awesome-copilot` extension, resolved as `internal/copilot` does |
| marketplace | `.claude-plugin/marketplace.json` |

Codex and Cursor plugins (`.codex-plugin/`, `.cursor-plugin/`) are not cataloged.

Frontmatter is the YAML between a leading `---` line and the next `---` line. A file without it yields a record with empty `meta` and, for skills, an `errors` entry.

### Plugin ownership

A plugin owns a component when the component lies on one of the plugin's component paths, resolved relative to the plugin's dir. The rules follow the Claude Code plugin manifest reference (`code.claude.com/docs/en/plugins-reference`, "How each key combines with its default location"), confirmed 2026-10-09:

| Component | Default path | Declared in `plugin.json` | Combines with the default |
| --- | --- | --- | --- |
| skills | `skills/` — each direct subdirectory holding `SKILL.md` (one level, as Claude Code scans it; nested skills must be declared); a plugin with `SKILL.md` at its own root, no `skills/` and no `skills` key is one skill | `skills`: a path or array of paths (`"."` / `"./"` = plugin root), each a skill dir or a dir of direct skill dirs | **adds** |
| agents | `agents/` — `*.md` at any depth (subfolders are part of the agent name) | `agents`: a path or array of `.md` files | **replaces** |
| commands | `commands/*.md` | `commands`: a path or array (files or dirs), or an object map of name → `{source}` or `{content}` | **replaces** |
| hooks | `hooks/hooks.json` | `hooks`: a path, an inline object, or an array of either | **merges** (both own) |
| mcp | `.mcp.json` | `mcpServers`: a `.json` path, an inline map, or an array of either; `.mcpb`/`.dxt` bundles and bundle URLs are not cataloged (warning) | **merges** (both own) |

A `commands` map entry with `source` owns that file's command record; one with `content` becomes an inline command record `command:<repo>:<plugin.json path>#<name>`. Inline hook objects in one `plugin.json` form one record `hook:<repo>:<plugin.json path>` whose events are their union. Being under the plugin's dir without being on a component path does not confer ownership. A component may be owned by several plugins; both sides record the link. A component owned by none has `plugins: []`.

A copilot-plugin owns the skills its extension lists.

### Marketplace entries

Each entry of `marketplace.json`'s `plugins` resolves by its `source`:

| `source` | Result |
| --- | --- |
| a relative path (string) to a dir with `.claude-plugin/plugin.json` | a link to that plugin record, the entry kept in its `entry` field; unless `strict: false`, the entry's `skills`, `agents`, `commands` are appended to the plugin's own and its `hooks` (inline object only) owned too |
| a relative path to a dir without one | an inline plugin record `plugin:<repo>:<dir>#<entry name>`: the entry is its manifest, with the same rules as `plugin.json` — except that when the source is the marketplace's own dir and the entry lists `skills`, only those load (the default `skills/` is not scanned) |
| an object (`github`, `url`, `git-subdir`, …) | an external plugin record; `source_repo` from `repo` or `url`; also a `repo:` record with `followed: false` for that source, unless it is a repo of interest |

Relative paths resolve against the marketplace's dir (the parent of its `.claude-plugin`, the repo root for a root marketplace), joined with `metadata.pluginRoot` when set. An entry that resolves to nothing (a missing dir) is a warning, not a record.

## Commands

All Go, in `cli/internal/cli`, registered in the command table, reaching git and `gh` only through `System`.

### `aihs index`

Loads the repos of interest, checks each is pinned and cached, extracts, links, and writes `catalog/`. Prints a count per type and each warning (`! <id or path>: <message>`), then a summary line. Exits 0 when the files are written, warnings included; 1 when a repo is unpinned or uncached or a file cannot be written. Writes nothing on error.

### `aihs stats`

For every `repo:` record in `catalog/repos.jsonl` that is a GitHub `owner/repo` (or a `https://github.com/…` URL), runs `gh api repos/<owner>/<repo>` and records `stars`, `forks`, `description`, `topics`, `license` (SPDX id), `pushed_at`, `archived`, `default_branch`, `fetched_at`. Non-GitHub repos are skipped with a note. A failed repo keeps its previous `github.jsonl` record and is reported; the command exits 1 if any failed. Requires `gh` on PATH and logged in; otherwise it says so and exits 1. Runs four requests at a time.

### `aihs promote <repo>...`

Appends a `[[repo]]` entry per argument to the root `manifest.toml`, as text, so existing comments and order are kept. Refuses (exit 1, nothing written) a repo already listed. Accepts `owner/repo`, `https://github.com/owner/repo(.git)`, or an external plugin ID (`plugin:<repo>#<name>`, promoting its `source_repo`). Prints the next step: `aihs sync && aihs index`. Stage 3's dashboard calls the same function.

### Removed and kept

`aihs repos` is removed: it reports on packs, which no longer exist. `aihs scan` stays as the quick summary.

## Files

```text
catalog/
  repos.jsonl  marketplaces.jsonl  plugins.jsonl  copilot-plugins.jsonl
  skills.jsonl  agents.jsonl  commands.jsonl  hooks.jsonl  mcp.jsonl
  github.jsonl
```

One record per line, sorted by `id`, object keys sorted (Go's `encoding/json` sorts map keys; records are built as maps or as structs with fields in sorted order), no HTML escaping, trailing newline. `aihs index` rewrites every file but `github.jsonl`; `aihs stats` writes only `github.jsonl`. All are committed. A file whose type has no records is written empty, so its absence never means "not indexed".

## Code layout

| Path | Responsibility |
| --- | --- |
| `cli/internal/catalog/walk.go` | find markers in a checkout |
| `cli/internal/catalog/parse.go` | frontmatter and JSON parsing into records |
| `cli/internal/catalog/link.go` | plugin ownership and marketplace entry resolution |
| `cli/internal/catalog/write.go` | deterministic JSONL output |
| `cli/internal/github/github.go` | stats via a `gh` runner |
| `cli/internal/cli/index.go`, `stats.go`, `promote.go` | commands |

New dependency: `gopkg.in/yaml.v3` for frontmatter.

## Testing

All under `mise run cli:test`, in-process with the existing fake `System`.

- Fixture checkouts built in temp dirs (as `cli_test.go` does), indexed, compared to expected JSONL.
- Ownership: default paths; declared paths; a skill owned by two plugins; a skill under a plugin's dir but off its component paths (not owned); skills nested in category folders.
- Marketplace entries: local plugin link, inline plugin, external plugin creating a `followed: false` repo record, missing dir warning.
- Parsing: bad YAML and bad JSON yield `errors` and keep going; YAML timestamps become strings; MCP from `.mcp.json` both shapes and from `plugin.json` inline.
- Determinism: index twice, files byte-identical.
- `stats`: fake `gh` output; failure keeps the previous record and exits 1; `gh` missing.
- `promote`: comments kept; duplicate refused; external plugin ID resolves to its source repo.
- The real catalog: after implementation, `aihs index` runs over the 19 repos of interest with zero errors other than upstream files that are genuinely malformed, each listed in the plan's verification step.

## Docs

`AGENTS.md`, `CLAUDE.md`, `cli/README.md`, `docs/architecture.md` and `docs/manifest-reference.md` gain the catalog, its commands and files; references to packs as the publishing unit are updated to the root manifest and catalog.

## Out of scope for stage 1

Typesense, embeddings, `aihs search` (stage 2); the dashboard and `packs/` (stage 3); following external repos automatically; Codex and Cursor plugins; indexing skill bodies (frontmatter only, as agreed).
