# Catalog Search (Stage 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `aihs serve`, `aihs load`, `aihs search`, `aihs show`, `aihs tree` and `aihs graph`: a local Typesense index over `catalog/*.jsonl` with hybrid search, and relationship views read straight from the catalog.

**Architecture:** `internal/typesense` is a thin `net/http` client. `internal/search` maps catalog records to documents, builds the schema and query parameters, and renders results. `internal/catgraph` loads the catalog as nodes and edges and renders trees and Mermaid/DOT. Commands in `internal/cli` wire them to `System`; Docker runs through `System.Output`, HTTP through a new `System.HTTP`.

**Tech Stack:** Go 1.26 standard library (`net/http`, `net/http/httptest`, `crypto/rand`); Docker with `typesense/typesense:30.2` at runtime.

**Spec:** `docs/superpowers/specs/2026-10-09-catalog-search-design.md` — read it first; this plan does not repeat its tables.

## Global Constraints

- No new Go dependency.
- Typesense image `typesense/typesense:30.2`; container `aihs-typesense`; port bound to `127.0.0.1:8108`; data `<root>/.aihs/typesense/data`; key file `<root>/.aihs/typesense/api-key`, mode 0600, 32 hex characters.
- Collection `artifacts`; model `ts/all-MiniLM-L12-v2`; import batch size 500; search `--limit` default 20, max 250.
- `show`, `tree`, `graph` read only `catalog/` and never contact Typesense.
- The gate is `mise run cli:test` (gofmt, go vet, go test); it must need neither Docker nor network.
- All commands are Go in `cli/internal/cli`, registered in the `commands` table; they reach Docker only through `System.Output` and HTTP only through `System.HTTP`.
- Markdown prose is not hard-wrapped.
- Commits: one per task, on `main`, only if the user approves committing at the execution handoff; otherwise each commit step is a checkpoint. Never push without asking.

## Review Focus

- Port 8108 already served by a different Typesense (wrong key) — expect `load`/`search` to say the API key was rejected and that another server may own port 8108, not a raw 401. Test in Task 5.
- Filter values with special characters (`repo` containing `/`, `.`, `-`; a tag containing `,` or `&&`) — expect each value backtick-quoted in `filter_by` so the filter is not broken. Test in Task 3.
- No catalog yet (`aihs index` never run) — expect `load`, `show`, `tree`, `graph` to say run `aihs index`. Test in Tasks 5 and 8.
- Catalog IDs and names containing `:`, `#`, `/`, quotes or brackets in Mermaid and DOT output — expect sanitized node IDs and escaped labels so the diagram parses. Test in Task 7.
- An empty query (`aihs search` with no words) — expect a usage error, not a Typesense error; `*` stays allowed. Test in Task 6.

---

### Task 1: Probe a live Typesense 30.2

**Files:**
- Modify: `docs/superpowers/specs/2026-10-09-catalog-search-design.md` (only if a probe contradicts it)

**Interfaces:**
- Produces: confirmed facts later tasks encode — (a) the accepted form of `query_by_weights` when `embedding` is in `query_by` (a trailing `0`, omitted, or rejected); (b) the import response line shape (`{"success":true}` / `{"success":false,"error":…,"document":…}`); (c) the search response fields used: `found`, `search_time_ms`, `hits[].document`, `hits[].text_match`, `hits[].vector_distance`, `facet_counts[].counts[].{value,count}`; (d) that IDs like `skill:o/r:skills/a#x` are accepted; (e) the status code for searching a missing collection.

- [ ] **Step 1:** `docker run -d --rm --name aihs-ts-probe -p 127.0.0.1:18108:8108 typesense/typesense:30.2 --data-dir /tmp --api-key probe`; wait for `curl -s 127.0.0.1:18108/health` → `{"ok":true}`.
- [ ] **Step 2:** Create a collection with the spec's schema (`embedding` with `embed.from` and `ts/all-MiniLM-L12-v2`), import three documents including an ID `skill:o/r:skills/a#x`, and run: a hybrid search with `query_by_weights` `4,2,2,1,0`, then `4,2,2,1`, then none; a `--keyword`-style search; a search on a missing collection. Record each status and the response shapes above.
- [ ] **Step 3:** `docker stop aihs-ts-probe`. Write the five facts into the ledger as `Task 1: Ruling:` lines; if (a) shows weights are rejected in hybrid mode, edit the spec's search paragraph to say weights apply to `--keyword` only.
- [ ] **Step 4:** Commit the spec change if any: `git commit -m "docs: confirm Typesense hybrid query details"`.

