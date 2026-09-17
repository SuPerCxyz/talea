## 1. Model and index persistence

- [x] 1.1 Add `Session.ResumeLaunchArgs` as a separate field from generic adapter resume arguments.
- [x] 1.2 Bump Talea index schema and add `resume_launch_args_json` with an empty-array default for legacy rows.
- [x] 1.3 Persist and query launch arguments through all session upsert/search paths, decoding invalid or missing JSON conservatively.
- [x] 1.4 Add migration, round-trip, legacy-default, and search-loading tests for the new field.

## 2. Agent-specific extraction and command construction

- [x] 2.1 Parse Claude Code `permission-mode` records, whitelist known modes, and append validated modes before `--resume <id>`.
- [x] 2.2 Parse Codex `turn_context` policy records, map full and partial policies to current CLI options, and place global options before `resume <id>`.
- [x] 2.3 Add adapter-level validation/fallback coverage so unsupported or unknown fields never add arguments and future adapters can opt in without core Agent branching.
- [x] 2.4 Keep OpenCode and generic adapters on their existing commands when no verified launch-argument source is available.

## 3. Restore flow and user-visible behavior

- [x] 3.1 Ensure TUI Enter, `talea go`, and `--dry-run` all consume the persisted launch arguments through the shared resume command path.
- [x] 3.2 Add command-construction tests for Codex ordering, Claude permission modes, legacy sessions, invalid persisted arguments, and OpenCode fallback.
- [x] 3.3 Verify parameter arrays remain shell-free and that privileged arguments are only emitted from explicit structured records.

## 4. Documentation and verification

- [x] 4.1 Update Codex/Claude format and resume documentation with normalized argument behavior and unknown-field limitations.
- [x] 4.2 Run gofmt, targeted adapter/index/resume/CLI tests, `go vet`, `go test ./...`, available lint checks, and linux/amd64 plus linux/arm64 builds.
