# Bare-Minimum Quality Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring this repo up to the `code-architecture.bare-minimum-quality` bar: commits are refused unless formatted, linted, vetted and unit-tested; a tiered CI pipeline with two stable gates guards `main`; integration suites cannot self-skip; a release is refused without a green full suite.

**Architecture:** `mise.toml` becomes the single definition of every check: `mise run verify` is the gate, and each CI job runs one `ci:<job>` task. Plain bash hooks in `.githooks/` call the same tasks. One workflow, `ci.yml`, has a `plan` job whose logic lives in a script that is tested locally; a second workflow, `release.yml`, is the deploy: it asserts the full-suite gate, then tags `cli/vX.Y.Z`.

**Tech Stack:** Go 1.26, mise (tasks and pinned tools), golangci-lint, shellcheck, shfmt, actionlint, GitHub Actions, GitHub rulesets, Docker (Typesense, integration only), pnpm 12.10.1, Node 24.

**Spec:** `.claude/skills/code-architecture.bare-minimum-quality/SKILL.md` and its `references/github-modes.md` and `references/ci-pipeline-shape.md`. `.claude/` is gitignored, so the executor needs the skill installed locally.

## What the skill asks for, and what applies here

| Skill item | Applies? | In this repo |
| --- | --- | --- |
| Pillar 1, pre-commit and pre-push hooks | Yes | Tasks 1 to 3 |
| One verify command that CI runs | Yes | `mise run verify` (Task 1); `cli:test` stays as an alias |
| Pillar 2, tiered CI with two gates, draft and docs-only handling, concurrency | Yes | Tasks 5 and 6 |
| Modes, ruleset, auto-merge | Yes | Task 7. The repo is public, so rulesets and auto-merge are free |
| Deploy gate | Yes, as a release | Task 8. There are no tags today, so `go install …@latest` and the `curl … install.sh` one-liner both ship `main`'s HEAD. The release workflow makes a tag the thing users get |
| Pillar 3, unit tests | Yes | Exist (17 packages, about 20s uncached). Task 4 stops them self-skipping |
| Pillar 3, integration tests | Yes | Real pnpm tests exist but skip silently without pnpm. Nothing runs real Typesense: `ipm serve`, `ipm load` and `ipm search` are only tested against fakes. Task 4 |
| Pillar 3, E2E and the embarrassment-prevention smoke suite | No | No web UI, no routes, no login. The TUI is covered by golden tests, which are unit tests |
| Pillar 4, fsconfig | No | `ipm` reads no secrets and has no `.env`; its only configuration is `IPM_HOME`, `IPM_CACHE_DIR`, `IPM_BIN_DIR`, `NO_COLOR` and `COLUMNS`. CI needs no secrets beyond the workflow token |
| Branch to environment mapping, preview, required reviewers, reuse of the built artifact | No | There are no environments. Users build from source with `go install` |
| The docs site | Out of scope | GitHub Pages builds `docs/` from `main` on every push (legacy build). It stays that way; a docs-only merge publishes without a gate |

## Decision needed before Task 7

The mode. The skill says to ask when the user hasn't named one.

- **fast (recommended):** PRs run Tier 1 only; a merge to `main` runs nothing; the full suite runs on `gh workflow run ci.yml --ref main`. It fits because one person and several agents are merging short PRs, and after Task 8 nothing reaches users until a release, which refuses a commit without a green full suite.
- **prod:** every PR and every merge runs the full suite, including Docker. Choose it if Task 8 is dropped, because then every merge to `main` is what `go install …@latest` gives users.

Tasks 1 to 6 are the same in both modes.

## Global Constraints

