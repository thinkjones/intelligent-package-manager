---
title: Troubleshooting
nav_order: 6
---

# Troubleshooting
{: .no_toc }

Start with `ipm doctor`: it checks everything `ipm` depends on and says what to fix. Then find your message below.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Setup

| Message | Cause and fix |
| --- | --- |
| `ipm fetches sources with pnpm 12 or later …` | Neither pnpm nor `npx` is on your `PATH`. Install pnpm 12+ or Node. |
| `ipm needs pnpm 12 or later (found …)` | Your pnpm is too old and `npx` is missing. Upgrade pnpm, or install Node so `ipm` can run pnpm 12 through `npx`. |
| `IPM_HOME must be an absolute path` | Set `IPM_HOME` (or `IPM_CACHE_DIR`) to an absolute path, or unset it. |
| `ipm: command not found` after `go install` | Add `$(go env GOPATH)/bin` to your `PATH`. |

## Sources and sync

| Message | Cause and fix |
| --- | --- |
| `pnpm failed — the catalog is unchanged` | pnpm's output above says why: often the network, or a ref that doesn't exist. Fix it and rerun `ipm sync`. |
| `no sources yet — add some with ipm source add owner/repo` | Add a source first. |
| `sources are not synced yet`, `… has no pinned commit in pnpm-lock.yaml`, `… is not installed` | Run `ipm sync`. |
| `… is already a source` | It's listed already; `ipm source list` shows it. |
| `… and … would both install as "<alias>" — remove one first` | Two repos differ only in case or punctuation. Keep one. |
| `… is not owner/repo or a git URL` | Use `owner/repo[@ref]`, or a URL starting `https://`, `ssh://`, `file://` or `git@`. |
| `no plugin plugin:… in the catalog — run ipm index` | That plugin ID isn't in your catalog. Check it with `ipm search --type plugin`. |
| `gh is not logged in` | Run `gh auth login`, then `ipm stats`. |
| A non-GitHub source is missing files | pnpm applies npm's packing rules (`files`, `.npmignore`) to non-GitHub git sources. A known limitation: use a GitHub mirror if there is one. |

## Search

| Message | Cause and fix |
| --- | --- |
| `the search server is not running — run ipm serve`, `no search server set up` | Run `ipm serve`. |
| `Docker is not running`, `ipm serve runs Typesense in Docker` | Start or install Docker (or OrbStack). |
| `the search index is empty — run ipm load` | Run `ipm load`. |
| `the search server rejected the API key` | Another Typesense is using port 8108. Check `docker ps` and stop it. |
| A result you expect is missing | Run `ipm load` after `ipm sync`; the index doesn't update itself. Try `--keyword` to see exact matches only. |

## Packs

| Message | Cause and fix |
| --- | --- |
| `no packs in … — create one with ipm pack new <name>` | There's no `ipm.toml` here or above, or it has no packs. Run `ipm pack new`, `cd` into the project, or add `-g`. |
| `no item … in the catalog — find IDs with ipm search` | The ID is mistyped, or its repo isn't one of your sources. `ipm source add` it, then `ipm sync`. |
| `… comes only with its plugin: ipm pack add …` | Hooks, MCP servers and inline commands come with their plugin. Run the command it prints. |
| `… is a repo …`, `… is a marketplace …` | Pick the items it holds; `ipm tree <id>` lists them. |
| `… can't be a pack name …` | Use letters, digits, `.`, `_` and `-`, starting with a letter or digit, and avoid `claude-…`, `anthropic-…` and `official`. |
| `pack … already exists`, `no pack …` | `ipm pack list` shows the packs in this scope. Packs in your user scope need `-g`. |

## Install

| Message | Cause and fix |
| --- | --- |
| `… is not in the catalog or ipm.lock — drop it with ipm pack remove …` | An item in `ipm.toml` is in neither, perhaps because it was deleted upstream. Remove it as shown, or run `ipm sync` if it's new upstream. |
| `can't resolve … without the catalog` | An item isn't in `.ipm/ipm.lock` and your catalog can't be read. Run `ipm sync`. |
| `… in ipm.lock: … — fix it, or run ipm install --update …` | The lock has an entry `ipm` won't trust: a path outside its repo, a commit that isn't a full SHA, or a malformed source. It was probably edited by hand or merged badly. Run `ipm install --update` to re-resolve it from your catalog, and review the diff. |
| `… pack name … must start with a letter or digit …` | A pack name in `ipm.toml` isn't valid, perhaps from a hand edit. Rename it in `ipm.toml`. |
| `… is pinned at two commits in ipm.lock — run ipm install --update` | Two items from the same repo are locked at different commits, which one build can't hold. `--update` re-pins them together. |
| `… would be published as "…", already used by …` | A whole plugin's entry name (`<pack>-<plugin>`) clashes with a pack or another plugin. Rename a pack. |
| `… is not pinned upstream …` (a warning) | An external plugin's marketplace entry doesn't name a commit, so Claude Code installs whatever it points at that day. Nothing to fix in `ipm`; pick a different plugin if you need it pinned. |
| `… is not installed in …/.ipm/node_modules` | pnpm didn't fetch a repo. Rerun `ipm install` and read pnpm's output. |
| `claude plugin validate --strict failed` | Claude Code's output is shown above it. Rerun with `--no-validate` to inspect `.ipm/` yourself. |

## Claude Code

| Symptom | Cause and fix |
| --- | --- |
| `claude plugin marketplace add .ipm` is rejected | Use a path Claude Code reads as local: `./.ipm`, or an absolute path. |
| A picked item doesn't appear after a rebuild | Run `/plugin marketplace update <marketplace>` in Claude Code, then reinstall the pack. |
| You want to try a build without changing your Claude config | Use a throwaway config: `CLAUDE_CONFIG_DIR=$(mktemp -d) claude …`. |
