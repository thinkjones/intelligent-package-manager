# ipm Part 3 — Packs and `ipm install`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** in any project (or at user scope with `-g`), `ipm pack new/add/remove/list/show` edits packs of catalog items in `ipm.toml`, and `ipm install` builds one Claude Code plugin per pack in `.ipm/plugins/<pack>/` — picked skills, agents and commands as files (`<pack>:<item>` in Claude), whole plugins as pinned marketplace dependencies — plus `.ipm/.claude-plugin/marketplace.json`, validated with `claude plugin validate --strict`. A teammate who clones the project and runs `ipm install` gets identical plugins without needing your catalog.

**Architecture:** `internal/project` finds the scope root and reads/writes `ipm.toml` and `.ipm/ipm.lock`. `internal/assemble` turns resolved items into a build plan (files to place, dependency entries, provenance) and writes it. pnpm (part 2's `findPnpm`, `writePnpmfile`) fetches the pinned repos into `.ipm/node_modules`; ipm hard-links (else copies) the picked files into the plugin. `internal/marketplace` (kept from the old flow) writes the marketplace.

**Tech Stack:** Go 1.26, BurntSushi/toml (existing), pnpm ≥ 12 via part 2's helpers, `claude` CLI for validation.

**Spec:** `docs/superpowers/specs/2026-10-09-ipm-design.md` (Built plugin, Commands, Parts → 3). Deviations below are recorded in Task 1 and the spec updated.

## Global Constraints

- Pickable item by item: `skill`, `agent`, `command` (file-based, not `#name` inline ones), `copilot-plugin` (expanded to its skills); whole plugins: `plugin` (local `…:<dir>`, inline `…:<dir>#<name>`, external `…#<name>`). Not pickable: `hook`, `mcp`, inline commands, `repo`, `marketplace` — `pack add` refuses and names the owning plugin(s) to add instead (from the record's `plugins`).
- Scope root: project = nearest ancestor of the working directory containing `ipm.toml` (`pack new` creates it in the working directory if none); user = `$IPM_HOME/global` with `-g`.
- Pins: `.ipm/package.json` has one dependency per source repo, `github:Owner/Repo#<40-hex sha>` (alias as in part 2). A repo already pinned keeps its pin on `ipm install`; `ipm install --update` re-pins every repo to the commit in your current catalog. Non-GitHub repos: `git+<url>#<sha>`.
- `.ipm/ipm.lock` (TOML, committed): one `[[item]]` per picked item — `id`, `type`, `name`, `repo`, `path`, `sha`, and for whole plugins the marketplace `source` (JSON) — sorted by id. `install` resolves items from the lock first, then the catalog; an item in neither → error naming it and `ipm sync`.
- Files are placed by hard link, falling back to copy across volumes (`os.Link` → copy). Skills are folders; agents and commands single `.md` files.
- Names inside a pack: the item's own `name`; a clash within one pack type gets `-<repo owner lowercased>` appended, then `-2`, `-3`.
- Generated `plugin.json`: `name` = pack, `description`, `version` `"0.0.0-ipm"`, `author.name` (git `user.name` or `"ipm"`), `dependencies` (whole-plugin entry names), `metadata.ipm.items` = `[{id, type, name, repo, path, sha, file}]`. Whole-plugin entries in the marketplace are named `<pack>-<slug>` and tagged `ipm`; the pack entry is `source: "./plugins/<pack>"`.
- Marketplace: `.ipm/.claude-plugin/marketplace.json`, name `<project dir name>-ipm` (user scope: `ipm-global`), owner = git `user.name` or `ipm`, every entry tagged `ipm`.
- Committed vs ignored: `ipm.toml`, `.ipm/package.json`, `.ipm/pnpm-lock.yaml`, `.ipm/ipm.lock`, `.ipm/.pnpmfile.cjs`, `.ipm/.claude-plugin/marketplace.json` committed; `ipm install` writes `.ipm/.gitignore` containing `node_modules/` and `plugins/`.
- pnpm runs exactly as in part 2 (`--ignore-scripts`, `--config.pnpmfile`, `--config.ignore-pnpmfile=false`, `--no-frozen-lockfile`, dedicated `--store-dir $IPM_HOME/store`), with `--dir <root>/.ipm`.
- Gate: `mise run cli:test`. Commit per task.

## Review Focus

- Two picked skills with the same `name` from different repos in one pack — expect distinct folder names (`-<owner>`), both loading. Test in Task 4.
- An `ipm.toml` item that is no longer in the catalog and not in `ipm.lock` (upstream deleted it) — expect `install` to name the item and suggest `ipm pack remove`, writing nothing. Test in Task 5.
- `.ipm/` on a different volume from `$IPM_HOME` (hard link fails with EXDEV) — expect a copy, same result. Test in Task 4 (simulate with a link function that fails).
- A pack name that Claude Code reserves (`claude-…`, `anthropic-…`, `official` next to `claude`) — expect `pack new` to refuse. Test in Task 3.
- Running `ipm install` twice — expect byte-identical `.ipm` outputs (deterministic marketplace, plugin.json, lock) and no stale files left in `plugins/<pack>/` from removed items. Test in Task 5.

---

### Task 1: Spike and spec update

- [ ] **Step 1:** In a scratch dir, hand-build `.ipm/` from the live store: `plugins/csharp-sdlc/` with `skills/dotnet-backend-patterns/` and `agents/csharp-pro.md` and `commands/accessibility-audit.md` hard-linked from `~/Library/ipm/sources/node_modules/wshobson--agents/…`, a `plugin.json` as specified (with `metadata.ipm`, a dependency on `csharp-sdlc-stripe`), and a marketplace with the pack entry plus `csharp-sdlc-stripe` (the stripe entry's own pinned `git-subdir` source). `claude plugin validate --strict .ipm`; in `CLAUDE_CONFIG_DIR=$(mktemp -d)`: `claude plugin marketplace add <dir>/.ipm`, `claude plugin install csharp-sdlc@<name>`, `claude plugin details csharp-sdlc@<name>` → 1 skill, 1 agent, the command (shown as a skill), stripe installed as a dependency. Record in the ledger; hard links must not break validation.
- [ ] **Step 2:** Update the spec's Built plugin / Layout sections: per-repo pins (not per-item `path:` deps — the part 2 fetcher hook drops `path:`), `.ipm/ipm.lock`, hard link → copy (no APFS clone; no new dependency), `install --update`. Commit `docs: ipm part 3 spec details`.

### Task 2: `internal/project` — scope root, `ipm.toml`, `ipm.lock`

**Files:** Create `cli/internal/project/project.go`, `project_test.go`.

**Produces:** `type Scope struct { Root string; Global bool }` with `IPMDir()` (`Root/.ipm`); `func Find(cwd string, global bool, home string) (Scope, bool, error)` (found=false when no `ipm.toml` up the tree; global → `home/global`); `type Pack struct { Name, Description string; Items []string }`; `func LoadPacks(root) ([]Pack, error)` (sorted by name; missing file → none), `func SavePacks(root, []Pack) error` (`[packs.<name>]` tables, description then items, stable); `type LockItem struct { ID, Type, Name, Repo, Path, SHA, Source string }`; `func LoadLock(ipmDir) (map[string]LockItem, error)`, `func SaveLock(ipmDir, map[string]LockItem) error` (sorted by id).

- [ ] **Step 1:** Tests: `TestFindWalksUp` (ipm.toml two levels up found; none → found=false, Root=cwd; `-g` → home/global), `TestPacksRoundTrip` (bytes stable, order kept), `TestLockRoundTrip`, `TestLoadPacksRejectsUnknownKeys`. Run → FAIL.
- [ ] **Step 2:** Implement. Run → PASS. Commit `feat(project): scope root, ipm.toml and ipm.lock`.

### Task 3: `ipm pack`

**Files:** Create `cli/internal/cli/packcmd.go`; modify `cli.go` (register `{"pack", "create and edit packs of catalog items in ipm.toml (-g for your user scope)", "<new|add|remove|list|show> …", packFlags}`); tests in `cli_test.go`.

- `pack new <name> [--description …]`: validate `pack.ValidName` and not `pack.Reserved`; refuse an existing pack.
- `pack add <pack> <id>…`: each ID must be in the catalog (`catalog.Load` of every type, or `catgraph.Load`); refuse unpickable types — for hooks/MCP/inline commands say "comes only with its plugin: `ipm pack add <pack> <plugin id>`" listing the record's `plugins`; for repos/marketplaces say what to pick instead; duplicates ignored; order kept.
- `pack remove <pack> <id>…`; `pack list` (pack, description, item count); `pack show <pack>` (table: TYPE NAME REPO ★ ID, from the catalog, falling back to `ipm.lock`).
- `-g` flag on all actions.

- [ ] **Step 1:** Tests: `TestPackNewAddShow`, `TestPackNewRefusesReservedNames` (`claude-tools`, `official-claude`), `TestPackAddRefusesUnpickable` (hook → message names its plugin id), `TestPackAddUnknownID`, `TestPackRemove`, `TestPackGlobalScope` (`-g` writes `$IPM_HOME/global/ipm.toml`), `TestPackWalksUpToProject`. Run → FAIL.
- [ ] **Step 2:** Implement. `mise run cli:test` → PASS. Commit `feat(cli): ipm pack`.

### Task 4: `internal/assemble` — build plan and placement

**Files:** Create `cli/internal/assemble/assemble.go`, `assemble_test.go`.

**Consumes:** `project.Pack`, `project.LockItem`, `sources.Alias`, `marketplace.Entry/Source/SourceFor`, `pack.NormalizeRepo`.

**Produces:**
- `func Resolve(packs []project.Pack, lock map[string]project.LockItem, catalogDir string) (map[string]project.LockItem, error)` — lock first, then catalog records (copilot-plugins expand to their skills' lock items); unknown → error naming the item and `ipm pack remove`.
- `type Plan struct { Repos map[string]string /* alias → spec at sha */; Packs []PackPlan }`, `type PackPlan struct { Name, Description string; Files []Placed; Deps []marketplace.Entry; Items []project.LockItem }`, `type Placed struct { From /* repo-relative */, To /* plugin-relative */, Alias string; Dir bool }`.
- `func Build(packs []project.Pack, items map[string]project.LockItem) Plan` — names and clashes per the constraints; whole plugins → `marketplace.Entry` (`<pack>-<slug>`): local → `SourceFor(repo, path, "", sha)`, external → the recorded upstream `source`, inline → git-subdir plus the entry's component fields.
- `func Place(plan PackPlan, nodeModules, pluginDir string, link func(old, new string) error) error` — clears `pluginDir` first (no stale files), hard-links via `link`, copies on any link error.

- [ ] **Step 1:** Tests: `TestResolveFromLockThenCatalog`, `TestResolveUnknownItem`, `TestBuildNamesAndClashes` (two `code-review` skills → `code-review`, `code-review-mattpocock`), `TestBuildWholePlugins` (local/external/inline entries exact), `TestBuildExpandsCopilotPlugin`, `TestPlaceLinksElseCopies` (link func failing → files copied, same bytes), `TestPlaceRemovesStaleFiles`. Run → FAIL.
- [ ] **Step 2:** Implement. Run → PASS. Commit `feat(assemble): resolve picks and place plugin files`.

### Task 5: `ipm install`

**Files:** Create `cli/internal/cli/install.go`; modify `cli.go` (register `{"install", "build each pack in ipm.toml into a Claude Code plugin under .ipm/ (-g for your user scope)", "", installFlags}` with `-g`, `--update`, `--no-validate`); tests.

Flow: find scope → load packs, lock → `assemble.Resolve` → write `ipm.lock` → `.ipm/package.json` (keep existing pins unless `--update`; root name via `sources.RootName`-style generation) + `.pnpmfile.cjs` + `.gitignore` → pnpm install (`findPnpm`) → `Build` → `Place` each pack → write each `plugin.json` → marketplace via `marketplace.Build/Write` → `claude plugin validate --strict <.ipm>` (skip with a note if `claude` missing or `--no-validate`) → print `claude plugin marketplace add <abs .ipm>` and `/plugin install <pack>@<marketplace>` per pack.

- [ ] **Step 1:** Tests through the fake `System` with a fixture catalog and a fake `.ipm/node_modules` (fake pnpm): `TestInstallBuildsPlugin` (files placed, plugin.json fields and metadata, marketplace entries, lock written, `.gitignore`), `TestInstallKeepsPinsUnlessUpdate`, `TestInstallFromLockWithoutCatalog`, `TestInstallUnknownItem` (writes nothing), `TestInstallIsDeterministic` (run twice → identical bytes, no stale files), `TestInstallValidates` (claude called with `plugin validate --strict`; failure → exit 1), `TestInstallGlobal`. Plus a real-pnpm e2e in `cmd/ipm` (skipped without pnpm): local `file://` source with a skill and an agent, `ipm sync`, `pack new/add`, `ipm install --no-validate` → `.ipm/plugins/<pack>/skills/<x>/SKILL.md` and `agents/<y>.md` exist.
- [ ] **Step 2:** Implement. `mise run cli:test` → PASS. Commit `feat(cli): ipm install builds pack plugins`.

### Task 6: Live run and docs

- [ ] **Step 1:** Live, in a scratch project dir: `ipm pack new csharp-sdlc`; add `skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns`, `agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md`, `command:wshobson/agents:plugins/accessibility-compliance/commands/accessibility-audit.md`, `plugin:anthropics/claude-plugins-official#stripe`; try adding a hook (refused, names its plugin); `ipm install` → validate passes; sandbox install as in Task 1 → `claude plugin details csharp-sdlc@…` shows the skill, the agent and the command under `csharp-sdlc`, stripe installed as a dependency. Second `ipm install` → no changes. Record in the ledger.
- [ ] **Step 2:** Docs: `README.md`, `AGENTS.md`, `CLAUDE.md` (local), `cli/README.md`, `docs/architecture.md`, `docs/user-guide.md` (a "Build your own plugin" section), `docs/sources-reference.md` (link), and remove "part 3 not built" notes. Commit `docs: packs and ipm install`.
