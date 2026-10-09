# ipm Part 2 — Sources and relocation on pnpm

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `ipm` stops depending on this checkout. Your repos of interest live in `$IPM_HOME/sources/package.json`, pnpm fetches and pins them, the catalog and search data live in your caches folder, and every command works from any directory.

**Architecture:** a new `internal/paths` resolves `IPM_HOME`, the cache dir and everything under them. A new `internal/sources` owns `sources/package.json` (one git dependency per repo, alias `owner--repo`) and reads `pnpm-lock.yaml` for commits. `ipm sync` runs pnpm; `ipm index` walks `sources/node_modules/<alias>`. The `upstream` package (git fetching), `config`/`IPM_ROOT`, `aihs.lock`, the old pack flow and `create-plugin` go; part 3 brings packs back as `ipm install`.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3` (already a dependency) for `pnpm-lock.yaml`, pnpm ≥ 10 at runtime.

**Spec:** `docs/superpowers/specs/2026-10-09-ipm-design.md` — Layout, Commands, Parts → 2.

## Global Constraints

- Paths: `IPM_HOME` overrides the home; else macOS `~/Library/ipm`, Linux `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm`, Windows `%LOCALAPPDATA%\ipm`. Cache: `IPM_CACHE_DIR` overrides; else `$XDG_CACHE_HOME/ipm`, else macOS `~/Library/Caches/ipm`, Linux `~/.cache/ipm`, Windows `%LOCALAPPDATA%\ipm\cache`. Home comes from `HOME` (Windows: `USERPROFILE`) via `System.Getenv` — tests set `IPM_HOME`/`IPM_CACHE_DIR`.
- Under home: `sources/` (package.json, pnpm-lock.yaml, node_modules), `store/`. Under cache: `catalog/*.jsonl` (incl. `github.jsonl`), `typesense/` (data, api-key).
- pnpm always runs with `--ignore-scripts --store-dir <home>/store --dir <home>/sources`; it is reached only through `System.Output`. pnpm missing → error naming pnpm.
- Typesense container renamed `ipm-typesense`; `serve` stops and removes a legacy `aihs-typesense` container if one exists (it holds port 8108).
- Behaviour of `index` output, `search`, `show`, `tree`, `graph`, `stats` is unchanged apart from where files live.
- Gate: `mise run cli:test`. Commit per task.

## Review Focus

- A source whose pnpm lock entry has no 40-hex commit (e.g. a non-GitHub `git+https` source, or a lock not yet written) — expect `index` to name the source and say run `ipm sync`, not crash or index with an empty commit. Test in Task 2.
- Two repos whose aliases collide after lowercasing (`Foo/bar` and `foo/bar`) — expect `source add` to refuse with both names. Test in Task 2.
- `IPM_HOME` set to a relative path — expect an error asking for an absolute path. Test in Task 1.
- `ipm sync` when pnpm fails part-way (network) — expect pnpm's output shown and exit 1, and the previous catalog left intact (index only runs after a successful install). Test in Task 3.
- A legacy `aihs-typesense` container still running on 8108 — expect `serve` to stop and remove it, then start `ipm-typesense`. Test in Task 4.

---

### Task 1: `internal/paths`

**Files:** Create `cli/internal/paths/paths.go`, `paths_test.go`.

**Interfaces — Produces:**
- `type Paths struct { Home, Cache string }` with methods `Sources() string` (`Home/sources`), `Store() string`, `Catalog() string` (`Cache/catalog`), `Typesense() string` (`Cache/typesense`).
- `func Resolve(getenv func(string) string, goos string) (Paths, error)` — rules above; relative `IPM_HOME`/`IPM_CACHE_DIR` → error; missing `HOME` → error.

- [ ] **Step 1:** Tests: `TestResolveDefaults` (darwin → `~/Library/ipm`, `~/Library/Caches/ipm`; linux → `~/.local/share/ipm`, `~/.cache/ipm`; linux with `XDG_DATA_HOME`/`XDG_CACHE_HOME` → those + `/ipm`; windows → `%LOCALAPPDATA%\ipm`, `…\ipm\cache`), `TestResolveOverrides` (`IPM_HOME`, `IPM_CACHE_DIR` win), `TestResolveRejectsRelative`, `TestResolveNeedsHome`. Run `go test ./internal/paths/` → FAIL.
- [ ] **Step 2:** Implement with `path/filepath` (use `filepath.Join`; Windows separator only matters on Windows). Run → PASS. Commit `feat(paths): IPM_HOME and cache dir resolution`.

### Task 2: `internal/sources`

**Files:** Create `cli/internal/sources/sources.go`, `sources_test.go`.

**Interfaces — Consumes:** `paths.Paths`. **Produces:**
- `type Source struct { Alias, Repo, Ref, Spec string }` — `Repo` as written (`Owner/Repo` or a git URL), `Spec` the pnpm specifier (`github:Owner/Repo[#ref]` or `git+https://…[#ref]`).
- `func Alias(repo string) string` — `owner--repo` lowercased, non `[a-z0-9._-]` → `-`.
- `func Load(dir string) ([]Source, error)` (missing `package.json` → empty), `func Add(dir string, repos []string) ([]Source, error)` (accepts `owner/repo[@ref]`, GitHub URLs via `pack.NormalizeRepo`, other git URLs; refuses duplicates and alias collisions, naming both), `func Remove(dir string, repos []string) error`.
- `package.json` shape: `{"name":"ipm-sources","private":true,"dependencies":{<alias>: <spec>}}`, keys sorted, 2-space indent, trailing newline.
- `type Checkout struct { Source; Dir, Commit string }` and `func Checkouts(dir string) ([]Checkout, error)` — for each source: `Dir` = `dir/node_modules/<alias>` (must exist), `Commit` = the 40-hex sha in that dependency's `version` in `pnpm-lock.yaml` (`importers["."].dependencies[alias].version`); missing → error naming the repo and `ipm sync`.

- [ ] **Step 1:** Tests: `TestAlias`, `TestAddLoadRemove` (round trip, `@ref` → `#ref`, GitHub URL normalized, file bytes stable), `TestAddRefusesDuplicateAndCollision`, `TestCheckouts` (fixture `pnpm-lock.yaml` copied from the spike shape with a codeload tarball version and a `#path:` variant → commit parsed; `node_modules/<alias>` created in the test), `TestCheckoutsNeedSync` (no lock / no 40-hex / no node_modules dir → error mentioning `ipm sync`). Run → FAIL.
- [ ] **Step 2:** Implement. Run → PASS. Commit `feat(sources): pnpm package.json of repos of interest`.

### Task 3: `ipm source` and `ipm sync` on pnpm; `index` and `scan` read sources

**Files:** Create `cli/internal/cli/source.go`; rewrite `cli/internal/cli/sync.go`, `index.go`, `scan.go`; modify `cli.go` (command table: add `source` with args `<add|remove|list|import> …`; remove `promote`, `create-plugin`); delete `promote.go`, `createplugin.go`; adapt `cli_test.go`.

**Interfaces — Consumes:** `paths.Resolve`, `sources.*`, `catalog.Build/Write`, `catalog.Load` (for plugin IDs). **Produces:** `func ipmPaths(sys System) (paths.Paths, error)` (`paths.Resolve(sys.Getenv, runtime.GOOS)`), used by every command from here on.

- `ipm source add <repo|plugin-id>…` (plugin IDs resolve to `source_repo` as `promote` did), `remove`, `list` (alias, repo, pinned commit if synced), `import <manifest.toml>` (reads `[[repo]]` entries — `repo`, optional `version` → `@ref` — and adds them; also copies a sibling `catalog/github.jsonl` into the cache if present, so stats aren't refetched).
- `ipm sync [--upgrade]`: `pnpm install` (or `pnpm update --latest` with `--upgrade`) `--ignore-scripts --store-dir … --dir …`; on success run index; on failure print pnpm's output, exit 1, leave the catalog alone.
- `ipm index`: `sources.Checkouts` → `catalog.Checkout{Repo, Commit, Dir}` → `Build` → `Write(paths.Catalog())`.
- `ipm scan [repo…]`: scans synced sources only; a repo that isn't a source → error suggesting `ipm source add`.

- [ ] **Step 1:** Tests through the fake `System`: `TestSourceAddListRemove`, `TestSourceAddPluginID`, `TestSourceImport` (manifest with a `version` and a sibling `catalog/github.jsonl`), `TestSyncRunsPnpm` (exact args recorded; then index writes `<cache>/catalog/*.jsonl` from a fixture `node_modules` + lock), `TestSyncPnpmFailureKeepsCatalog`, `TestSyncUpgrade`, `TestIndexNeedsSync`, `TestScanSources`. Existing index/scan tests move to these fixtures. Run → FAIL.
- [ ] **Step 2:** Implement; delete `promote`/`create-plugin` and their tests. Run `mise run cli:test` → PASS. Commit `feat(cli): ipm source and pnpm-backed sync`.

### Task 4: Catalog, stats and search read the cache; Typesense container rename

**Files:** Modify `stats.go`, `load.go`, `search.go`, `show.go`, `serve.go`, `doctor.go`; tests.

- All `filepath.Join(cfg.Root, "catalog")` → `p.Catalog()`; Typesense dir → `p.Typesense()`; `tsContainer = "ipm-typesense"`; before `docker run`, if `docker inspect aihs-typesense` succeeds → `docker rm -f aihs-typesense` with a printed note.
- `doctor`: rows `home`, `cache`, `pnpm` (version via `pnpm --version`; missing → problem), `sources` (count; unsynced → warning), `catalog` (records; missing → warning), `docker`, `typesense`, `claude`. Pack and lock rows go.

- [ ] **Step 1:** Tests: existing stats/load/search/show/tree/graph/serve tests re-pointed at `IPM_HOME`/`IPM_CACHE_DIR` temp dirs; `TestServeRemovesLegacyContainer`; `TestDoctorRows`. Run → FAIL.
- [ ] **Step 2:** Implement. Run `mise run cli:test` → PASS. Commit `feat(cli): catalog and search data move to the user cache`.

### Task 5: Remove the checkout-bound pieces

**Files:** Delete `cli/internal/upstream/`, `cli/internal/config/`, the old pack flow in `cli/internal/pack/pack.go` (keep `interest.go`'s `NormalizeRepo`/`ManifestFile` only if still used — move `NormalizeRepo` to `sources` if `pack` empties), `marketplace.Entries/stack` and old-flow tests (keep `Source`, `Entry`, `Build`, `Write`, `IsOurs`, `Describe` for part 3), `copilot.Expand/plugin/group/Result` (keep `SkillPaths`); `bin/ipm` stops exporting `IPM_ROOT`. The committed data files (`aihs.lock`, `catalog/`, `.claude-plugin/marketplace.json`, `manifest.toml`) stay until Task 6 has imported from them.

- [ ] **Step 1:** Delete; fix compile errors; `mise run cli:test` → PASS; `go test -race ./...` → PASS; `git grep -n 'IPM_ROOT\|upstream\.\|config\.Load'` → nothing.
- [ ] **Step 2:** Commit `refactor: drop checkout-bound config, git fetching and the old pack flow`.

### Task 6: Live migration, docs

- [ ] **Step 1:** Live: `./bin/ipm source import manifest.toml` → 19 sources; `./bin/ipm sync` → pnpm installs all 19 into `~/Library/ipm/sources`, index writes `~/Library/Caches/ipm/catalog`; record counts per type (expect ≈ stage 1's 3,280, commits may move to current default branches); `./bin/ipm serve` (removes `aihs-typesense`) `&& ./bin/ipm load && ./bin/ipm search c# testing`; `./bin/ipm doctor` clean apart from known warnings. Record timings and counts in the ledger.
- [ ] **Step 2:** `git rm aihs.lock catalog/ .claude-plugin/marketplace.json manifest.toml`; remove `.aihs/` locally (cache now unused); `.gitignore` drops `/.aihs/`.
- [ ] **Step 3:** Docs: `AGENTS.md`, `CLAUDE.md` (local), `README.md`, `cli/README.md`, `docs/architecture.md`, `docs/user-guide.md`, `docs/manifest-reference.md` (becomes "Sources reference": package.json shape, `ipm source`, aliases) — describe home/cache layout, pnpm prerequisite, the commands; drop the "data paths still use aihs" note.
- [ ] **Step 4:** `mise run cli:test`; commit `docs: ipm on pnpm — sources, home and cache`.
