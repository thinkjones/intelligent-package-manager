# ipm external references — design

Status: approved in conversation 2026-10-10; for review as a document · builds on [TUI modes and deploy](2026-10-10-ipm-tui-modes-deploy-design.md)

## Intent

A marketplace can list a plugin that lives in another repo: `anthropics/claude-plugins-official`'s marketplace lists `cloudflare`, whose source is `cloudflare/skills`. Today the catalog graph links that entry to the `cloudflare/skills` repo as if the repo were its child, so the repo gains a parent and the tree stacks two repo hierarchies: a skill in `cloudflare/skills` shows `anthropics/claude-plugins-official › claude-plugins-official › cloudflare › cloudflare/skills › cloudflare › cloudflare › agents-sdk`. 265 marketplace entries in the user's catalog point at another repo this way.

A repo is the root of everything it holds. A reference from another repo is information about an artifact — and a strong sign that its repo is useful — not part of its ancestry.

### Success criteria

- Every artifact's tree is rooted at the repo it lives in; repos have no parents.
- External entries are marked as external and lead to what they reference; the way back is visible in the tree.
- How many other repos reference an artifact's repo is visible in search results and the tree, and `e` lists those references.
- A query matching a repo name or a plugin name ranks those artifacts higher; equal relevance orders by references, then stars.

## The graph (`internal/catgraph`)

- **No edge from an external entry to its source repo.** The `source_repo` link in `link()` becomes a reference instead.
- **References, both ways:**
  - `RefTarget(id) (string, bool)` — what an external entry references: the plugin in the source repo whose folder matches the entry's source path (`git-subdir` `path`), else the source repo's plugin with the entry's name, else the source repo's only plugin, else the source repo itself.
  - `RefsTo(id) []string` — the external entries whose target is `id`, sorted by ID.
  - `UsedBy(repo string) []string` — the distinct other repos whose external entries target anything in `repo`, sorted.
- **Repos are roots:** `Children(repoID, up=true)` is always empty.
- `ipm show` prints a `used by:` section (the referencing entries, one per line, with their repo) after `up:` and `down:`; `ipm tree --up` no longer climbs through external entries into other repos; `ipm tree` from an external entry shows `↗ <target>` as its only child line, without expanding it.

## Data to the TUI

`tui.Node` gains:

```go
External *Ref // for an external entry: what it references
UsedBy   []Ref // external entries that reference this node (Ref.Repo is the entry's repo)
UsedByRepos int // distinct other repos referencing this node's repo
```

`tui.Ref` gains `Repo string` (the repo it lives in) and `External bool` (an external entry), so tree rows and the `e` list can be drawn without loading every node. `tui.Hit` gains `UsedBy int` (the repo-level count) and `External bool`.

## The tree

- External entry rows: the name is prefixed `↗` and drawn dim; the description column starts with the target in the target's type colour, `↗ cloudflare/skills — Skills for the Cloudflare…`.
- Rows for nodes with references end with `↗ used by N` (N = repos) in the muted colour when space allows.
- **Jumping:** `→` (or `enter` in tree mode) on an external entry moves the cursor to its target and re-roots the tree at the target's repo, pushing the entry onto a **back stack**.
- **The back root:** while the back stack is not empty, the tree's top row is a second root, `↩ <repo of the last entry> — back to <entry name>`, above the current repo root. In tree mode it is selectable with `↑` from the root row; `enter` or `→` on it pops the stack and puts the cursor back on that entry (re-rooting at its repo). Moving the search selection clears the back stack.
- The breadcrumb shows the current repo's path only.

## `e`: external references

`e` (search or tree mode) opens a modal listing every external reference to the cursor's repo, grouped by referencing repo:

```text
External references to cloudflare/skills — used by 3 repos
▶ anthropics/claude-plugins-official ★37.6k   cloudflare   (marketplace claude-plugins-official)
  someone/plugins ★120                        cf-skills    (marketplace plugins)
  …
enter jump · esc close
```

`enter` jumps to that entry in its own repo's tree (pushing the current cursor onto the back stack); with no references the modal says `no other repo references cloudflare/skills`.

## Results

Result rows gain a `↗N` cell before stars (wide terminals only, blank when 0); external entries show `↗` before their name.

## Search

- **Schema:** `repo` becomes searchable (it is already indexed); new fields `used_by` (int32, default 0) and `external` (bool); `token_separators: ["/", "-", "_"]` so `cloudflare` matches `cloudflare/skills` and `tdd` matches `tdd-guard`.
- **Weights:** `query_by` = `name,repo,plugin_names,description,tags` with weights `4,3,3,2,2` (plus `embedding` 0 in hybrid).
- **Sort:** `_text_match:desc,used_by:desc,stars:desc`.
- **Documents:** `used_by` is the document's repo's `UsedBy` count; an external entry's `used_by` is its target repo's count; `external` marks external entries.
- **Old index:** a search error naming a missing `used_by` field (an index built by an older `ipm`) becomes `the search index is from an older ipm — run ipm load`, in `ipm search` and the TUI's status bar.
- **Local search** (TUI without the server): weights name 4, repo 3, plugin names 3, description 2, tags 2 per matching query word; ties by used-by count, then stars, then ID.

## Errors and edge cases

- An external entry whose source repo is not among the user's sources has no target: its row says `↗ <source repo> (not a source — ipm source add <id>)`, and `→` does nothing but show that in the status bar.
- A repo referencing itself (an entry whose source is its own repo) is not external.
- The back stack holds at most 20 entries; older ones drop.
- Reference cycles (A lists B, B lists A) are fine: jumps are explicit, and the tree never follows references on its own.

## Testing

- **catgraph:** an entry with a git-subdir path resolves to the matching plugin; by name; to the only plugin; to the repo; repos have no parents; `RefsTo` and `UsedBy` (distinct repos, self-references excluded); `ipm show` used-by section; `ipm tree --up` stays in the repo.
- **tree/TUI:** external row marking and target text; `→` on an external entry re-roots and shows the back root; `↑` onto the back root and `enter` returns; moving the result clears it; unresolved external entry message; `e` modal listing and jumping, empty state; result `↗N` cells; goldens for an external entry, the back root, and the `e` modal.
- **search:** schema fields and token separators; `query_by` and weights; sort; documents' `used_by` and `external`; the old-index error message; local-search weights and tie-break.
- **By hand:** the screenshot's case — `cloudflare/skills`' agents-sdk roots at `cloudflare/skills` with `↗ used by N`; from `anthropics/claude-plugins-official`'s marketplace, `→` on `cloudflare` jumps in and the back root returns; `ipm search cloudflare` puts `cloudflare/skills` artifacts first after `ipm load`.

## Docs

The TUI guide (external entries, jumping and the back root, `e`, the used-by column), the commands reference (`ipm show`'s used-by section), troubleshooting (the old-index message), the architecture page (references in the graph, the search weights).
