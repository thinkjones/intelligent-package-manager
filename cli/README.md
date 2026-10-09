# ipm

The Go CLI that builds this repo's Claude Code marketplace. It is local-only: every command is Go code in this module, it installs and configures nothing outside the checkout, and it is separate from fsh and Gas City. For using it see the [user guide](../docs/user-guide.md); for the design see the [architecture](../docs/architecture.md).

## Usage

```text
ipm sync [--upgrade]           resolve every upstream's version, pin it in aihs.lock, cache it
ipm create-plugin [flags]      write .claude-plugin/marketplace.json pinned to the lock, then validate it
ipm doctor                     check config, every manifest, the lock and the marketplace
ipm index                      write catalog/*.jsonl from the repos of interest at their locked commits
ipm stats                      fetch GitHub stats for every catalog repo into catalog/github.jsonl (needs gh)
ipm promote <repo|plugin-id>   add repos, or an external plugin's source repo, to the root manifest.toml
ipm serve [--stop]             run the local Typesense search server in Docker (data in .aihs/typesense/)
ipm load                       rebuild the search index from catalog/
ipm search <query> [flags]     hybrid search with a SCORE column: --type, --repo, --tag (repeatable), --keyword,
                                --limit, --page N, --all, --explain (KW/SEM columns), --json
ipm show <id> [--json]         an artifact's fields, GitHub stats and neighbours
ipm tree <id> [--up]           relationships as a tree (--depth N, --all)
ipm graph <id> [--format dot]  the neighbourhood as Mermaid (default) or Graphviz
ipm scan [--paths] [repo...]   cache repos (default: the root manifest.toml) and report their plugin, skills, hooks and agents folders
ipm version
ipm help [command]             usage, or one command's flags
```

`create-plugin` flags: `--pack NAME` (repeatable; default every pack), `--dry-run`, `--force` (overwrite a marketplace ipm didn't write), `--no-validate`, `--name`, `--description`, `--owner`, `--repo-url`.

Exit codes: `0` success; `1` failure (bad config or manifest, unpinned or failed upstream, refused overwrite, failed validation, doctor problems); `2` malformed command line.

## Setup

```bash
direnv allow     # once: .envrc puts the repo's bin/ on PATH
ipm doctor
```

`bin/ipm` is a shell shim: it builds `cli/bin/ipm` on first use and whenever a Go source in `cli/` changes, sets `IPM_ROOT` to the checkout root, and execs the binary. `IPM_ROOT` is the only configuration ipm reads, and only the shim sets it, so outside the checkout `ipm` is not on PATH.

## Layout

```text
cmd/ipm/main.go           wires the process into cli.Run: argv, real git and claude, exit codes
internal/cli/              the commands table, help, and one file per command
internal/config/           IPM_ROOT, validated
internal/pack/             manifest.toml parsing and validation; Problems; Reserved names
internal/upstream/         versions, aihs.lock, the commit-keyed cache, Sync, Checkout
internal/copilot/          Copilot plugin.json → upstream skill references (nothing copied)
internal/marketplace/      packs → marketplace.json entries pinned to the lock; atomic Write; IsOurs
```

Commands reach the outside world only through `cli.System` — stdout, stderr, environment, `LookPath`, and an `Output` runner used for git and claude — so tests drive them in-process with a fake.

## Adding a command

Add `internal/cli/<name>.go` with a `<name>Flags()` function returning the command's `flag.FlagSet` and its run function, and a row in the `commands` table in `cli.go`. Write its tests in `internal/cli` against the fake `System`.

## Testing

```bash
mise run cli:test      # gofmt check, go vet, go test ./... — the gate for changes here
mise run cli:build     # cli/bin/ipm (bin/ipm also does this on demand)
```

- `internal/pack`, `internal/marketplace` and `internal/copilot` test the manifest format, the manifest-only rule, conflict rules, JSON shape and Copilot expansion against fixtures.
- `internal/upstream` runs real `git` against local repositories served over `file://`.
- `internal/cli` drives every command in-process with a fake `System`; no real git or claude.
- `cmd/ipm` builds the real binary and runs `sync` then `create-plugin` against a `file://` upstream, plus exit codes.
