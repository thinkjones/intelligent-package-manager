# Artifact Catalog (Stage 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `aihs index`, `aihs stats` and `aihs promote`: deterministic extraction of every repo, marketplace, plugin, skill, agent, command, hook and MCP server from the cached repos of interest into committed `catalog/*.jsonl`, plus GitHub stats and promoting external repos into the root `manifest.toml`.

**Architecture:** A new `internal/catalog` package walks each cached checkout (`walk.go`), parses markers into `Record`s (`parse.go`), links plugin ownership and marketplace entries (`link.go`), assembles all repos (`build.go`) and writes sorted JSONL (`record.go`, `write.go`). `internal/github` fetches stats through a `gh` runner. Commands in `internal/cli` wire them to `System`, like `sync` and `scan`.

**Tech Stack:** Go 1.26, `github.com/BurntSushi/toml` (existing), `gopkg.in/yaml.v3` (new), `gh` CLI at runtime for `aihs stats`.

**Spec:** `docs/superpowers/specs/2026-10-08-artifact-catalog-design.md` — read it first; this plan implements it and does not repeat its tables.

## Global Constraints

- All commands are Go in `cli/internal/cli`, registered in the `commands` table in `cli.go`; they reach git and `gh` only through `System`.
- The gate for every task: `mise run cli:test` (gofmt, go vet, go test) passes.
- `aihs index` reads only `aihs.lock` and the cache, never the network.
- Output files: one record per line, sorted by `id` (`github.jsonl`: by `repo`), object keys sorted, no HTML escaping, LF, trailing newline; a type with no records is written as an empty file.
- Paths in records are repo-relative, `/`-separated, `.` for the repo root.
- Codex and Cursor plugins (`.codex-plugin/`, `.cursor-plugin/`) are not cataloged.
- Never hand-edit `aihs.lock` or `.claude-plugin/marketplace.json`.
- Markdown prose is not hard-wrapped.
- Commits: the repo has no commits yet and the user has not authorized committing. Each task's last step is a checkpoint; commit only if the user has said to.

## Review Focus

- A `plugin.json` component path that escapes the repo (`../../x`, or absolute) — expect a warning and no record, never a read outside the checkout. Test in Task 5.
- Frontmatter written with CRLF line endings (`---\r\n`) — expect it parsed like LF. Test in Task 3.
- A marketplace source given as `https://github.com/o/r.git` when `o/r` is a repo of interest — expect one `repo:o/r` record, `followed: true`. Test in Task 6.
- A skill, agent or command whose frontmatter has no `name` — expect the name to fall back to the dir or file stem. Test in Task 3.
- A symlinked directory in an upstream (including one pointing at an ancestor) — expect it not followed, no loop. Test in Task 2.

---

### Task 1: Confirm Claude Code's plugin component rules

**Files:**
- Modify: `docs/superpowers/specs/2026-10-08-artifact-catalog-design.md` (only if a rule differs)

**Interfaces:**
- Produces: the confirmed rule per component (skills, agents, commands, hooks, mcpServers): does a declared path in `plugin.json` add to the default path or replace it; which value shapes are allowed (string, array, inline object). Task 5 encodes these.

- [ ] **Step 1:** Read the current Claude Code plugin reference (`https://code.claude.com/docs/en/plugins-reference`, via the claude-code-guide agent or WebFetch) for the `plugin.json` component fields and their default directories.
- [ ] **Step 2:** Write the five rules as a short list at the top of Task 5's `link.go` comment (Task 5 Step 3 copies it). If any component replaces rather than adds, change the spec's "Plugin ownership" paragraph to say so.
- [ ] **Step 3:** Checkpoint.

---

### Task 2: Record type, writer and walker

**Files:**
- Create: `cli/internal/catalog/record.go`, `cli/internal/catalog/write.go`, `cli/internal/catalog/walk.go`
- Test: `cli/internal/catalog/record_test.go`, `cli/internal/catalog/walk_test.go`

