# Command re-org (PR 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Regroup `ipm`'s commands by workflow (onboard → marketplace → catalog → search → pack → install), rename `deploy` to `install claude`, make install build only when stale, and record installs.

**Architecture:** Cobra commands in `cli/internal/cli` move under new parent commands (`marketplace`, `catalog`, `show`, `pack`, `install`) that reuse today's `run*` functions. The root help uses Cobra command groups. Old top-level names become hidden stubs returning a `UsageError`. Install records live in `ipm.toml` (project) or `$IPM_HOME/installs.json` (global). A `.ipm/build.stamp` digest lets `install` skip an unchanged build.

**Tech Stack:** Go, Cobra (`github.com/spf13/cobra`), BurntSushi TOML, existing `fake` System in `cli/internal/cli/cli_test.go`.

**Spec:** `docs/superpowers/specs/2026-10-14-ipm-workflow-commands-gascity-tui-design.md` — Part 1.

## Global Constraints

- Commands are `new<Name>Cmd(sys System)` in `cli/internal/cli`; argument checks use `noArgs`, `exactArgs`, `minArgs`, `verbs` from `cli.go`; malformed command lines return `UsageError` (exit 2).
- Every `claude`, `pnpm`, `docker`, `gh` call goes through `sys.Output`; tests use the `fake` from `cli_test.go`, never the real home (`IPM_HOME`/`IPM_CACHE_DIR` absolute temp paths).
- `ipm pack build` must stay deterministic: two runs give byte-identical `.ipm/`, including `build.stamp`.
- Claude install keeps today's defaults: `--scope` project, or user with `-g`; `--path` the scope root.
- Gate: `mise run verify` green before each commit (format, lint, build, vet, unit tests).
- Markdown: never hard-wrap prose; one line per paragraph or list item.
- `docs/superpowers/` is history: do not edit older specs and plans.
- Removed names: `source`, `stats`, `sync`, `index`, `load`, `serve`, `scan`, `tree`, `graph`, `deploy`. `install` is reused, not removed.

## Review Focus

- `ipm show tree` with no ID, or `ipm show tree --up <id>`: must reach the `tree` subcommand, not `show` with ID "tree" (Task 3 tests both).
- Recording an install rewrites `ipm.toml`; that must not mark the build stale (the stamp hashes packs, not file bytes) — Task 5 test `TestInstallRecordKeepsStampFresh`.
- A user's existing `ipm.toml` without `installs` must still load, and one with an unknown key must still fail (Task 5 tests both).
- `ipm deploy --scope user kit` (old flags after a removed name) must print the rename message, not a flag error (Task 7: stubs disable flag parsing).
- `ipm catalog sync` with Docker absent must succeed and print the serve hint, not fail on the load step (Task 2).

---

### Task 1: `ipm marketplace`

**Files:**
- Create: `cli/internal/cli/marketplace.go` (parent command, bare listing)
- Modify: `cli/internal/cli/source.go` (subcommand constructors stay; `newSourceCmd` removed), `cli/internal/cli/stats.go` (`newStatsCmd` becomes `marketplace stats`)
- Modify: `cli/internal/cli/cli.go` (register `newMarketplaceCmd`, drop `newSourceCmd`, `newStatsCmd`)
- Test: `cli/internal/cli/source_list_test.go`, `cli/internal/cli/cli_test.go` (rename `ipm source …` / `ipm stats` invocations)

**Interfaces:**
- Consumes: `sourceAdd`, `sources.Remove`, `sourceList`, `runStats`, `sourcePacks`, `curated.Fetch/Parse`, onboard's list loading (reuse the function `runOnboard` uses to read `--list`).
- Produces: `newMarketplaceCmd(sys System) *cobra.Command`; `marketplaceList(sys System, p paths.Paths, o marketOpts) error`; `type marketOpts struct{ List string; Lists []string; Followed bool }`.

