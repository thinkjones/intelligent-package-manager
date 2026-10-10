---
title: Build your own plugin
parent: Guides
nav_order: 3
---

# Build your own plugin
{: .no_toc }

Pick items from several repos into a pack, build it into a Claude Code plugin, and install it.
{: .fs-6 .fw-300 }

1. TOC
{:toc}

This walkthrough builds `csharp-sdlc`, a C# pack with a skill, an agent and a command from `wshobson/agents`, plus the whole Stripe plugin from `anthropics/claude-plugins-official`. To follow it exactly, add those sources first:

```bash
ipm source add wshobson/agents anthropics/claude-plugins-official
ipm sync && ipm load
```

## 1. Find the items

```bash
ipm search c# --type skill --type agent
ipm search accessibility --type command
ipm search stripe --type plugin
```

Each hit's ID is on the line under it. Check one with `ipm show`:

```bash
ipm show 'agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md'
```

## 2. Create a pack and add items

Run these in your project. `ipm pack new` creates `ipm.toml` in the current folder, unless a folder above already has one, in which case that folder is the project.

```bash
ipm pack new csharp-sdlc --description "C# development lifecycle"
ipm pack add csharp-sdlc \
  skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns \
  agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md \
  command:wshobson/agents:plugins/accessibility-compliance/commands/accessibility-audit.md \
  'plugin:anthropics/claude-plugins-official#stripe'
ipm pack show csharp-sdlc
```

`ipm pack show` lists each item's type, name, repo, stars and ID. `ipm.toml` now holds:

```toml
[packs.csharp-sdlc]
description = "C# development lifecycle"
items = [
  "skill:wshobson/agents:plugins/dotnet-contribution/skills/dotnet-backend-patterns",
  "agent:wshobson/agents:plugins/jvm-languages/agents/csharp-pro.md",
  "command:wshobson/agents:plugins/accessibility-compliance/commands/accessibility-audit.md",
  "plugin:anthropics/claude-plugins-official#stripe",
]
```

`ipm pack add` checks every ID before writing anything: if one is wrong, nothing is added.

### What you can pick

| Pick | Becomes |
| --- | --- |
| A skill, an agent, or a command in its own file | A file in your pack's plugin, loading as `<pack>:<name>` |
| A Copilot plugin | Its skills, placed in your pack's plugin |
| A whole plugin (local, inline or external) | A dependency of your pack, keeping its own name, hooks and MCP servers |
| A hook, an MCP server or an inline command | Refused: they come only with their plugin, and `ipm pack add` prints the command that adds it |
| A repo or a marketplace | Refused: pick what it holds (`ipm tree <id>` lists it) |

### Pack names

Pack names start with a letter or digit and use letters, digits, `.`, `_` and `-`. Names Claude Code reserves for Anthropic (`claude-…`, `anthropic-…`, and `official` next to claude or anthropic) are refused.

## 3. Build it

```bash
ipm install
```

```text
✓ csharp-sdlc: 3 item(s), 1 plugin dependency(ies)
validate: ok

Install in Claude Code:
  claude plugin marketplace add /path/to/project/.ipm
  /plugin install csharp-sdlc@project-ipm
```

`ipm install`:

1. resolves every item to a repo, path and commit;
2. has pnpm fetch those commits into `.ipm/node_modules` (with `--ignore-scripts`);
3. places the picked files in `.ipm/plugins/csharp-sdlc/` (skills in `skills/`, agents in `agents/`, commands in `commands/`) with a `plugin.json` recording where each came from;
4. writes the marketplace `.ipm/.claude-plugin/marketplace.json`, named `<project folder>-ipm`, listing `csharp-sdlc` and its dependency `csharp-sdlc-stripe`;
5. records every pin in `.ipm/ipm.lock`;
6. runs `claude plugin validate --strict` on the result.

Validation is skipped, with a note, when `claude` isn't on your `PATH`, or with `--no-validate`. If anything fails, your previous build and lock are left as they were.

If an external plugin's upstream entry isn't pinned to a commit, `ipm install` warns you: Claude Code will install whatever that entry points at on the day it installs.

## 4. Install it in Claude Code

```bash
claude plugin marketplace add ./.ipm
```

Then in Claude Code:

```text
/plugin install csharp-sdlc@<project folder>-ipm
```

Claude Code installs `csharp-sdlc-stripe` with it. Your picks load under the pack's name: the skill and the command (commands show as skills) as `csharp-sdlc:<name>`, alongside the `csharp-pro` agent. The Stripe plugin keeps its own namespace.

{: .note }
Adding a marketplace changes your Claude Code configuration. To try a build first, use a throwaway config: `export CLAUDE_CONFIG_DIR=$(mktemp -d)` before running `claude`.

## Change a pack

```bash
ipm pack add csharp-sdlc '<another id>'
ipm pack remove csharp-sdlc 'plugin:anthropics/claude-plugins-official#stripe'
ipm install
```

Rerun `ipm install` after every change. Items you remove leave no files behind, and building twice gives identical output. In Claude Code, run `/plugin marketplace update <project folder>-ipm` to pick up the new build.

`ipm pack list` lists every pack in the scope with its item count and description.

## Keep it current

`ipm install` keeps every item at the commit in `.ipm/ipm.lock`, so your plugin doesn't change when your sources do. To move forward:

```bash
ipm sync --upgrade        # move your sources to their latest commits
ipm install --update      # re-pin every item to your current catalog
```

## Don't edit the built files

Files in `.ipm/plugins/` are hard links to pnpm's copies: editing one changes pnpm's copy too, and the next `ipm install` rebuilds the folder anyway. To change an item, change it upstream, or fork it into a repo of your own, add that as a source and pick from it.

## Packs for every project

Add `-g` to any `ipm pack` command or to `ipm install` to use your user scope, `$IPM_HOME/global`, instead of a project:

```bash
ipm pack new everyday -g
ipm pack add everyday '<id>' -g
ipm install -g
claude plugin marketplace add ~/Library/ipm/global/.ipm      # macOS; ~/.local/share/ipm/global/.ipm on Linux
```

Its marketplace is called `ipm-global`.
