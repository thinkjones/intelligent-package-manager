---
title: Search and explore
parent: Guides
nav_order: 2
---

# Search and explore
{: .no_toc }

Find artifacts across all your sources, see where they come from, and get their IDs.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

## Start the search server

```bash
ipm serve       # Typesense in Docker: container ipm-typesense on 127.0.0.1:8108
ipm load        # index the catalog
```

Run `ipm load` again after every `ipm sync` or `ipm stats`; it rebuilds the index from the catalog in a few seconds. `ipm serve --stop` stops the server and keeps its data.

The first `ipm load` also has Typesense download its small embedding model, which takes a minute. Embeddings are computed locally; nothing leaves your machine.

## Search

```bash
ipm search c# testing
ipm search tdd --type skill --repo obra/superpowers --limit 10
ipm search "code review" --type agent --type command
ipm search hooks --tag security
```

| Flag | Does |
| --- | --- |
| `--type T` | Only this type: `repo`, `marketplace`, `plugin`, `copilot-plugin`, `skill`, `agent`, `command`, `hook`, `mcp`. Repeatable. |
| `--repo R` | Only artifacts from this repo. Repeatable. |
| `--tag T` | Only artifacts with this tag or GitHub topic. Repeatable. |
| `--limit N` | Results per page (default 20, max 250). |
| `--page N` | Show page N. |
| `--all` | Every result, fetched 250 at a time. |
| `--keyword` | Keyword ranking only, without semantic matching. |
| `--explain` | Add KW (✓ when the keywords matched) and SEM (semantic similarity) columns. |
| `--json` | One JSON line per hit, with every field including the ID. |

Search is **hybrid**: keyword matches on name, description, tags and plugin names are fused with semantic matches, and ties go to the repo with more stars. `c#` and `c++` are searchable as words.

**SCORE** shows how each hit ranks against the best hit in the same query (100). It is not comparable between queries. SEM, with `--explain`, is comparable: it is the semantic similarity between your query and the artifact, from 0 to 1.

In hybrid mode `--all` returns the keyword matches plus about 100 nearest semantic neighbours; use `--keyword --all` for exact matches only.

## Get IDs

The results table leaves IDs out to stay readable. Use `--json`:

```bash
ipm search c# --type skill --json | jq -r '.document.id'
ipm search c# --type skill --json | jq -r '[.document.id, .document.description] | @tsv'
```

## Inspect an artifact

```bash
ipm show 'skill:obra/superpowers:skills/test-driven-development'
ipm show 'skill:obra/superpowers:skills/test-driven-development' --json
```

`ipm show` prints the artifact's fields, its repo's GitHub stats, and its neighbours: what contains it (up) and what it contains (down). `--json` prints the whole catalog record.

## See relationships

```bash
ipm tree 'repo:obra/superpowers'                     # what it contains
ipm tree 'repo:wshobson/agents' --depth 2 --all      # two levels, every child
ipm tree 'skill:obra/superpowers:skills/test-driven-development' --up   # what contains it
ipm graph 'repo:obra/superpowers'                    # Mermaid
ipm graph 'repo:obra/superpowers' --format dot | dot -Tsvg > superpowers.svg
```

Repos contain marketplaces, marketplaces list plugins, and plugins own skills, agents, commands, hooks and MCP servers. An external plugin points at the repo it lives in, so `--up` from a skill also shows the other marketplaces that list its repo.

`show`, `tree` and `graph` read the catalog directly. They don't need the search server.