**Interfaces:**
- Produces:
  - Type constants `TypeRepo = "repo"`, `TypeMarketplace = "marketplace"`, `TypePlugin = "plugin"`, `TypeCopilotPlugin = "copilot-plugin"`, `TypeSkill = "skill"`, `TypeAgent = "agent"`, `TypeCommand = "command"`, `TypeHook = "hook"`, `TypeMCP = "mcp"`; `Types []string` in that order.
  - `Files map[string]string` type → file name: `repos.jsonl`, `marketplaces.jsonl`, `plugins.jsonl`, `copilot-plugins.jsonl`, `skills.jsonl`, `agents.jsonl`, `commands.jsonl`, `hooks.jsonl`, `mcp.jsonl`.
  - `type Record struct { ID, Type, Name, Description string; Tags []string; Repo, Commit, Path string; Meta map[string]any; ContentHash string; Errors []string; Fields map[string]any }` with `MarshalJSON`.
  - `func Write(dir string, recs []Record) error` — writes every file in `Files` under `dir` (created if missing); never touches `github.jsonl`.
  - `func Load(dir, typ string) ([]map[string]any, error)` — reads one type's file; missing file is an error naming `aihs index`.
  - `type Found struct { Skills, Plugins, CopilotPlugins, Marketplaces []string; Agents, Commands, HookFiles, MCPFiles []string }` and `func Walk(dir string) (Found, error)`.

- [ ] **Step 1: Write the failing tests**

```go
func TestRecordJSON(t *testing.T) {
	r := Record{ID: "skill:o/r:skills/a", Type: TypeSkill, Name: "a", Description: "x < y",
		Tags: []string{"b", "a", "b"}, Repo: "o/r", Commit: "c1", Path: "skills/a",
		Meta: map[string]any{"name": "a"}, ContentHash: "h", Fields: map[string]any{"plugins": []string(nil)}}
	got, err := r.MarshalJSON() // directly: json.Marshal would re-escape "<"
	want := `{"commit":"c1","content_hash":"h","description":"x < y","id":"skill:o/r:skills/a","meta":{"name":"a"},"name":"a","path":"skills/a","plugins":[],"repo":"o/r","tags":["a","b"],"type":"skill"}`
	// err == nil && string(got) == want
}

func TestRecordOmitsEmptyOptionalFields(t *testing.T) {
	// external plugin: no commit, path, meta, content_hash, errors keys; tags is [] not null
}

func TestWriteSortsAndWritesEveryFile(t *testing.T) {
	// two skills given out of order → skills.jsonl lines sorted by id, each ending "\n";
	// all nine Files exist; agents.jsonl is empty (0 bytes); a pre-existing github.jsonl is unchanged;
	// "x < y" appears literally, not as <
}

func TestWalk(t *testing.T) {
	// tree: skills/a/SKILL.md, docs/b/SKILL.md (outside skills/), agents/r.md, agents/README.md,
	// commands/go.md, hooks/hooks.json, .mcp.json, p/.claude-plugin/plugin.json,
	// c/plugin.json, c/.codex-plugin/plugin.json, .claude-plugin/marketplace.json,
	// node_modules/x/SKILL.md, .git/y/SKILL.md
	// want Found{Skills: ["docs/b","skills/a"], Agents: ["agents/r.md"], Commands: ["commands/go.md"],
	//   HookFiles: ["hooks/hooks.json"], MCPFiles: [".mcp.json"], Plugins: ["p"], CopilotPlugins: ["c"], Marketplaces: ["."]}
}

func TestWalkDoesNotFollowSymlinkedDirs(t *testing.T) {
	// skills/a/SKILL.md plus a symlink skills/loop -> .. ; Walk returns, Skills == ["skills/a"]
}
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/catalog/` — expect FAIL (package does not exist).
- [ ] **Step 3:** Implement `record.go`: `MarshalJSON` builds a `map[string]any` — always `id`, `type`, `name`, `description`, `tags` (sorted, deduplicated, `[]` when empty), `repo`; `commit`, `path`, `content_hash`, `meta` only when non-empty; `errors` only when non-empty; then every `Fields` entry, with nil slices emitted as `[]` — and encodes it with a `json.Encoder` that has `SetEscapeHTML(false)`, trimming the encoder's trailing newline. A `Fields` key colliding with a common field panics (programmer error).
- [ ] **Step 4:** Implement `write.go`: group by `Type`, sort by `ID`, encode each line with `SetEscapeHTML(false)`, write each file to `<name>.tmp` then rename.
- [ ] **Step 5:** Implement `walk.go` per the spec's marker table, using `filepath.WalkDir` (does not follow symlinks), skipping `.git` and `node_modules`; every slice sorted.
- [ ] **Step 6:** Run `cd cli && go test ./internal/catalog/` — expect PASS.
- [ ] **Step 7:** Checkpoint.

