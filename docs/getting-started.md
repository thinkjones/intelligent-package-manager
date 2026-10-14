---
title: Getting started
nav_order: 2
---

# Getting started
{: .no_toc }

Install `ipm`, pick the repos to follow, search them and build your first plugin. It takes about ten minutes, most of it waiting for the first fetch.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Requirements

| You need | For | Notes |
| --- | --- | --- |
| [Go](https://go.dev/dl/) 1.26 or later | Building `ipm` | |
| [pnpm](https://pnpm.io/installation) 12 or later, **or** [Node.js](https://nodejs.org) | `ipm catalog sync`, `ipm pack build` | With only Node, `ipm` runs `npx -y pnpm@12.10.1` for you. |
| [Docker](https://www.docker.com) (or OrbStack) | `ipm catalog serve`, `ipm search` | Runs the local Typesense search server. Everything else works without it. |
| [GitHub CLI](https://cli.github.com) `gh`, logged in | `ipm marketplace stats` | Optional: stars, topics and descriptions. |
| [Claude Code](https://code.claude.com) `claude` | Validating and using the plugins you build | Needed for `ipm install claude`; optional for `ipm pack build`, which skips validation without it. |

`ipm` is tested on macOS and Linux. Windows paths are defined but untested.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/install.sh | bash
```

The script runs `go install` for the latest release of `ipm` into `~/.local/bin` (set `IPM_BIN_DIR` to choose another folder). It needs Go. If that folder isn't on your `PATH`, it prints the `export PATH=…` line to add to your shell's startup file; if another `ipm` comes first on your `PATH`, it says which. In a terminal it then starts `ipm onboard`; otherwise it prints `next: ipm onboard`.

For tab completion of commands and flags, install the script for your shell, for example zsh: `ipm completion zsh > "${fpath[1]}/_ipm"`. `ipm completion --help` lists the shells.

### From a clone

If you want to work on `ipm`, or track `main` closely:

```bash
git clone https://github.com/thinkjones/intelligent-package-manager.git
cd intelligent-package-manager
./install.sh        # builds this checkout into ~/.local/bin/ipm
```

From a checkout, `install.sh` builds `cli/` instead of fetching the published module. If you plan to commit, install [mise](https://mise.jdx.dev), then run `mise run setup` once to install the git hooks, and `mise run verify` before a pull request. To run your working copy without installing it, use `bin/ipm <command>`: it rebuilds the CLI whenever its source changes.

### With `go install`

```bash
go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest
```

This puts the latest release of `ipm` in `$(go env GOPATH)/bin`; make sure that folder is on your `PATH`. `ipm version` prints which release you have. Then run `ipm onboard`, or follow the manual steps below.

## Onboard

```bash
ipm onboard
```

`ipm onboard` takes you from nothing to a searchable library. It first prints the four concepts, one line each: marketplace (curated repos you can follow), catalog (the repos you follow, fetched, pinned and searchable), pack (the skills, agents and plugins you pick) and install (a pack, built and installed into Claude Code). Then it loads a curated list of repos, shows them grouped into lists, and lets you tick the ones to follow. Two lists, "AI · Agentic Engineering Methods" and "AI · Skills, Plugins & Registries", are ticked for you on a fresh start.

| Key | Does |
| --- | --- |
| `space` | Tick or untick a repo, or a whole list |
| `→` / `←` | Expand or collapse a list |
| `/` | Filter by repo, description or topic (`esc` clears) |
| `a` / `n` | Tick all or none of the current list |
| `enter`, then `y` | Continue with the ticked repos |
| `q` | Quit without adding any sources |

Each repo shows its stars, language and how long ago it was last pushed (these two at 100 columns or wider), and its description; the line above the key hints shows the highlighted repo's topics and homepage. Then `ipm onboard` adds the repos as sources and runs `ipm catalog sync`, `ipm marketplace stats` and `ipm catalog serve` with `ipm catalog rebuild`, printing `✓` or `✗` for each step. While it works, one line at the bottom shows a progress bar, the step, what it is doing and the time so far, for example `[████████░░░░░░░░░░░░] 3/5 stats · 17/42 anthropics/skills · 0:48`. Stats are skipped when `gh` is missing or logged out, and search when Docker isn't running; it says so and what to run later. It ends with `✓ library ready — N sources, M artifacts` and a few searches to try.

If `ipm` already has data, `ipm onboard` says what it found and asks whether to use it or back it up and start fresh. Using it keeps your sources: they show `✓` in the checklist and nothing is ticked for you. Backing up renames the home and cache folders to `<folder>.backup-YYYYMMDD-HHMM` and prints how to restore them. If a step fails, it prints the command to rerun; running `ipm onboard` again picks up where you left off.

To use your own curated list, pass `--list <path or URL>`; the format is in [Curated lists](guides/sources.md#curated-lists). Every flag is in the [commands reference](reference/commands.md#ipm-onboard).

## Check your setup

```bash
ipm doctor
```

`ipm doctor` prints one row each for its home and cache folders, pnpm, your sources, the catalog, Docker, the search server and `claude`. Rows marked `!` are warnings (for example, no sources yet); `✗` is a problem to fix, and makes it exit 1.

`ipm` keeps its data in your user folders, never in the directory you run it from. See [Files and paths](reference/files.md) to move them.

## 1. Follow some repos from the marketplace

`ipm onboard` does this step and the next for you. To do them by hand:

```bash
ipm marketplace                    # the curated repos, with ✓ on the ones you follow
ipm marketplace add obra/superpowers anthropics/skills
ipm catalog sync
```

`ipm marketplace` lists the curated repos and marks the ones you follow. `ipm marketplace add` records the repos; `ipm catalog sync` has pnpm fetch each one, pin it to a commit and build the catalog. The first sync downloads every repo; later ones only fetch what changed. `ipm catalog` alone prints a summary: sources followed, items by type, the last sync and the search server's state.

## 2. Search the catalog

```bash
ipm catalog serve   # start Typesense in Docker, once per boot
ipm catalog rebuild # rebuild the catalog and index it for search
ipm search tdd
```

`ipm catalog sync` and `ipm catalog rebuild` load the search index themselves when the server is up; with it off they print a hint to run `ipm catalog serve`.

Each result shows a SCORE, the artifact's type, name, repo, stars, the plugins it belongs to and its description, with its **ID** on the line beneath, for example `skill:obra/superpowers:skills/test-driven-development`. IDs are how you refer to anything in `ipm`:

```bash
ipm show 'skill:obra/superpowers:skills/test-driven-development'
ipm show tree 'repo:obra/superpowers'
```

## 3. Build a plugin

In any project folder:

```bash
ipm pack new my-pack --description "My picks"
ipm pack add my-pack 'skill:obra/superpowers:skills/test-driven-development'
ipm pack build
```

`ipm pack build` writes `.ipm/` in the project (`ipm pack new` already created `ipm.toml`) and ends with the command to install the result. It also writes `.ipm/build.stamp`, which lets `ipm install` skip a build that is already current.

## 4. Use it in Claude Code

```bash
ipm install claude my-pack
```

`ipm install claude` builds the pack when `.ipm/` is out of date, then runs `claude plugin marketplace add` and `claude plugin install` for this project (`--scope user` or `local` choose another scope; `--dry-run` prints the commands without running them). It records the install in `ipm.toml`, so a bare `ipm install` later reinstalls everything recorded. The skill is available as `my-pack:test-driven-development`.

To wire it yourself instead: `claude plugin marketplace add ./.ipm`, then in Claude Code `/plugin install my-pack@<project folder>-ipm`.

{: .note }
Adding a marketplace changes your Claude Code configuration. To try it without that, point `CLAUDE_CONFIG_DIR` at an empty folder first: `CLAUDE_CONFIG_DIR=$(mktemp -d) claude plugin marketplace add ./.ipm`.

## Next

- [Concepts](concepts.md): sources, the catalog, IDs, packs and scopes.
- [Build your own plugin](guides/build-a-plugin.md): a full walkthrough with items from several repos.
- [Share packs with a team](guides/teams.md): what to commit.
