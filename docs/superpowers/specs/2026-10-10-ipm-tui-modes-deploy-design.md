# ipm TUI modes and deploy — design

Status: approved in conversation 2026-10-10; for review as a document · revises [the TUI design](2026-10-09-ipm-tui-design.md)

## Intent

Two changes from using the TUI. First, judging an artifact means understanding where it sits — its plugin, marketplace and repo, its siblings and what it contains — so the TUI gets a tree that is always visible and navigable, in two modes: **search mode** (find things, see their place) and **tree mode** (walk the tree). Second, a pack built in the TUI can't be used until it is installed in Claude Code, so the TUI and the CLI get **deploy**: install the built pack into Claude Code at user, project or local scope.

Deploy changes a stated principle: until now `ipm` never touched the user's Claude Code configuration. From here, `ipm` installs into Claude Code only when the user deploys, explicitly, after seeing the exact commands.

### Success criteria

- Moving through search results always shows the selected artifact in its tree: the path to its root, its siblings and its children, each with a description, in aligned columns.
- `←` `→` move up and down the tree from search mode; tree mode adds `↑` `↓` across siblings; `tab` or `esc` returns to search.
- A pack can be built (`b`) and deployed (`D`) from the TUI, and deployed from the CLI (`ipm deploy`), to user, project or local scope.
- Tests never run the real `claude` or touch the user's Claude configuration; the by-hand check uses a throwaway `CLAUDE_CONFIG_DIR`.

## Layout

```text
ipm  repo › claude-code-workflows › systems-programming › golang-pro    ● hybrid
┌ search: cli ─────────────────────────────────────────────────────────────────┐
│  88  skill    cli                        vinnie357/claude-skills       ★26   │
│▶ 71  agent    golang-pro                 wshobson/agents               ★40.3k│
│  52  plugin   shell-scripting            wshobson/agents               ★40.3k│
└──────────────────────────────────────────────────────────────────────────────┘
┌ tree ── agent:wshobson/agents:plugins/systems-programming/agents/golang-pro.md ┐
│ repo         wshobson/agents          Production-ready subagents and plugins…│
│ └ market     claude-code-workflows    Orchestrated workflows for Claude Code…│
│   └ plugin   systems-programming      Systems programming with Rust, Go, C…  │
│     ├ agent  c-pro                    Write efficient C code with proper…    │
│     ├ agent ▶golang-pro ────────────▶ Master Go 1.21+ with modern patterns,  │
│     │                                 advanced concurrency, and generics.    │
│     ├ agent  rust-pro                 Master Rust 1.75+ with modern async…   │
│     └ skill  go-concurrency-patterns  Production patterns for Go concurren…  │
└──────────────────────────────────────────────────────────────────────────────┘
 go-cli-dev (6) • not built · project /work
 ↑↓ results  ←→ tree  tab tree mode  a add  b build  D deploy  ? help
```

