<div align="center">

<img src="assets/talea-logo.png" alt="Talea logo" width="140"/>

# Talea

**Trace the session. Resume the work.**

Local-first session index, token analysis and resume launcher for AI coding agents.

[English](README.md) · [简体中文](README.zh-CN.md)

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/github/license/SuPerCxyz/talea)](LICENSE)
[![Release](https://img.shields.io/github/v/release/SuPerCxyz/talea)](https://github.com/SuPerCxyz/talea/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/SuPerCxyz/talea/ci.yml?branch=master)](https://github.com/SuPerCxyz/talea/actions)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-8892b0)](https://github.com/SuPerCxyz/talea/releases)

</div>

---

Talea indexes the session history of every AI coding agent on your machine so you can
find, inspect and **resume** past work without remembering which agent you used, the
session ID, the working directory, or the exact resume command. It never touches your
agents' data — it reads them **read-only** and keeps everything **local**.

## Features

- 🔎 **Cross-agent full-text search** — SQLite FTS5 with Chinese (trigram) support.
- ▶️ **One-command resume** — a full or partial session ID is matched automatically;
  resumes in the agent's own TUI via the native `claude --resume`, `codex resume` or
  `opencode -s` command.
- 📊 **Token analysis** — per-request token data, model summary, context window curve,
  terminal charts (visible in the TUI detail page).
- 🖥️ **Interactive TUI** — session list with an aggregated detail page, context curve,
  token charts and a `t` key to expand user turns.
- 🌐 **i18n** — English by default, Chinese automatically when your locale starts with `zh`.
- 🔌 **Extensible** — add agents via `internal/adapters/<name>` packages or ship an
  external `talea-adapter-<name>` executable (JSON Lines over stdio, any language).
- 🔒 **Private & safe** — agents' data opened read-only, index file permission `0600`,
  no telemetry, no network access.
- 🚀 **Fast** — incremental indexing with offset-resume; 10k sessions re-index in ~0.8s.

## Supported Agents

| Agent | Data source | Resume command |
|-------|------------|----------------|
| Claude Code | `~/.claude/projects/<enc-cwd>/<sessionId>.jsonl` | `claude --resume <id>` |
| Codex CLI | `~/.codex/sessions/<Y>/<M>/<D>/rollout-*.jsonl` | `codex resume <id>` |
| OpenCode | `~/.local/share/opencode/opencode.db` (SQLite) | `opencode -s <id>` |
| Any | external `talea-adapter-<name>` plugin | — |

## Installation

### Prebuilt binaries

Download the latest archive for your platform from the
[Releases page](https://github.com/SuPerCxyz/talea/releases):

| Platform | Archive |
|----------|---------|
| Linux amd64 | `talea_*_linux_amd64.tar.gz` |
| Linux arm64 | `talea_*_linux_arm64.tar.gz` |
| macOS amd64 | `talea_*_darwin_amd64.tar.gz` |
| macOS arm64 (Apple Silicon) | `talea_*_darwin_arm64.tar.gz` |
| Windows amd64 | `talea_*_windows_amd64.zip` |
| Windows arm64 | `talea_*_windows_arm64.zip` |

**Linux / macOS:**

```sh
tar xzf talea_*.tar.gz
sudo install -m 0755 talea /usr/local/bin/talea
talea version
```

**Windows:**

Extract `talea.exe` from the zip, then add the folder to your `PATH` or move it
to a directory already in `PATH`. Run `talea version` in a terminal to verify.

### Build from source

Requires Go 1.25+ (no cgo, no external dependencies):

```sh
git clone https://github.com/SuPerCxyz/talea.git
cd talea
make build
sudo install -m 0755 bin/talea /usr/local/bin/talea   # Linux/macOS
```

## Quick Start

```sh
# Build the index of all detected agents
talea index

# Open the TUI
talea

# List recent sessions
talea list

# Resume a session by full or partial ID (dry run to preview the command)
talea go <session-id> --dry-run
talea go <session-id>

# Interactively pick a session, restricted to a directory (exact, no subdirs)
talea go --path /home/user/myproject
```

## TUI

```text
talea          # session list, newest end time first (most recent 500)
talea --path /path            # restrict the list to sessions in /path (exact dir, no subdirs)
talea --agent opencode        # restrict the list to sessions from one agent
talea --path /p --agent codex # combine both filters
```

- Sessions are always sorted by end time (newest first), independent of
  `default_sort` in the config.
- `↑` / `↓` select, `Enter` resume, `d` open details, `o` resume from details,
  `/` filter (type then `Enter` to apply, then `Enter`/`o` to enter), `q` quit.
- The detail page aggregates: session info (two-column layout), first question,
  user turns (`t` to expand/collapse), a **context window curve** (area chart with a
  token y-axis and time axis), per-model summary, token chart, token summary and
  sub-agent sessions.
- New sessions appear automatically — the TUI re-indexes in the background.

## CLI Reference

| Command | Description |
|---------|-------------|
| `talea` | Open the TUI (`--path` restricts to an exact directory, no subdirs; `--agent` filters by agent; both accept `./`, `../` etc. and can be combined) |
| `talea list` | List sessions (`--agent/--path/--project/--branch/--today/--active/--sort/--limit/--format`) |
| `talea go [session-id]` | Resume a session by full/prefix ID, or pick interactively (`--path/--target/--dry-run`) |
| `talea index` | Incremental index (`--rebuild/--metadata-only`) |
| `talea doctor` | Environment diagnostics (`--json/--agent`) |
| `talea web` | Local read-only web view on localhost (`--port`) |
| `talea export` / `talea import` | Offline multi-device transfer (JSON) |
| `talea config` | `config path|init|validate` |

Output formats: `--format table|json|jsonl|csv|markdown`.

Exact / estimated / unknown token values are kept strictly distinct — missing data shows
`unknown`, never `0`. Timeline events are deduplicated by `source_identity`; sub-agent
tokens are stored separately and never merged into the parent session by default.

## i18n

English is the default. When `LANG` / `LC_ALL` / `LC_MESSAGES` / `LANGUAGE` starts with
`zh`, Talea switches to Chinese across the TUI, CLI output, command help and error
messages.

## Configuration

`talea config init` creates `~/.config/talea/config.toml`. Highlights:

```toml
[general]
default_sort = "last_activity"

[path_mapping]               # remap moved/renamed directories
"/old/project" = "/new/project"
```

## Data & Privacy

- Agents' original session files and databases are opened **read-only** and never
  modified, renamed or deleted.
- Index: `${XDG_DATA_HOME:-~/.local/share}/talea/index.db`, file permission `0600`,
  directory `0700`.
- No uploads, no telemetry, no online update checks. All network features are explicitly
  opt-in (e.g. `talea web` binds to localhost only).

## Extending Talea

- **In-tree**: add an `internal/adapters/<name>/` package and register it in
  `internal/adapters/registry.go`. No `switch agent` in core code.
- **Out-of-tree**: drop an executable implementing the `talea-adapter-<name>` protocol
  (JSON Lines over stdio, methods `info/detect/discover/parse/messages/usage/timeline`)
  into your `PATH`. See `docs/adapters/`.

## Roadmap

See [`docs/plan/03-enhancement-roadmap.md`](docs/plan/03-enhancement-roadmap.md) for the
planned `talea stats` global reporting, session forking, web dashboard enhancements and
more.

## FAQ

**Is my data sent anywhere?** No. Everything runs locally and read-only.

**Do I need Python/Node/a browser?** No — a single Go binary, zero external runtime deps.

**Can it resume sessions from a moved directory?** Yes. Use `talea go <id> --target <dir>`
or configure `[path_mapping]`; `talea` prompts for a new directory when the original is gone.

## Development

```sh
make build    # build bin/talea
make test     # go test ./...
make lint     # golangci-lint
make vet      # go vet ./...
```

Contributions are welcome — open an issue or a pull request.

## License

[MIT](LICENSE) © 2026 superc
