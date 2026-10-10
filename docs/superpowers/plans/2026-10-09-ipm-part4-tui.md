# ipm part 4 — TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `ipm tui`, a Bubble Tea screen for the curation loop — search, judge, collect into packs, build — over ipm's existing search, catalog, packs and install.

**Architecture:** `cli/internal/tui` holds the Bubble Tea model and depends only on four interfaces (`Searcher`, `Catalog`, `Packs`, `Installer`). `cli/internal/cli/tui.go` implements them over existing packages and registers the Cobra command. Pack create/add/remove logic moves out of printing code so the CLI and the TUI share it.

**Tech Stack:** Go 1.26, `charm.land/bubbletea/v2` v2.1.0, `charm.land/bubbles/v2` v2.2.1 (`textinput`, `viewport`), `charm.land/lipgloss/v2` v2.0.6, `github.com/charmbracelet/x/ansi` (already a Lip Gloss dependency) for stripping styles in golden tests.

**Spec:** `docs/superpowers/specs/2026-10-09-ipm-tui-design.md`

## Global Constraints

- The gate is `mise run cli:test` (gofmt, vet, test); run `go test -race ./...` from `cli/` before the final review.
- `internal/tui` imports no ipm package except through its own interfaces; it never touches `System`, files, Typesense or pnpm.
- The TUI adds no behaviour to packs, search ranking or install: hybrid search uses `search.Query` and the Typesense client exactly as `ipm search` does; pack edits go through the functions shared with `ipm pack`; install is `runInstall`.
- Palette (from `docs/assets/brand`): background `#11151C`, text `#F3EFE6`, secondary text `#C9D1D9`, muted `#8B95A3`, border `#2C3747`, amber `#F0A847` (pins, focus, primary), teal `#5CC8A8` (✓).
- Copy fixed by the spec: banner "server off: keyword search only — run `ipm serve` and `ipm load` for semantic search"; empty-index banner "search index empty — run `ipm load`"; errors "ipm tui needs a terminal" and "no catalog — run `ipm sync`"; tray marker "• not built".
- Results are capped at 50 per search; debounce is 150 ms; the detail pane hides below 100 columns; below 60×15 the screen asks to enlarge the terminal.
- Prose in Markdown is not hard-wrapped.

## Review Focus

1. A query typed quickly while an earlier search is still running — only the newest query's results may appear (Task 2: `TestStaleSearchDropped`).
2. Adding an item already in the active pack — no duplicate, and the status says it is already there (Task 4: `TestAddExistingItem`).
3. Very long names, descriptions and IDs in a narrow terminal — rows are cut with `…`, never wrapped, and the layout holds (Task 5: golden `narrow-long`).
4. `ipm.toml` made invalid or edited outside the TUI during a session — the tray reload shows the error in the status bar and the TUI keeps running (Task 4: `TestPacksErrorShown`).
5. Install failing (pnpm or validation) — the modal shows the output and `✗ <error>`, `• not built` stays, and `q` works afterwards (Task 5: `TestInstallFailure`).

---

### Task 1: Shared pack operations and an exported SCORE

**Files:**
- Modify: `cli/internal/cli/packcmd.go`
- Modify: `cli/internal/search/search.go`
- Test: `cli/internal/cli/cli_test.go`, `cli/internal/search/search_test.go`

**Interfaces:**
- Produces (package `cli`):
  - `createPack(scope project.Scope, name, description string) error` — validation and save from `packNew`, no output.
  - `addToPack(sys System, scope project.Scope, name string, ids []string) (project.Pack, error)` — loads the catalog, checks every ID with `pickable` before writing (all-or-nothing), skips IDs already present, saves, returns the updated pack.
  - `removeFromPack(scope project.Scope, name string, ids []string) (project.Pack, error)`.
  - `packNew`, `packEdit` print exactly what they print today, calling these.
- Produces (package `search`): `Score(h typesense.Hit, top int64) string` (the current unexported `score`, renamed; `Table` uses it).