- [ ] **Step 1: Write failing tests** in `source_list_test.go`:
  - `TestMarketplaceListsCuratedWithFollowed`: fake with a local `--list` file holding two lists (`A`: `ada/skills`, `bob/agents`; `B`: `cy/tools`) and `ada/skills` followed. `ipm marketplace --list <file>` output contains list headers `A` then `B`, a `✓` on the `ada/skills` row and no `✓` on `bob/agents`.
  - `TestMarketplaceFollowedOnly`: `ipm marketplace --followed` output equals what `sourceList` printed before (reuse the existing `TestSourceListShowsCommits` expectations).
  - `TestMarketplaceListsFilter`: `--lists B` shows only `cy/tools`.
  - `TestMarketplaceFollowedNotCurated`: a followed repo absent from the list appears under `Other`.
  - Rename existing tests' command lines: `source add|remove|import|list` → `marketplace add|remove|import`, `marketplace --followed`; `stats` → `marketplace stats`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/cli -run 'Marketplace|Source|Stats'` — expect FAIL (unknown command "marketplace").
- [ ] **Step 3: Implement.** `marketplace` is not a `verbs` parent: bare run lists. `Args: noArgs` would block `unknownCommand`-style suggestions, so use `unknownCommand`-like check: unknown word → usage error listing `add, remove, import, stats`. Flags: `--list` (default `curated.DefaultURL`), `--lists` (repeatable, exact names, unknown name → usage error naming the available lists), `--followed`. Row columns: `✓`/blank, repo, ★ stars, description truncated to terminal width like `sourceList`. Hints and messages that said `ipm source add` / `ipm sync` now say `ipm marketplace add` / `ipm catalog sync`.
- [ ] **Step 4: Run** the same tests — PASS. Run `mise run verify` — PASS.
- [ ] **Step 5: Commit** `feat(cli): ipm marketplace replaces ipm source and ipm stats`.

### Task 2: `ipm catalog`

**Files:**
- Create: `cli/internal/cli/catalogcmd.go` (parent, bare summary, `rebuild`, `searchUp`)
- Modify: `sync.go` (`newSyncCmd` → `catalog sync`; `runSync` ends with `loadIfUp`), `index.go`, `load.go` (drop their `new*Cmd`; keep `runIndex`, `runLoad`), `serve.go` (`newServeCmd` → `catalog serve`; doctor/serve messages use new names), `scan.go` (`catalog scan`), `search.go` (registered under `catalog` and at root), `cli.go`
- Test: `cli/internal/cli/cli_test.go` (rename invocations; new tests below)

**Interfaces:**
- Consumes: `runSync`, `runIndex`, `runLoad`, `runServe`, `dockerReady`, `tsClient`, `search.Documents`, `sources.Load`, `sources.Checkouts`.
- Produces: `newCatalogCmd(sys System) *cobra.Command`; `searchUp(sys System) bool` (docker ready, client builds, `Health()` nil); `loadIfUp(sys System) error` (runs `runLoad` when `searchUp`, else prints `- search not loaded: run ` + "`ipm catalog serve`" + ` then ` + "`ipm catalog rebuild`"); `runRebuild(sys System) error` = `runIndex` then `loadIfUp`; `catalogSummary(sys System) error`.

- [ ] **Step 1: Write failing tests:**
  - `TestCatalogSyncLoadsWhenServerUp`: fake docker ready and Typesense healthy (as in `TestLoadRebuildsCollection`); `ipm catalog sync` calls pnpm, writes the catalog and imports documents.
  - `TestCatalogSyncWithoutDocker`: docker absent; exit 0, output contains `ipm catalog serve`.
  - `TestCatalogRebuild`: indexes then loads; without server prints the hint, exit 0.
  - `TestCatalogSummary`: two followed sources, one synced; output has rows `sources`, `items`, `last index`, `search`; `items` lists counts per type in `catalog.Types` order, skipping zeros.
  - `TestTopLevelSearchMatchesCatalogSearch`: `ipm search x` and `ipm catalog search x` produce identical output.
  - Rename existing tests' invocations: `sync` → `catalog sync`, `index` → `catalog rebuild`, `serve` → `catalog serve`, `load` → `catalog rebuild`, `scan` → `catalog scan`.
