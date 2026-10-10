# ipm TUI — design

Status: approved in conversation 2026-10-09; for review as a document · part 4 of [the ipm design](2026-10-09-ipm-design.md)

## Intent

`ipm` already finds artifacts (`ipm search`), explains them (`ipm show`, `ipm tree`), collects them (`ipm pack add`) and builds them (`ipm install`), but going from an idea to a built pack takes several commands and copied IDs. The TUI is for that curation loop — find → judge → collect → build — in one screen: the user's own words were "logging artifacts as I search for a potential new bespoke plugin".

### Success criteria

- From `ipm tui`, a user can go from a query ("c# testing") to a built, validated pack without leaving the TUI or copying an ID.
- Every TUI action is also possible from the CLI, and both share the same logic: the TUI adds no behaviour of its own to packs, search ranking or install.
- The TUI works without the search server (keyword search over the catalog) and says so.
- `mise run cli:test` covers the TUI model, its rendering and its adapters without a real terminal, server or pnpm.

### Scope

In: search, a detail pane with walkable relationships, adding and removing items in packs, switching and creating packs, running install with live output, project or user scope.

Out (stay in the CLI): sources (`ipm source`), `ipm sync`, `ipm stats`, starting or loading the search server, editing pack descriptions, deleting packs, clipboard integration.

## Decisions

| Decision | Choice | Why |
| --- | --- | --- |
| Framework | Bubble Tea v2 (`charm.land/bubbletea/v2`), Bubbles (`charm.land/bubbles/v2`: text input, viewport), Lip Gloss (`charm.land/lipgloss/v2`) | The standard Go TUI stack; the model/update/view split keeps state testable. |
| Command | `ipm tui [-g]`; bare `ipm` keeps printing help | Scripts and habits that run `ipm` stay unchanged. |
| Search backend | Typesense when the server is up (the same hybrid ranking as `ipm search`); otherwise keyword matching over the in-memory catalog, with a banner | Best ranking when available, and a usable first run without Docker. |
| Layout | Results left, detail right, the active pack as a tray along the bottom | The user watches the pack grow while searching. |
| Structure | `internal/tui` depends only on four interfaces; `internal/cli` implements them over existing packages | Testable with fakes; no duplicated logic; no parsing of CLI text. |

## Architecture

```text
cli/internal/tui/
  deps.go      the interfaces and their data types
  model.go     root model: focus, layout, key and message routing, status bar
  results.go   result list
  detail.go    detail pane and its navigation history
  tray.go      active pack tray
  picker.go    modal: switch pack, or create one
  install.go   modal: streamed install output
  keys.go      key map and the ? help overlay
  style.go     Lip Gloss styles in ipm's palette (docs/assets/brand)
cli/internal/cli/
  tui.go       `ipm tui` (Cobra), and the adapters below
```

### Interfaces (`deps.go`)

```go
type Filter struct{ Type string }           // "" is every type

type Hit struct {
	ID, Type, Name, Repo, Description string
	Stars                             int
	Plugins                           []string
	Score                             string // as ipm search prints it; "" in local mode
}

type Results struct {
	Hits  []Hit
	Found int
}

type Searcher interface {
	Search(ctx context.Context, query string, f Filter) (Results, error)
	Mode() string // "hybrid" or "local"
}

type Neighbour struct{ ID, Type, Name string }

type Detail struct {
	ID, Type, Name, Repo, Path, Commit, Description string
	Stars                                          int
	Up, Down                                       []Neighbour // what contains it; what it contains
}

type Catalog interface {
	Detail(id string) (Detail, error)
}

type Pack struct {
	Name, Description string
	Items             []string // catalog IDs, in order
}

type Packs interface {
	Scope() string // "project /path" or "global"
	List() ([]Pack, error)
	Create(name, description string) error
	Add(pack string, ids ...string) error    // refusals carry the CLI's message
	Remove(pack string, ids ...string) error
}

type Installer interface {
	Install(ctx context.Context, out io.Writer) error
}
```

`tui.New(Deps{Searcher, Catalog, Packs, Installer}) tea.Model` is the package's entry point.

### Adapters (`internal/cli/tui.go`)

- **Searcher:** at start, the Typesense client's `Health` and `NumDocuments` pick hybrid mode when the server answers and its index is not empty (the existing `search.Query` and client, `Limit` 50, and the SCORE that `search.Table` prints, exported from the search package) or local mode otherwise. Local mode scores catalog records by query terms in name (weight 4), description and tags (1 each), ties by stars, and returns the top 50 with the total count. `ctrl+r` repeats the health check.
- **Catalog:** `catgraph.Load` once; `Detail` maps a node and its `Children(id, up)` to `Detail`.
- **Packs:** the logic of `pack new`, `pack add` and `pack remove` moves from printing code in `packcmd.go` into functions that return results and errors; the CLI commands print those results, and the adapter calls the same functions. Adding keeps the existing rules (`pickable`): hooks, MCP servers and inline commands are refused with a message naming the owning plugin.
- **Installer:** calls `runInstall` with a `System` whose `Stdout` is the writer the TUI streams from.
- **Command:** `ipm tui` fails with "ipm tui needs a terminal" (exit 1) when stdout is not a terminal (`System.TermWidth` returns 0). With no catalog it fails with "no catalog — run `ipm sync`".