- Gate: `mise run verify`. `mise run cli:test` keeps working and runs the same thing.
- Task names: `setup`, `fmt`, `lint`, `test:unit`, `test:integration`, `verify`, `ci:unit`, `ci:shell`, `ci:integration`, `ci:plan-test`.
- Gate names, exact: `CI Gate: all required checks passed` and `CI Gate (full suite)`.
- Repo variable: `QUALITY_MODE`, `fast` or `prod`, no default; a missing or other value fails CI.
- Go build tag for tests that need real pnpm, Node or Docker: `integration`.
- Strictness switch: `IPM_TEST_STRICT=1` turns a missing tool from a skip into a failure. Every mise task sets it; only a bare `go test` skips.
- Tool versions come from `[tools]` in `mise.toml`, pinned to exact versions with `mise use --pin`. pnpm is `12.10.1` (the `pnpmPinned` constant in `cli/internal/cli/pnpm.go`), so no test falls back to `npx`.
- Tests never touch the real home: absolute temp `IPM_HOME` and `IPM_CACHE_DIR`.
- The search server is always container `ipm-typesense` on `127.0.0.1:8108` (constants in `cli/internal/cli/serve.go`). A test that starts it must refuse to run when that container already exists or the port is taken, so it never replaces a developer's own server.
- Release tags are `cli/vX.Y.Z`: the Go module is in `cli/`, so Go only sees tags with that prefix.
- Docs-only paths, one list in `.github/docs-only.txt`, read by both workflows: `docs/**`, `*.md`, `LICENSE`. Never `mise.toml`, `.githooks/**`, `.github/**`, `install.sh`, `bin/**`.
- Markdown prose is not hard-wrapped. `docs/superpowers/` is history; this plan is the only file added there.
- Changing repo settings, creating the ruleset and pushing a tag are outward-facing: Tasks 7 and 8 stop for the user's go-ahead before each `gh api` write.

## Review Focus

1. **A partially staged file.** The hook checks the working tree, so a file with both staged and unstaged changes would be judged on content that isn't being committed. Expected: the commit is refused with the file named. Pinned in Task 2.
2. **Pushing `main` under another local name** (`git push origin HEAD:main`, `git push origin feature:main`). Expected: refused, because the hook reads the remote ref, not the local branch. Pinned in Task 3.
3. **`QUALITY_MODE` unset or misspelled.** Expected: `plan` fails with the variable named, and the merge gate fails; no job is silently skipped into a green gate. Pinned in Task 5.
4. **A PR that touches `docs/` and `mise.toml` together.** Expected: not docs-only, Tier 1 runs. Pinned in Task 5.
5. **Integration job on a runner without pnpm, Node or Docker.** Expected: red, naming the missing tool, never green with every test skipped. Pinned in Task 4.

---

### Task 1: Tasks and pinned tools in `mise.toml`

**Files:**
- Modify: `mise.toml`
- Create: `cli/.golangci.yml`
- Modify: Go files that the new linters flag

**Interfaces:**
- Produces: tasks `setup` (defined here as a no-op placeholder that Task 2 fills), `fmt`, `lint`, `test:unit`, `verify`, `ci:unit`, `ci:shell`; `[env] IPM_TEST_STRICT = "1"`.

- [ ] **Step 1: Pin the tools.** Upgrade mise first (`mise self-update`; the local one is 2026.1.7). Then, from the repo root: `mise use --pin go@1.26 golangci-lint@latest shellcheck@latest shfmt@latest actionlint@latest node@24 pnpm@12.10.1`. Check that `[tools]` holds exact versions and that `mise exec -- pnpm --version` prints `12.10.1`.

- [ ] **Step 2: Add `cli/.golangci.yml`** with golangci-lint's default linters (errcheck, govet, ineffassign, staticcheck, unused) and nothing disabled. Run `mise exec -- golangci-lint run ./...` in `cli/` and record the findings.

- [ ] **Step 3: Fix the findings.** Fix each in code. Where a finding is deliberate (for example the unchecked `io.WriteString` calls to a terminal in `cli/internal/cli/progress.go`), exclude that function or path in `.golangci.yml` with a comment giving the reason; never disable a whole linter. Expected: `golangci-lint run ./...` exits 0.

- [ ] **Step 4: Define the tasks.**

| Task | Runs |
| --- | --- |
| `fmt` | `gofmt -w` over `cli/`, `shfmt -w -i 2` over `install.sh`, `bin/ipm`, `.githooks/*`, `.github/scripts/*.sh` |
| `lint` | the gofmt check from today's `cli:test`, `shfmt -d -i 2` on the same shell files, `golangci-lint run ./...` in `cli/` |
| `test:unit` | `go build ./... && go vet ./... && go test ./...` in `cli/` |
| `verify` | `depends = ["lint", "test:unit"]` |
| `cli:test` | `depends = ["verify"]`, description says it is the old name |
| `ci:unit` | `depends = ["verify"]` |
| `ci:shell` | `shellcheck` on the shell files above, `actionlint` |
| `setup` | placeholder: `echo "no hooks yet"` |