---

### Task 3: Component parsers

**Files:**
- Create: `cli/internal/catalog/parse.go`
- Modify: `cli/go.mod`, `cli/go.sum` (`go get gopkg.in/yaml.v3`)
- Test: `cli/internal/catalog/parse_test.go`

**Interfaces:**
- Consumes: `Record`, type constants (Task 2).
- Produces:
  - `type Checkout struct { Repo, Commit, Dir string }` — `Dir` is the cached checkout.
  - `func Frontmatter(data []byte) (meta map[string]any, found bool, err error)` — CRLF normalized; YAML values converted to plain JSON types (maps with string keys, `time.Time` → RFC 3339 string).
  - `func ParseSkill(co Checkout, dir string) Record`, `func ParseAgent(co Checkout, file string) Record`, `func ParseCommand(co Checkout, file string) Record`.
  - `func ParseHookFile(co Checkout, file string) Record` and `func InlineHook(co Checkout, pluginJSON string, hooks map[string]any) Record`.
  - `func ParseMCPFile(co Checkout, file string) ([]Record, []string)` (records, warnings) and `func InlineMCP(co Checkout, pluginJSON string, servers map[string]any) []Record`.
  - `func ContentHash(data []byte) string` — `sha256` hex of LF-normalized bytes.

- [ ] **Step 1: Write the failing tests**

```go
func TestParseSkill(t *testing.T) {
	// skills/tdd/SKILL.md: "---\nname: tdd\ndescription: Test first\ncategory: testing\nmetadata:\n  tags: [go, quality]\n---\nbody"
	// want ID "skill:o/r:skills/tdd", Name "tdd", Description "Test first", Tags ["go","quality","testing"],
	// Commit/Path set, Meta["name"]=="tdd", ContentHash == ContentHash(file bytes), Fields["plugins"] == []string{}
}
func TestFrontmatterCRLF(t *testing.T)            // "---\r\nname: a\r\n---\r\n" → meta{"name":"a"}, found
func TestFrontmatterTimestampBecomesString(t *testing.T) // "date: 2025-01-02" → "2025-01-02T00:00:00Z"
func TestParseSkillBadYAML(t *testing.T)          // "---\nname: [\n---" → Errors non-empty, Meta == {"raw": "<frontmatter text>"}, Name falls back to dir base
func TestParseSkillWithoutName(t *testing.T)      // no name key → Name == "tdd" (dir base)
func TestParseAgentAndCommand(t *testing.T)       // agents/rev.md → ID "agent:o/r:agents/rev.md", Name from frontmatter else "rev"; commands/ship.md → Name "ship"
func TestParseHookFile(t *testing.T)              // {"hooks":{"PreToolUse":[...],"preToolUse":[...]}} → Fields["events"] == ["PreToolUse","preToolUse"], Tags same, Description "events: PreToolUse, preToolUse", Name "hooks" (dir base)
func TestParseMCPFileBothShapes(t *testing.T)     // {"mcpServers":{"gh":{"command":"npx","args":["x"]}}} and {"web":{"type":"http","url":"https://u"}}
	// → IDs "mcp:o/r:.mcp.json#gh" / "#web"; Fields transport "stdio"/"http", command+args / url; Description "stdio: npx x" / "http: https://u"
func TestParseMCPFileBadJSON(t *testing.T)        // → no records, one warning naming the file
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/catalog/ -run 'Parse|Frontmatter'` — expect FAIL.
- [ ] **Step 3:** Implement `parse.go`. Transport: `type` field when present, else `stdio` when `command` is set, else `http` when `url` is set. Skill `Tags`: `category` (string) plus `metadata.tags` (string list). Agents and commands have no tags. A missing-frontmatter skill gets `Errors: ["no frontmatter"]`; agents and commands without frontmatter just get empty `Meta`.
- [ ] **Step 4:** Run the same command — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 4: Plugin, Copilot plugin and marketplace parsers