- [ ] **Step 2: Run** `go test ./internal/cli -run 'Catalog|Sync|Serve|Load|Index|Scan|Search'` — FAIL.
- [ ] **Step 3: Implement.** Bare `catalog` prints the summary with tabwriter: `sources  <n> followed, <m> synced`; `items  skill 340 · agent 120 · …`; `last index  <time> (<ago>)` from the newest file mtime in `p.Catalog()` excluding `github.jsonl`, or `never — run ` + "`ipm catalog sync`"; `search  running, <n> document(s)` or the `searchHealth` warning text. Unknown word after `catalog` → usage error listing `sync, rebuild, serve, scan, search`. `search` is built by calling `newSearchCmd(sys)` twice (one instance per parent).
- [ ] **Step 4: Run** tests — PASS; `mise run verify` — PASS.
- [ ] **Step 5: Commit** `feat(cli): ipm catalog groups sync, rebuild, serve, scan and search`.

### Task 3: `ipm show tree` and `ipm show graph`

**Files:**
- Modify: `cli/internal/cli/show.go` (attach `newTreeCmd`, `newGraphCmd` as subcommands with `Use: "tree <id>"`, `"graph <id>"`), `cli.go`
- Test: `cli_test.go` (`TestTreeAndGraphCommands`, `TestTreeDefaultDepth`)

**Interfaces:**
- Produces: `newShowCmd(sys)` with subcommands `tree`, `graph`; flags unchanged.

- [ ] **Step 1: Tests:** rewrite the tree/graph tests to `ipm show tree <id>`, `ipm show tree --up <id>`, `ipm show graph <id> --format dot`; add `TestShowTreeNeedsID`: `ipm show tree` exits 2 with `give one artifact ID`; `TestShowStillShows`: `ipm show <id>` output unchanged.
- [ ] **Step 2: Run** `go test ./internal/cli -run 'Show|Tree|Graph'` — FAIL.
- [ ] **Step 3: Implement.** Move the subcommands; update `Short`s to drop "(reads the catalog, no search server)" duplication only where the parent says it. Hints elsewhere that say `ipm tree` say `ipm show tree`.
- [ ] **Step 4: Run** — PASS; `mise run verify`.
- [ ] **Step 5: Commit** `feat(cli): tree and graph move under ipm show`.

### Task 4: `ipm pack build` and the build stamp

**Files:**
- Rename: `cli/internal/cli/install.go` → `cli/internal/cli/build.go` (`git mv`); `runInstall` → `runBuild`
- Modify: `cli/internal/cli/packcmd.go` (add `build` subcommand), `cli/internal/project/project.go` (add `PacksDigest`; `SavePacks` header comment says `ipm pack build`), `cli/internal/cli/tui.go` (`tuiInstaller` calls `runBuild`), `cli.go` (drop top-level `newInstallCmd` registration; Task 6 adds the new one)
- Test: `cli/internal/project/project_test.go`, `cli_test.go` (`TestInstall*` → `TestPackBuild*`, invocations `install` → `pack build`)

**Interfaces:**
- Produces:
  - `project.PacksDigest(packs []Pack) string` — hex SHA-256 over each pack's name, description and items in name order (installs excluded).
  - `const StampFile = "build.stamp"` in `project`.
  - `buildStamp(scope project.Scope, version string) (string, error)` in `build.go` — hex SHA-256 over `PacksDigest(LoadPacks(root))`, `\x00`, the bytes of `.ipm/ipm.lock` (empty if missing), `\x00`, version.
  - `buildCurrent(sys System, scope project.Scope) bool` — `.ipm/plugins/`, `.ipm/.claude-plugin/marketplace.json` and `build.stamp` exist and the stamp equals `buildStamp`.
  - `build(...)` writes `build.stamp` as its last write.

- [ ] **Step 1: Tests:**
  - `TestPacksDigestIgnoresInstalls` (project): same packs with and without `Installs` → same digest; changing an item changes it.
  - `TestPackBuildWritesStamp`: after `ipm pack build`, `.ipm/build.stamp` exists and `buildCurrent` is true; editing `ipm.toml` items makes it false.
  - `TestPackBuildIsDeterministic` (renamed `TestInstallIsDeterministic`): two builds byte-identical including the stamp.