Add `[env] IPM_TEST_STRICT = "1"`. `ci:shell` must pass when `.github/` doesn't exist yet.

- [ ] **Step 5: Verify.** `mise run verify` and `mise run ci:shell` exit 0. Add a space to a Go file: `mise run verify` fails naming the file; `mise run fmt` fixes it. Revert.

- [ ] **Step 6: Commit** on a branch: `build: mise verify gate, pinned tools and golangci-lint`.

### Task 2: Pre-commit hook

**Files:**
- Create: `.githooks/pre-commit`
- Modify: `mise.toml` (`setup`, `[hooks]`)

**Interfaces:**
- Consumes: `mise run fmt`-equivalent commands and `mise run verify` from Task 1.
- Produces: `mise run setup`, which runs `git config core.hooksPath .githooks`.

- [ ] **Step 1: Write `.githooks/pre-commit`** (bash, `set -euo pipefail`). In order:
  1. List staged files (`git diff --cached --name-only --diff-filter=ACMR`). If any of them also appears in `git diff --name-only`, exit 1 with `pre-commit: <file> has unstaged changes — stage or stash them, the hook checks whole files`.
  2. Run `gofmt -w` on the staged `.go` files and `shfmt -w -i 2` on the staged shell files, then `git add` exactly those files. Never `git add -u`.
  3. Run `mise run verify` (whole repo).

- [ ] **Step 2: Fill in `setup`** and add `[hooks] enter = "mise run setup"` so a shell with mise activated installs the hooks on entering the checkout. Check the hook fires: `git config --unset core.hooksPath`, leave and re-enter the directory in a shell with `mise activate`, and confirm `git config core.hooksPath` prints `.githooks`. If `[hooks]` needs a setting to be enabled, record that in `AGENTS.md` in Task 9 and keep `mise run setup` as the documented step.

- [ ] **Step 3: Verify by doing, in a scratch clone** (`git clone . <scratch>/hooktest`, then `mise trust && mise run setup` there). Each of these must behave as stated:
  - A staged, badly formatted `.go` file is committed formatted.
  - A staged file with a vet error (`fmt.Printf("%d", "x")`) is refused.
  - A staged failing unit test is refused.
  - A staged `unused` variable in a function is refused by golangci-lint.
  - A file staged and then edited again is refused with the message from Step 1.
  - An unstaged, unrelated edit in another file stays unstaged after a successful commit.

- [ ] **Step 4: Commit:** `build: pre-commit hook runs format and the verify gate`.

### Task 3: Pre-push hook

**Files:**
- Create: `.githooks/pre-push`

- [ ] **Step 1: Write `.githooks/pre-push`.** Git passes one line per ref on stdin: `<local ref> <local sha> <remote ref> <remote sha>`. If any remote ref is `refs/heads/main`, exit 1 with `pre-push: direct pushes to main are refused — open a PR: gh pr create --fill && gh pr merge --auto --squash`.

- [ ] **Step 2: Verify in the scratch clone** against a local bare remote (`git init --bare`), never GitHub: `git push origin main`, `git push origin HEAD:main` and `git push origin feature:main` are all refused; `git push origin feature` succeeds; deleting a feature branch on the remote succeeds.

- [ ] **Step 3: Commit:** `build: pre-push hook refuses pushes to main`.

### Task 4: Integration suite that cannot self-skip

**Files:**
- Create: `cli/internal/testtool/testtool.go`, `cli/internal/testtool/testtool_test.go`
- Create: `cli/cmd/ipm/pnpm_integration_test.go` (the three real-pnpm tests moved out of `main_test.go`), `cli/cmd/ipm/search_integration_test.go`
- Modify: `cli/cmd/ipm/main_test.go`, `cli/internal/cli/pnpmfile_test.go`, `mise.toml`

**Interfaces:**
- Produces: `testtool.Need(t testing.TB, name string) string` returns the tool's path; when it is missing it calls `t.Fatalf("%s is required when IPM_TEST_STRICT=1", name)` if `IPM_TEST_STRICT` is `1`, else `t.Skipf("%s is not installed", name)`. Tasks `test:integration` and `ci:integration`.

- [ ] **Step 1: Test `Need`** with a fake `testing.TB`: a tool that exists returns its path; a missing tool skips when the variable is unset and fails when it is `1`. Run it, see it fail, implement, see it pass.