**Files:**
- Modify: `cli/internal/catalog/parse.go`, `cli/internal/copilot/copilot.go`
- Test: `cli/internal/catalog/parse_test.go`, `cli/internal/copilot/copilot_test.go`

**Interfaces:**
- Consumes: Task 3's `Checkout`, `ContentHash`.
- Produces:
  - `type Components struct { Skills, Agents, Commands []string; Hooks any; MCP any }` — declared values from `plugin.json` (or a marketplace entry): path lists as written (a string becomes a one-element list), `Hooks`/`MCP` nil, a string path, or `map[string]any`.
  - `func ParsePlugin(co Checkout, dir string) (Record, Components)` — ID `plugin:<repo>:<dir>`; Name, Description from JSON; Tags from `keywords` + `category`; `Fields` `version`, `author` (as written), `license`.
  - `func ParseCopilotPlugin(co Checkout, dir string) Record` — `Fields["skills"]` = skill IDs from `copilot.SkillPaths`.
  - `type Entry struct { Name string; Source any; Raw map[string]any }` and `func ParseMarketplace(co Checkout, dir string) (Record, []Entry, []string)` — record ID `marketplace:<repo>:<dir>`, `Fields["plugin_root"]` from `metadata.pluginRoot`; warnings for entries missing `name` or `source`.
  - `func copilot.SkillPaths(dir, pluginPath string) ([]string, error)` — repo-relative skill dirs listed in the plugin's `com.github.awesome-copilot` extension, resolved with the existing `resolve` rule; `plugin()` is refactored to use it.

- [ ] **Step 1: Write the failing tests**

```go
func TestParsePlugin(t *testing.T) {
	// p/.claude-plugin/plugin.json {"name":"tools","description":"d","keywords":["k"],"category":"dev","version":"1.0","skills":"./extra","agents":["./a/x.md"],"hooks":"./h.json","mcpServers":{"s":{"command":"c"}}}
	// want ID "plugin:o/r:p", Tags ["dev","k"], Fields version "1.0";
	// Components{Skills:["./extra"], Agents:["./a/x.md"], Hooks:"./h.json", MCP: map[s:...]}
}
func TestParseCopilotPlugin(t *testing.T) // plugins/go/plugin.json with extension skills ["./skills/go-sdk"], skill at skills/go-sdk/SKILL.md (repo root) → Fields["skills"] == ["skill:o/r:skills/go-sdk"]
func TestParseMarketplace(t *testing.T)   // entries: {"name":"a","source":"./plugins/a"}, {"name":"b","source":{"source":"github","repo":"x/b"}}, {"source":"./c"} → 2 entries, 1 warning ("entry 2 has no name")
func TestSkillPaths(t *testing.T)         // in copilot_test.go: same fixture → ["skills/go-sdk"]
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/catalog/ ./internal/copilot/` — expect FAIL.
- [ ] **Step 3:** Implement. Extract `SkillPaths` from `copilot.plugin()` (the extension-reading and `resolve` loop); `plugin()` keeps its own checks (`group`, root refusal) on top. Existing copilot tests must still pass unchanged.
- [ ] **Step 4:** Run the same command — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 5: Plugin ownership (one repo)

**Files:**
- Create: `cli/internal/catalog/link.go`
- Test: `cli/internal/catalog/link_test.go`

