# ipm install, onboard and uninstall — design

Status: approved in conversation 2026-10-10; for review as a document

## Intent

Today `ipm` is something you work on, not something you install: it runs from the checkout's `bin/` through direnv, and a new user starts with an empty list of sources and no idea which repos are worth following. This adds the path a new user takes: install `ipm` with one script, pick sources from a curated list, and end with a searchable library — and the way back out, `ipm uninstall`.

The curated list is [`starred_repos.toml`](https://thinkjones.github.io/ai-sdlc-repo-list/starred_repos.toml) from `thinkjones/ai-sdlc-repo-list`: 315 repos in 22 lists, each with stars, language, last push and a description. ipm reads it as published; nothing in the user's current setup changes unless they choose to back it up during onboarding.

### Success criteria

- From a machine with Go and an old ipm setup: `ipm uninstall`, then `./install.sh`, asks whether to use or back up the old data; backing up and picking the two pre-ticked lists ends with `✓ library ready` and `ipm search tdd` returning results.
- Choosing **Use it** keeps every existing source and adds only what is ticked.
- A failure in any step names the step and the command to rerun; running `ipm onboard` again resumes.
- Uninstall never deletes data without an explicit `--purge` and confirmation, and never deletes a development build by accident.

## `install.sh` (repo root)

- Needs Go (`go version`); without it, says so with a link to go.dev/dl and exits 1.
- In a checkout of this repo (the script's folder has `cli/go.mod`): `go build -o ~/.local/bin/ipm ./cmd/ipm` from `cli/`. Otherwise (piped from curl): `GOBIN=~/.local/bin go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest`.
- Warns, without failing, when `~/.local/bin` is not on PATH (with the line to add for the user's shell) and when `command -v ipm` finds a different binary first.
- Ends by running `ipm onboard` when stdin and stdout are a terminal; otherwise prints `next: ipm onboard`.
- Passes `shellcheck`; `set -euo pipefail`.

`.envrc` drops `PATH_add bin`, so inside the repo the installed `ipm` is the one on PATH; `bin/ipm` stays for running a development build explicitly. `mise run cli:test` is unchanged.

## `ipm uninstall [--purge] [--yes] [--force]`

1. Stops and removes the `ipm-typesense` container when Docker is available (no Docker: skipped with a note).
2. Removes the running binary (`os.Executable()`, symlinks resolved). It refuses when that binary is inside a git checkout's `bin/` — a development build — unless `--force`.
3. Without `--purge`, prints where the data stays: `kept your data: <home>, <cache> — ipm onboard will offer to use or back it up`.
4. With `--purge`, also deletes home and cache after the user types the home path to confirm; `--yes` skips the confirmation (and is required when stdin is not a terminal).

## `ipm onboard [--list <path|url>] [--lists <name>]… [--keep|--fresh] [--yes]`

1. **Existing data.** When home has sources or the cache has a catalog, print a summary (`26 sources · 3,280 artifacts · synced 2026-10-09`) and ask:
   - **Use it** — keep everything (`--keep`).
   - **Back it up and start fresh** — rename home and cache to `<dir>.backup-YYYYMMDD-HHMM` (with `-2`, `-3` on a clash), print both paths and how to restore them (`ipm uninstall`, then move the folders back), and continue with empty ones (`--fresh`).
   - **Quit.**
   No existing data: no question.
2. **Curated list.** Fetch `--list` (default the published URL), cache it in `$IPM_CACHE_DIR/curated/<sha of url>.toml`, and fall back to the cached copy, with a note, when offline. Unreadable and no cache: stop with the error and `--list <path>` as the way around it.
3. **Pick** in the checklist (below). With `--lists` (repeatable, exact list names) the checklist is skipped; `--yes` accepts the pre-ticked lists without showing it. Non-terminal stdin without either: a usage error.
4. **Pipeline,** each step printing `✓ <step>` or `✗ <step>: <error>` with the command to rerun:
   1. add each new pick as a source (the same path as `ipm source add`);
   2. sync (`runSync`: pnpm, then index);
   3. stats (`runStats`), skipped with a note when `gh` is missing or not logged in;
   4. serve then load (`runServe`, `runLoad`), skipped with `start Docker, then ipm serve && ipm load` when Docker is not running.
   Steps run in-process, writing to the same output. A failing step stops the pipeline; sources already added stay added.
5. **Done:** `✓ library ready — 47 sources, 2,914 artifacts`, then `try: ipm search <q>` for three queries taken from the picked lists' most common topics, and `ipm tui`.

Running `ipm onboard` again on a working setup is the **Use it** path: existing sources shown ticked and locked, new ticks added.

## The checklist (`internal/onboard`)

A Bubble Tea program of its own (not a mode of `ipm tui`), reusing the TUI's styles and colours.

```text
Choose sources — 315 repos in 22 lists                         47 picked
  [x] AI · Agentic Engineering Methods          21 repos   ★412k
▶ [~] AI · Skills, Plugins & Registries         26 repos   ★388k
      [x] anthropics/skills          ★52.1k  Python  3d   Public repository for Agent Skills
      [ ] someone/awesome-skills     ★8.2k   —       2mo  A curated list of Claude skills and…
  [ ] AI · Multi-Agent Orchestration            14 repos   ★201k
space tick · ←→ collapse/expand · / filter · a all · n none · enter continue · q quit
```

- **List rows:** tick state (`[x]` all, `[~]` some, `[ ]` none), name, repo count, summed stars.
- **Repo rows** (under an expanded list): tick, repo in the repo colour, `★` stars, language, last push as an age (`3d`, `2mo`, `1y`), and the description filling the rest of the width, cut the way search results are. Under 100 columns language and age drop out.
- **Keys:** `↑↓` move; `←→` collapse/expand (`→` on a repo does nothing); `space` ticks the row (a list ticks or unticks all its repos); `/` filters repo rows by repo, description or topic and expands lists with matches; `a` / `n` tick all / none in the current list; `enter` goes to a confirm line, `47 repos to follow — y continue · esc back`; `q` / `ctrl+c` quits without changing anything.
- **Pre-ticked:** on a fresh start, the lists `AI · Agentic Engineering Methods` and `AI · Skills, Plugins & Registries` (by name; absent names are ignored). With existing sources, those sources are `✓` and locked instead, and nothing else is pre-ticked. A repo in two lists ticks in both. Existing sources not in the curated list are kept and not shown.

## The curated-list format (`internal/curated`)

`[[repos]]` tables; ipm reads `repo` (required, `owner/repo`), `list` (required), `stars`, `description`, `language`, `last_push` (RFC 3339 or a date) and `topics`, and ignores every other field. `[meta]` is optional; `meta.count`, when present, is not checked. Rows with a missing or malformed `repo` are skipped and counted in a note. Lists are ordered as they first appear in the file; repos within a list by stars, descending. Any TOML of this shape works, so others can publish their own.

## Errors and edge cases

- Backup rename fails (permissions, cross-device): stop before the checklist; nothing has changed.
- A pick is already a source (resumed onboarding): not added again.
- Nothing ticked: `enter` says `tick at least one repo` and stays.
- `IPM_HOME` / `IPM_CACHE_DIR` set: detection, backup and purge use them, as every other command does.

## Testing

- **curated:** parse the fixture (lists in order, repos by stars, malformed rows skipped), cached fallback when the fetch fails, error with no cache.
- **onboard checklist:** ticking a list and a repo, `[~]`, filter, `a`/`n`, locked rows, nothing-ticked message; goldens for collapsed, expanded, filtered and narrow.
- **onboard command** (fake `System`, temp `IPM_HOME`/`IPM_CACHE_DIR`): detection summary; backup naming and clash suffix; `--keep`/`--fresh`/`--lists`/`--yes`; step order; stats skipped without `gh`; serve/load skipped without Docker; a failing sync stops the pipeline with its rerun line.
- **uninstall:** container removed; binary removed; refusal inside a checkout's `bin/`; `--purge` confirmation and `--yes`.
- **install.sh:** `shellcheck`; run once into a temp `HOME`.
- **By hand:** the success-criteria run in a temp `HOME`, then the user's own run.

## Docs

Getting started (install script and `ipm onboard` as the first steps), the commands reference (`onboard`, `uninstall`), the sources guide (curated lists and the format), README quick start, `CLAUDE.md` and `AGENTS.md` (commands table; `.envrc` no longer puts `bin/` on PATH).
