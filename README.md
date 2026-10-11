<p align="center">
  <img src="./docs/assets/brand/hero.svg" width="100%" alt="ipm, the intelligent package manager for Claude Code: four items picked from two GitHub repos, each pinned to a commit, built by ipm install into one validated Claude Code plugin.">
</p>

<p align="center">
  <a href="https://thinkjones.github.io/intelligent-package-manager/">Documentation</a> ·
  <a href="https://thinkjones.github.io/intelligent-package-manager/getting-started.html">Getting started</a> ·
  <a href="#install">Install</a> ·
  <a href="LICENSE">MIT</a>
</p>

Good agent artifacts are spread across hundreds of repos. Installing a whole plugin to get one skill drags in agents, hooks and MCP servers you didn't want, under names you don't control, at whatever commit is current today. `ipm` lets you follow the repos, search everything in them, and build exactly the parts you want into a plugin of your own.

## See it

Search every repo you follow, with keyword and semantic matching, on your machine:

```text
$ ipm search "c# testing" --limit 3
SCORE  TYPE            NAME                       REPO                    ★      PLUGINS
71     agent           csharp-pro                 wshobson/agents         40.3k  jvm-languages
       agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md
65     copilot-plugin  csharp-dotnet-development  github/awesome-copilot  39.9k  —
       copilot-plugin:github/awesome-copilot:plugins/csharp-dotnet-development
24     skill           dotnet-backend-patterns    wshobson/agents         40.3k  dotnet-contribution
       skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns
```

Each hit's ID, under it, is what `ipm show`, `ipm tree` and `ipm pack add` take.

Pick what you want into a pack, and build it:

```text
$ ipm pack add csharp-sdlc \
    skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns \
    agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md \
    command:wshobson/agents:plugins/accessibility-compliance/commands/accessibility-audit.md \
    'plugin:anthropics/claude-plugins-official#stripe'
$ ipm install
✓ csharp-sdlc: 3 item(s), 1 plugin dependency(ies)
validate: ok

Install in Claude Code:
  claude plugin marketplace add ./.ipm
  /plugin install csharp-sdlc@my-project-ipm
```

In Claude Code the skill, agent and command load as `csharp-sdlc:<name>`, and the Stripe plugin comes along as a pinned dependency.

## Why it is different

- **Pinned and reproducible.** Every picked item is locked to a commit in `.ipm/ipm.lock`. Commit it, and a teammate's `ipm install` builds the same plugin from the same commits, without your catalog.
- **pnpm does the fetching.** Your repos are git dependencies of one pnpm project, so fetching, pinning, a content-addressable store and cheap upgrades come from pnpm.
- **No upstream code runs.** pnpm always runs with `--ignore-scripts`, repos' own npm dependencies are never installed, and a teammate's `ipm.toml` or lock can't make `ipm` read or write outside its folders.
- **Your namespace.** Picked items load under your pack's name. Whole plugins keep theirs and come with their hooks and MCP servers.
- **Relationships, not just files.** The catalog knows which plugin owns which skill and which marketplaces list which plugins, so `ipm tree` and `ipm graph` show where anything comes from.

## How it works

<p align="center">
  <img src="./docs/assets/brand/workflow.svg" width="100%" alt="How ipm works: source add lists the repos you follow; sync has pnpm fetch and pin each repo and ipm catalogs it; search finds artifacts with keyword and semantic search on your machine; pack add picks skills, agents, commands and plugins; install builds one validated plugin per pack.">
</p>

1. **`ipm source add`** records the repos you follow.
2. **`ipm sync`** has pnpm fetch each repo and pin it to a commit, then catalogs every repo, marketplace, plugin, skill, agent, command, hook and MCP server in them.
3. **`ipm search`** runs hybrid keyword and semantic search over the catalog on a local Typesense server.
4. **`ipm pack add`** picks skills, agents, commands and whole plugins into a pack in your project's `ipm.toml`.
5. **`ipm install`** builds each pack into a Claude Code plugin and marketplace under `.ipm/`, and validates it. **`ipm deploy`** installs it into Claude Code at user, project or local scope, when you ask.

