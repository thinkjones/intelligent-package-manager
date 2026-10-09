---
title: Getting started
nav_order: 2
---

# Getting started
{: .no_toc }

Install `ipm`, follow a couple of repos, search them and build your first plugin. It takes about ten minutes, most of it waiting for the first fetch.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Requirements

| You need | For | Notes |
| --- | --- | --- |
| [Go](https://go.dev/dl/) 1.26 or later | Building `ipm` | |
| [pnpm](https://pnpm.io/installation) 12 or later, **or** [Node.js](https://nodejs.org) | `ipm sync`, `ipm install` | With only Node, `ipm` runs `npx -y pnpm@12.10.1` for you. |
| [Docker](https://www.docker.com) (or OrbStack) | `ipm serve`, `ipm search` | Runs the local Typesense search server. Everything else works without it. |
| [GitHub CLI](https://cli.github.com) `gh`, logged in | `ipm stats` | Optional: stars, topics and descriptions. |
| [jq](https://jqlang.org) | Listing IDs from `ipm search --json` | Optional. |
| [Claude Code](https://code.claude.com) `claude` | Validating and using the plugins you build | Optional for `ipm install`, which skips validation without it. |

`ipm` is tested on macOS and Linux. Windows paths are defined but untested.

## Install

### With `go install`

```bash
go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest
ipm version
```

This puts `ipm` in `$(go env GOPATH)/bin`; make sure that folder is on your `PATH`.

### From a clone

If you want to work on `ipm`, or track `main` closely:

```bash
git clone https://github.com/thinkjones/intelligent-package-manager.git
cd intelligent-package-manager
direnv allow        # puts bin/ on PATH; bin/ipm rebuilds the CLI whenever its source changes
```

Without [direnv](https://direnv.net), build once with `cd cli && go build -o bin/ipm ./cmd/ipm` and put `cli/bin` on your `PATH`.

## Check your setup

```bash
ipm doctor
```

`ipm doctor` prints one row each for its home and cache folders, pnpm, your sources, the catalog, Docker, the search server and `claude`. Rows marked `!` are warnings (for example, no sources yet); `✗` is a problem to fix, and makes it exit 1.

`ipm` keeps its data in your user folders, never in the directory you run it from. See [Files and paths](reference/files.md) to move them.

## 1. Follow some repos

```bash
ipm source add obra/superpowers anthropics/skills
ipm sync
```

`ipm source add` records the repos; `ipm sync` has pnpm fetch each one, pin it to a commit and build the catalog. The first sync downloads every repo; later ones only fetch what changed.

## 2. Search them

```bash
ipm serve           # start Typesense in Docker, once per boot
ipm load            # index the catalog; rerun after each sync
ipm search tdd
```

Each result shows a SCORE, the artifact's type, name, repo, stars, the plugins it belongs to and its description. To refer to an artifact you need its **ID**, for example `skill:obra/superpowers:skills/test-driven-development`. The table doesn't show IDs; `--json` does:

```bash
ipm search tdd --type skill --json | jq -r '.document.id'
ipm show 'skill:obra/superpowers:skills/test-driven-development'
ipm tree 'repo:obra/superpowers'
```

## 3. Build a plugin

In any project folder:

```bash
ipm pack new my-pack --description "My picks"
ipm pack add my-pack 'skill:obra/superpowers:skills/test-driven-development'
ipm install
```

`ipm install` writes `ipm.toml` and `.ipm/` in the project and ends with the commands to install the result.

## 4. Use it in Claude Code

```bash
claude plugin marketplace add ./.ipm
```

Then, in Claude Code, run `/plugin install my-pack@<project folder>-ipm`. The skill is available as `my-pack:test-driven-development`.

{: .note }
Adding a marketplace changes your Claude Code configuration. To try it without that, point `CLAUDE_CONFIG_DIR` at an empty folder first: `CLAUDE_CONFIG_DIR=$(mktemp -d) claude plugin marketplace add ./.ipm`.

## Next

- [Concepts](concepts.md): sources, the catalog, IDs, packs and scopes.
- [Build your own plugin](guides/build-a-plugin.md): a full walkthrough with items from several repos.
- [Share packs with a team](guides/teams.md): what to commit.