- [ ] **Step 2: Move the real-pnpm tests.** `TestSourceSyncIndexWithPnpm`, `TestSyncUpgradeMovesBranchesNotTags` and `TestPackInstallWithPnpm` go to `pnpm_integration_test.go` under `//go:build integration`, using `testtool.Need` for `pnpm` and `node` in place of their `t.Skip`. `TestExitCodes` and `TestMain` stay untagged. `pnpmfile_test.go` stays a unit test and uses `testtool.Need(t, "node")`.

- [ ] **Step 3: Write `TestServeLoadSearch`** in `search_integration_test.go` (tag `integration`). It needs `docker`, `pnpm`, `node`. Before anything else it fails with `an ipm-typesense container already exists — this test would replace it; run it in CI, or remove yours with ipm serve --stop && docker rm ipm-typesense` if `docker inspect ipm-typesense` succeeds, and with a similar message if `127.0.0.1:8108` accepts a connection. Then, with temp `IPM_HOME` and `IPM_CACHE_DIR` and the local `file://` git source the pnpm tests build (one skill, `skills/one`): `source add`, `sync`, `serve`, `load`, then `search one --keyword --json` must exit 0 and contain `skills/one`. `t.Cleanup` runs `docker rm -f ipm-typesense`. Use `--keyword` so the test doesn't depend on downloading an embedding model; add one hybrid `search one` assertion only if it passes on the CI runner within the job's time limit.

- [ ] **Step 4: Define the tasks.** `test:integration` runs `go test -tags integration -count=1 ./cmd/ipm/...` in `cli/`. `ci:integration` first asserts the tools reached the runner (`pnpm --version` is `12.10.1`, `node --version`, `docker info`), each failing with the tool's name, then runs `test:integration`.

- [ ] **Step 5: Verify.** `mise run verify` passes and no longer runs the pnpm tests (`go test -v ./cmd/ipm/ | grep -c Pnpm` is 0). `PATH=/usr/bin:/bin IPM_TEST_STRICT=1 go test -tags integration ./cmd/ipm/` fails naming `pnpm`. `mise run ci:integration` passes on a machine with Docker running and no `ipm-typesense` container; on the maintainer's machine, where that container exists, confirm it fails with the message from Step 3 and leaves the container running, and take the green run from CI in Task 5.

- [ ] **Step 6: Commit:** `test: integration tag, real Typesense test, no silent skips`.

### Task 5: The CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`, `.github/scripts/plan.sh`, `.github/scripts/plan_test.sh`, `.github/docs-only.txt`
- Modify: `mise.toml` (`ci:plan-test`)

**Interfaces:**
- Produces: `plan.sh` reads `MODE`, `EVENT`, `SUITE` and a newline-separated `CHANGED` file list from the environment and prints `gate=`, `tier1=`, `full=` lines (the workflow appends them to `$GITHUB_OUTPUT`). `docs_only <file>...` is a function in `plan.sh` that Task 8 sources.

- [ ] **Step 1: Write `plan_test.sh`**, a table of cases run by `mise run ci:plan-test`, asserting `plan.sh`'s output:

| MODE | EVENT | SUITE | CHANGED | gate | tier1 | full |
| --- | --- | --- | --- | --- | --- | --- |
| fast | pull_request | | `cli/x.go` | true | true | false |
| prod | pull_request | | `cli/x.go` | true | true | true |
| fast | push | | `cli/x.go` | false | false | false |
| prod | push | | `cli/x.go` | true | true | true |
| fast | workflow_dispatch | full | | true | true | true |
| fast | workflow_dispatch | fast | | true | true | false |
| prod | pull_request | | `docs/a.md`, `README.md` | true | false | false |
| prod | pull_request | | `docs/a.md`, `mise.toml` | true | true | true |
| prod | pull_request | | `.github/workflows/ci.yml` | true | true | true |
| (unset) | pull_request | | `cli/x.go` | exit 1, stderr names `QUALITY_MODE` | | |
| `Fast` | pull_request | | `cli/x.go` | exit 1 | | |

Run it and see it fail.

- [ ] **Step 2: Write `plan.sh` and `.github/docs-only.txt`** (the three patterns from Global Constraints) until the table passes. Add `ci:plan-test` to `ci:shell`.