**Interfaces:**
- Consumes: `Walk`, every parser (Tasks 2–4), the rules from Task 1.
- Produces:
  - `type RepoResult struct { Records []Record; Warnings []string; Sources []string }` — `Sources`: external `source_repo` values found (Task 6 fills them).
  - `func IndexRepo(co Checkout) (RepoResult, error)` — walks, parses, links. Plugin records get `Fields` `skills`, `agents`, `commands`, `hooks`, `mcp` (sorted IDs, `[]` when none). Component records' `plugins` are filled later by `Build` (Task 6) from these lists.

- [ ] **Step 1: Write the failing tests**

```go
func TestOwnershipDefaults(t *testing.T) {
	// p/.claude-plugin/plugin.json (no component keys); p/skills/a/SKILL.md; p/skills/group/b/SKILL.md;
	// p/agents/r.md; p/commands/c.md; p/hooks/hooks.json; p/.mcp.json {"mcpServers":{"s":{"command":"x"}}};
	// p/docs/z/SKILL.md (under p but off its paths)
	// plugin:o/r:p Fields: skills [skill:o/r:p/skills/a, skill:o/r:p/skills/group/b], agents [agent:o/r:p/agents/r.md],
	//   commands [command:o/r:p/commands/c.md], hooks [hook:o/r:p/hooks/hooks.json], mcp [mcp:o/r:p/.mcp.json#s]
	// skill:o/r:p/docs/z exists and is not in the plugin's skills
}
func TestOwnershipDeclaredPathsAddToDefaults(t *testing.T) {
	// plugin.json "skills":"./more", "agents":["./custom/x.md"], "hooks":"./cfg/h.json", "mcpServers":{"i":{"url":"https://u"}}
	// → skills include p/skills/* and p/more/*; agent:o/r:p/custom/x.md is created (walker would not find it) and owned;
	//   hook:o/r:p/cfg/h.json created and owned; mcp:o/r:p/.claude-plugin/plugin.json#i created and owned
	// (adjust "add to" per Task 1 if a rule differs)
}
func TestSkillOwnedByTwoPlugins(t *testing.T)   // p1 and p2 both declare "../shared/skills" → both list skill:o/r:shared/skills/s
func TestDeclaredPathEscapingRepo(t *testing.T) // "skills":"../../../etc" → warning containing the plugin ID, no record, no error
func TestCopilotPluginSkillsLinked(t *testing.T) // copilot-plugin record Fields["skills"] lists the skill ID
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/catalog/ -run Ownership\|Owned\|Escaping\|CopilotPluginSkills` — expect FAIL.
- [ ] **Step 3:** Implement `IndexRepo`: walk; parse everything found into an `id → Record` map; for each plugin, resolve default + declared paths with `path.Clean(path.Join(pluginDir, p))`, rejecting results that start with `..` or are absolute (warning `"<plugin id>: path <p> leaves the repo"`). Skills: every skill dir equal to or under a skills root. Agents/commands: a declared `.md` file is parsed if not already present; a declared dir contributes its direct `*.md` children (not `README.md`). Hooks/MCP: a string path is parsed with `ParseHookFile`/`ParseMCPFile` if not present; an inline object uses `InlineHook`/`InlineMCP` with the `plugin.json` path. Start the file with the Task 1 rule list as a comment.
- [ ] **Step 4:** Run the same command — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 6: Marketplaces, repo records and Build

**Files:**
- Modify: `cli/internal/catalog/link.go`
- Create: `cli/internal/catalog/build.go`
- Modify: `cli/internal/pack/interest.go` (add `NormalizeRepo`)
- Test: `cli/internal/catalog/build_test.go`, `cli/internal/pack/interest_test.go`

**Interfaces:**
- Consumes: `IndexRepo`, `ParseMarketplace`, `Entry` (Tasks 4–5).
- Produces:
  - `func pack.NormalizeRepo(s string) string` — `https://github.com/o/r`, `https://github.com/o/r.git`, `git@github.com:o/r.git` → `o/r`; anything else unchanged.
  - Marketplace resolution inside `IndexRepo`: marketplace record `Fields["plugins"]` (entry order); inline plugin records; external plugin records ID `plugin:<repo>#<name>` with `Fields` `source_repo` (normalized), `entry` (raw); `RepoResult.Sources`.
  - `func Build(cos []Checkout) ([]Record, []string, error)` — runs `IndexRepo` per checkout; adds `repo:` records (`followed: true`, `commit`, `counts` map type → count for followed; `followed: false` for each `Source` not among `cos` after normalization); sets `plugins` on skill/agent/command/hook/mcp records and `marketplaces` on plugin records (reverse links, sorted); sets a hook's `Name` to its first owning plugin's `Name` (by sorted plugin ID); sets plugin `entry` when a marketplace lists a local plugin.