- **Header:** `ipm`, a breadcrumb of the tree cursor's path (types or names, cut from the left when long), and `● hybrid` / `○ local`. The local-mode banner stays as before.
- **Results panel** (titled with the search box) and **tree panel** (titled with the cursor's ID), bordered. Heights by weight: results 40%, tree 60% of the space left after the header, tray, status and hints. The panel with focus has an amber border and title; the other is dim.
- **Results rows:** score, type, name, repo, stars, `✓` when in the active pack; cut, never wrapped.
- **Tray, status and hints** as before (pack first, then scope), hints changing with mode.
- Text in panels is always cut explicitly with `…` to the panel's inner width (panel width − 4); it never wraps, except the cursor's description, which wraps to at most 3 lines inside its column.
- Under 24 rows the tree shows the path and siblings without children; under 60×15 the screen asks for a bigger terminal. Under 100 columns repo and stars drop from result rows.

## The tree

The tree is centred on a **cursor** node:

1. **Path:** the cursor's ancestors from its root (a repo) down to its parent, one row each.
2. **Siblings:** the parent's children with the cursor among them, marked `▶`. A root (a repo) has no parent, so it is shown alone, with its children below.
3. **Children:** the cursor's children, one level, indented under it, so `→` always has a visible target.
4. **Also in:** when the cursor has more than one parent, a row `also in: <type> <name>, …` under it.

Rows: tree guides (`└ ├ │`), a fixed-width type label (6 characters: `repo`, `market`, `plugin`, `copilot`, `skill`, `agent`, `cmd`, `hook`, `mcp`), the name cut to 24 characters, then the description in one column that starts at the same position on every row. The cursor row draws `▶<name> ───▶` across to its description. Sibling and child lists longer than the space scroll to keep the cursor in view, with `↑ N more` / `↓ N more` rows.

**Parents:** the path follows the first parent (`catgraph` order), unless the user has chosen another for that node in this session (see tree mode).

**Colour by type**, the same in results, tree and breadcrumb: repo blue `#7AA2F7`, marketplace purple `#BB9AF7`, plugin and copilot plugin amber `#F0A847`, skill teal `#5CC8A8`, agent pink `#F7768E`, command green `#9ECE6A`, hook and MCP server grey `#8B95A3`. Stars are tinted by magnitude (under 100 muted, under 10k soft, 10k and over bright). With `NO_COLOR` set, colour is off and everything else is unchanged.

## Modes and keys

**Search mode** (the default; focus in the search box or the results):

| Key | Does |
| --- | --- |
| typing (search box) | search, 150 ms after the last keystroke |
| `↓`, `tab` (search box) | focus the results |
| `/` | focus the search box |
| `↑` `↓`, `j` `k` | move through results; the tree's cursor resets to the selected result |
| `←`, `h` | move the tree's cursor to its parent |
| `→`, `l` | move the tree's cursor to its first child |
| `tab`, `enter` (results) | tree mode |
| `a` / `d` | add / remove the selected result |
| `f` | cycle the type filter |
| `p` | pack picker |
| `b` (or `i`) | build every pack |
| `D` | deploy the active pack |
| `ctrl+r`, `?`, `q`, `ctrl+c` | as before |

**Tree mode** (focus in the tree):

| Key | Does |
| --- | --- |
| `↑` `↓`, `k` `j` | move between siblings (and onto `also in` entries) |
| `←` `h` / `→` `l` | parent / first child |
| `enter` | on an `also in` entry: follow that parent for this node |
| `a` / `d` | add / remove the cursor node |
| `p`, `b`, `D`, `?` | as in search mode |
| `tab`, `esc` | search mode; the tree recentres on the selected result |
| `q`, `ctrl+c` | quit |

The help overlay lists both tables. Tree mode replaces the detail pane's neighbour list and back stack from part 4.

## Build and deploy

- **Build** (`b`, alias `i`) is `ipm install`, as before.
- **Deploy** (`D`) opens a modal:
  1. **Target:** `user`, `project` or `local` (`↑` `↓`, `enter`). The default is `project` with the pack's project root as path; a `-g` scope defaults to `user`.
  2. **Path** (project and local only): a text input prefilled with the default; must be an existing directory.
  3. **Confirm:** the exact commands, then `y` deploys, `esc` cancels:
     ```text
     Deploy go-cli-dev to Claude Code — project scope in /work
       ipm install
       claude plugin marketplace add /work/.ipm --scope project
       claude plugin install go-cli-dev@work-ipm --scope project
     y deploy · esc cancel
     ```
  4. **Run:** output streams into the install modal's view; it ends `✓ deployed go-cli-dev (project /work)` or `✗ <error>`.
- Deploy always builds first (build is deterministic, so this costs a pnpm run and guarantees Claude Code gets the current pack). The marketplace name comes from the built `.ipm/.claude-plugin/marketplace.json`.
- **Re-deploy:** when Claude Code already knows the marketplace, deploy updates it instead of adding it; when the plugin is already installed at that scope, deploy updates it instead of installing. The exact commands for each case are fixed by the plan's spike against a throwaway `CLAUDE_CONFIG_DIR` (`claude` 2.1.296), and the confirm step shows whichever will run.
- Project and local scope run `claude` with the chosen path as its working directory.

### `ipm deploy`

```text
ipm deploy [pack] [--scope user|project|local] [--path DIR] [-g]
```

Builds, then deploys `pack` (default: the only pack in the scope; with several, the pack is required). It prints each command before running it and needs no confirmation: typing the command is the decision. Missing `claude` is an error naming it.

## Architecture

`internal/tui` keeps its rule: it depends only on its interfaces.

```go
type Ref struct{ ID, Type, Name, Description string }

type Node struct {
	ID, Type, Name, Repo, Path, Description string
	Stars                                   int
	Parents, Children                       []Ref
}

type Catalog interface {
	Node(id string) (Node, error) // replaces Detail
}

type Target struct {
	Scope string // "user", "project" or "local"
	Path  string // project and local only
}

type Deployer interface {
	Default(pack string) Target
	Plan(pack string, t Target) ([]string, error) // the commands deploy will run, for the confirm step
	Deploy(ctx context.Context, pack string, t Target, out io.Writer) error
}
```

`Deps` gains `Deployer`. New files in `internal/tui`: `tree.go` (the cursor, path and window computation, and rendering), `deploy.go` (the modal); `detail.go`'s neighbour walking is removed. `internal/cli` gets `deploy.go` (`ipm deploy`, the `claude` command plan and runner, shared by the TUI adapter) and `tuiCatalog.Node`. `claude` runs through `System.Output`, so tests fake it.

## Errors and edge cases

- Deploy without `claude` on PATH: the confirm step says so and offers nothing to run; `ipm deploy` fails naming `claude`.
- A failing `claude` command stops the deploy, shows its output and `✗`; nothing is retried.
- A path that isn't a directory is refused at the path step.
- A node whose catalog record is missing (stale cache): the tree shows what it has and the status bar the error.
- Cycles cannot occur in the catalog graph's parent links; the path walk still stops after 10 levels.

## Testing

- **Tree** (`internal/tui`): window computation for path, siblings and children, scrolling with `↑ N more` / `↓ N more`, the aligned description column at depths 0–4, the cursor arrow, `also in` and switching parents, name cutting at 24, `←` at a root and `→` at a leaf doing nothing.
- **Modes:** search-mode `←` `→` move the tree without changing the result; moving the result resets the cursor; tree-mode `↑` `↓` across siblings; `tab` / `esc` return and recentre; `a` in tree mode adds the cursor node.
- **Deploy modal:** default target per scope, the path step refusing a non-directory, the confirm step showing `Plan`'s commands, `y` streaming output and `✓`, a failure showing `✗`, `esc` at each step cancelling.
- **Rendering:** goldens for search mode, tree mode, a deep tree with scrolling, `also in`, the deploy confirm step, and `NO_COLOR`.
- **CLI:** `ipm deploy` builds then runs the planned `claude` commands with the right working directory and scopes (fake `System`); re-deploy commands; missing `claude`; pack required with several packs.
- **By hand:** in a real terminal, walk a plugin's tree from a search result, then build and deploy a pack to project scope with `CLAUDE_CONFIG_DIR` pointing at a temporary directory, and check `claude plugin list` there shows it.

## Docs

The TUI guide (modes, tree, keys, build and deploy), the commands reference (`ipm deploy`), troubleshooting (deploy errors), the README and home page (the principle now reads "installs into Claude Code only when you deploy"), the architecture page, AGENTS.md, CLAUDE.md and cli/README.md.
