# AGENTS.md

Router for agents working in [thinkjones/intelligent-package-manager](https://github.com/thinkjones/intelligent-package-manager): the source code of `ipm` ("intelligent package manager"), a local Go CLI that fetches repos of agent artifacts with pnpm, catalogs every skill, agent, command, hook, MCP server and plugin in them, searches the catalog, and builds packs of picked items into Claude Code plugins. Read the linked file for detail; this page is only where things are.

`ipm` was called `aihs`. The design and its parts are in [the ipm design](docs/superpowers/specs/2026-10-09-ipm-design.md); parts 1 (rename), 2 (sources on pnpm), 3 (packs built into plugins with `ipm install`) and 4 (`ipm tui`, a Bubble Tea TUI over search and packs) are built.

## Where things are

| Path | What it is |
| --- | --- |
| [`cli/`](cli/README.md) | `ipm`, the Go CLI. Every command is in `cli/internal/cli`; the TUI's model is `cli/internal/tui` (Bubble Tea, behind four interfaces that `cli/internal/cli/tui.go` implements), which also holds `ipm onboard`'s checklist (`checklist.go`); `cli/internal/curated` reads the curated source lists `ipm onboard` offers. `cli/cmd/sourcepacks` is a maintainer's tool, not part of the installed `ipm`: see [Source packs](#source-packs). |
| `bin/ipm` | Shim that builds `cli/bin/ipm` when a Go source changes and runs it: `bin/ipm <command>` runs a development build. It isn't on PATH; `.envrc` no longer adds it. |
| `install.sh` | Installs `ipm` into `~/.local/bin` (or `$IPM_BIN_DIR`): builds `cli/` from a checkout, or runs `go install …@latest` when piped from curl; then starts `ipm onboard` in a terminal. |
| [`docs/`](docs/) | The user documentation, published to GitHub Pages at <https://thinkjones.github.io/intelligent-package-manager/> (Jekyll, Just the Docs theme, `docs/_config.yml`): [getting started](docs/getting-started.md), [concepts](docs/concepts.md), `guides/`, `reference/` ([commands](docs/reference/commands.md), [sources](docs/reference/sources.md), [files](docs/reference/files.md)), [troubleshooting](docs/troubleshooting.md), [how it works](docs/architecture.md). |
| `docs/assets/brand/` | The hero, workflow and logo SVGs, shared by the README and the site. The palette (`#f8fafc` background, `#0f172a` text, `#e85d26` orange for the wordmark and pins, `#19a0a6` teal for verified) is also in `docs/_sass/custom/setup.scss`; change both together. |
| `docs/plans/` | Early notes. Historical, and excluded from the site. |
| `docs/superpowers/` | Design specs and implementation plans. Historical: never rewrite them to match the code. Excluded from the site. |
| `LICENSE` | MIT. |

`ipm`'s data is outside this repo, per user:

| Path | What it is |
| --- | --- |
| `$IPM_HOME/sources/` | Your repos of interest as a pnpm project: `package.json` (one git dependency per repo, alias `owner--repo`), `pnpm-lock.yaml` (pinned commits), `node_modules/<alias>/` (the installed repos), `.pnpmfile.cjs` (generated). |
| `$IPM_HOME/store/` | ipm's dedicated pnpm store. |
| `$IPM_HOME/global/` | User scope (`-g`): `ipm.toml` and `.ipm/`, laid out as in a project. |
| `$IPM_CACHE_DIR/catalog/*.jsonl` | The catalog, written by `ipm index` (`github.jsonl` by `ipm stats`). Rebuildable; never committed. See [the catalog spec](docs/superpowers/specs/2026-10-08-artifact-catalog-design.md). |
| `$IPM_CACHE_DIR/typesense/` | The local search server's data and API key. Disposable; `ipm load` rebuilds the index. |
| `$IPM_CACHE_DIR/curated/` | The last copy of each curated list `ipm onboard` fetched, used when offline. |

In a project (the nearest ancestor of the working directory holding `ipm.toml`):

| Path | What it is |
| --- | --- |
| `ipm.toml` | Packs: `[packs.<name>]` with `description` and `items` (catalog IDs). Edited with `ipm pack`. Committed. |
| `.ipm/ipm.lock` | Every picked item resolved to repo, path and commit. Generated, committed. |
| `.ipm/package.json`, `.ipm/pnpm-lock.yaml`, `.ipm/.pnpmfile.cjs` | One pnpm dependency per source repo, pinned to a commit. Generated, committed. |
| `.ipm/.claude-plugin/marketplace.json` | The marketplace listing each pack and its whole-plugin dependencies. Generated, committed. |
| `.ipm/plugins/<pack>/`, `.ipm/node_modules/` | Built plugins (hard links into pnpm's files) and pnpm's output. Gitignored by the generated `.ipm/.gitignore`. |

Defaults: `IPM_HOME` is `~/Library/ipm` on macOS, `$XDG_DATA_HOME/ipm` or `~/.local/share/ipm` on Linux, `%LOCALAPPDATA%\ipm` on Windows; `IPM_CACHE_DIR` is `~/Library/Caches/ipm` on macOS, `$XDG_CACHE_HOME/ipm` or `~/.cache/ipm` on Linux.

## Conventions

- **Generated files are regenerated, not edited.** Change sources with `ipm source`, then `ipm sync`; change packs with `ipm pack`, then `ipm install`.
- **No upstream code runs.** pnpm always runs with `--ignore-scripts`, and sources' own npm dependencies are never installed.
- **`ipm` is self-contained.** It works from any directory, keeps its data only in `IPM_HOME` and `IPM_CACHE_DIR` (plus `ipm.toml` and `.ipm/` in a project), installs into Claude Code only on `ipm deploy`, and shares no configuration with fsh or Gas City.
- **All CLI commands are Go**, built with [Cobra](https://cobra.dev), in `cli/internal/cli`. There are no script commands. Flags are POSIX style: long flags take two dashes.
- **Prose in Markdown is not hard-wrapped**: one line per paragraph or list item.
- **User docs change with the code.** A change to a command, flag, message or file format updates `docs/reference/commands.md`, the guide that uses it and `docs/troubleshooting.md` in the same commit. Each site page has Just the Docs front matter (`title`, `nav_order`, `parent` under `guides/` and `reference/`); link between pages with relative `.md` links, which work on GitHub and on the site. Don't link to `docs/superpowers/` with a relative link from a site page (it isn't published); use its GitHub URL.

## Quality gate

- **The gate is `mise run verify`**: gofmt and shfmt checks, golangci-lint, `go build`, `go vet` and the unit tests, over the whole repo. Run it, not a subset. `mise run fmt` fixes formatting. Tool versions are pinned in `mise.toml`.
- **Git hooks**: `mise run setup` installs them, once per clone (a shell with mise activated does it on entering the checkout). `pre-commit` formats the staged files and runs the gate; it refuses a file that is staged and then edited again. `pre-push` refuses any push to `main`.
- **CI** (`.github/workflows/ci.yml`) runs mise tasks, so `mise run ci:<job>` reproduces a job: `ci:unit` (the gate), `ci:shell` (shellcheck, actionlint, the tests of the CI scripts), `ci:integration` (real pnpm, Node and Typesense in Docker). `.github/scripts/plan.sh` decides what a run schedules.
- **The mode is fast** (repo variable `QUALITY_MODE`; CI fails without it). A pull request runs `unit` and `shell`; a merge to `main` runs nothing. So a break in the integration suite only shows when someone runs the full suite: `gh workflow run ci.yml --ref main`. Switch with `gh variable set QUALITY_MODE --body prod`, which runs the full suite on every pull request and merge.
- **Open a pull request with** `gh pr create --fill && gh pr merge --auto --squash`. It merges by itself when `CI Gate: all required checks passed` is green, and does not have to be up to date with `main`. Two green pull requests can still combine into a red `main`; when that happens, stop merging until a fix is green.
- **A pull request that only touches docs** (`.github/docs-only.txt`: `docs/**`, `*.md`, `LICENSE`) schedules no jobs and still gets a green gate. Draft pull requests schedule nothing.
- **Releasing**: `gh workflow run release.yml -f version=X.Y.Z` tags `cli/vX.Y.Z` on `main`. It refuses a commit without a green `CI Gate (full suite)`, so run the full suite first. `ipm version` reads the version from the build, so the tag is the only place it is set. Once a release exists, `go install …/cli/cmd/ipm@latest` and `install.sh` give users the newest release; until then they give `main`. To roll back, release a new patch version from a good commit; a user can pin one with `go install …/cli/cmd/ipm@vX.Y.Z`.
- **The docs site is not gated**: GitHub Pages publishes `docs/` on every merge to `main`.

| Guard | Where | Can it be bypassed? |
| --- | --- | --- |
| `pre-commit`, `pre-push` hooks | Your clone | Yes, with `--no-verify` |
| Ruleset on `main`: pull request and merge gate required, no force-push, no deletion | GitHub | No |
| The release workflow refuses a commit without a green full suite | GitHub | Yes: nothing stops someone with write access pushing a `cli/v*` tag by hand. Release only through the workflow |

## Source packs

`source-packs.toml` is the curated list `ipm onboard` offers by default and `ipm source list` groups by (`curated.DefaultURL` reads it from `main`, so a merge changes what every user is offered). It holds only repos with something `ipm` can catalog, grouped into named lists (the source packs). It and `source-packs.skipped.txt` are generated: never edit them by hand.

- **`mise run packs:update`** reads [thinkjones/agentic-tools](https://github.com/thinkjones/agentic-tools)'s `starred_repos.toml`, downloads each repo it hasn't seen, and looks for the markers the catalog indexes (`cli/internal/catalog`'s `Walk`). A repo holding something other than MCP files joins its list in `source-packs.toml` with its stars, description, topics, homepage and counts; any other goes in `source-packs.skipped.txt` and isn't fetched again (`-- --recheck` does). A repo that can't be fetched (`--timeout`, 10 minutes each) is recorded nowhere and tried on the next run. It prints a Markdown report of the new repos by pack.
- **The Source packs workflow** (`.github/workflows/source-packs.yml`) runs that task by hand: `gh workflow run source-packs.yml`. The report is the run's summary. If the files changed, it pushes them to the branch `source-packs/update` and opens a draft pull request with the report as its body.
- **The pull request is a draft on purpose.** One opened with the workflow's token starts no CI run, so the merge gate would never report. Read the list, then mark it ready (`gh pr ready source-packs/update`), which starts CI; `gh pr merge --auto --squash source-packs/update` then merges it.
- The repo setting "Allow GitHub Actions to create and approve pull requests" must stay on for the workflow to open one.

[`CLAUDE.md`](CLAUDE.md) has the commands and the sharp edges.