- [ ] **Step 1: Write the failing tests**

```go
func TestMarketplaceEntries(t *testing.T) {
	// .claude-plugin/marketplace.json {"name":"m","metadata":{"pluginRoot":"./plugins"},"plugins":[
	//   {"name":"a","source":"./a"},                                   // plugins/a has .claude-plugin/plugin.json
	//   {"name":"b","source":"./b","strict":false,"skills":["./s"]},   // plugins/b/s/SKILL.md, no plugin.json
	//   {"name":"c","source":{"source":"github","repo":"x/c"}},
	//   {"name":"d","source":"./missing"}]}
	// marketplace:o/r:. Fields["plugins"] == [plugin:o/r:plugins/a, plugin:o/r:plugins/b, plugin:o/r#c]
	// plugin:o/r:plugins/b owns skill:o/r:plugins/b/s; plugin:o/r#c Fields source_repo "x/c", no commit/path
	// one warning naming entry "d"; Sources == ["x/c"]
}
func TestBuildRepoRecordsAndReverseLinks(t *testing.T) {
	// two checkouts o/r and x/c; o/r's marketplace references x/c (via "https://github.com/x/c.git") and y/z
	// → repo:o/r and repo:x/c followed true (one record each), repo:y/z followed false with no commit;
	// repo:o/r Fields counts {"skill":1,...}; skill records' plugins filled; plugin marketplaces filled
}
func TestBuildIsDeterministic(t *testing.T) // Build twice over the same fixture, Write both to temp dirs, every file byte-identical
func TestHookNamedForOwningPlugin(t *testing.T) // hook owned by plugin "tools" → Name "tools"
func TestNormalizeRepo(t *testing.T)            // in interest_test.go: the three GitHub forms → "o/r"; "https://gitlab.com/g/p" unchanged
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/catalog/ ./internal/pack/` — expect FAIL.
- [ ] **Step 3:** Implement per the spec's "Marketplace entries" table; relative sources resolve against the marketplace's dir joined with `plugin_root`; an entry naming a local dir with `plugin.json` links that plugin; without one builds an inline plugin from the entry's component keys (same ownership code as Task 5, `Components` built from the entry) with `content_hash` of the entry's canonical JSON; object sources become external plugins. `Build` returns an error only for unreadable checkouts.
- [ ] **Step 4:** Run the same command — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 7: `aihs index`