- [ ] **Step 2: Run** `go test ./internal/project ./internal/cli -run 'Digest|PackBuild|Stamp'` — FAIL.
- [ ] **Step 3: Implement** as in Interfaces. `pack build` flags: `-g/--global`, `--update`, `--no-validate` (same help text as today's `install`). Its closing hint prints `ipm install claude` instead of the raw `claude plugin marketplace add` lines, followed by the manual `claude plugin marketplace add <dir>` line for people who wire it themselves.
- [ ] **Step 4: Run** — PASS; `mise run verify`.
- [ ] **Step 5: Commit** `feat(cli): ipm pack build, with a build stamp`.

### Task 5: Install records

**Files:**
- Create: `cli/internal/cli/installs.go`
- Modify: `cli/internal/project/project.go` (`Install` type; `Pack.Installs`; load and save `[[packs.<name>.installs]]`), `cli/internal/cli/packcmd.go` (`pack show` lists installs)
- Test: `cli/internal/project/project_test.go`, `cli/internal/cli/installs_test.go`

**Interfaces:**
- Produces:
  - `project.Install{ Target, Path, Scope, Name string }` with TOML keys `target`, `path`, `scope` (omitempty), `name` (omitempty). `Pack.Installs []Install`.
  - `SavePacks` writes, after each pack's `items`, one `[[packs.<name>.installs]]` table per install in stored order with keys in the order `target, path, scope, name`, omitting empty `scope`/`name`.
  - `type installRecord struct{ Pack string; project.Install }`.
  - `recordInstall(sys System, scope project.Scope, pack string, in project.Install) error` — project scope: path made relative to `scope.Root` when inside it, then replaces the entry with the same `(Target, Path, Name)` or appends; global scope: same rule against `$IPM_HOME/installs.json` (sorted by pack, target, path; absolute paths; JSON keys `pack, target, path, scope, name`, 2-space indent, trailing newline).
  - `loadInstalls(sys System, scope project.Scope) ([]installRecord, error)` — sorted by pack, target, path; project paths returned absolute.

- [ ] **Step 1: Tests:**
  - `TestLoadPacksWithInstalls` / `TestSavePacksInstallsRoundTrip` (project): exact TOML text for one pack with one claude (`scope = "project"`) and one gascity (`name = "kit"`) install.
  - `TestLoadPacksUnknownKeyStillFails`: `[packs.kit] colour = "x"` still errors.
  - `TestRecordInstallReplaces`: recording the same target+path twice leaves one entry; different path appends.
  - `TestRecordInstallRelativePath`: a path inside the project is stored as `.`/`sub`.
  - `TestRecordInstallGlobal`: `-g` scope writes `$IPM_HOME/installs.json` with the exact JSON; `ipm.toml` under `$IPM_HOME/global` is untouched.
  - `TestInstallRecordKeepsStampFresh`: after `ipm pack build`, `recordInstall` leaves `buildCurrent` true.
  - `TestPackShowListsInstalls`: `ipm pack show kit` ends with `installed: claude project <path>`.
- [ ] **Step 2: Run** `go test ./internal/project ./internal/cli -run 'Install|PackShow'` — FAIL.
- [ ] **Step 3: Implement** per Interfaces.
- [ ] **Step 4: Run** — PASS; `mise run verify`.
- [ ] **Step 5: Commit** `feat(cli): record where packs are installed`.

### Task 6: `ipm install claude` and bare `ipm install`

**Files:**
- Rename: `cli/internal/cli/deploy.go` → `cli/internal/cli/install_claude.go` (`git mv`); `runDeploy` → `installClaude`, `deployTarget` → `claudeTarget`, `deployPack` → `pickPack`, `deployDefaults` → `claudeDefaults`
- Create: `cli/internal/cli/install.go` (the `install` parent used at root and under `pack`, the target table, replay)
- Modify: `cli/internal/cli/tui.go` (`tuiDeployer` calls `installClaude`), `cli.go`
- Test: `cli_test.go` (`TestPlanDeploy*`, `TestRunDeploy*`, `TestTuiDeployer*` keep their assertions, renamed and re-pointed), new tests below

**Interfaces:**
- Consumes: `buildCurrent`, `build`, `recordInstall`, `loadInstalls`, `planDeploy`.
- Produces:
  - `newInstallCmd(sys System) *cobra.Command` — `Use: "install [target] [pack]"`; registered at root and under `pack`; subcommand `claude [pack]`; flags on `claude`: `-s/--scope`, `-p/--path`, `-g/--global`, `--update`, `--dry-run`.
  - `type installer func(sys System, scope project.Scope, pack string, in project.Install, o installOpts, out io.Writer) (string, error)`; `var installers = map[string]installer{"claude": …}` (PR 2 adds `"gascity"`).
  - `type installOpts struct{ Global, Update, DryRun bool }`.
  - `installClaude` builds only when `o.Update || !buildCurrent(...)` (else prints `build is current`), runs the plan, then `recordInstall` with `{Target: "claude", Path: t.Path, Scope: t.Scope}`. With `DryRun` it prints each `$ claude …` line from `planDeploy` and returns without building, running or recording.
  - `replayInstalls(sys System, global bool) error` — bare `ipm install [-g]`: each record in `loadInstalls` order through `installers[rec.Target]`; none recorded → `usagef("install: say claude or gascity — nothing is recorded for this scope yet. Try `+"`ipm help install`"+`.")`; a record with an unknown target → error naming it.

- [ ] **Step 1: Tests:**
  - `TestInstallClaudeBuildsWhenStale` and `TestInstallClaudeSkipsCurrentBuild`: second run prints `build is current` and makes no pnpm call.
  - `TestInstallClaudeUpdateRebuilds`.
  - `TestInstallClaudeDryRun`: prints `$ claude plugin marketplace add …` and `$ claude plugin install kit@…`; no `claude plugin install` in `f.calls`, no `.ipm/` written, `ipm.toml` unchanged.
  - `TestInstallClaudeRecords`: `ipm.toml` gains the `[[packs.kit.installs]]` table.
  - `TestPackInstallIsInstall`: `ipm pack install claude kit` and `ipm install claude kit` make the same `claude` calls.
  - `TestBareInstallReplays`: two recorded installs replay in order; `TestBareInstallNothingRecorded` exits 2 with the message above.
  - `TestInstallClaudeShortFlags`: `-s user -p .` equals `--scope user --path .`.
- [ ] **Step 2: Run** `go test ./internal/cli -run 'Install|Deploy|Tui'` — FAIL.
- [ ] **Step 3: Implement** per Interfaces. `needClaude`'s message becomes `ipm install claude needs claude (Claude Code) on PATH`; usage errors say `install claude` instead of `deploy`.
- [ ] **Step 4: Run** — PASS; `mise run verify`.
- [ ] **Step 5: Commit** `feat(cli): ipm install claude replaces ipm deploy, and records installs`.

### Task 7: Root help groups, renamed-command stubs, onboard intro

**Files:**
- Modify: `cli/internal/cli/cli.go`, `cli/internal/cli/onboard.go`
- Test: `cli_test.go` (`TestHelpAndVersion`, `TestUsageErrors`, `TestCommandLineHints`), `onboard_test.go`

**Interfaces:**
- Produces: `renamed(old, now string) *cobra.Command` — `Hidden: true`, `DisableFlagParsing: true`, `RunE` returns `usagef("ipm %s was renamed: use ipm %s. Try `+"`ipm help`"+`.", old, now)`.

- [ ] **Step 1: Tests:**
  - `TestRootHelpGroups`: `ipm -h` contains `Workflow: onboard → marketplace → catalog → search → pack → install`, then group titles in order `Getting started:`, `Marketplace — curated repos you can follow:`, `Catalog — the repos you follow, fetched and searchable:`, `Packs — your picks, built and installed:`, `Tools:`; `onboard` appears under the first, `marketplace` under the second, `catalog`, `search`, `show` under the third, `pack`, `install` under the fourth, `tui`, `doctor`, `uninstall`, `version`, `completion`, `help` under Tools; none of the removed names appear.
  - `TestRenamedCommands`: table of (args → message): `deploy --scope user kit` → `ipm deploy was renamed: use ipm install claude`; `source list` → `use ipm marketplace`; `stats` → `ipm marketplace stats`; `sync --upgrade` → `ipm catalog sync`; `index`, `load` → `ipm catalog rebuild`; `serve` → `ipm catalog serve`; `scan` → `ipm catalog scan`; `tree x` → `ipm show tree`; `graph x` → `ipm show graph`. Each exits 2.
  - `TestOnboardPrintsConcepts`: onboard output begins with the intro block below.
- [ ] **Step 2: Run** `go test ./internal/cli -run 'Help|Renamed|Onboard|Usage|Hints'` — FAIL.
- [ ] **Step 3: Implement.** Groups via `root.AddGroup` with IDs `start`, `market`, `catalog`, `packs`, `tools`, titles exactly as in the test; `root.SetHelpCommandGroupID("tools")`, `root.SetCompletionCommandGroupID("tools")`. Root `Long`: the existing first line, a blank line, `Workflow: onboard → marketplace → catalog → search → pack → install`, a blank line, the existing Config block. Shorts:
  - onboard: `learn the concepts, pick repos from the marketplace, build your catalog`
  - marketplace: `the curated repos and which you follow (add, remove, import, stats)`
  - catalog: `what's in your catalog (sync, rebuild, serve, scan, search)`
  - search: `search your catalog (same as catalog search)`
  - show: `an item's fields and neighbours (tree, graph)`
  - pack: `create, edit and build packs (new, add, remove, list, show, build, install)`
  - install: `install a pack into claude or gascity (same as pack install)`

  Onboard intro, printed by `runOnboard` before the existing-data check:
  ```
  ipm in four steps:
    marketplace  curated repos you can follow — pick the ones you care about
    catalog      the repos you follow, fetched, pinned and searchable
    pack         the skills, agents and plugins you pick from the catalog
    install      a pack, built and installed into Claude Code
  ```
  Onboard's rerun hints use new names (`ipm catalog sync`, `ipm marketplace stats`, `ipm catalog serve && ipm catalog rebuild`).
- [ ] **Step 4: Run** — PASS; `mise run verify`.
- [ ] **Step 5: Commit** `feat(cli): ipm help follows the workflow; old names say where they went`.

### Task 8: Sweep messages, TUI strings and docs

**Files:**
- Modify: any non-test `.go` under `cli/` still naming a removed command (`grep -rnE 'ipm (source|stats|sync|index|load|serve|scan|tree|graph|deploy)\b|ipm install\b' cli`), `cli/internal/tui/*.go` hint strings and `internal/tui/testdata/*.golden` (regenerate with `go test ./internal/tui -run Golden -update`, read the diff), `docs/reference/commands.md` (reorganised into the five groups), `docs/getting-started.md`, `docs/concepts.md`, `docs/troubleshooting.md`, `docs/architecture.md`, `docs/index.md`, `docs/guides/tui.md`, `docs/guides/build-a-plugin.md`, `README.md`, `cli/README.md`, `AGENTS.md`, `CLAUDE.md` (Commands table and sharp edges), `.claude/` or `mise.toml` tasks that call removed names, `install.sh` if it runs `ipm` commands.

- [ ] **Step 1:** Run the grep above; every hit outside `docs/superpowers/` and tests is a defect. Expected after the sweep: no hits.
- [ ] **Step 2:** Update each file; in docs, every command and flag must match `bin/ipm <cmd> -h` output (verify by running it).
- [ ] **Step 3:** `mise run verify` — PASS. Run `bin/ipm -h`, `bin/ipm marketplace -h`, `bin/ipm catalog`, `bin/ipm install claude --dry-run` in a temp `IPM_HOME` and paste the output in the PR description.
- [ ] **Step 4: Commit** `docs: commands follow the workflow groups`.

### Task 9: PR

- [ ] Push `feat/command-reorg`, open a PR titled `Commands follow the workflow: marketplace, catalog, pack, install claude`. Body: summary, the mapping table from the spec, the help output, test evidence, and review comments pointing at `install.go` (replay), `installs.go` (records) and `cli.go` (groups and stubs).
