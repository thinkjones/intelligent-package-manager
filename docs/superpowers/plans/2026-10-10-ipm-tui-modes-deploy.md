# ipm TUI modes and deploy — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `ipm tui` a search mode and a tree mode around an always-visible, aligned, colour-coded artifact tree, and add deploy (TUI `D`, CLI `ipm deploy`) that installs a built pack into Claude Code at user, project or local scope.

**Architecture:** `internal/tui` keeps depending only on its interfaces: `Catalog.Detail` becomes `Catalog.Node`, and a fifth interface, `Deployer`, joins `Deps`. The tree is a pure computation (`buildTree`) plus a renderer, both in `internal/tui/tree.go`. Deploy's planning and running live in `internal/cli/deploy.go`, shared by `ipm deploy` and the TUI adapter; `claude` always runs through `System.Output`.

**Tech Stack:** Go 1.26, Cobra, Bubble Tea v2.1.0, Bubbles v2.2.1, Lip Gloss v2.0.6, `claude` 2.1.296.

**Spec:** `docs/superpowers/specs/2026-10-10-ipm-tui-modes-deploy-design.md`

## Global Constraints

- Work on branch `feat/tui-modes-deploy`; the gate is `mise run cli:test`; run `go test -race ./...` from `cli/` before the final review.
- Tests never run the real `claude`; every `claude` call goes through `System.Output(dir, "claude", args...)`. Anything run by hand against `claude` uses `CLAUDE_CONFIG_DIR` set to a temporary directory.
- `internal/tui` imports no ipm package; new behaviour reaches it only through `Catalog`, `Deployer` and the existing interfaces.
- Tree layout values: type label 6 cells (`repo`, `market`, `plugin`, `copilot`, `skill`, `agent`, `cmd`, `hook`, `mcp`); names cut to 24 cells; the cursor's description wraps to at most 3 lines; path walk stops after 10 levels; results 40% / tree 60% of the body; tree drops children under 24 rows; repo and stars drop from result rows under 100 columns; enlarge message under 60×15.
- Type colours: repo `#7AA2F7`, marketplace `#BB9AF7`, plugin and copilot-plugin `#F0A847`, skill `#5CC8A8`, agent `#F7768E`, command `#9ECE6A`, hook and mcp `#8B95A3`. Focused panel border and title amber `#F0A847`, unfocused `#2C3747`. `NO_COLOR` set → no colour, same layout.
- Copy fixed by the spec: the confirm header `Deploy <pack> to Claude Code — <scope> scope[ in <path>]`, the line `y deploy · esc cancel`, the result `✓ deployed <pack> (<scope>[ <path>])`, the row `also in: <type> <name>, …`, scroll rows `↑ N more` / `↓ N more`.
- Deploy always runs `ipm install` first; the marketplace name is read from `<ipm dir>/.claude-plugin/marketplace.json`.
- Prose in Markdown is not hard-wrapped.

## Review Focus

1. `claude plugin list --json` or `marketplace list --json` output that can't be parsed (another `claude` version) — deploy stops with an error naming the command instead of guessing (Task 1: `TestPlanDeployUnreadableClaudeJSON`).
2. A project or local path typed as `~/work` or with a trailing slash — expanded and cleaned before use and in the confirm step (Task 1: `TestDeployTargetPathExpanded`).
3. A plugin with 80 children or siblings — the cursor stays visible between `↑ N more` / `↓ N more` rows and nothing wraps (Task 2: `TestTreeScrollsLongSiblingLists`).
4. Wide characters (CJK, emoji) in names — the description column still starts at the same cell on every row (Task 2: `TestTreeColumnAlignedWithWideNames`).
5. `D` with no active pack, or while install or deploy is running — a status message, no modal, no crash (Task 5: `TestDeployWithoutPack`, `TestDeployIgnoredWhileRunning`).

---

### Task 1: Deploy in the CLI (`ipm deploy`)

**Files:**
- Create: `cli/internal/cli/deploy.go`, `cli/internal/cli/testdata/claude-plugin-list.json`, `cli/internal/cli/testdata/claude-marketplace-list.json`
- Modify: `cli/internal/cli/cli.go` (register `newDeployCmd(sys)`)
- Test: `cli/internal/cli/cli_test.go`

