# Manifest reference

Every table and key `aihs` reads, the lock it writes, and the rules it enforces. Unknown keys and tables are an error, so a typo never passes silently.

## Where manifests live

A pack is any directory at the checkout root that contains `manifest.toml` — and nothing else (an OS-generated `.DS_Store` is tolerated). The directory name is the pack's name, and the pack is published as a stack plugin of that name, so it must use only letters, digits, `.`, `_` and `-`, start with a letter or digit, and avoid the names Claude Code reserves (see [Naming rules](#naming-rules)).

## Repos of interest: the root `manifest.toml`

The `manifest.toml` at the checkout root is not a pack. It lists upstream repos worth watching, and publishes nothing. `aihs sync` pins and caches each one alongside the packs' upstreams; `aihs scan` reports the Claude Code plugins (`.claude-plugin/plugin.json`), Copilot plugins (a bare `plugin.json`), marketplaces (`.claude-plugin/marketplace.json`) and the `skills`, `hooks` and `agents` folders each holds. `aihs scan owner/repo` scans a repo that is not listed: it is cached at its remote's current commit until the next `aihs sync` prunes it. `aihs promote <owner/repo>` appends a `[[repo]]` entry, keeping the file's comments; given an external plugin's catalog ID (`plugin:<marketplace repo>#<name>`) it promotes the repo that plugin lives in. `aihs index` then catalogs every repo listed here into `catalog/*.jsonl`; the record fields are in the [catalog spec](superpowers/specs/2026-10-08-artifact-catalog-design.md#data-model).

| Key | Type | Required | Meaning |
| --- | --- | --- | --- |
| `[[repo]] repo` | string | yes | GitHub `owner/repo`, or a full git URL. Listed once. |
| `[[repo]] version` | string | no | A tag, branch or full commit, npm style. Default: the repo's default branch. |

## `[pack]`

Optional. Describes the pack's stack plugin.

| Key | Type | Meaning |
| --- | --- | --- |
| `description` | string | Shown in `/plugin`; `aihs` appends a summary such as `(9 plugins; includes user-core)`. |
| `version` | string | The stack plugin's version label. |
| `includes` | array of pack names | Packs this stack builds on. Installing this pack installs them. |

## `[[plugin]]`

An upstream Claude Code plugin, or a selection of upstream skill folders, referenced in place.

| Key | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Plugin id. For an upstream plugin, use its own `plugin.json` name so its skills keep their prefix. |
| `repo` | string | yes | GitHub `owner/repo`, or a full git URL (`https://…`, `git@…`, `file://…`). |
| `path` | string | no | Subdirectory to fetch (a sparse checkout). Omit for the whole repo. |
| `version` | string | no | A tag, branch or full commit, npm style. Default: the repo's default branch. |
| `skills` | array of paths | no | Skill folders relative to `path`, each starting with `./`. `"./"` is `path` itself: a folder of skill folders, or one skill folder. Omit to use the upstream plugin as is. |
| `agents` | array of paths | no | Agent files relative to `path`, each starting with `./` and inside an `agents/` folder — Claude Code loads plugin agents from nowhere else. |
| `dependencies` | array of names | no | Other plugins in this marketplace this one needs. |
| `description` | string | no | Shown in `/plugin` before install. |

The published source is `git-subdir` when `path` is set, `github` for an `owner/repo` without `path`, and `url` otherwise. Its `sha` is always the locked commit, and its `ref` is the `version` when that names a branch or tag. An upstream Claude Code plugin is published as it is, including any hooks and MCP servers.

## `[[copilot-plugin]]`

A GitHub Copilot plugin — an [agent-plugins.org](https://agent-plugins.org) `plugin.json` — whose skills are published in place.

| Key | Type | Required | Meaning |
| --- | --- | --- | --- |
| `repo` | string | yes | As for `[[plugin]]`. |
| `path` | string | yes | The directory holding `plugin.json` (or `.github/plugin/plugin.json`). |
| `version` | string | no | As for `[[plugin]]`. |
| `name` | string | no | Overrides the plugin name, which defaults to `plugin.json`'s `name`. |
| `description` | string | no | Overrides the upstream description. |

The skills listed under the `com.github.awesome-copilot` extension become one `git-subdir` plugin over their common folder. Agents, hooks, MCP servers and other extensions are not published; `create-plugin` reports them. A Copilot plugin with no skills is an error. See [Copilot plugins](architecture.md#copilot-plugins) for why.

## Unpublished references

Record what cannot be published as a trailing comment, so nothing is silently lost:

```toml
# Not published — no Claude Code equivalent could be found:
#   github/awesome-copilot/agents/devils-advocate.agent.md  — Copilot agent — Claude Code loads plugin agents only from an agents/ folder inside the plugin; awesome-copilot keeps them at its repo root
#   github/awesome-copilot/instructions/rust.instructions.md  — Copilot instructions file — no Claude Code plugin component loads it as is
```

`aihs` ignores comments; this is a convention for people.

## `aihs.lock`

Written by `aihs sync`; commit it, do not edit it. Like npm's `package-lock.json`, it records each upstream's requested version and the commit it resolved to:

```toml
[[repo]]
repo = "github/awesome-copilot"
commit = "143a3d976b3c1603cc8932984d5e1f28501cb5fc"

[[repo]]
repo = "obra/superpowers"
version = "v5.0.0"
commit = "…"
```

One entry per distinct `repo` and `version` (`version` is absent for the default branch). Upstreams no manifest uses are dropped on the next sync.

## Generated files

| Path | Written by | Commit? |
| --- | --- | --- |
| `aihs.lock` | `aihs sync` | yes |
| `.aihs/cache/<owner>/<repo>/<commit>/` | `aihs sync` — a shallow checkout per locked commit | no (gitignored) |
| `.claude-plugin/marketplace.json` | `aihs create-plugin` | yes |

`aihs` writes nothing into a pack.

## Naming rules

Every plugin and pack name must:

- use only letters, digits, `.`, `_` and `-`, starting with a letter or digit;
- not start with `claude-`, `anthropic-`, `anthropics-` or `cc-plugin-`, not be `claude`, `anthropic`, `anthropics`, `claude-code` or `claude-mods`, and not put `official` beside `claude` or `anthropic` — Claude Code reserves these for Anthropic's own plugins;
- be unique across the marketplace, except that several packs may list the same plugin when every listing has the same `repo`, `path`, `version`, `skills`, `agents` and `dependencies` (descriptions may differ);
- not equal a pack name.

`aihs doctor` reports every violation; `aihs create-plugin` refuses to publish until they are fixed.