**Files:**
- Create: `cli/internal/cli/index.go`
- Modify: `cli/internal/cli/cli.go` (register `{"index", "write catalog/*.jsonl from the repos of interest at their locked commits", "", indexFlags}`)
- Test: `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `pack.LoadInterest`, `upstream.ReadLock`, `upstream.Checkout`, `catalog.Build`, `catalog.Write`, `catalog.Types`.
- Produces: `func indexFlags() (*flag.FlagSet, func(System) error)`; `catalog/` written under `cfg.Root`.

- [ ] **Step 1: Write the failing tests** (reuse `checkout`, `cached`, `newFake` from `cli_test.go`)

```go
func TestIndexWritesCatalog(t *testing.T) {
	// root manifest.toml [[repo]] cloudflare/skills; cached(...) with .claude-plugin/plugin.json and skills/one/SKILL.md
	// f.run("index") == nil; catalog/skills.jsonl has 1 line containing "skill:cloudflare/skills:skills/one";
	// stdout contains "skill" count 1 and "0 warning(s)"; len(f.calls) == 0 (no git)
}
func TestIndexRefusesUnpinnedRepoAndWritesNothing(t *testing.T) // manifest lists a/b not in lock → error contains "aihs sync"; catalog/ does not exist
func TestIndexWithNoReposOfInterest(t *testing.T)               // no root manifest → error mentions manifest.toml
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/cli/ -run Index` — expect FAIL.
- [ ] **Step 3:** Implement: load interest, lock; `upstream.Checkout` each (first error returned as is, before anything is written); `Build`; `Write(filepath.Join(root, "catalog"), recs)`; print `<type>  <count>` per `catalog.Types` with a tabwriter, each warning as `! <warning>` on stdout, then `catalog: <n> record(s), <w> warning(s) — catalog/`.
- [ ] **Step 4:** Run the same command, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 8: `aihs stats`

**Files:**
- Create: `cli/internal/github/github.go`, `cli/internal/cli/stats.go`
- Modify: `cli/internal/cli/cli.go` (add `Now func() time.Time` to `System`; register `{"stats", "fetch GitHub stats for every repo in the catalog into catalog/github.jsonl", "", statsFlags}`), `cli/cmd/aihs/main.go` (`Now: time.Now`), `cli/internal/cli/cli_test.go` (fake `Now` returns `2026-10-09T00:00:00Z`)
- Test: `cli/internal/github/github_test.go`, `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `catalog.Load(dir, catalog.TypeRepo)`, `pack.NormalizeRepo`.
- Produces:
  - `type Stats struct` with fields in this order and JSON names: `Archived bool "archived"`, `DefaultBranch string "default_branch"`, `Description string "description"`, `FetchedAt string "fetched_at"`, `Forks int "forks"`, `License string "license"`, `PushedAt string "pushed_at"`, `Repo string "repo"`, `Stars int "stars"`, `Topics []string "topics"` (`[]` when none).
  - `type GH func(args ...string) (string, error)`; `func Fetch(gh GH, ownerRepo string, now time.Time) (Stats, error)` — runs `gh api repos/<ownerRepo>`, maps `stargazers_count`, `forks_count`, `description`, `topics`, `license.spdx_id`, `pushed_at`, `archived`, `default_branch`; `FetchedAt` = `now.UTC().Format(time.RFC3339)`.
  - `func Read(path string) (map[string]Stats, error)` (missing file → empty map) and `func Write(path string, stats map[string]Stats) error` (sorted by `Repo`, no HTML escaping).

- [ ] **Step 1: Write the failing tests**

```go
func TestFetch(t *testing.T)            // fake gh returns canned API JSON → Stats{Repo:"o/r", Stars:12, License:"MIT", Topics:["a"], FetchedAt:"2026-10-09T00:00:00Z", ...}; gh called with ["api","repos/o/r"]
func TestWriteSortedAndReadBack(t *testing.T)
func TestStatsCommand(t *testing.T) {
	// catalog/repos.jsonl with repo:o/r (followed) and repo:https://gitlab.com/g/p; fake Output answers "gh api" with JSON
	// → catalog/github.jsonl has one line for o/r; stdout notes gitlab repo skipped; exit nil
}
func TestStatsKeepsPreviousRecordOnFailure(t *testing.T) // existing github.jsonl line for o/r; gh fails → line unchanged, ExitError code 1
func TestStatsNeedsGh(t *testing.T)                      // LookPath("gh") fails → error mentions gh; nothing written
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/github/ ./internal/cli/ -run 'Fetch|WriteSorted|Stats'` — expect FAIL.
- [ ] **Step 3:** Implement. The command: `LookPath("gh")`, then `gh auth status` through `Output` (failure → error saying to run `gh auth login`); repos from `catalog.Load`, each `repo` field normalized; non-GitHub skipped with a note; fetch four at a time; merge into the previous `Read` map; `Write`; print `<n> fetched, <s> skipped, <f> failed`; exit 1 if any failed. Add the fake's `"gh api"`/`"gh auth"` keys to `newFake`'s results map as the tests need.
- [ ] **Step 4:** Run the same command, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 9: `aihs promote`