**Interfaces:**
- Produces (package `cli`):
  - `type deployTarget struct{ Scope, Path string }` — Scope `user`, `project` or `local`; Path only for project and local.
  - `type claudeCmd struct{ Dir string; Args []string }`; `func (c claudeCmd) String() string` → `claude <args joined>` (used in the confirm step and printed before running).
  - `func cleanTarget(t deployTarget, cwd, home string) (deployTarget, error)` — validates Scope, expands a leading `~`, makes Path absolute and clean, requires an existing directory; user scope clears Path.
  - `func planDeploy(sys System, ipmDir, pack string, t deployTarget) ([]claudeCmd, error)` — reads the marketplace name from `<ipmDir>/.claude-plugin/marketplace.json`; runs `claude plugin marketplace list --json` and `claude plugin list --json` (Dir = t.Path, or `ipmDir` for user scope); returns `marketplace add <ipmDir> --scope S` or `marketplace update <mp>`, then `install <pack>@<mp> --scope S` or `update <pack>@<mp> --scope S`, each with `Dir` = t.Path (user scope: the scope root).
  - `func runDeploy(sys System, global bool, pack string, t deployTarget, out io.Writer) error` — `runInstall` with `Stdout = out` and `noValidate=false`, then `planDeploy`, then each command printed as `$ <cmd>` and run via `sys.Output(cmd.Dir, "claude", cmd.Args...)`, its output written to `out`; stops at the first failure with an error naming the command; ends by printing `✓ deployed <pack> (<scope>[ <path>])`.
  - `newDeployCmd(sys) *cobra.Command`: `ipm deploy [pack] [--scope user|project|local] [--path DIR] [-g|--global]`; default scope `project` with path = scope root (`user` with `-g`); pack defaults to the only pack, else a `UsageError` "deploy: name a pack — this scope has <n>".

- [ ] **Step 1: Spike (no code kept).** With `export CLAUDE_CONFIG_DIR=$(mktemp -d)` and a scratch project built by `ipm install` (two packs), run and record in the ledger: `claude plugin marketplace add <dir>/.ipm --scope project` (cwd = project), `claude plugin install <pack>@<mp> --scope project`, then `claude plugin marketplace list --json` and `claude plugin list --json`, then the same `add` and `install` again (record whether they fail or succeed), then `claude plugin marketplace update <mp>` and `claude plugin update <pack>@<mp> --scope project`. Save the two `--json` outputs as the testdata fixtures. Ledger which fields identify a marketplace by name and an installed plugin by `<pack>@<mp>` and scope. If a repeated `add`/`install` already succeeds as an update, record the ruling and still use `update` when the state says it exists.
- [ ] **Step 2: Write the failing tests** (fake `System`; `installFake` builds the `.ipm/`):
  - `TestPlanDeployFirstTime`: empty JSON lists (`[]`) → exactly `marketplace add <ipm> --scope project` and `install kit@work-ipm --scope project`, both with `Dir` = the project root.
  - `TestPlanDeployAgain`: the fixtures (marketplace and plugin present at project scope) → `marketplace update work-ipm` and `update kit@work-ipm --scope project`.
  - `TestPlanDeployUserScope`: user scope → `--scope user` on both and no path in the commands.
  - `TestPlanDeployUnreadableClaudeJSON`: `claude plugin list --json` returning `not json` → error containing `claude plugin list --json`.
  - `TestDeployTargetPathExpanded`: `cleanTarget({project, "~/work/"}, …)` with HOME set → Path `<home>/work`; a file path or missing directory → error; scope `team` → error.
  - `TestRunDeployBuildsThenRuns`: `f.run("deploy", "kit")` runs pnpm (the build) before any `claude` call, then the planned `claude` calls in order, prints `$ claude plugin install kit@work-ipm --scope project` and `✓ deployed kit (project <root>)`.
  - `TestRunDeployStopsOnFailure`: the `install` call failing → error naming it, no later calls, no `✓`.
  - `TestDeployNeedsClaude` (claude not on PATH → error mentioning `claude`) and `TestDeployPackRequired` (two packs, no name → `UsageError`).