- [ ] **Step 1: Write the failing tests** in `cli_test.go`: `TestPackOperationsWithoutOutput` — with `packFake`, `createPack` then `addToPack(…, []string{"skill:o/r:p/skills/s1"})` returns a pack whose `Items` equal `["skill:o/r:p/skills/s1"]`; adding the same ID again leaves one copy; `addToPack` with a hook ID returns an error containing "comes only with its plugin" and leaves `Items` unchanged; `removeFromPack` empties it; nothing is written to `f.out`. In `search_test.go`, `TestScoreExported`: `Score(hit("x", 0, 0.42, -1), 0) == "42"`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/cli ./internal/search -run 'TestPackOperationsWithoutOutput|TestScoreExported'` — Expected: build failure, `undefined: createPack` / `undefined: Score`.
- [ ] **Step 3: Implement** the three functions in `packcmd.go` by moving logic out of `packNew`/`packEdit`; rename `score` → `Score` in `search.go`.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS (existing pack tests unchanged).
- [ ] **Step 5: Commit** `refactor(cli): pack operations shared by the CLI and the TUI`.

### Task 2: TUI model — search, results, status bar

**Files:**
- Create: `cli/internal/tui/deps.go`, `model.go`, `results.go`, `keys.go`, `style.go`
- Test: `cli/internal/tui/model_test.go` (fakes and helpers live here; later tasks extend them)

**Interfaces:**
- Produces (package `tui`), as in the spec's Interfaces section, with these refinements:
  - `Searcher` also has `Notice() string` (the banner; `""` in hybrid mode) and `Recheck()` (repeats the health check; `ctrl+r`).
  - `type Deps struct{ Searcher Searcher; Catalog Catalog; Packs Packs; Installer Installer }`
  - `func New(d Deps) *Model`; `*Model` implements `tea.Model` (`View() tea.View`, `AltScreen: true`).
  - Unexported messages: `searchTick{seq int}`, `searchDone{seq int; res Results; err error}`.
  - `Model.debounce time.Duration` (150 ms from `New`; tests set 0).
- Test helpers (in `model_test.go`): `key(s string) tea.KeyPressMsg` (single runes, `"enter"`, `"esc"`, `"tab"`, `"up"`, `"down"`, `"left"`, `"right"`, `"backspace"`, `"ctrl+c"`, `"ctrl+r"`); `typeText(m, s)`; `drain(m, cmd)` that executes commands and feeds their messages back until none remain (unwrapping `tea.BatchMsg`, skipping `tea.QuitMsg`); `fakeSearcher` recording queries, with per-query results and an optional delay channel.

- [ ] **Step 1: Write the failing tests:**
  - `TestTypingSearches`: type "tdd", drain → `fakeSearcher.queries` ends with `"tdd"`, the results list holds the fake's hits, selection is 0, and the view shows "<len(hits)> of <Found>" (e.g. "50 of 100").
  - `TestStaleSearchDropped`: deliver `searchDone{seq: 1}` after `searchDone{seq: 2}` → results are seq 2's.
  - `TestEmptyQueryClears`: type "x", drain, backspace, drain → no results and no new search.
  - `TestSearchErrorKeepsResults`: a second search failing shows the error in `m.status` and keeps the first results; the next keystroke clears `m.status`.
  - `TestTypeFilterCycles`: `f` with results focused cycles `"" → skill → agent → command → plugin → ""` and re-searches with that `Filter.Type`.
  - `TestNavigation`: `down` from search focuses results; `j`/`k` and `down`/`up` move the selection within bounds; `/` refocuses search.
  - `TestQuit`: `q` (results focused) and `ctrl+c` return `tea.Quit`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui` — Expected: FAIL (package does not build).
- [ ] **Step 3: Implement** the model: `textinput` for the query; keystrokes in search focus update the query and schedule `tea.Tick(m.debounce, …searchTick{seq})`; a matching tick starts the search command with a context; results render one row each as `SCORE TYPE ✓ NAME` cut to the pane width; status bar line under the tray; header shows `● hybrid` / `○ local` and the `Notice()` banner. Add the Charm modules with `go get charm.land/bubbletea/v2@v2.1.0 charm.land/bubbles/v2@v2.2.1 charm.land/lipgloss/v2@v2.0.6`.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): search model with debounced results`.

### Task 3: Detail pane and narrow layouts

**Files:**
- Create: `cli/internal/tui/detail.go`
- Modify: `cli/internal/tui/model.go`
- Test: `cli/internal/tui/model_test.go` (`fakeCatalog`)

**Interfaces:**
- Consumes: `Catalog.Detail(id string) (Detail, error)`.
- Produces: detail focus with a neighbour cursor and a back stack (`[]string` of IDs).

- [ ] **Step 1: Write the failing tests:**
  - `TestDetailFollowsSelection`: moving the selection calls `Detail` with that hit's ID; the detail shows its ID, repo, stars and `Up`/`Down` neighbours.
  - `TestWalkNeighbours`: `right` focuses neighbours, `down` then `enter` opens the second neighbour (Detail called with its ID), `backspace` returns to the previous ID; `left` returns focus to results.
  - `TestNarrowHidesDetail`: after `tea.WindowSizeMsg{Width: 90, Height: 30}` the view has no detail pane; `enter` on a result shows the detail full-width; `esc` returns.
  - `TestTooSmall`: `WindowSizeMsg{Width: 50, Height: 12}` → the view asks to enlarge the terminal and keys other than `q`/`ctrl+c` do nothing.
  - `TestDetailError`: a `Detail` error appears in the status bar.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Detail|Walk|Narrow|TooSmall'` — Expected: FAIL.