## Install

You need Go 1.26+, pnpm 12+ or Node (`ipm` then runs pnpm 12 through `npx`), and Docker for search.

```bash
curl -fsSL https://raw.githubusercontent.com/thinkjones/intelligent-package-manager/main/install.sh | bash
```

The script installs `ipm` into `~/.local/bin` and starts `ipm onboard`, which lets you tick repos from a curated list, then fetches, catalogs and indexes them for search. Prefer to do it yourself? `go install github.com/thinkjones/intelligent-package-manager/cli/cmd/ipm@latest`, then follow the steps below.

## Build your first plugin

Skip the first three lines if you ran `ipm onboard`.

```bash
ipm source add obra/superpowers anthropics/skills
ipm sync                                  # fetch, pin and catalog
ipm serve && ipm load                     # start local search and index the catalog
ipm search tdd --type skill              # each hit shows its ID

# in your project
ipm pack new my-pack --description "My picks"
ipm pack add my-pack 'skill:obra/superpowers:skills/test-driven-development'
ipm install
ipm deploy my-pack                        # into Claude Code, for this project
```

The [build your own plugin](https://thinkjones.github.io/intelligent-package-manager/guides/build-a-plugin.html) guide walks through a pack built from several repos. Prefer keys to IDs? [`ipm tui`](https://thinkjones.github.io/intelligent-package-manager/guides/tui.html) does the same search, collect and build in one screen.

## Commands

| Command | Does |
| --- | --- |
| `ipm onboard` | Pick repos from a curated list and build a searchable library |
| `ipm source add\|remove\|list` | Edit the repos you follow |
| `ipm sync [--upgrade]` | Fetch and pin every repo with pnpm, then rebuild the catalog |
| `ipm serve`, `ipm load`, `ipm search` | Run local search, index the catalog, search it |
| `ipm show`, `ipm tree`, `ipm graph` | Inspect an artifact and its relationships |
| `ipm pack new\|add\|remove\|list\|show` | Edit packs in `ipm.toml` (`-g` for your user scope) |
| `ipm install [--update]` | Build every pack into a plugin and validate it |
| `ipm deploy [pack] [--scope] [--path]` | Build, then install a pack into Claude Code |
| `ipm tui [-g]` | Search, see the tree, collect, build and deploy, in one screen |
| `ipm doctor` | Check everything `ipm` depends on |
| `ipm uninstall [--purge]` | Remove `ipm` and its search container (`--purge` also deletes your data) |

Every flag is in the [commands reference](https://thinkjones.github.io/intelligent-package-manager/reference/commands.html).

## Limits

- **`ipm` installs into Claude Code only when you deploy.** Building never touches your Claude Code configuration.
- **Hooks and MCP servers come only with their whole plugin.** They (and inline commands) can't be picked on their own.
- **Non-GitHub sources may lose files**, because pnpm applies npm's packing rules to them.
- **No packaged release yet**, and Windows is untested.

## Documentation

The [documentation site](https://thinkjones.github.io/intelligent-package-manager/) is built from [`docs/`](docs/): [getting started](docs/getting-started.md), [concepts](docs/concepts.md), guides to [sources](docs/guides/sources.md), [search](docs/guides/search.md), [building a plugin](docs/guides/build-a-plugin.md) and [sharing packs](docs/guides/teams.md), the [commands](docs/reference/commands.md), [sources](docs/reference/sources.md) and [files](docs/reference/files.md) references, [troubleshooting](docs/troubleshooting.md) and [how it works](docs/architecture.md).

## Contributing

```bash
git clone https://github.com/thinkjones/intelligent-package-manager.git
cd intelligent-package-manager
./install.sh          # builds this checkout into ~/.local/bin/ipm
bin/ipm doctor        # bin/ipm runs a development build, rebuilt when its source changes
mise run cli:test     # gofmt, go vet, go test: the gate for every change
```

See [`cli/README.md`](cli/README.md) for the code layout. `ipm` was previously called `aihs`; design history is in [`docs/superpowers/`](docs/superpowers/).

## License

[MIT](LICENSE) · Maintained by Dr Gene Conroy-Jones.
