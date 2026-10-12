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
| `ipm is built with Go — install it from https://go.dev/dl, then rerun this script` | `install.sh` needs Go. Install it and run the script again. |
| `! … is not on your PATH — add this line to …` | The folder `install.sh` installed into (`~/.local/bin`, or `$IPM_BIN_DIR`) isn't on your `PATH`. Add the `export PATH=…` line it prints to that file, then open a new shell. |
| `! another ipm comes first on your PATH: …` | An older `ipm` (often from `go install`, in `$(go env GOPATH)/bin`) runs instead of the one just installed. Delete it, or move `~/.local/bin` earlier in your `PATH`. `command -v ipm` shows which one runs. |
| `ipm: command not found` after `go install` | Add `$(go env GOPATH)/bin` to your `PATH`. |

## Onboard and uninstall

| Message | Cause and fix |
| --- | --- |
| `onboard: ipm data exists — pass --keep to use it or --fresh to back it up` | Not in a terminal, so it can't ask. In a script, pass `--keep` to use your data or `--fresh` to back it up first. |
| `onboard: pick sources with --lists <name> or --yes when not in a terminal` | The checklist needs a terminal. Pass `--lists "<name>"` (repeatable) or `--yes` for the pre-ticked lists, for example `ipm onboard --keep --yes`. |
| `no list named "…"; the lists are: …` | `--lists` takes a list's exact name, including capitals and punctuation. Copy one from the names it prints. |
| `! offline: using the copy of … saved earlier` | The curated list couldn't be fetched, so the copy from your last onboard is used. It may be out of date; rerun when you're online for the latest. |
| `can't read the curated list at … — pass a local copy with --list <path>` | Offline, with no cached copy. Download the list elsewhere and pass the file with `--list`. |
| `skipped N rows without a valid owner/repo` | Some `[[repos]]` tables in the curated list lack a `repo` of the form `owner/repo`, or a `list`. The rest load; fix the list if it's yours. |
| `- stats skipped: gh is not installed`, `… gh is not logged in (gh auth login)` | Onboard carries on without GitHub stats. Install or log in to `gh`, then run `ipm stats` and `ipm load`. |
| `- search skipped: start Docker, then ipm serve && ipm load` | Docker isn't running. Start it and run those two commands. |
| `✗ <step>` then `rerun: <command>` | A step failed; the output above, which is everything the step printed, says why. Fix it and run the command shown. Running `ipm onboard` again also works: sources you already follow are kept. |
| `can't back up …: … — nothing was changed` | The home or cache folder couldn't be renamed. Check its permissions; both folders are as they were. |
| `… is a development build in a git checkout — rerun with --force to remove it` | You ran `ipm uninstall` with `bin/ipm` or a binary built inside an `ipm` checkout. Uninstall the installed one by running it by path (`~/.local/bin/ipm uninstall`), or pass `--force` to delete the development build. |
| `uninstall: --purge needs --yes when not in a terminal` | It can't ask you to type the home path. Pass `--yes` if you mean it. |
| `nothing deleted` | What you typed wasn't the home path exactly. Nothing was removed; run it again. |

## Sources and sync

| Message | Cause and fix |
| --- | --- |
| `pnpm failed — the catalog is unchanged` | pnpm's output above says why: often the network, or a ref that doesn't exist. Fix it and rerun `ipm sync`. |
| `no sources yet — add some with ipm source add owner/repo` | Add a source first. |
| `sources are not synced yet`, `… has no pinned commit in pnpm-lock.yaml`, `… is not installed` | Run `ipm sync`. |
| `… is already a source` | It's listed already; `ipm source list` shows it. |
| `- not grouped: no copy of the source packs yet — ipm onboard fetches one` | `ipm source list` groups by the curated list, and it has no saved copy and couldn't fetch one. Your sources are all listed under `Sources`. Run it again when you are online. |
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
| `the search index is from an older ipm — run ipm load` | The index was built before this version of `ipm`. Run `ipm load` to rebuild it. |
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

## TUI

| Message | Cause and fix |
| --- | --- |
| `ipm tui needs a terminal` | Its output is piped or redirected. Run it directly in a terminal. |
| `no catalog — run ipm sync` | Run `ipm sync` to build the catalog first. |
| `server off: keyword search only …` (banner) | The search server isn't running. Results still come from the catalog by keyword; for semantic search run `ipm serve` and `ipm load`, then `ctrl+r`. |
| `search index empty — run ipm load` (banner) | The server is running with an empty index. Run `ipm load`, then `ctrl+r`. |
| `ipm needs at least 60×15 — enlarge the terminal` | Make the terminal bigger, or use the CLI commands. |
| Letters go into the search box instead of acting | Keys like `a`, `p` and `i` act outside the search box: press `↓` or `tab` first. |
| `install is running — wait for it to finish`, `deploy is running — …` | Build and deploy can't be stopped halfway; the modal closes with `esc` when it's done. |
| `no pack to deploy — press p to create one` | Create a pack (`p`, then `n`) and add to it first. |

## Deploy

| Message | Cause and fix |
| --- | --- |
| `ipm deploy needs claude (Claude Code) on PATH` | Install Claude Code, or put `claude` on your `PATH`. |
| `Claude Code already has a marketplace named … from …` | Another project whose folder has the same name already deployed a marketplace with this name. Deploying would repoint it, and its plugins, here. Rename one of the folders, or remove the other with the `claude plugin marketplace remove` command shown. |
| `… is not a folder` | The project or local folder doesn't exist. Check the path; `~` and relative paths work. |
| `deploy: name a pack — this scope has N` | Give the pack: `ipm deploy <pack>`. `ipm pack list` shows them. |
| `can't read the output of claude plugin list --json` | Your `claude` prints a format `ipm` doesn't know. Update Claude Code, or install by hand with the commands from `ipm install`. |
| `` `claude plugin …` failed `` | Claude Code's own output is shown above it. Fix what it says and deploy again; nothing is retried. |
| A rebuilt pack doesn't change in Claude Code | Claude Code reads it from `.ipm/plugins/`; start a new session or run `/reload-plugins`. |

## Claude Code

| Symptom | Cause and fix |
| --- | --- |
| `claude plugin marketplace add .ipm` is rejected | Use a path Claude Code reads as local: `./.ipm`, or an absolute path. |
| A picked item doesn't appear after a rebuild | Run `/plugin marketplace update <marketplace>` in Claude Code, then reinstall the pack. |
| You want to try a build without changing your Claude config | Use a throwaway config: `CLAUDE_CONFIG_DIR=$(mktemp -d) claude …`. |