- [ ] **Step 3: Implement** the detail pane (a `viewport` for long descriptions), the neighbour cursor and back stack, and layout by width: `≥100` results + detail side by side (results 45%), `60–99` results only, `<60×15` the enlarge message.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): detail pane with walkable relationships`.

### Task 4: Packs — tray, add/remove, picker

**Files:**
- Create: `cli/internal/tui/tray.go`, `picker.go`
- Modify: `cli/internal/tui/model.go`, `results.go`
- Test: `cli/internal/tui/model_test.go` (`fakePacks`)

**Interfaces:**
- Consumes: `Packs` (spec), `Scope() string`.
- Produces: `m.active string` (active pack name), `m.dirty bool` (drives "• not built").

- [ ] **Step 1: Write the failing tests:**
  - `TestActivePackAtStart`: with packs `b` and `a`, the tray shows `a` (first by name) with its count and `Scope()`.
  - `TestAddAndMark`: `a` on a selected result calls `Add("a", id)`; the tray reloads; the result row shows `✓`; the tray header contains "• not built".
  - `TestAddExistingItem`: `a` on an item already in the pack does not call `Add` and the status says it is already in the pack.
  - `TestAddRefused`: `Add` returning "x comes only with its plugin: `ipm pack add a plugin:o/r:p`" puts that text in the status bar; nothing else changes.
  - `TestRemove`: `d` on a result in the pack calls `Remove`; `tab` from results focuses the tray, `d` there removes the tray's selected item; `tab` from the tray returns focus to search.
  - `TestPickerSwitchAndCreate`: `p` lists packs; `down`, `enter` switches `m.active`; `p`, `n`, typing "kit", `enter`, typing "My kit", `enter` calls `Create("kit", "My kit")` and makes it active; `esc` cancels.
  - `TestNoPacksAddOpensNew`: with no packs, `a` opens the picker in new-pack mode.
  - `TestPacksErrorShown`: `List` returning an error shows it in the status bar and keeps the TUI running.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Pack|Add|Remove|Picker'` — Expected: FAIL.