---

### Task 2: Typesense client

**Files:**
- Create: `cli/internal/typesense/client.go`, `cli/internal/typesense/client_test.go`

**Interfaces:**
- Produces:
  - `type Client struct { BaseURL, APIKey string; HTTP *http.Client }`
  - `func (c *Client) Health() error`
  - `func (c *Client) NumDocuments(collection string) (int, error)` — `GET /collections/<name>`; `ErrNoCollection` on 404
  - `func (c *Client) DeleteCollection(name string) error` — 404 is not an error
  - `func (c *Client) CreateCollection(schema map[string]any) error`
  - `type ImportResult struct { Success bool; Error, Document string }`; `func (c *Client) Import(collection string, docs []map[string]any) ([]ImportResult, error)` — JSONL body, `action=create`
  - `type Hit struct { Document map[string]any; TextMatch int64; VectorDistance *float64 }`; `type FacetCount struct { Value string; Count int }`; `type Result struct { Found, SearchTimeMS int; Hits []Hit; Facets map[string][]FacetCount }`; `func (c *Client) Search(collection string, params url.Values) (Result, error)`
  - Errors: `ErrUnreachable` (connection refused / timeout), `ErrUnauthorized` (401/403), `ErrNoCollection` (404 on a collection path); others wrap the status and body.
  - Every request sends header `X-TYPESENSE-API-KEY`.

- [ ] **Step 1: Write the failing tests** against an `httptest.Server`:

```go
func TestHealth(t *testing.T)               // 200 {"ok":true} → nil; server closed → errors.Is(err, ErrUnreachable)
func TestRequestsCarryTheKey(t *testing.T)   // handler asserts r.Header.Get("X-TYPESENSE-API-KEY") == "k" for every call
func TestUnauthorized(t *testing.T)          // 401 on any call → errors.Is(err, ErrUnauthorized)
func TestCollections(t *testing.T)           // DeleteCollection: 404 → nil; CreateCollection posts the schema JSON verbatim; NumDocuments reads "num_documents", 404 → ErrNoCollection
func TestImport(t *testing.T)                // body is one JSON object per line; query has action=create; response lines {"success":true} and {"success":false,"error":"bad","document":"{…}"} → 2 results, second has Error "bad"
func TestSearch(t *testing.T)                // canned response (Task 1's shape) → Found, SearchTimeMS, Hits[0].Document["name"], TextMatch, VectorDistance non-nil, Facets["type"][0] == {Value:"skill",Count:3}; 404 → ErrNoCollection
```

- [ ] **Step 2:** `cd cli && go test ./internal/typesense/` — expect FAIL (package missing).
- [ ] **Step 3:** Implement. `HTTP` nil means a client with a 30 s timeout; `Import` uses a 10 min timeout (embedding is slow). Map `*url.Error` with a dial or timeout cause to `ErrUnreachable`.
- [ ] **Step 4:** `cd cli && go test ./internal/typesense/` — expect PASS.
- [ ] **Step 5:** `git add cli/internal/typesense && git commit -m "feat(typesense): minimal HTTP client"`

---

### Task 3: Documents, schema, query parameters and the result table

**Files:**
- Create: `cli/internal/search/search.go`, `cli/internal/search/search_test.go`

**Interfaces:**
- Consumes: `catalog.Load`, `catalog.Types`, `github.Read`, `pack.NormalizeRepo`, `typesense.Result`.
- Produces:
  - `const Collection = "artifacts"`, `const Model = "ts/all-MiniLM-L12-v2"`
  - `func Schema() map[string]any` — the spec's table; `default_sorting_field: "stars"`
  - `func Documents(catalogDir string) ([]map[string]any, error)` — every record of every type, sorted by ID
  - `type Query struct { Text string; Types, Repos, Tags []string; Keyword bool; Limit int }` and `func (q Query) Params() url.Values`
  - `func Stars(n int) string` — `0`→`0`, `999`→`999`, `31234`→`31.2k`, `296686`→`297k`, `1234567`→`1.2M`
  - `func Table(w io.Writer, res typesense.Result, width int)` — table then the summary line

