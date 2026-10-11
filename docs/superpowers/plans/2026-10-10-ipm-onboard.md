# ipm install, onboard and uninstall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new user installs `ipm` with `install.sh`, picks sources from a curated list in a checklist, and ends with a searchable library; `ipm uninstall` takes it back out.

**Architecture:** `internal/curated` fetches and parses the curated TOML. The checklist is a Bubble Tea model in `internal/tui` over plain types (so it reuses the TUI's unexported styles and keeps `internal/tui` free of ipm imports). `cli/onboard.go` and `cli/uninstall.go` are Cobra commands that call the existing `runSync`, `runStats`, `runServe` and `runLoad` in-process. `install.sh` builds into `~/.local/bin`.

**Tech Stack:** Go 1.26, Cobra, Bubble Tea v2 (`charm.land/bubbletea/v2`), Lip Gloss v2, `github.com/BurntSushi/toml` v1.6.0 (already a dependency), bash + shellcheck.

**Spec:** `docs/superpowers/specs/2026-10-10-ipm-onboard-design.md`

## Global Constraints

- Default curated list: `https://thinkjones.github.io/ai-sdlc-repo-list/starred_repos.toml`.
- Pre-ticked lists on a fresh start: `AI · Agentic Engineering Methods`, `AI · Skills, Plugins & Registries` (absent names ignored).
- Backup names: `<dir>.backup-YYYYMMDD-HHMM`, then `-2`, `-3` on a clash; time from `sys.Now`.
- Binary install path: `~/.local/bin/ipm`. Module path: `github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm`.
- Container: `ipm-typesense` (the existing `tsContainer` constant).
- Step lines: `✓ <step>` / `✗ <step>: <error>` plus a rerun line; done line `✓ library ready — <N> sources, <M> artifacts`.
- Tests never touch the real home: absolute temp `IPM_HOME` and `IPM_CACHE_DIR`; no test runs real Docker, gh, pnpm or the network (httptest only).
- Commands follow CLAUDE.md: `new<Name>Cmd(sys System)` registered in `newRootCmd`; malformed command lines return `usagef(...)` (exit 2).
- Markdown prose is not hard-wrapped.
- Gate per task: `mise run cli:test` (gofmt, vet, test); also `go test -race ./...` at the end.

## Review Focus

1. **Quitting the checklist after a backup** (`q` or `ctrl+c`): the backup has already happened; the user should see the restore line again and no source should be added. Pinned in Task 4 (`TestOnboardQuitAfterBackupReprintsRestore`).
2. **Cache inside home** (`IPM_CACHE_DIR` under `IPM_HOME`): renaming home moves the cache with it; backup must succeed and not fail on the second rename. Pinned in Task 3 (`TestBackupCacheInsideHome`).
3. **`last_push` spelled three ways** — TOML local date (`2026-10-10`), datetime, or a quoted string — and missing: all parse; missing shows `—`. Pinned in Task 1 (`TestParseLastPushForms`).
4. **The list URL answers 404 or HTML**: treated as a failed fetch (fall back to the cache), and a body that doesn't parse never overwrites a good cached copy. Pinned in Task 1 (`TestFetchBadResponseKeepsCache`).
5. **Descriptions with emoji or CJK** at 80 columns: rows are exactly the terminal width, never wrapped. Pinned in Task 2 (golden `checklist-wide-chars`).

---

### Task 1: `internal/curated` — fetch and parse

**Files:**
- Create: `cli/internal/curated/curated.go`, `cli/internal/curated/curated_test.go`, `cli/internal/curated/testdata/lists.toml`

**Interfaces:**
- Produces:
  - `const DefaultURL = "https://thinkjones.github.io/ai-sdlc-repo-list/starred_repos.toml"`
  - `type Repo struct { Repo, List, Description, Language string; Stars int; LastPush time.Time; Topics []string }`
  - `type List struct { Name string; Repos []Repo }`
  - `func Parse(data []byte) (lists []List, skipped int, err error)` — lists in first-appearance order; repos within a list by stars desc, then repo; rows without a valid `owner/repo` (`^[\w.-]+/[\w.-]+$`) counted in `skipped`.
  - `func Fetch(client *http.Client, src, cacheDir string) (data []byte, cached bool, err error)` — `src` is a path or an `http(s)://` URL. URLs: GET; on success (200 and `Parse` succeeds with ≥1 list) write `cacheDir/curated/<hex sha256(src)[:16]>.toml` and return it; on any failure return the cached copy with `cached=true`, or, with no cache, an error naming the URL and suggesting `--list <path>`. Paths are read directly, never cached.

- [ ] **Step 1: Write the failing tests** in `curated_test.go`, with a fixture of 3 lists / 7 rows (one row with `repo = "not a repo"`, one repo duplicated in two lists):
  - `TestParseOrdersListsAndRepos` — list names in file order; repos in one list sorted by stars desc; `skipped == 1`; duplicated repo appears in both lists.
  - `TestParseLastPushForms` — `last_push = 2026-10-10`, `last_push = 2026-10-10T08:00:00Z`, `last_push = "2026-10-10"` all give `time.Date(2026,10,10,…)` (date part equal); missing gives the zero time.
  - `TestParseIgnoresUnknownFieldsAndMeta` — a file with `[meta]` and extra keys (`forks`, `license`) parses; a file with no `[meta]` parses.
  - `TestFetchCachesAndFallsBack` — httptest server returns the fixture → `cached=false`, cache file written; server then returns 500 → same bytes, `cached=true`.
  - `TestFetchBadResponseKeepsCache` — prime the cache; server returns 404, then 200 with `<html>`: both return the primed bytes with `cached=true` and the cache file is unchanged.
  - `TestFetchNoCacheErrors` — server 500, empty cache dir → error containing the URL and `--list`.
  - `TestFetchPath` — a file path returns its bytes; nothing written to the cache dir.

- [ ] **Step 2: Run** `cd cli && go test ./internal/curated/` — Expected: FAIL (package has no non-test files / undefined: Parse).

- [ ] **Step 3: Implement** `curated.go`. Decode into `struct{ Repos []map[string]any \`toml:"repos"\` }` (so unknown fields and value types never fail), then read fields: `last_push` may be `time.Time`, `toml.LocalDate`, `toml.LocalDateTime` or a `string` (`2006-01-02` or RFC 3339); `stars` may be `int64`.

- [ ] **Step 4: Run** `cd cli && go test ./internal/curated/` — Expected: PASS.

- [ ] **Step 5: Commit** — `git add cli/internal/curated && git commit -m "feat(curated): fetch and parse curated source lists"`

### Task 2: the checklist (`internal/tui/checklist.go`)

The spec names `internal/onboard`; the model lives in `internal/tui` instead so it reuses `fit`, `selectedRow`, `starStyle`, `stars` and the colours without exporting them, and keeps the rule that `internal/tui` imports no ipm packages. (Ledger this as a ruling.)

**Files:**
- Create: `cli/internal/tui/checklist.go`, `cli/internal/tui/checklist_test.go`, goldens `cli/internal/tui/testdata/checklist-*.golden`

**Interfaces:**
- Produces:
  - `type CuratedRepo struct { Repo, Description, Language string; Stars int; LastPush time.Time; Topics []string }`
  - `type CuratedList struct { Name string; Repos []CuratedRepo }`
  - `func NewChecklist(lists []CuratedList, ticked, locked map[string]bool, now time.Time) *Checklist` — keys are `strings.ToLower(owner/repo)`; locked rows render `✓`, can't be unticked, and are not returned by `Picked`.
  - `func (c *Checklist) Init() tea.Cmd`, `Update(tea.Msg) (tea.Model, tea.Cmd)`, `View() tea.View` — a full `tea.Model`.
  - `func (c *Checklist) Picked() []string` — ticked, unlocked repos, in list then row order, each once.
  - `func (c *Checklist) Confirmed() bool` — true only after `enter` then `y`.

- [ ] **Step 1: Write the failing tests** in `checklist_test.go` (drive with `tea.KeyPressMsg`, size with `tea.WindowSizeMsg{Width:120,Height:30}`):
  - `TestChecklistTickList` — `space` on a collapsed list row ticks all its repos; the row shows `[x]`; again → `[ ]`.
  - `TestChecklistPartial` — `→` expands, `↓` onto a repo, `space` → that repo ticked, list row shows `[~]`.
  - `TestChecklistRepoInTwoLists` — ticking it in one list shows it ticked in the other; `Picked` holds it once.
  - `TestChecklistAllNone` — `a` ticks every repo in the cursor's list, `n` clears them; locked repos unchanged.
  - `TestChecklistLocked` — a locked repo renders `✓`, `space` doesn't change it, `Picked` excludes it.
  - `TestChecklistFilter` — `/`, type `tdd`, `enter`: only repos whose repo, description or a topic contains `tdd` (case-insensitive) are shown, under their expanded lists; lists with no match hidden; `esc` clears the filter.
  - `TestChecklistNothingTicked` — `enter` with nothing new ticked shows `tick at least one repo` and stays (not `Confirmed`).
  - `TestChecklistConfirm` — tick, `enter` shows `N repos to follow — y continue · esc back`; `esc` returns; `enter`, `y` → `Confirmed()` and a `tea.Quit` cmd.
  - `TestChecklistQuit` — `q` → `tea.Quit`, `Confirmed()` false.
  - `TestAge` — `age(now, now-3d) == "3d"`, `-65d == "2mo"`, `-400d == "1y"`, zero time `== "—"` (days < 30 → `Nd`; < 365 → `days/30` `mo`; else `days/365` `y`).
  - `TestGoldenChecklist` — goldens (with `NO_COLOR`-free plain rendering the existing goldens use): `checklist-collapsed` (120×20), `checklist-expanded`, `checklist-filtered`, `checklist-narrow` (80 cols: language and age dropped), `checklist-wide-chars` (80 cols, a description with emoji and CJK; assert every line's `ansi.StringWidth` ≤ 80).

- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Checklist|Age'` — Expected: FAIL (undefined: NewChecklist).

- [ ] **Step 3: Implement** `checklist.go`. Layout as the spec's sketch: header `Choose sources — <R> repos in <L> lists` with `<N> picked` right-aligned; list rows `[x]/[~]/[ ] name  <n> repos  ★<sum>`; repo rows indented 4 under their list: tick, repo (repo colour), `★` stars (`starStyle`), language, age, description filling the rest via `fit`. Under 100 columns drop language and age. Caret and selection with `selectedRow(row, w, true)`. Rows scroll to keep the cursor visible. Footer is the key line from the spec.

- [ ] **Step 4: Run** `cd cli && go test ./internal/tui -run 'Golden' -update`, read every new golden, then `go test ./internal/tui` — Expected: PASS, and the existing goldens unchanged.

- [ ] **Step 5: Commit** — `git add cli/internal/tui && git commit -m "feat(tui): curated-source checklist"`

### Task 3: existing data — detect and back up

**Files:**
- Modify: `cli/internal/cli/cli.go` (System fields), `cli/cmd/ipm/main.go` (wire them)
- Create: `cli/internal/cli/onboard.go` (helpers only in this task), `cli/internal/cli/onboard_test.go`

**Interfaces:**
- Produces (System, all nil-safe):
  - `Stdin io.Reader` — nil reads nothing.
  - `Interactive func() bool` — stdin and stdout are terminals; nil is false. Wired in main with `term.IsTerminal` on both fds.
  - `Executable func() (string, error)` — nil uses `os.Executable`.
  - `RunChecklist func(*tui.Checklist) error` — nil runs `tea.NewProgram(c).Run()`; tests replace it to send keys or set ticks.
- Produces (onboard.go):
  - `type existing struct { Sources, Artifacts int; Synced time.Time }` and `func detect(p paths.Paths) (existing, bool, error)` — present when `sources.Load(p.Sources())` has ≥1 source or `<catalog>/repos.jsonl` exists; `Artifacts` = records across `catalog.Types` except `repo`; `Synced` = mtime of `repos.jsonl` (zero if absent).
  - `func (e existing) String() string` — `26 sources · 3,280 artifacts · synced 2026-10-09` (thousands separator; `· synced …` omitted when zero).
  - `func backup(p paths.Paths, now time.Time) (moved []string, err error)` — renames home then cache to `<dir>.backup-YYYYMMDD-HHMM` (`-2`, `-3`… on a clash), skipping a dir that doesn't exist, and skipping cache when it lived inside home (already moved). Returns the new paths. On a failed first rename nothing has changed; on a failed second rename, rename the first back before returning the error.
  - `func restoreHint(p paths.Paths, moved []string) string` — `to restore: ipm uninstall, then mv <backup> <dir>` for each.

- [ ] **Step 1: Write the failing tests** (temp `IPM_HOME`/`IPM_CACHE_DIR` via the existing `newFake`):
  - `TestDetectEmpty` — fresh dirs → `false`.
  - `TestDetectSummary` — 2 sources (`withRepos`-style package.json), catalog with 3 skills + 1 repo, `repos.jsonl` mtime 2026-10-09 → `2 sources · 3 artifacts · synced 2026-10-09`.
  - `TestBackupRenamesBoth` — now `2026-10-10 15:30` → home and cache moved to `….backup-20261010-1530`; originals gone.
  - `TestBackupClash` — pre-create `<home>.backup-20261010-1530` → home goes to `…-1530-2`.
  - `TestBackupCacheInsideHome` — cache under home → one rename, no error, `len(moved)==1`.
  - `TestBackupRollsBack` — make the cache's parent read-only so its rename fails → error, home back at its original path.

- [ ] **Step 2: Run** `cd cli && go test ./internal/cli -run 'Detect|Backup'` — Expected: FAIL (undefined: detect).

- [ ] **Step 3: Implement** the System fields, main wiring, and the helpers above.

- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.

- [ ] **Step 5: Commit** — `git commit -am "feat(onboard): detect and back up existing data" ` (add the new files first).

### Task 4: `ipm onboard`

**Files:**
- Modify: `cli/internal/cli/onboard.go`, `cli/internal/cli/onboard_test.go`, `cli/internal/cli/cli.go` (register `newOnboardCmd(sys)`)

**Interfaces:**
- Consumes: Task 1 `curated.Fetch/Parse/DefaultURL`; Task 2 `tui.NewChecklist`, `Picked`, `Confirmed`; Task 3 `detect`, `backup`, `restoreHint`, System fields; existing `runSync(sys, false)`, `runStats(sys)`, `runServe(sys, false)`, `runLoad(sys)`, `dockerReady(sys)`, `sources.AddSkippingExisting`, `sources.Load`.
- Produces: `func newOnboardCmd(sys System) *cobra.Command`; `func runOnboard(sys System, o onboardOpts) error` with `type onboardOpts struct { List string; Lists []string; Keep, Fresh, Yes bool }`; `var defaultLists = []string{"AI · Agentic Engineering Methods", "AI · Skills, Plugins & Registries"}`.

Behaviour, in order (spec §`ipm onboard`):
1. `--keep` with `--fresh` → usage error. If `detect` finds data: print `found existing ipm data: <summary>`; with neither flag, need `Interactive()` (else usage error naming `--keep` or `--fresh`) and ask `[u]se it · [b]ack it up and start fresh · [q]uit:` reading a line from `Stdin` (`u`/`b`/`q`, case-insensitive, re-ask on anything else, EOF = quit). Back up → print each moved path and `restoreHint`.
2. `curated.Fetch(sys.HTTP or &http.Client{}, list, p.Cache)`; `cached` → print `(offline: using the copy from <mtime>)`. `Parse`; `skipped>0` → note.
3. Picks: `--lists` names must each exist (else error listing the available names); their repos are the picks. Otherwise `--yes` → the pre-ticked set. Otherwise need `Interactive()` (else usage error naming `--lists` or `--yes`): build the checklist (existing sources locked; `defaultLists` ticked only when there were no existing sources), call `RunChecklist`; not `Confirmed()` → print `nothing changed` plus `restoreHint` if a backup happened, return nil.
4. Pipeline (stop at the first `✗`, which prints `rerun: <command>`):
   - `add sources` — `AddSkippingExisting`; line `✓ added N sources (M already followed)`. Rerun: `ipm onboard`.
   - `sync` — `runSync`. Rerun: `ipm sync`.
   - `stats` — skipped (`- stats skipped: <reason>`) when `LookPath("gh")` fails or `gh auth status` fails; else `runStats`. Rerun: `ipm stats`.
   - `search` — skipped with `- search skipped: start Docker, then ipm serve && ipm load` when `dockerReady` fails; else `runServe` then `runLoad`. Rerun: `ipm serve && ipm load`.
   On failure return `&ExitError{Code: 1}` after printing.
5. Done: `✓ library ready — <N> sources, <M> artifacts` (counts via `sources.Load` and `detect`), then `try: ipm search <t1> · ipm search <t2> · ipm search <t3> · ipm tui`, topics most common across the picked repos (ties alphabetical), skipping `ai, llm, llms, agents, ai-agents, agent, claude, claude-code, anthropic, openai` (ledger this stop list as a ruling); fewer than 3 topics → as many as there are.

- [ ] **Step 1: Write the failing tests** (fake System; curated fixture served by httptest or `--list <path>`; `RunChecklist` replaced; pnpm, gh and docker faked through the existing `Output`/`onPath` fakes — reuse how `TestServe*` and the sync tests fake them):
  - `TestOnboardFreshWithYes` — no data, `--yes --list fixture` → package.json holds the pre-ticked lists' repos; output has `✓ added`, `✓ sync`, stats/search lines, and `✓ library ready`.
  - `TestOnboardAsksWhenDataExists` — data present, `Interactive` true, Stdin `"x\nb\n"` → re-asks once, then backs up (backup dirs exist), prints the restore hint.
  - `TestOnboardKeepLocksExisting` — `--keep`, `RunChecklist` inspects the model: existing sources render `✓`, nothing pre-ticked; it ticks one new repo and confirms → that repo added, existing ones untouched.
  - `TestOnboardNonInteractiveNeedsFlags` — data present, not interactive, no flags → `UsageError` mentioning `--keep`; no data, not interactive, no `--lists`/`--yes` → `UsageError` mentioning `--lists`.
  - `TestOnboardUnknownList` — `--lists "Nope"` → error listing available list names; nothing written.
  - `TestOnboardQuitAfterBackupReprintsRestore` — Stdin `b`, `RunChecklist` quits → output has `nothing changed` and the restore hint; no `package.json` in the new home.
  - `TestOnboardSyncFailureStops` — pnpm fake fails → `✗ sync: …`, `rerun: ipm sync`, no stats/search lines, `ExitError{1}`; the picks remain in package.json.
  - `TestOnboardSkipsStatsAndSearch` — no `gh`, docker not on PATH → both `skipped` lines, still `✓ library ready`.
  - `TestOnboardTopics` — picked repos' topics `[ai, tdd, testing]`, `[tdd, hooks]`, `[testing]` → `try: ipm search tdd · ipm search testing · ipm search hooks · ipm tui`.

- [ ] **Step 2: Run** `cd cli && go test ./internal/cli -run Onboard` — Expected: FAIL (undefined: runOnboard / unknown command "onboard").

- [ ] **Step 3: Implement** `newOnboardCmd` (flags `--list` default `curated.DefaultURL`, `--lists` repeatable `StringArray`, `--keep`, `--fresh`, `--yes`; `Args: noArgs`; Short `pick sources from a curated list and build your library`) and `runOnboard`; register in `newRootCmd`.

- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(onboard): ipm onboard — curated sources to a searchable library"`

### Task 5: `ipm uninstall`

**Files:**
- Create: `cli/internal/cli/uninstall.go`, `cli/internal/cli/uninstall_test.go`
- Modify: `cli/internal/cli/cli.go` (register `newUninstallCmd(sys)`)

**Interfaces:**
- Consumes: Task 3 `Executable`, `Interactive`, `Stdin`; existing `tsContainer`, `ipmPaths`.
- Produces: `func newUninstallCmd(sys System) *cobra.Command`; `func runUninstall(sys System, purge, yes, force bool) error`; `func devBuild(bin string) bool` — the binary's folder is named `bin` and it or an ancestor of it holds `.git`.

Behaviour (spec §`ipm uninstall`), printing a `✓` line per step:
1. Docker on PATH and `docker info` succeeds → `docker rm -f ipm-typesense` (output containing `No such container` is fine: `- no search container`); otherwise `- docker not available: skipped the search container`.
2. Binary: `Executable()`, `filepath.EvalSymlinks`; `devBuild` and not `--force` → error `<path> is a development build in a git checkout — rerun with --force to remove it` (before step 1 runs: check first, act after). Else `os.Remove`.
3. Without `--purge`: `kept your data: <home>, <cache> — ipm onboard will offer to use or back it up`.
4. With `--purge`: not `--yes` and not `Interactive()` → usage error; not `--yes` → prompt `type <home> to delete it and <cache>:`, compare trimmed line to home exactly, mismatch → `nothing deleted`, `ExitError{1}`; match or `--yes` → `os.RemoveAll` both.

- [ ] **Step 1: Write the failing tests:**
  - `TestUninstallKeepsData` — `Executable` returns a temp file → file removed; `docker rm -f ipm-typesense` called; home and cache still exist; output has `kept your data`.
  - `TestUninstallRefusesDevBuild` — binary at `<tmp>/repo/cli/bin/ipm` with `<tmp>/repo/.git` → error mentioning `--force`; file still exists; no docker call. With `--force` → removed.
  - `TestUninstallPurgeConfirm` — interactive, Stdin = home path → home and cache gone; Stdin = `nope` → both remain, `nothing deleted`.
  - `TestUninstallPurgeNeedsYesWhenNotInteractive` — `UsageError`; `--purge --yes` → deleted without reading Stdin.
  - `TestUninstallWithoutDocker` — docker not on PATH → skipped line, binary still removed.

- [ ] **Step 2: Run** `cd cli && go test ./internal/cli -run Uninstall` — Expected: FAIL.

- [ ] **Step 3: Implement** and register.

- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.

- [ ] **Step 5: Commit** — `git commit -m "feat(uninstall): ipm uninstall [--purge]"`

### Task 6: `install.sh`, `.envrc` and docs

**Files:**
- Create: `install.sh` (repo root, executable)
- Modify: `.envrc`, `mise.toml` (comment), `README.md`, `docs/getting-started.md`, `docs/reference/commands.md`, `docs/guides/sources.md`, `docs/troubleshooting.md`, `CLAUDE.md`, `AGENTS.md`, `cli/README.md`, `cli/cmd/ipm/main.go` (package comment command list)

- [ ] **Step 1: Write `install.sh`** per spec §`install.sh`: `set -euo pipefail`; `BIN="${IPM_BIN_DIR:-$HOME/.local/bin}"` (the override exists so the test below can use a temp dir); checkout detection via `"$(dirname "${BASH_SOURCE[0]:-$0}")/cli/go.mod"`; the two build paths; PATH warning with the line for `$SHELL` (`zsh` → `~/.zshrc`, `bash` → `~/.bashrc`, else `~/.profile`); shadow warning when `command -v ipm` ≠ `$BIN/ipm`; `[ -t 0 ] && [ -t 1 ]` → `exec "$BIN/ipm" onboard`, else `echo "next: ipm onboard"`.

- [ ] **Step 2: Verify:** `shellcheck install.sh` — Expected: no output. Then `HOME=$(mktemp -d) IPM_BIN_DIR=$HOME/bin ./install.sh </dev/null` — Expected: builds `$HOME/bin/ipm`, prints the PATH warning and `next: ipm onboard`; `$HOME/bin/ipm version` prints `ipm 0.2.0`.

- [ ] **Step 3: `.envrc`** — remove `PATH_add bin`; replace its comment with: development builds run as `bin/ipm`; `./install.sh` installs `ipm` on PATH.

- [ ] **Step 4: Docs** — getting started opens with `./install.sh` (or `curl -fsSL https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/install.sh | bash`) and `ipm onboard`; commands reference sections for `ipm onboard` and `ipm uninstall` with every flag and the messages above; sources guide section "Curated lists" with the format (spec §format); troubleshooting entries for the offline note, `--keep`/`--fresh` in scripts, and the dev-build refusal; README quick start; CLAUDE.md commands table (replace the `direnv allow` row with `./install.sh`, add `onboard` and `uninstall` rows) and AGENTS.md; `cli/README.md` and main.go's command list.

- [ ] **Step 5: Run** `mise run cli:test` — Expected: PASS. Commit — `git add -A install.sh .envrc mise.toml README.md docs CLAUDE.md AGENTS.md cli && git commit -m "feat: install.sh, and docs for onboard and uninstall"` (exclude the untracked `ipm.toml` and `.ipm/`).

### Task 7: live run

No code; prove the success criteria. Uses a temp `HOME` so the user's real data and binary are untouched.

- [ ] **Step 1:** `export T=$(mktemp -d); HOME=$T IPM_HOME=$T/ipm IPM_CACHE_DIR=$T/cache IPM_BIN_DIR=$T/bin ./install.sh </dev/null` → `next: ipm onboard`.
- [ ] **Step 2:** `HOME=$T IPM_HOME=$T/ipm IPM_CACHE_DIR=$T/cache $T/bin/ipm onboard --yes` — Expected: `✓ added 47 sources`, `✓ sync`, stats line, search line, `✓ library ready — 47 sources, …`. (Its `ipm serve` recreates `ipm-typesense` with the temp key and data folder.)
- [ ] **Step 3:** same env, `$T/bin/ipm search tdd --limit 3` → results. Then `$T/bin/ipm onboard --fresh --lists "AI · Diagrams"` → backup paths printed, `✓ library ready`. Then `$T/bin/ipm uninstall --purge --yes` → binary and both temp dirs gone.
- [ ] **Step 4:** Restore the real search server: `ipm serve && ipm load` with the real environment (`bin/ipm` from the checkout) — Expected: `ipm search tdd` works on the real library again.
- [ ] **Step 5:** `go test -race ./...` in `cli/` — Expected: PASS. Record the outputs in the ledger.