- [ ] **Step 3: Implement** the tray (one line of items, cut with `…`, header `<scope> · <pack> (<n>)` plus "• not built" when dirty), the picker modal (list, then two `textinput`s for name and description), and the `a`/`d` handling for results, detail neighbours and tray.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): collect into packs from search and detail`.

### Task 5: Install modal, help overlay, styling and golden views

**Files:**
- Create: `cli/internal/tui/install.go`, `cli/internal/tui/testdata/*.golden`, `cli/internal/tui/view_test.go`
- Modify: `cli/internal/tui/model.go`, `keys.go`, `style.go`
- Test: `cli/internal/tui/model_test.go` (`fakeInstaller`), `view_test.go`

**Interfaces:**
- Consumes: `Installer.Install(ctx context.Context, out io.Writer) error`.
- Produces: unexported `installLine{text string}`, `installDone{err error}`; the install command writes into an `io.Pipe` and a reader command emits one `installLine` per line until EOF.

- [ ] **Step 1: Write the failing tests:**
  - `TestInstallStreams`: `i` starts install; the fake writes "✓ kit: 2 item(s)" and "validate: ok"; draining shows both lines in the modal, then "✓ installed"; `m.dirty` is false; `esc` closes the modal.
  - `TestQuitIgnoredDuringInstall`: before `installDone`, `q` and `ctrl+c` return no `tea.Quit` and the status says install is running; `esc` does not close the modal.
  - `TestInstallFailure`: the fake returns an error after one line → modal shows the line and "✗ <error>", `m.dirty` stays true, `esc` closes, then `q` quits.
  - `TestHelpOverlay`: `?` shows every key from the spec's key table; `?` or `esc` hides it.
  - `TestRecheck`: `ctrl+r` calls `Searcher.Recheck` and re-runs the current query.
  - `view_test.go`: `TestGoldenViews` renders `ansi.Strip(m.View().Content)` at fixed sizes and compares with `testdata/<name>.golden` (`-update` rewrites): `main` (120×30, results, detail, tray), `narrow-long` (80×24, a 120-character name, description and ID), `picker`, `install`, `local-banner` (Notice set), `help`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Install|Help|Recheck|Golden'` — Expected: FAIL.
- [ ] **Step 3: Implement** the install modal (`viewport` following the tail), the help overlay from the key map, and Lip Gloss styles in the palette (amber focus border, teal ✓, muted secondary text). Generate goldens with `go test ./internal/tui -run Golden -update`, then read each golden file and check it against the spec's layout sketch before committing them.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): install modal, help and styling`.

### Task 6: `ipm tui` — adapters and command

**Files:**
- Create: `cli/internal/cli/tui.go`
- Modify: `cli/internal/cli/cli.go` (register `newTuiCmd(sys)`), `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: Task 1's pack functions and `search.Score`; `tui.New`, `tui.Deps`.
- Produces:
  - `newTuiCmd(sys System) *cobra.Command` — `ipm tui [-g|--global]`, `noArgs`.
  - `tuiSearcher` (hybrid via `tsClient`, `Health`, `NumDocuments(search.Collection) > 0`; local otherwise) with `localSearch(records []map[string]any, stars func(string) (int, bool), query string, f tui.Filter) tui.Results`: score = 4 per query term in name + 1 per term in description + 1 per term in tags (case-insensitive), drop zero scores, sort by score then stars then ID, top 50, `Found` = all matches.
  - `tuiCatalog` over `catgraph.Graph` (`Node`, `Children(id, false)` → `Down`, `Children(id, true)` → `Up`, `Stars`).
  - `tuiPacks` over `project.Scope` and Task 1's functions; `Scope()` is `"project <root>"` or `"global"`.
  - `tuiInstaller` calling `runInstall(System{…sys, Stdout: out}, global, false, false)`.
  - The command runs `tea.NewProgram(tui.New(deps)).Run()`; with `sys.TermWidth` returning 0 it fails with "ipm tui needs a terminal"; when `catgraph.Load` fails it fails with "no catalog — run `ipm sync`".

- [ ] **Step 1: Write the failing tests:**
  - `TestLocalSearchRanking`: records named "tdd", "tdd-guard" (desc "test-driven"), "pdf" (desc "tdd notes") → query "tdd" ranks `tdd`, `tdd-guard`, `pdf`; ties break by stars; type filter "agent" returns none; 60 matches return 50 hits with `Found` 60.
  - `TestTuiSearcherFallsBack`: with no search server set up (no API key in the cache, so `tsClient` fails), `Mode()` is "local" and `Notice()` is the server-off banner; with an `httptest` server answering `/health` and reporting `num_documents: 0` for the collection, `Mode()` is "local" and `Notice()` is the empty-index banner; reporting 3 documents, `Mode()` is "hybrid" and `Notice()` is "".
  - `TestTuiPacks`: through `tuiPacks`, `Create`, `Add`, `List`, `Remove` round-trip `ipm.toml`; a hook ID is refused with the CLI's message.
  - `TestTuiCatalogDetail`: `Detail("skill:o/r:p/skills/s1")` on `withGraphCatalog` has `Up` containing `plugin:o/r:p`.
  - `TestTuiInstallerStreams`: with `installFake`, the writer receives the "✓ kit:" line.
  - `TestTuiNeedsTerminal`: `f.run("tui")` with `f.width = 0` returns an error "ipm tui needs a terminal"; with `f.width = 120` and no catalog, "no catalog — run `ipm sync`".
- [ ] **Step 2: Run** `cd cli && go test ./internal/cli -run 'Tui|LocalSearch'` — Expected: FAIL.
- [ ] **Step 3: Implement** `tui.go` and register the command.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(cli): ipm tui`.

### Task 7: Docs and a real-terminal walkthrough

**Files:**
- Create: `docs/guides/tui.md` (front matter `title: Use the TUI`, `parent: Guides`, `nav_order: 3`; later guides move down one)
- Modify: `docs/reference/commands.md` (`ipm tui` section), `docs/troubleshooting.md` (the two new errors and the banners), `docs/guides/index.md`, `docs/index.md`, `README.md` (commands table and limits: drop "No TUI yet"), `AGENTS.md`/`CLAUDE.md` (`internal/tui`, parts 1–4 built), `cli/README.md` (layout, testing), `docs/architecture.md` (TUI section and code layout).

- [ ] **Step 1: By hand**, in a real terminal (not the agent's shell): `ipm tui` in the demo project, search "c# testing", walk `csharp-pro` → its plugin → back, add the skill, agent and command and `plugin:anthropics/claude-plugins-official#stripe` to a new pack `csharp-tui`, press `i`, see "validate: ok"; then stop Typesense (`ipm serve --stop`), restart `ipm tui`, see the local banner and keyword results; `ipm serve` again. Record what was seen in the ledger.
- [ ] **Step 2: Write the docs** — the guide covers starting, the layout, the key table (copied from the spec), local mode, and building a pack; every key and message matches the code.
- [ ] **Step 3: Run** `mise run cli:test` and the link check from the docs work (`python3` relative-link checker over `docs/` excluding `superpowers/` and `plans/`, README, AGENTS, CLAUDE, cli/README) — Expected: PASS, `broken 0`.
- [ ] **Step 4: Commit** `docs: ipm tui guide and references`.