- [ ] **Step 1: Write the failing tests**

```go
func TestSchema(t *testing.T)    // field "embedding": type float[], embed.from ["name","description","tags"], model_config.model_name Model; type/tags/repo facet; stars int32; plugins index false
func TestDocuments(t *testing.T) {
	// fixture catalog dir: repos.jsonl (repo:o/r followed, repo:x/c unfollowed), plugins.jsonl (plugin:o/r:p "tools"; plugin:o/r#c external source_repo "x/c"),
	// skills.jsonl (skill owned by plugin:o/r:p), the other type files empty; github.jsonl: o/r stars 10 description "D" topics ["t"], x/c stars 99
	// → skill doc: stars 10, plugin_names ["tools"], plugins [...]; external plugin doc stars 99, source_repo "x/c";
	//   repo:o/r doc: name "o/r", description "D", tags ["t"], stars 10; no "embedding" key in any doc; docs sorted by id
	// missing catalog dir → error containing "aihs index"
}
func TestParamsHybridAndKeyword(t *testing.T) // hybrid: query_by ends with ",embedding", exclude_fields "embedding", sort_by "_text_match:desc,stars:desc", facet_by "type", num_typos "2", per_page "20"; Keyword: query_by has no "embedding"; weights per Task 1's ruling
func TestParamsFiltersAreQuoted(t *testing.T) // Types ["skill","agent"], Repos ["o/r.x-y"], Tags ["a,b && c"] → filter_by == "type:=[`skill`,`agent`] && repo:=[`o/r.x-y`] && tags:=[`a,b && c`]"
func TestParamsLimitCapped(t *testing.T)      // Limit 0 → 20; Limit 999 → 250
func TestStars(t *testing.T)                  // the values above
func TestTable(t *testing.T) {
	// Result with 2 hits (one with plugin_names, one without), Found 41, SearchTimeMS 7, Facets type skill 30 agent 11
	// → header "TYPE  NAME  REPO  ★  PLUGINS  DESCRIPTION"; "—" for no plugins; no line longer than 120 runes;
	//   last line "41 match(es) · skill 30 · agent 11 · 7 ms"; zero hits → "no matches"
}
```

- [ ] **Step 2:** `cd cli && go test ./internal/search/` — expect FAIL.
- [ ] **Step 3:** Implement. Backtick-quote every filter value; a value containing a backtick is dropped with no filter (Typesense cannot express it) — note it in a comment. `Table` uses `text/tabwriter` and cuts the description so each row fits `width`.
- [ ] **Step 4:** `cd cli && go test ./internal/search/` — expect PASS.
- [ ] **Step 5:** `git add cli/internal/search && git commit -m "feat(search): documents, schema, query params and result table"`

---

### Task 4: `System.HTTP` and `aihs serve`

