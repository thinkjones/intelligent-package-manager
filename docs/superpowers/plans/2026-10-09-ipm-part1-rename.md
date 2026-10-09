# ipm Part 1 — Rename aihs → ipm

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** the tool is called `ipm` everywhere a user meets it — command, shim, env var, help, messages, docs — with no change in behaviour.

**Architecture:** mechanical rename. Data-carrying names (the `.aihs/` cache dir, `aihs.lock`, the `aihs-typesense` container and its data dir) stay until part 2, which relocates them; renaming them now would migrate data twice.

**Tech Stack:** Go 1.26, bash shim, mise.

**Spec:** `docs/superpowers/specs/2026-10-09-ipm-design.md` (Parts → 1).

## Global Constraints

- No behaviour change: every existing test passes with only names updated.
- Keep: `.aihs/` paths, `aihs.lock`, container `aihs-typesense`, Go module path `github.com/thinkjones/fs-skills/cli`, historical docs under `docs/superpowers/`, the generated `.claude-plugin/marketplace.json` (deleted in part 2), catalog data.
- Gate: `mise run cli:test`. Commit per task.

## Review Focus

- A marketplace.json written by the old `aihs` (entries tagged `aihs`) — expect `IsOurs` still to recognise it, so `create-plugin` overwrites it without `--force`. Test in Task 1.
- A shell still exporting `AIHS_ROOT` — expect `ipm` to ignore it and use `IPM_ROOT` set by `bin/ipm`. Covered by the config test in Task 1.

---

### Task 1: Code and tooling

**Files:** `git mv cli/cmd/aihs cli/cmd/ipm`, `git mv bin/aihs bin/ipm`; modify `cli/cmd/ipm/main.go` and `main_test.go`, `cli/internal/config/config.go` (`RootVar = "IPM_ROOT"`), `cli/internal/cli/*.go` (help and messages saying `aihs` → `ipm`), `cli/internal/marketplace/marketplace.go` (`OwnerTag = "ipm"`; `IsOurs` also accepts the legacy `aihs` tag), other `cli/internal/**` messages, all `_test.go` expectations, `bin/ipm` (builds `cli/bin/ipm`, exports `IPM_ROOT`), `mise.toml` (`cli:build` → `bin/ipm ./cmd/ipm`), `.envrc` and `.gitignore` comments, `cli/.gitignore` if it names the binary.

- [ ] **Step 1:** Add a test `TestIsOursAcceptsLegacyTag` in `cli/internal/marketplace/marketplace_test.go`: a marketplace file whose entries are tagged `aihs` → `IsOurs` true; tagged `ipm` → true; untagged → false. Run `cd cli && go test ./internal/marketplace/ -run Legacy` — expect FAIL (only one tag known today… after the OwnerTag change).
- [ ] **Step 2:** Rename: the `git mv`s, then replace user-facing `aihs` with `ipm` in the files above (`AIHS_ROOT` → `IPM_ROOT`; `aihs <command>` → `ipm <command>` in help, errors and comments), leaving `.aihs/`, `aihs.lock` and `aihs-typesense` untouched. Implement `IsOurs` legacy acceptance.
- [ ] **Step 3:** `mise run cli:test` — expect all PASS; `git grep -n -i aihs -- cli bin mise.toml .envrc` shows only `.aihs`, `aihs.lock`, `aihs-typesense` and the legacy tag.
- [ ] **Step 4:** `./bin/ipm --version` prints `ipm …`; `./bin/ipm help` lists commands as `ipm`; `./bin/ipm search --limit 3 c# testing` returns results from the running server.
- [ ] **Step 5:** Commit `refactor: rename aihs to ipm (command, shim, env var, messages)`.

### Task 2: Docs

**Files:** `README.md`, `AGENTS.md`, `CLAUDE.md` (gitignored; edit locally), `cli/README.md`, `docs/architecture.md`, `docs/user-guide.md`, `docs/manifest-reference.md`, `assets/readme/*.svg` (visible text), the memory index note.

- [ ] **Step 1:** Replace the tool name `aihs` with `ipm` in prose, commands and tables; keep `.aihs/` and `aihs.lock` where they name files; add one line in `AGENTS.md`/`CLAUDE.md`: "`ipm` was called `aihs`; data paths still use the old name until part 2".
- [ ] **Step 2:** `git grep -n -i aihs -- README.md AGENTS.md cli/README.md docs/*.md assets` shows only kept file/path names. SVGs still render (open one; `xmllint --noout`).
- [ ] **Step 3:** Commit `docs: rename aihs to ipm`.