- [ ] **Step 3: Run** `cd cli && go test ./internal/cli -run 'Deploy'` — Expected: FAIL (`undefined: planDeploy` …).
- [ ] **Step 4: Implement** `deploy.go` and register the command.
- [ ] **Step 5: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 6: Commit** `feat(cli): ipm deploy — install a built pack into Claude Code`.

### Task 2: The tree — `Catalog.Node`, `buildTree`, and its rendering

**Files:**
- Create: `cli/internal/tui/tree.go`, `cli/internal/tui/tree_test.go`
- Modify: `cli/internal/tui/deps.go` (`Ref`, `Node`, `Catalog.Node` replacing `Detail`, `Neighbour`), `cli/internal/tui/detail.go` (removed), `cli/internal/tui/model.go`, `cli/internal/tui/model_test.go` (`fakeCatalog` serves `Node`), `cli/internal/cli/tui.go` (`tuiCatalog.Node`)
- Test: `cli/internal/tui/tree_test.go`, `cli/internal/cli/cli_test.go`

**Interfaces:**
- Produces (package `tui`), types as in the spec's Architecture section, plus:
  - `type treeView struct { Path []Ref; Siblings []Ref; Cursor int; Node Node; Children []Ref; AlsoIn []Ref }` — `Path` runs root → parent; `Siblings[Cursor]` is the node.
  - `func buildTree(c Catalog, id string, chosen map[string]string) (treeView, error)` — `chosen[id]` is the parent the user picked for `id` (tree mode); otherwise `Parents[0]`. A root's `Siblings` is just itself. `AlsoIn` is the node's other parents. The walk stops after 10 levels.
  - `func (tv treeView) lines(w, rows int, sel int, focused bool) []string` — exactly `rows` lines of width `w`; `sel` indexes the selectable rows (siblings with the node's `AlsoIn` inserted right after it) and is `-1` in search mode (the cursor is marked `▶` either way).
  - `func typeLabel(t string) string` (the 6-cell labels) and `func typeStyle(t string) lipgloss.Style` (the type colours).
- Produces (package `cli`): `tuiCatalog.Node(id) (tui.Node, error)` with `Parents = Children(id, true)` and `Children = Children(id, false)` as `tui.Ref`s carrying each node's description.

- [ ] **Step 1: Write the failing tests** in `tree_test.go` against a small in-memory catalog (repo → marketplace → plugin → skills `a`, `b`, `c`; skill `b` also in plugin `p2`):
  - `TestBuildTreePath`: cursor `b` → `Path` IDs repo, marketplace, plugin; `Siblings` a, b, c; `Cursor` 1; `AlsoIn` [p2].
  - `TestBuildTreeChosenParent`: `chosen{b: p2}` → the path goes through p2 and `AlsoIn` is [plugin].
  - `TestBuildTreeRoot` (repo cursor: no path, siblings only itself, children = marketplace) and `TestBuildTreeDepthCap` (a 15-level chain stops at 10).
  - `TestTreeLines`: at 100×12 the rows show the guides, `skill ` labels, names, and every description starting at the same cell; the cursor row contains `▶b ` and `▶` before its description; the cursor's long description wraps to 3 lines; the `also in: plugin p2` row follows.
  - `TestTreeScrollsLongSiblingLists`: 80 siblings, cursor 60, 14 rows → `↑` and `↓ … more` rows, the cursor row visible, every line exactly 14 rows of the width.
  - `TestTreeColumnAlignedWithWideNames`: names `日本語のスキル` and `🚀 rocket` → description column starts at the same display cell (measured with `lipgloss.Width` on the stripped prefix).
  - `TestTreeNameCut`: a 40-character name renders as 23 characters plus `…`.
  - In `cli_test.go`, `TestTuiCatalogNode`: `skill:o/r:p/skills/s1` on `withGraphCatalog` has `Parents` containing `plugin:o/r:p` with its description.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui ./internal/cli -run 'Tree|TuiCatalogNode'` — Expected: FAIL.
- [ ] **Step 3: Implement** `tree.go`, switch the model's detail pane to the tree with `cursor` = the selected result (search mode only; navigation is Task 3), delete `detail.go` and the tests that exercised its neighbour walking (`TestWalkNeighbours`, `TestDetailNeighboursScroll`, `TestDetailWithoutNeighbours` are replaced by Task 3's tree-mode tests), and update the adapter.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS (goldens rewritten with `go test ./internal/tui -run Golden -update`; read each diff).
- [ ] **Step 5: Commit** `feat(tui): an aligned artifact tree in place of the detail pane`.

### Task 3: Search mode and tree mode

**Files:**
- Modify: `cli/internal/tui/model.go`, `cli/internal/tui/tray.go` (`selectedID` from the tree), `cli/internal/tui/keys.go` (two help tables)
- Test: `cli/internal/tui/model_test.go`

**Interfaces:**
- Consumes: Task 2's `buildTree`, `treeView.lines`.
- Produces: `focusTree` replaces `focusDetail`; model fields `cursor string`, `chosen map[string]string`, `treeSel int`; `keyHelp` becomes `searchKeys` and `treeKeys` (`[][2]string`, the spec's two tables).

- [ ] **Step 1: Write the failing tests:**
  - `TestSearchModeArrowsMoveTree`: results focused, `right` → cursor = the result's first child, `m.sel` unchanged; `left` twice → its grandparent; `down` → cursor resets to the new result.
  - `TestLeftAtRootRightAtLeaf`: `left` at a repo and `right` at a leaf leave the cursor unchanged.
  - `TestTreeModeSiblings`: `tab` → `focusTree`; `down` → cursor = next sibling; `up` back; `k`/`j` too.
  - `TestTreeModeAlsoIn`: on `b` (also in p2), `down` selects the `also in` row without moving the cursor; `enter` → path through p2 (`m.chosen["b"] == p2`).
  - `TestTreeModeExit`: `esc` and `tab` → search mode, cursor = the selected result.
  - `TestTreeModeAdd`: `a` in tree mode on a plugin cursor → `Add(pack, pluginID)`.
  - `TestEnterFromResultsOpensTreeMode`: `enter` on a result (wide terminal) → `focusTree`.
  - `TestHelpListsBothModes`: `?` shows every row of `searchKeys` and `treeKeys`.
  - `TestBreadcrumb`: header contains `repo › market › plugin › b` names joined by ` › `.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Mode|Arrows|LeftAt|AlsoIn|Breadcrumb|Help'` — Expected: FAIL.
- [ ] **Step 3: Implement** the key routing for both modes, the breadcrumb (cut from the left with `…` to fit), and the help tables.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): search mode and tree mode`.

### Task 4: Layout and colour

**Files:**
- Modify: `cli/internal/tui/model.go` (`View`), `cli/internal/tui/results.go`, `cli/internal/tui/style.go`, `cli/internal/tui/tray.go`
- Test: `cli/internal/tui/view_test.go`, `cli/internal/tui/testdata/*.golden`

**Interfaces:**
- Produces: `func panel(title string, lines []string, w int, focused bool) []string` (rounded border, title in the top border, amber when focused); `func starStyle(n int) lipgloss.Style` (under 100 muted, under 10k soft, else bright); colour disabled when `os.Getenv("NO_COLOR") != ""` at `New`.

- [ ] **Step 1: Write the failing tests:**
  - `TestPanelsByWeight`: at 120×40 the results panel's height is 40% of the body (rounded down) and the tree's the rest; both panels' lines are exactly the width.
  - `TestFocusedPanelBorder`: with results focused the results panel's top border has the amber colour code and the tree's doesn't; in tree mode the reverse.
  - `TestResultRowsNarrow`: at 90 columns result rows have no repo or stars; at 120 they do.
  - `TestShortTerminalTreeWithoutChildren`: at 120×22 the tree shows path and siblings and no children rows.
  - `TestNoColor`: with `NO_COLOR=1` `View().Content` has no `\x1b[` sequences and equals the stripped coloured view.
  - Goldens (rewrite and read each): `search-mode`, `tree-mode`, `deep-tree-scroll`, `also-in`, `narrow`, `no-color`; keep `picker`, `install`, `local-banner`, `help`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Panel|Focused|Narrow|ShortTerminal|NoColor|Golden'` — Expected: FAIL.
- [ ] **Step 3: Implement** the bordered layout, type colours in results, tree and breadcrumb, star tints, hints by mode, and `NO_COLOR`.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): bordered panels and colour by type`.

### Task 5: Build and deploy in the TUI

**Files:**
- Create: `cli/internal/tui/deploy.go`
- Modify: `cli/internal/tui/deps.go` (`Target`, `Deployer`, `Deps.Deployer`), `cli/internal/tui/model.go`, `cli/internal/tui/install.go` (the streaming view reused for deploy), `cli/internal/cli/tui.go` (`tuiDeployer`)
- Test: `cli/internal/tui/model_test.go` (`fakeDeployer`), `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: Task 1's `cleanTarget`, `planDeploy`, `runDeploy`.
- Produces: `tuiDeployer{sys, scope, global}` implementing `tui.Deployer`: `Default` (project + scope root, or user with `-g`), `Plan` (`["ipm install", <claudeCmd.String()>…]` after `cleanTarget`), `Deploy` (`runDeploy`).

- [ ] **Step 1: Write the failing tests:**
  - `TestBuildKey`: `b` and `i` both start the install modal.
  - `TestDeployFlow`: `D` → target step with `project` selected (the `Default`); `enter` → path step prefilled with the default path; `enter` → confirm step listing the fake's `Plan` lines under `Deploy kit to Claude Code — project scope in /work` and `y deploy · esc cancel`; `y` → streamed lines then `✓ deployed kit (project /work)`.
  - `TestDeployUserSkipsPath`: choosing `user` goes straight to confirm.
  - `TestDeployBadPath`: `Plan` returning an error (not a directory) keeps the path step open with the error in the status bar.
  - `TestDeployFailure`: `Deploy` failing → `✗ <error>` in the modal; `esc` closes.
  - `TestDeployCancel`: `esc` at each step closes without calling `Deploy`.
  - `TestDeployWithoutPack`: no packs → status `no pack to deploy — press p to create one`, no modal.
  - `TestDeployIgnoredWhileRunning`: `D` and `b` while install runs → the running status message, no new modal.
  - In `cli_test.go`, `TestTuiDeployerPlan`: `Plan("kit", {project, root})` on `installFake` returns `ipm install` then the `claude` commands of Task 1.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui ./internal/cli -run 'Deploy|BuildKey'` — Expected: FAIL.
- [ ] **Step 3: Implement** the modal (target list, `textinput` path, confirm, streamed run sharing `install`'s pipe reader) and the adapter.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): build and deploy packs to Claude Code`.

### Task 6: Docs and a real-terminal check

**Files:**
- Modify: `docs/guides/tui.md`, `docs/reference/commands.md` (`ipm deploy`), `docs/troubleshooting.md`, `README.md`, `docs/index.md`, `docs/concepts.md`, `docs/architecture.md`, `AGENTS.md`, `CLAUDE.md`, `cli/README.md` — the principle "ipm never installs into Claude Code" becomes "ipm installs into Claude Code only when you deploy", everywhere it appears.

- [ ] **Step 1: By hand** under tmux with `CLAUDE_CONFIG_DIR=$(mktemp -d)`: `ipm tui` in a scratch project; search, `←`/`→` through a plugin's tree in search mode; `tab` into tree mode, `↑`/`↓` siblings, follow an `also in`; `D` → project → confirm → `y` → `✓ deployed`; `claude plugin list` in that config shows the pack; deploy again → the update path runs. Ledger what was seen.
- [ ] **Step 2: Write the docs**; every key, command and message matches the code.
- [ ] **Step 3: Run** `mise run cli:test`, the relative-link check over `docs/` (excluding `superpowers/`, `plans/`), README, AGENTS, CLAUDE and cli/README, and a local Jekyll build — Expected: PASS, `broken 0`, `done`.
- [ ] **Step 4: Commit** `docs: TUI modes, tree and deploy`.