**Files:**
- Modify: `cli/internal/cli/cli.go` (add `HTTP *http.Client` to `System`; register `{"serve", "start the local Typesense search server in Docker (--stop to stop it)", "", serveFlags}`), `cli/cmd/aihs/main.go` (`HTTP: &http.Client{}`), `cli/internal/cli/cli_test.go` (fake gets `HTTP`), `cli/internal/cli/doctor.go`
- Create: `cli/internal/cli/serve.go`
- Test: `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `typesense.Client`, `search.Collection`.
- Produces (in `serve.go`, used by Tasks 5–6):
  - `const tsImage = "typesense/typesense:30.2"`, `const tsContainer = "aihs-typesense"`, `const tsURL = "http://127.0.0.1:8108"`
  - `func apiKey(root string, create bool) (string, error)` — reads `<root>/.aihs/typesense/api-key`; creates it (0600) when `create`; missing and `!create` → error saying run `aihs serve`
  - `func tsClient(sys System, root string) (*typesense.Client, error)`
  - package vars `healthEvery = 500 * time.Millisecond`, `healthFor = 60 * time.Second` (tests shrink them)
- Test helper (in `cli_test.go`): `func typesenseAt(f *fake, srv *httptest.Server)` — sets `f.http` to a client whose Transport rewrites every request's host to `srv`'s.

- [ ] **Step 1: Write the failing tests**

```go
func TestServeCreatesContainer(t *testing.T) {
	// fake: LookPath docker ok; "docker info" ok; "docker inspect" fails (no such container); httptest health ok
	// → a call "docker run -d --name aihs-typesense -p 127.0.0.1:8108:8108 -v <root>/.aihs/typesense/data:/data typesense/typesense:30.2 --data-dir /data --api-key <key>"
	// key file exists, 32 hex chars, mode 0600; a second run reuses the same key
}
func TestServeLeavesRunningContainer(t *testing.T) // "docker inspect" → "true" → no run/start call
func TestServeStartsStoppedContainer(t *testing.T) // "docker inspect" → "false" → "docker start aihs-typesense"
func TestServeStop(t *testing.T)                   // --stop → "docker stop aihs-typesense"
func TestServeWithoutDocker(t *testing.T)          // LookPath docker fails → error mentions Docker; "docker info" fails → error says Docker is not running
func TestServeHealthTimeout(t *testing.T)          // health never ok, healthFor 50ms → error mentions `docker logs aihs-typesense`
func TestDoctorReportsTypesense(t *testing.T)      // rows "docker" and "typesense" present; typesense unreachable → warning, not a problem
```

- [ ] **Step 2:** `cd cli && go test ./internal/cli/ -run 'Serve|DoctorReportsTypesense'` — expect FAIL.
- [ ] **Step 3:** Implement. Extend the fake's `Output` key scheme if needed so `docker info`, `docker inspect`, `docker run`, `docker start`, `docker stop` are distinguishable (its key is the program plus the first four characters of the first argument). Doctor: `docker` row warns when Docker is missing or not running; `typesense` row is `ok` with the document count, or a warning (not running / no collection) — never a problem.
- [ ] **Step 4:** `cd cli && go test ./internal/cli/ -run 'Serve|DoctorReportsTypesense'`, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** `git add cli && git commit -m "feat(cli): aihs serve runs Typesense in Docker"`

---

### Task 5: `aihs load`

**Files:**
- Create: `cli/internal/cli/load.go`
- Modify: `cli/internal/cli/cli.go` (register `{"load", "rebuild the search index from catalog/", "", loadFlags}`)
- Test: `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `tsClient`, `search.Documents`, `search.Schema`, `search.Collection`, `typesense.Client.{DeleteCollection,CreateCollection,Import}`.

- [ ] **Step 1: Write the failing tests** (fake Typesense via `httptest` recording requests; catalog fixture written into the test root; key file pre-created)

```go
func TestLoadRebuildsCollection(t *testing.T) {
	// → DELETE /collections/artifacts, POST /collections (schema has "embedding"), then imports;
	//   with 1,200 fixture docs → 3 import requests (500, 500, 200); stdout has per-type counts and "imported"
}
func TestLoadReportsFailures(t *testing.T)    // one import line {"success":false,"error":"bad"} → stdout names the doc ID and "bad"; ExitError code 1
func TestLoadNeedsServer(t *testing.T)        // no key file → error says `aihs serve`; server closed → error says `aihs serve`
func TestLoadWrongKey(t *testing.T)           // server answers 401 → error says the API key was rejected and port 8108 may belong to another Typesense
func TestLoadWithoutCatalog(t *testing.T)     // no catalog/ → error says `aihs index`; no request sent
```

- [ ] **Step 2:** `cd cli && go test ./internal/cli/ -run Load` — expect FAIL.
- [ ] **Step 3:** Implement: documents first (so a missing catalog sends nothing), then delete, create, import in batches of 500; per-type counts from the documents; elapsed from `sys.Now`.
- [ ] **Step 4:** `cd cli && go test ./internal/cli/ -run Load`, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** `git add cli && git commit -m "feat(cli): aihs load rebuilds the search index"`

---

### Task 6: `aihs search`