- [ ] **Step 3: Write `ci.yml`** from the workflow in `references/github-modes.md`, with these jobs and nothing inline beyond checkout, installing mise and calling a task:

| Job | Needs | Runs when | Runs |
| --- | --- | --- | --- |
| `plan` | | not a draft PR | checkout; for a PR, `gh pr diff --name-only` into `CHANGED`; `plan.sh` |
| `unit` | `plan` | `tier1 == 'true'` | `mise run ci:unit` |
| `shell` | `plan` | `tier1 == 'true'` | `mise run ci:shell` |
| `integration` | `plan`, `unit` | `full == 'true'` | `mise run ci:integration` |
| `gate`, named `CI Gate: all required checks passed` | all four | `!cancelled() && gate != 'false'` | every needed result is `success` or `skipped` |
| `full-gate`, named `CI Gate (full suite)` | all four | `!cancelled() && full == 'true'` | every needed result is `success` |

Triggers, the `concurrency` group (`ci-${{ github.event_name }}-${{ github.event.pull_request.number || github.ref }}`, `cancel-in-progress: true`) and `permissions: contents: read` (plus `pull-requests: read` on `plan`) are as in the reference. No `paths-ignore`. Pin `MISE_VERSION` in the workflow `env` to the version Task 1 was verified with, with a comment saying so; pin each action to a major version. Cache Go's module and build caches keyed on `cli/go.sum`.

- [ ] **Step 4: Verify.** `mise run ci:shell` passes (actionlint and the plan table). Push the branch, open a PR, and set the mode for the test with the user's go-ahead: `gh variable set QUALITY_MODE --body prod`. Confirm on GitHub: `unit`, `shell`, `integration` and both gates are green; a second push cancels the first run and the cancelled run posts no red gate; converting the PR to draft and pushing runs nothing. Then `gh variable delete QUALITY_MODE`, re-run, and confirm `plan` and the merge gate fail naming the variable. Restore the variable to the mode the user chose.

- [ ] **Step 5: Commit:** `ci: tiered pipeline with a merge gate and a full-suite gate`.

### Task 6: Docs-only PRs still get a gate

**Files:** none new; this task exercises Task 5 on GitHub.

- [ ] **Step 1:** After Task 5 is merged, open a PR that changes only `docs/troubleshooting.md` (a real fix or a throwaway line removed afterwards). Expected: `plan` and the merge gate run and are green; `unit`, `shell` and `integration` are skipped; no `CI Gate (full suite)` check exists.

- [ ] **Step 2:** Add a change to `mise.toml` (a comment) to the same PR. Expected: `unit` and `shell` run. Drop that commit before merging.

### Task 7: GitHub settings, ruleset and auto-merge

**Files:** none in the repo. Stop and get the user's mode choice and go-ahead before Step 1.

- [ ] **Step 1: Apply the settings** from `references/github-modes.md`, "Repo settings and the main-branch ruleset": `allow_auto_merge`, `allow_squash_merge` and `delete_branch_on_merge` on; `gh variable set QUALITY_MODE --body <mode>`; the `main` ruleset with `deletion`, `non_fast_forward`, `pull_request` (0 approvals) and `required_status_checks` requiring only `CI Gate: all required checks passed` with `integration_id: 15368` and `strict_required_status_checks_policy: false`. Today the repo has no rulesets and `allow_auto_merge` is false.

- [ ] **Step 2: Verify by doing.**
  - `git push --no-verify origin main` with a throwaway commit is rejected by GitHub. Reset the local commit afterwards.
  - A code PR opened with `gh pr create --fill && gh pr merge --auto --squash` merges by itself when the gate is green, without being updated from `main`.
  - A docs-only PR opened the same way also merges by itself.
  - Start `gh workflow run ci.yml --ref main`, merge a PR while it runs, and confirm the manual run is not cancelled.
  - In fast mode a merge starts no jobs beyond `plan`; in prod mode it starts the full suite.

### Task 8: Release workflow, the deploy gate

**Files:**
- Create: `.github/workflows/release.yml`, `.github/scripts/assert-full-gate.sh`
- Modify: `.github/scripts/plan_test.sh` (cases for the docs-only fallback)

