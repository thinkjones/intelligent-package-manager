# Source packs design

**Date:** 2026-10-11

## What was asked

A manually run CI task that parses [thinkjones/agentic-tools's `starred_repos.toml`](https://github.com/thinkjones/agentic-tools/blob/main/starred_repos.toml) and scans for new repos to add to the source packs, so they become part of users' search. Grouping of source repos, using that list to group, showing topics, homepage, repo, description and stars.

## What existed

`ipm onboard` already read that file as its default curated list and grouped repos by its `list` field, showing stars, language, last push and description. `ipm source list` was a flat table. Nothing showed topics or homepage. "Source packs" was not a concept, and a GitHub Actions job cannot reach a user's sources, which live in `IPM_HOME`. The list holds 344 repos in 24 lists, many with nothing `ipm` can catalog.

## Decisions (the user's)

1. A source pack is a named list in a file committed to this repo, `source-packs.toml`. The workflow proposes additions by pull request.
2. A repo qualifies only if it holds something `ipm` catalogs, found by fetching it.
3. The grouped view appears in `ipm source list`, the `ipm onboard` checklist, and the workflow's summary and pull request body.

## Design

**File.** `source-packs.toml` uses the curated-list format, so `ipm onboard` reads it unchanged. Two optional fields are added to the format: `homepage`, and `artifacts`, a table of counts by kind. `source-packs.skipped.txt` lists repos found to hold nothing, so they aren't fetched again.

**Scan.** `cli/internal/sourcepacks` downloads each unseen repo's default-branch archive, writes its files empty (only names matter), and runs `catalog.Walk`, the catalog's own marker search. This is not `ipm scan`'s folder report, which counts any `hooks/` folder: a web app's `src/hooks` must not qualify. A repo that fails to download is recorded nowhere and retried next run. If every download fails, nothing is written.

**Update.** Existing packs keep their order and repos. Repos still in the starred list take its current stars, description, topics and homepage. A repo removed from the starred list stays. A repo already in one pack joins another without a new scan.

**Task and workflow.** `mise run packs:update` runs `cli/cmd/sourcepacks`, a separate program so the installed `ipm` gains no maintainer command. `.github/workflows/source-packs.yml` is manual only: it runs the task, writes the report to the run summary, and opens a draft pull request from the branch `source-packs/update`. It is a draft because a pull request opened with the workflow's token starts no CI run; marking it ready does.

**Views.** `ipm source list` groups sources by the default curated list (the first list for a repo in several; `Other` for the rest), with stars, description, topics and homepage from the list, falling back to `ipm stats`. It uses the saved copy of the list and fetches only when that is missing or over a week old. The onboard checklist shows the highlighted repo's topics and homepage on the line above the key hints, so rows stay one line.

**Order of delivery.** First the tool, workflow and views, with `ipm onboard`'s default list unchanged. Then a workflow run produces `source-packs.toml` through its own pull request. Only then does the default list change to it, so the default never points at a file that doesn't exist.

## Not done

- No removal of repos that leave the starred list or lose their artifacts.
- No scheduled runs.
- `ipm source list` groups by the default list even when `ipm onboard` was given another with `--list`.