**Files:**
- Create: `cli/internal/cli/search.go`
- Modify: `cli/internal/cli/cli.go` (register `{"search", "hybrid search over the catalog (needs aihs serve and aihs load)", "<query>", searchFlags}`)
- Test: `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `tsClient`, `search.Query`, `search.Table`, `typesense.Client.Search`.

- [ ] **Step 1: Write the failing tests**

```go
func TestSearchPrintsTable(t *testing.T)      // f.run("search","--type","skill","c#","testing") → request q "c# testing", filter_by has type; stdout has the header and summary
func TestSearchKeyword(t *testing.T)          // --keyword → query_by without "embedding"
func TestSearchJSON(t *testing.T)             // --json → one JSON object per hit, each with "document"
func TestSearchNeedsQuery(t *testing.T)       // f.run("search") → UsageError
func TestSearchNoIndex(t *testing.T)          // 404 → error says `aihs load`; unreachable → `aihs serve`
```

- [ ] **Step 2:** `cd cli && go test ./internal/cli/ -run Search` — expect FAIL.
- [ ] **Step 3:** Implement. Query text is the positional arguments joined by one space. Repeatable `--type`, `--repo`, `--tag` use the existing `multi` flag type. Table width 120.
- [ ] **Step 4:** `cd cli && go test ./internal/cli/ -run Search`, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** `git add cli && git commit -m "feat(cli): aihs search"`

---

### Task 7: Catalog graph — trees and diagrams

**Files:**
- Create: `cli/internal/catgraph/graph.go`, `cli/internal/catgraph/render.go`, `cli/internal/catgraph/graph_test.go`

**Interfaces:**
- Consumes: `catalog.Load`, `catalog.Types`.
- Produces:
  - `type Graph struct` with `func Load(catalogDir string) (*Graph, error)` (missing catalog → error saying run `aihs index`)
  - `func (g *Graph) Node(id string) (map[string]any, bool)`
  - `func (g *Graph) Children(id string, up bool) []string` — down per the spec's edge table and repo rule; up reversed; ordered by `catalog.Types` order, then name, then ID
  - `type Options struct { Up bool; Depth int; All bool }` (`Depth` 0 = unlimited)
  - `func (g *Graph) Tree(w io.Writer, id string, o Options) error`
  - `func (g *Graph) Mermaid(w io.Writer, id string, o Options) error` and `func (g *Graph) DOT(w io.Writer, id string, o Options) error`

- [ ] **Step 1: Write the failing tests** (fixture catalog: repo o/r with marketplace m listing plugin p (skills s1, s2; hook h) and external plugin e (source_repo x/c, unfollowed repo record); a skill s3 owned by no plugin; a plugin q no marketplace lists; plugin p2 with 30 skills)

```go
func TestChildrenDown(t *testing.T)    // repo:o/r → [marketplace m, plugin q, skill s3, …] — p is not a direct child (its marketplace lists it); marketplace → [p, e]; p → [skills…, hook]
func TestChildrenUp(t *testing.T)      // s1 up → [plugin p]; p up → [marketplace m]; e up → [marketplace m]
func TestTree(t *testing.T) {
	// Tree(repo:o/r, Depth 0) exactly:
	// "repo        o/r ★ 10\n├─ marketplace m\n│  ├─ plugin      p\n│  │  ├─ skill       s1\n…"
	// (write the full expected string in the test from the fixture)
}
func TestTreeCutsAt25(t *testing.T)    // p2 → 25 skill lines then "└─ … 5 more"; All → 30
func TestTreeUp(t *testing.T)          // s1 Up → s1, then plugin p, then marketplace m, then repo o/r
func TestTreeMarksCycles(t *testing.T) // fixture plugin owning a component that lists it back via a hand-made edge → "(cycle)" and no infinite loop
func TestMermaid(t *testing.T)         // "flowchart LR\n  n0[\"repo: o/r\"]\n  n1[\"marketplace: m\"]\n  n0 --> n1\n…"; a name with `"` and `[` is escaped (`#quot;`), IDs are n0…
func TestDOT(t *testing.T)             // "digraph catalog {\n  rankdir=LR;\n  n0 [label=\"repo: o/r\"];\n…}" with `"` escaped as `\"`
func TestUnknownID(t *testing.T)       // Tree/Mermaid on "skill:nope" → error containing "aihs search"
```

- [ ] **Step 2:** `cd cli && go test ./internal/catgraph/` — expect FAIL.
- [ ] **Step 3:** Implement. Line format `<prefix><type padded to 11> <name>`; repos add ` ★ <search.Stars>` when `github.jsonl` knows them — `Load` reads `github.jsonl` via `github.Read`, and stars are formatted with `search.Stars` (catgraph may import search; search must not import catgraph). Depth: `Options.Depth` 0 = unlimited; Mermaid/DOT walk breadth-first, numbering nodes in visit order.
- [ ] **Step 4:** `cd cli && go test ./internal/catgraph/` — expect PASS.
- [ ] **Step 5:** `git add cli/internal && git commit -m "feat(catgraph): relationship trees and Mermaid/DOT export"`

---

### Task 8: `aihs show`, `aihs tree`, `aihs graph`

**Files:**
- Create: `cli/internal/cli/show.go` (all three commands)
- Modify: `cli/internal/cli/cli.go` (register `{"show", "an artifact's fields, GitHub stats and neighbours", "<id>", showFlags}`, `{"tree", "an artifact's relationships as a tree", "<id>", treeFlags}`, `{"graph", "an artifact's neighbourhood as Mermaid or Graphviz", "<id>", graphFlags}`)
- Test: `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `catgraph.Load`, `Graph.{Node,Children,Tree,Mermaid,DOT}`, `catgraph.Options`, `github.Read`.

