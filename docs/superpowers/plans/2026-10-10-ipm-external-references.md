# ipm external references — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make repos the roots of every tree, show references between repos as external entries and "used by" information (results, tree, `e`), and rank repo and plugin name matches higher.

**Architecture:** Reference resolution is a pure function in `internal/catalog` (`BuildRefs`), used by `internal/catgraph` (which stops linking external entries to their source repos as children) and by `internal/search` (the `used_by` and `external` document fields; `search` can't import `catgraph`, which imports `search`). The TUI gets references through `tui.Node`, `tui.Ref` and `tui.Hit`, and adds a back stack for jumps and an `e` modal.

**Tech Stack:** Go 1.26, Typesense 30.2, Bubble Tea v2.1.0, Lip Gloss v2.0.6.

**Spec:** `docs/superpowers/specs/2026-10-10-ipm-external-references-design.md`

## Global Constraints

- Branch `feat/external-refs` (stacked on `feat/tui-modes-deploy`); gate `mise run cli:test`; `go test -race ./...` from `cli/` before the final review.
- `internal/tui` imports no ipm package; references reach it only through `Node`, `Ref`, `Hit` and the existing interfaces.
- Search weights: `query_by` `name,repo,plugin_names,description,tags` weights `4,3,3,2,2` (hybrid adds `embedding` weight 0); sort `_text_match:desc,used_by:desc,stars:desc`; `token_separators` `["/", "-", "_"]`. Local search: name 4, repo 3, plugin names 3, description 2, tags 2 per matching query word; ties by used-by, then stars, then ID.
- Copy fixed by the spec: tree back root `↩ <repo> — back to <entry name>`; used-by suffix `↗ used by N`; `e` header `External references to <repo> — used by N repos`, empty `no other repo references <repo>`, hint `enter jump · esc close`; unresolved entry `↗ <source repo> (not a source — ipm source add <id>)`; old index `the search index is from an older ipm — run ipm load`.
- The back stack holds at most 20 entries.
- Prose in Markdown is not hard-wrapped.

## Review Focus

1. An external entry whose source repo isn't one of the user's sources — no target, the row says so, `→` only explains (Task 1: `TestRefsUnresolvedSource`; Task 4: `TestJumpUnresolved`).
2. A marketplace listing its own repo's plugin through a URL source (self-reference) — not external, not counted (Task 1: `TestRefsSelfReferenceNotExternal`).
3. Two repos listing each other (A ↔ B) — jumps are explicit, the back stack unwinds correctly, nothing loops (Task 4: `TestJumpCycleBackStack`).
4. An index built by an older ipm (no `used_by`) — the old-index message, in `ipm search` and the TUI, not a raw Typesense error (Task 2: `TestSearchOldIndex`).
5. The same repo referenced by many entries in one marketplace — `used by` counts distinct repos, the `e` list groups them (Task 1: `TestUsedByDistinctRepos`; Task 5: `TestExternalModalGroups`).

---

### Task 1: References in the catalog and the graph

**Files:**
- Create: `cli/internal/catalog/refs.go`, `cli/internal/catalog/refs_test.go`
- Modify: `cli/internal/catgraph/graph.go`, `cli/internal/catgraph/render.go` (tree from an external entry), `cli/internal/cli/show.go` (`used by:`)
- Test: `cli/internal/catgraph/graph_test.go`, `cli/internal/cli/cli_test.go`

**Interfaces:**
- Produces (package `catalog`):
  - `type Refs struct { Target map[string]string; By map[string][]string; UsedBy map[string][]string }` — `Target[entryID]` = target record ID; `By[targetID]` = entry IDs (sorted); `UsedBy[normalizedRepo]` = distinct other repos (normalized, sorted) with entries targeting anything in it.
  - `func BuildRefs(records map[string][]map[string]any) Refs` — `records` keyed by catalog type. An entry is external when it is a plugin record with `source_repo` whose normalized form (`pack.NormalizeRepo`) differs from its own `repo`. Target: the source repo's plugin whose `path` equals the entry's `entry.source.path` (git-subdir), else the source repo's plugin with the entry's `name`, else the source repo's only plugin, else `repo:<source repo>` when that record exists; none → no `Target`.
- Produces (package `catgraph`): `func (g *Graph) RefTarget(id string) (string, bool)`, `func (g *Graph) RefsTo(id string) []string`, `func (g *Graph) UsedBy(repo string) []string`, `func (g *Graph) External(id string) bool`; `Children(repoID, true)` is always empty.

- [ ] **Step 1: Write the failing tests:**
  - `refs_test.go`: `TestRefsByPath`, `TestRefsByName`, `TestRefsOnlyPlugin`, `TestRefsToRepo` (records: repo `a/m` with marketplace entry `plugin:a/m#x` sourced from `b/s`; repo `b/s` with plugins as each case needs); `TestRefsUnresolvedSource` (source `c/missing` → no `Target`, entry still external); `TestRefsSelfReferenceNotExternal` (`source_repo` `A/M` on repo `a/m` → no `Target`, not in `By`, not in `UsedBy`); `TestUsedByDistinctRepos` (two entries in `a/m` and one in `c/n` targeting `b/s` → `UsedBy["b/s"] == ["a/m", "c/n"]`).
  - `graph_test.go`: `TestRepoHasNoParents` (the graph catalog's `repo:x/c` has no `Children(…, true)` though `plugin:o/r#e` sources it), `TestRefTargetAndRefsTo`, `TestTreeUpStaysInRepo` (`Tree` with `Up` from a component of the external source repo never prints `o/r`).
  - `cli_test.go`: `TestShowUsedBy` — `ipm show repo:x/c` prints `used by:` followed by `plugin:o/r#e`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/catalog ./internal/catgraph ./internal/cli -run 'Refs|UsedBy|RepoHasNoParents|RefTarget|TreeUpStays|ShowUsedBy'` — Expected: FAIL.
- [ ] **Step 3: Implement** `BuildRefs`; in `catgraph.Load` build `Refs` from the loaded records and keep them on the graph; remove the `source_repo` edge in `link()`; `Tree` from an external entry prints one child line `↗ <target label>`; `ipm show` prints `used by:` (entries with their repo) after `down:` when `RefsTo` is not empty.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS (update any existing test that relied on the old edge, and ledger it).
- [ ] **Step 5: Commit** `feat(catalog): references between repos, not parents`.

### Task 2: Search — used-by, external, name boost

**Files:**
- Modify: `cli/internal/search/search.go` (schema, `Documents`, `Query.Params`, `Table` ↗ column), `cli/internal/cli/serve.go` (`tsError`), `cli/internal/cli/tui.go` (hybrid `Hit` fields, `localSearch` weights and tie-break)
- Test: `cli/internal/search/search_test.go`, `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: Task 1's `catalog.BuildRefs`.
- Produces: document fields `used_by` (int32) and `external` (bool); `tui.Hit` gains `UsedBy int`, `External bool` (declared in Task 3's `deps.go` change — this task adds them to `deps.go` first and Task 3 uses them).

- [ ] **Step 1: Write the failing tests:**
  - `TestSchemaReferences`: `Schema()` has fields `used_by` int32 with `optional` false and a default 0 in documents, `external` bool, and `token_separators` `["/", "-", "_"]`.
  - `TestParamsWeights`: hybrid `query_by` = `name,repo,plugin_names,description,tags,embedding`, weights `4,3,3,2,2,0`; keyword mode without `embedding` and weights `4,3,3,2,2`; `sort_by` = `_text_match:desc,used_by:desc,stars:desc`.
  - `TestDocumentsUsedBy`: on a fixture where `a/m`'s marketplace references `b/s`, every `b/s` document has `used_by` 1, the entry document has `external` true and `used_by` 1 (its target repo's count), others 0/false.
  - `TestTableUsedByColumn`: a hit with `used_by` 3 shows `↗3` before its stars at width 160; an `external` hit shows `↗` before its name.
  - `TestSearchOldIndex` (cli): a fake Typesense answering 404 `{"message":"Could not find a field named `used_by` in the schema for sorting."}` → `ipm search x` error is `the search index is from an older ipm — run ipm load`.
  - `TestLocalSearchNameBoost` (cli): records `other` (description "cloudflare tips", repo `x/y`) and `agents-sdk` (repo `cloudflare/skills`) → query `cloudflare` ranks `agents-sdk` first (repo weight 3 > description 2); equal scores order by used-by then stars.
- [ ] **Step 2: Run** `cd cli && go test ./internal/search ./internal/cli -run 'SchemaReferences|ParamsWeights|DocumentsUsedBy|TableUsedBy|OldIndex|LocalSearchNameBoost'` — Expected: FAIL.
- [ ] **Step 3: Implement** the schema, weights, sort, documents (build `Refs` from the loaded records), the ↗ column, the `tsError` mapping (match `used_by` in a Typesense error message), hybrid `Hit.UsedBy`/`External`, and local-search weights and tie-break (`localSearch` gains a `usedBy func(repo string) int` parameter).
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(search): rank repo and plugin names, tie-break by references`.

### Task 3: References in the tree and results

**Files:**
- Modify: `cli/internal/tui/deps.go` (`Ref.Repo`, `Ref.External`, `Node.External`, `Node.UsedBy`, `Node.UsedByRepos`), `cli/internal/tui/tree.go`, `cli/internal/tui/results.go`, `cli/internal/cli/tui.go` (`tuiCatalog.Node` fills them)
- Test: `cli/internal/tui/tree_test.go`, `cli/internal/tui/model_test.go`, `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: Task 1's graph methods; Task 2's `Hit.UsedBy`, `Hit.External`.
- Produces: `Node.External *Ref` (the target, nil when unresolved; `Node.Unresolved string` = the source repo when the entry is external but has no target), `Node.UsedBy []Ref`, `Node.UsedByRepos int`, `Ref.Repo`, `Ref.External`.

- [ ] **Step 1: Write the failing tests:**
  - `TestTreeExternalRow`: a sibling `Ref{External: true}` whose node has `External` = plugin `cloudflare` in `cloudflare/skills` renders `↗cloudflare` (or `↗` before the name for non-cursor rows) and, for the cursor, the description starting `↗ cloudflare/skills — `.
  - `TestTreeUnresolvedExternal`: cursor node with `Unresolved: "c/missing"` shows `↗ c/missing (not a source — ipm source add <id>)`.
  - `TestTreeUsedBySuffix`: a cursor node with `UsedByRepos` 3 shows `↗ used by 3` on its row.
  - `TestResultsUsedByCell`: a hit with `UsedBy` 3 at width 120 shows `↗3`; external hits show `↗` before the name.
  - `TestTuiCatalogNodeReferences` (cli): `tuiCatalog.Node("plugin:o/r#e")` has `External` pointing at `repo:x/c` (graph fixture) and `Ref.External` true for it in its marketplace's `Children`; `Node("repo:x/c")` has `UsedBy` = [`plugin:o/r#e`] with `Repo` `o/r`, `UsedByRepos` 1, and no `Parents`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui ./internal/cli -run 'External|Unresolved|UsedBy|NodeReferences'` — Expected: FAIL.
- [ ] **Step 3: Implement** the fields, the adapter, the row rendering (external rows dim, target text in the target's type colour), and the result cell.
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS (goldens rewritten and read).
- [ ] **Step 5: Commit** `feat(tui): external entries and used-by in the tree and results`.

### Task 4: Jumping and the back root

**Files:**
- Modify: `cli/internal/tui/model.go`, `cli/internal/tui/tree.go` (back root row in `lines`, selectable), `cli/internal/tui/keys.go`
- Test: `cli/internal/tui/model_test.go`, `cli/internal/tui/tree_test.go`

**Interfaces:**
- Produces: `Model.backStack []string` (entry IDs, max 20); `treeView.Back *Ref` (the last entry, rendered as the top row `↩ <Back.Repo> — back to <Back.Name>`, selectable index -2 in tree mode via `↑` from the root row); `func (m *Model) jump(entryID string)`; `func (m *Model) back()`.

- [ ] **Step 1: Write the failing tests:**
  - `TestJumpIntoExternal`: tree mode on an external entry, `right` → cursor = its target, tree path rooted at the target's repo, `m.backStack` = [entry], and the view's first tree row is `↩ <repo> — back to <name>`.
  - `TestBackRootReturns`: after the jump, `up` from the root row selects the back row; `enter` → cursor = the entry again, stack empty, no back row.
  - `TestJumpUnresolved`: `right` on an unresolved entry leaves the cursor and sets the status to the `ipm source add` hint.
  - `TestJumpCycleBackStack`: A lists B, B lists A; jump A→B→A → stack of 2; two backs return to the first entry; the stack never exceeds 20 (21 jumps → 20).
  - `TestMovingResultClearsBackStack`: after a jump, `esc` then `down` in results → stack empty.
  - Search mode: `right` on an external entry cursor jumps the same way.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'Jump|BackRoot|ClearsBack'` — Expected: FAIL.
- [ ] **Step 3: Implement** the stack, `jump`/`back`, the back row in `lines` and its selection, and the help rows (`→ on ↗` jumps, `↑` to `↩` goes back).
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): jump into external entries and back`.

### Task 5: `e` — external references

**Files:**
- Create: `cli/internal/tui/external.go`
- Modify: `cli/internal/tui/model.go`, `cli/internal/tui/keys.go`
- Test: `cli/internal/tui/model_test.go`, `cli/internal/tui/view_test.go`

**Interfaces:**
- Consumes: `Node.UsedBy`, `Node.UsedByRepos` of the cursor's repo node (the cursor itself when it is a repo, else `Catalog.Node("repo:<cursor's repo>")`); Task 4's `jump` semantics (pushes the current cursor).

- [ ] **Step 1: Write the failing tests:**
  - `TestExternalModal`: `e` on a node whose repo has two references → modal header `External references to b/s — used by 2 repos`, one row per entry with its repo, stars and marketplace, hint `enter jump · esc close`.
  - `TestExternalModalGroups`: three entries from two repos → rows grouped by repo, header says `used by 2 repos`.
  - `TestExternalModalJump`: `down`, `enter` → cursor = that entry in its own repo's tree, back stack holds the previous cursor, modal closed.
  - `TestExternalModalEmpty`: no references → `no other repo references b/s`; `esc` closes.
  - Golden `external-modal`.
- [ ] **Step 2: Run** `cd cli && go test ./internal/tui -run 'ExternalModal|Golden'` — Expected: FAIL.
- [ ] **Step 3: Implement** the modal (list, selection, grouping by `Ref.Repo`, stars from `Node.Stars` of each referencing repo node).
- [ ] **Step 4: Run** `mise run cli:test` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(tui): e lists external references`.

### Task 6: Docs and a by-hand check

**Files:**
- Modify: `docs/guides/tui.md` (external entries, jumping and the back root, `e`, the ↗ column), `docs/reference/commands.md` (`ipm show` used-by, search ranking), `docs/troubleshooting.md` (old-index message), `docs/architecture.md` (references, weights), `docs/concepts.md` (repos are roots; references)

- [ ] **Step 1: By hand** under tmux with the real catalog after `ipm load`: `ipm tui`, search `agents-sdk` → the tree roots at `cloudflare/skills` with `↗ used by N`; search `claude-plugins-official`, walk to `cloudflare` (↗), `→` jumps, `↑` to the back root, `enter` returns; `e` on a `cloudflare/skills` artifact lists the referencing repos; `ipm search cloudflare` lists `cloudflare/skills` artifacts first. Ledger what was seen.
- [ ] **Step 2: Write the docs**; every key and message matches the code.
- [ ] **Step 3: Run** `mise run cli:test`, the relative-link check, and a local Jekyll build — Expected: PASS, `broken 0`, `done`.
- [ ] **Step 4: Commit** `docs: external references`.