## Interaction

```text
┌ ipm ─ search: c# testing▌──────────────────────────── ● hybrid ─┐
│ 71 agent    csharp-pro         │ agent  csharp-pro              │
│ 65 copilot  csharp-dotnet-dev… │ wshobson/agents ★ 40.3k        │
│▶24 skill  ✓ dotnet-backend-pa… │ agent:wshobson/agents:plugins/…│
│ 15 agent    backend-developme… │ Write modern C# code with …    │
│                                │ up    plugin  jvm-languages    │
│ 50 of 100                      │       repo    wshobson/agents  │
├─ project · csharp-sdlc (3) • not built ─────────────────────────┤
│ skill dotnet-backend-patterns · agent csharp-pro · plugin stripe│
└ / search  a add  d remove  p packs  i install  f type  ? help ──┘
```

| Key | Does |
| --- | --- |
| typing (search focused) | updates the query; results refresh about 150 ms after the last keystroke |
| `/` | focus the search box |
| `↓`, `tab` from search | focus the results |
| `↑` `↓`, `j` `k` | move through results; the detail pane follows |
| `→`, `l` | focus the detail pane's neighbours |
| `enter` (detail) | open the selected neighbour; `backspace` goes back |
| `enter` (results, narrow terminal) | show the detail full-width; `esc` returns |
| `a` | add the selected result (or neighbour) to the active pack |
| `d` | remove the selected item from the active pack (results, detail or tray) |
| `tab` (results) | focus the tray; `tab` again returns to search |
| `p` | pack picker: `↑` `↓` and `enter` to switch, `n` for a new pack (name, then description) |
| `i` | run install in a modal that streams output; `esc` closes it once finished |
| `f` | cycle the type filter: all, skill, agent, command, plugin |
| `ctrl+r` | re-check the search server |
| `?` | help overlay listing every key |
| `q`, `ctrl+c` | quit (not while install runs) |

- Results mark items already in the active pack with `✓`.
- The header shows `● hybrid` or `○ local`; in local mode a banner reads "server off: keyword search only — run `ipm serve` and `ipm load` for semantic search" (or "search index empty — run `ipm load`" when the server is up but empty).
- The tray header shows the scope, pack name and item count, and `• not built` once the pack changed in this session and install hasn't run since.
- With no packs, `a` opens the picker straight on "new pack".
- The active pack at start is the first pack by name; the session remembers the last switch only while it runs.

## Data flow

- **Search:** each keystroke stores the query and schedules a `searchTick{seq}` after 150 ms; only a tick whose `seq` matches the latest starts a search, as a `tea.Cmd` with a context; its `searchDone{seq, results, err}` is dropped if a newer query exists. An empty query clears the results.
- **Detail:** selecting a result or neighbour calls `Catalog.Detail` synchronously (in-memory); the detail pane keeps a back stack.
- **Packs:** `Add`, `Remove` and `Create` write `ipm.toml` immediately, then the tray reloads with `List`. `.ipm/` is unchanged until install.
- **Install:** `i` starts `Installer.Install` in a command goroutine writing to a pipe; a reader sends each line as an `installLine` message; `installDone{err}` ends it. The modal shows the lines in a viewport that follows the end, then the result (`✓ validate: ok` or the error).

## Errors and edge cases

- Every error is shown in a one-line status bar (cleared by the next action); the TUI never exits on an action's error.
- Pick refusals and unknown IDs show the CLI's message unchanged.
- Width below 100 columns hides the detail pane; below 60×15 the screen asks the user to enlarge the terminal.
- `q` and `ctrl+c` are ignored while install runs, with a status message saying why.
- A search error in hybrid mode (other than the server being down) is shown and the previous results stay.

## Testing

- **Model** (`internal/tui`): drive `Update` with key and message values against fake `Searcher`, `Catalog`, `Packs` and `Installer`; assert state and calls — debounce drops stale results, `a` calls `Add` with the selected ID, a refusal reaches the status bar, the picker creates and switches packs, `• not built` appears and clears, `q` is ignored during install, the narrow layout hides the detail pane.
- **Rendering:** golden tests of `View()` at fixed sizes with colour off: main layout, narrow layout, picker, install modal, local banner, help overlay.
- **Adapters** (`internal/cli`): local search ranking, the pack functions shared with the CLI (existing pack tests keep passing), the installer streaming `runInstall` output, `ipm tui` failing without a terminal and without a catalog.
- **By hand:** in a real terminal, build the C# pack from [Build your own plugin](../../guides/build-a-plugin.md) entirely in the TUI.

## Docs

A "Use the TUI" guide under `docs/guides/`, `ipm tui` in the commands reference, a line in the README and the home page, and the new messages in troubleshooting.