- [ ] **Step 1: Write the failing tests** (catalog fixture in the test root)

```go
func TestShow(t *testing.T)            // skill → lines "name", "type", "repo", "commit", stats "stars", an "up:" list naming plugin p and a "down:" list
func TestShowExternalPlugin(t *testing.T) // last line "promote: aihs promote plugin:o/r#e"
func TestShowJSON(t *testing.T)        // --json → the catalog line verbatim
func TestTreeAndGraphCommands(t *testing.T) // "tree --up <skill>" output starts with the skill; "graph --format dot <id>" starts "digraph"; "--format svg" → UsageError
func TestShowWithoutCatalog(t *testing.T)   // → error says `aihs index`
func TestTreeDefaultDepth(t *testing.T)     // tree from a repo stops at depth 3; from a plugin is unlimited
```

- [ ] **Step 2:** `cd cli && go test ./internal/cli/ -run 'Show|TreeAnd|TreeDefault'` — expect FAIL.
- [ ] **Step 3:** Implement. Flags: `show --json`; `tree --up --depth N --all`; `graph --up --depth N --format mermaid|dot`. Defaults per the spec: `tree` depth 3 from a repo, else unlimited; `graph --depth` 2; any other `--format` is a usage error.
- [ ] **Step 4:** `cd cli && go test ./internal/cli/ -run 'Show|TreeAnd|TreeDefault'`, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** `git add cli && git commit -m "feat(cli): aihs show, tree and graph"`

---

### Task 9: Docs and live verification

**Files:**
- Modify: `AGENTS.md`, `CLAUDE.md` (gitignored — edit locally only), `cli/README.md`, `cli/cmd/aihs/main.go` (package comment), `docs/architecture.md` (a "Search" section: runtime, index, commands, the relationship views)

- [ ] **Step 1:** Update the docs as listed; `mise run cli:test` — expect PASS.
- [ ] **Step 2:** From the repo root: `./bin/aihs serve && ./bin/aihs load`. Expected: container running, `artifacts` holds every catalog record (3,280 as of stage 1), 0 failures; record the load time.
- [ ] **Step 3:** Run, and record the top 5 of each in the ledger, hybrid and `--keyword`: `search "c# testing"`, `search "brand guidelines"`, `search postgres`, `search "code review agent" --type agent`, `search "pre-commit hook"`, `search "draw diagrams"`. Expected: every query returns relevant hits; for at least three queries hybrid surfaces a relevant hit whose name and description lack the query's words. Note any query where hybrid is worse than keyword.
- [ ] **Step 4:** `./bin/aihs tree repo:obra/superpowers`, `./bin/aihs tree --up skill:obra/superpowers:skills/brainstorming`, `./bin/aihs graph plugin:obra/superpowers:. > /tmp/g.mmd` and check the Mermaid parses (mermaid validator tool, or paste into a Mermaid renderer). Expected: trees match the catalog; the diagram renders.
- [ ] **Step 5:** `./bin/aihs serve --stop`. `git add -A && git commit -m "docs: search and relationship commands"`
