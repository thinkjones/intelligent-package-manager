# User guide

How to install the marketplace, curate packs, and keep them current. For the exact manifest format see the [manifest reference](manifest-reference.md); for how it works inside see the [architecture](architecture.md).

## Concepts in one minute

- A **pack** is a folder at the repo root holding exactly one file, `manifest.toml`. It names upstream plugins and skills; it never holds content of its own.
- Every pack is also a **stack**: a plugin named after the folder that depends on everything the pack lists. Installing it installs the lot.
- **`ipm`** builds the marketplace. `ipm sync` pins every upstream's version to a commit in `aihs.lock`; `ipm create-plugin` writes `.claude-plugin/marketplace.json`.
- Nothing upstream is copied: Claude Code fetches every plugin from its repo, at the locked commit, when it is installed.

## Set up

You need Go, git, [direnv](https://direnv.net) and Claude Code.

```bash
cd ai-harness-skills
direnv allow          # once: .envrc puts bin/ on PATH
ipm sync             # fetch every upstream into .aihs/cache/ at its locked commit
ipm doctor           # checks config, every manifest, the lock and the marketplace
```

`bin/ipm` builds the CLI on first use and whenever its Go source changes. `ipm` only exists inside this checkout; it installs and configures nothing globally.

## Install the marketplace in Claude Code

```bash
claude plugin marketplace add /path/to/ai-harness-skills
claude plugin install code-core@ai-harness-skills-marketplace    # a whole stack
claude plugin install pdf@ai-harness-skills-marketplace          # or a single plugin
```

Inside a session, `/plugin` browses the marketplace. After `ipm create-plugin`, start a new session or run `/reload-plugins` to pick up changes. To remove everything: `claude plugin marketplace remove ai-harness-skills-marketplace`.

## The everyday loop

```bash
$EDITOR <pack>/manifest.toml   # declare what you want
ipm sync                      # resolve new versions, pin them, cache them
ipm create-plugin --dry-run   # see what would be published
ipm create-plugin             # write and validate the marketplace
ipm doctor
```

Commit `manifest.toml`, `aihs.lock` and `.claude-plugin/marketplace.json` together.

## Add content to a pack

### An upstream Claude Code plugin

Use the name from the plugin's own `.claude-plugin/plugin.json`, so its skills keep their prefix. It is published as it is — skills, agents, commands, hooks and MCP servers alike:

```toml
[[plugin]]
name = "pr-review-toolkit"
repo = "anthropics/claude-plugins-official"
path = "plugins/pr-review-toolkit"   # omit to use the whole repo
description = "Comprehensive PR review"
```

### Skill folders from any repo

Point `path` at the folder holding the skills and list the ones you want; `"./"` means every skill folder in `path`:

```toml
[[plugin]]
name = "arc"
repo = "vinnie357/claude-skills"
path = "plugins/core/skills"
skills = ["./twelve-factor", "./mise", "./code-review"]
```

When a repo is itself a plugin that bundles more than you want, pointing `path` at a skills folder publishes only your selection.

### A GitHub Copilot plugin

```toml
[[copilot-plugin]]            # an awesome-copilot plugin (agent-plugins.org plugin.json)
repo = "github/awesome-copilot"
path = "plugins/arize-ax"
```

Its skills are published from upstream as one plugin named for the Copilot plugin (`arize-ax`); set `name` to override. Its agents, hooks and MCP servers are not published, and `create-plugin` says so. A Copilot plugin with no skills is an error, because it would publish nothing. Copilot agents and instructions on their own cannot be published either; record them in the manifest's "Not published" comment instead.

### Build a stack on another stack

```toml
[pack]
description = "My stack"
includes = ["user-core"]
```

Installing this pack then installs `user-core` too. Includes must name packs being published and must not form a cycle.

## Pin a version

Every `[[plugin]]` and `[[copilot-plugin]]` takes an optional `version`, as in npm:

```toml
[[plugin]]
name = "superpowers"
repo = "obra/superpowers"
version = "v5.0.0"     # a tag, a branch, or a 40-character commit
```

Without `version`, the entry follows the upstream's default branch. Either way `ipm sync` records the resolved commit in `aihs.lock`, and the marketplace publishes exactly that commit.

## Keep upstreams current

```bash
ipm sync --upgrade      # re-resolve every version; branches move, tags and commits stay put
ipm create-plugin       # republish at the new commits
```

`ipm sync` prints each move as `old → new` and removes cached commits nothing is locked to.

## Create a new pack

```bash
mkdir my-stack
cat > my-stack/manifest.toml <<'EOF'
[pack]
description = "My stack"

[[plugin]]
name = "superpowers"
repo = "obra/superpowers"
EOF
ipm sync && ipm create-plugin
claude plugin install my-stack@ai-harness-skills-marketplace
```

The folder name is the stack's plugin name, so it must be a valid plugin name and must not start with `claude-` or `anthropic-`. The folder must hold nothing but `manifest.toml`.

## Troubleshooting

| Message | Cause and fix |
| --- | --- |
| `IPM_ROOT is not set` | You are running the binary directly or outside the checkout. Run `direnv allow` in the checkout and use `ipm`. |
| `a pack holds only manifest.toml — <x> does not belong here` | Remove the file or folder; reference the content upstream instead. |
| `aihs.lock does not pin <repo> — run ipm sync` | A manifest names an upstream the lock has not seen. Run `ipm sync`. |
| `the cache does not hold <repo> at its locked commit … — run ipm sync` | The cache was removed or never filled, for example on a fresh clone. Run `ipm sync`. |
| `<repo> has no version "<version>"` | The tag or branch does not exist upstream. |
| `plugin "<name>" is defined differently in <pack> and <pack>` | Two packs list the same name with different `repo`, `path`, `skills`, `version` or dependencies. Make them identical or rename one. |
| `name "<name>" is reserved by Claude Code for Anthropic's own plugins` | Pick another name; the plugin's skills keep their upstream prefix regardless. |
| `<pack> includes "<x>", which is not a pack being published` | The include is misspelled, the pack publishes nothing, or `--pack` left it out. |
| `publishes nothing: its plugin.json lists no skills` | The Copilot plugin has only agents (or hooks). Remove it and record it as not published. |
| `this is a Claude Code plugin — list it as a [[plugin]]` | A `[[copilot-plugin]]` points at a Claude Code plugin. |
| `… not found upstream` | The item moved or was removed upstream at the locked commit. Fix `path`, or remove the entry and record it as not published. |
| `.claude-plugin/marketplace.json exists and was not written by ipm` | Someone hand-wrote the file. Pass `--force` to replace it. |
| `claude plugin validate failed` | Read the listed errors; most are naming rules. `ipm doctor` catches the common ones first. |

Packs that publish nothing (no `[[plugin]]`, no `[[copilot-plugin]]`, no includes) are left out of the marketplace and reported as warnings by `ipm doctor`.