**Interfaces:**
- Consumes: `docs_only` from `plan.sh` and `.github/docs-only.txt` (Task 5).
- Produces: `assert-full-gate.sh <sha>` exits 0 when the check run `CI Gate (full suite)` from GitHub Actions (app id 15368) concluded `success` for `<sha>`, or for its newest ancestor that has one when every file changed since is docs-only. Otherwise it exits 1 with `no green "CI Gate (full suite)" for <sha> — run: gh workflow run ci.yml --ref main, wait for green, then release again`.

- [ ] **Step 1: Write the script** so the two GitHub lookups (check runs for a commit, files changed between two commits) are functions that `plan_test.sh` can replace, and add cases: gate green on the commit passes; gate missing fails with the message; gate failed fails; gate on the parent and only `docs/a.md` changed passes; gate on the parent and `cli/x.go` changed fails. See them fail, implement, see them pass.

- [ ] **Step 2: Write `release.yml`:** `workflow_dispatch` with a `version` input (`X.Y.Z`, validated), refused on any ref but `main`; `concurrency: { group: release, cancel-in-progress: false }`; `permissions: contents: write`. Steps: checkout with full history, `assert-full-gate.sh "$GITHUB_SHA"`, refuse if `cli/v<version>` exists, then `gh release create cli/v<version> --target "$GITHUB_SHA" --generate-notes`.

- [ ] **Step 3: Verify by doing,** with the user's go-ahead for the first tag. In fast mode, dispatch the release on a commit that has only a PR run: it must fail with the message from the script and create no tag. Run `gh workflow run ci.yml --ref main`, wait for green, dispatch again with `0.1.0`: the tag `cli/v0.1.0` exists, and `GOBIN=<scratch> GOFLAGS=-modcacherw go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest` in a clean module cache installs that tag (`go version -m <scratch>/ipm` shows `v0.1.0`).

- [ ] **Step 4: Commit:** `ci: release workflow that asserts the full-suite gate`.

### Task 9: Write it down

**Files:**
- Modify: `AGENTS.md`, `CLAUDE.md`, `cli/README.md`, `docs/getting-started.md`

- [ ] **Step 1: `AGENTS.md`** gets a "Quality gate" section stating: the verify command (`mise run verify`) and that `mise run ci:<job>` reproduces each CI job; `mise run setup` installs the hooks; the mode, and in fast mode that integration regressions only surface on `gh workflow run ci.yml --ref main`, with that command; PRs are opened with `gh pr create --fill && gh pr merge --auto --squash`; when `main` is red, pause merging until a fix-forward PR is green; the enforcement table (pre-commit and pre-push hooks: local, bypassable with `--no-verify`; ruleset and auto-merge: server-side; release gate: server-side, not bypassable); how to release and how to roll back (release a new patch version from a good commit; a user can pin with `go install …/cli/cmd/ipm@vX.Y.Z`); that the docs site publishes on every merge without a gate.

- [ ] **Step 2: `CLAUDE.md`:** the commands table names `mise run verify` as the gate, adds `mise run setup` and `mise run test:integration`, and the sharp edges gain one line: the search integration test refuses to run beside an existing `ipm-typesense` container.

- [ ] **Step 3: `cli/README.md` and `docs/getting-started.md`:** replace `mise run cli:test` where it is named as the gate, and in "From a clone" add `mise run setup` for contributors. After Task 8, say that the one-line installer installs the latest release.

- [ ] **Step 4: Verify** each sentence against a run from Tasks 1 to 8, not from memory, then `mise run verify` and commit: `docs: quality gate, modes and enforcement`.

## Definition of done

Each line is checked by doing it, and the executor reports what was run and what was not:

- [ ] A fresh clone gets both hooks from `mise run setup` (or on entering the directory with mise activated). This is one step, not zero: Go has no package-manager prepare step; `AGENTS.md` says so.
- [ ] A commit with a format, lint, vet or unit-test failure is refused (Task 2, Step 3).
- [ ] `mise run verify` is the documented gate, and every CI job runs one task.
- [ ] Tier 1, Tier 2, two gates with the exact names, drafts skipped, superseded runs cancelled.
- [ ] `QUALITY_MODE` is set and named in `AGENTS.md`.
- [ ] A direct push to `main` is rejected locally and by GitHub; a green PR auto-merges without being updated.
- [ ] The release workflow refuses a commit without a green full-suite gate.
- [ ] Unit and integration suites run in CI, and a missing tool is a failure.
- [ ] Not applicable and left out: the UI smoke suite, fsconfig, environments.