**Files:**
- Modify: `cli/internal/pack/interest.go` (add `AppendInterest`), `cli/internal/cli/cli.go` (register `{"promote", "add repos (or an external plugin's source repo) to the root manifest.toml", "<repo|plugin-id>...", promoteFlags}`)
- Create: `cli/internal/cli/promote.go`
- Test: `cli/internal/pack/interest_test.go`, `cli/internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `pack.LoadInterest`, `pack.NormalizeRepo`, `catalog.Load(dir, catalog.TypePlugin)`.
- Produces: `func pack.AppendInterest(root string, repos []string) error` — refuses (nothing written) any repo already listed, naming it; appends `\n[[repo]]\nrepo = "<repo>"\n` per repo to `manifest.toml`, creating it if missing; existing bytes are kept verbatim.

- [ ] **Step 1: Write the failing tests**

```go
func TestAppendInterestKeepsComments(t *testing.T) // manifest "# keep me\n\n[[repo]]\nrepo = \"a/a\"\n" + AppendInterest(["b/b"]) → file starts with the original bytes, LoadInterest gives a/a and b/b
func TestAppendInterestRefusesDuplicate(t *testing.T) // AppendInterest(["a/a"]) → error contains "a/a", file unchanged
func TestPromoteExternalPlugin(t *testing.T) {
	// catalog/plugins.jsonl has plugin:o/r#c with source_repo "x/c"; f.run("promote", "plugin:o/r#c", "https://github.com/y/z.git")
	// → manifest gains x/c and y/z; stdout contains "aihs sync && aihs index"
}
func TestPromoteUnknownPluginID(t *testing.T) // → error naming the ID, manifest unchanged
func TestPromoteNeedsArgs(t *testing.T)       // f.run("promote") → UsageError
```

- [ ] **Step 2:** Run `cd cli && go test ./internal/pack/ ./internal/cli/ -run 'AppendInterest|Promote'` — expect FAIL.
- [ ] **Step 3:** Implement. An argument starting `plugin:` is looked up in `catalog/plugins.jsonl` and must have a `source_repo` (else "is not an external plugin"); others go through `NormalizeRepo`. Zero arguments is a `usagef`.
- [ ] **Step 4:** Run the same command, then `mise run cli:test` — expect PASS.
- [ ] **Step 5:** Checkpoint.

---

### Task 10: Remove `aihs repos`, update docs, index the real repos

**Files:**
- Delete: `cli/internal/cli/repos.go`; remove `TestReposRanksByEntries` and `TestReposSkipsBrokenPacks` from `cli/internal/cli/cli_test.go`; remove the `repos` row from `cli.go`
- Modify: `AGENTS.md`, `CLAUDE.md`, `cli/README.md`, `cli/cmd/aihs/main.go` (package comment), `docs/architecture.md`, `docs/manifest-reference.md`

**Interfaces:**
- Consumes: all commands above.

- [ ] **Step 1:** Remove `aihs repos` and its tests; run `mise run cli:test` — expect PASS.
- [ ] **Step 2:** Update docs: `catalog/` row in AGENTS.md's table; `index`, `stats`, `promote` rows in CLAUDE.md's command table (drop the `repos` row) and in `cli/README.md`'s usage block; a "Catalog" section in `docs/architecture.md` (inputs, files, determinism, the three stages); in `docs/manifest-reference.md`, `promote` in the root-manifest section and the `catalog/*.jsonl` record fields (link to the spec's data model rather than copying it).
- [ ] **Step 3:** From the repo root run `./bin/aihs sync && ./bin/aihs index`. Expected: exits 0; counts roughly 1,700+ skills, ~500 agents, ~170 plugins (plus inline and external), every type file present. List each warning; for any that is not a genuinely malformed upstream file, fix the extractor (with a test) and rerun.
- [ ] **Step 4:** Run `./bin/aihs index` again and `shasum catalog/*.jsonl` before and after — expect identical hashes.
- [ ] **Step 5:** Run `./bin/aihs stats` — expect ~250 repos fetched, non-GitHub ones skipped, 0 failed; spot-check `catalog/github.jsonl` for `obra/superpowers` stars.
- [ ] **Step 6:** Checkpoint.
