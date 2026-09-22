## 1. OpenCode v2 storage discovery

- [x] 1.1 Add schema-flavor detection via `sqlite_master` (presence of `session_v2`) with legacy fallback when absent or unreadable.
- [x] 1.2 Make full discovery use `session_v2` plus legacy-only `session` rows with `UNION` de-duplication, preserving `(id)` uniqueness.
- [x] 1.3 Apply the same union shape to the incremental high-watermark query so the existing cursor and overlap semantics stay unchanged.
- [x] 1.4 Add focused tests for: v2-only table, legacy-only table, both tables with a legacy-only session (no duplicates, no loss), and unreadable flavor detection fallback.
- [x] 1.5 Carry the detected storage shape in the incremental cursor and, when the shape differs from the one the cursor was built with (including legacy cursors without a shape field), ignore the old high-watermark and fall back to one full discovery, so sessions written only to the new table with a `time_updated` older than the existing watermark are never skipped.

## 2. OpenCode v2 session metadata

- [x] 2.1 Resolve session metadata from `session_v2` first, falling back to `session` when the row is absent.
- [x] 2.2 Accept nullable `title` and other v2 columns without scan errors; keep unknown Token values unknown rather than 0.
- [x] 2.3 Add focused tests covering v2 nullable `title`, v2 row present alongside legacy row, and legacy-only row fallback.

## 3. OpenCode v2 message and timeline paths

- [x] 3.1 Route message reading by data presence: sessions with rows in legacy `message` keep the `message`/`part` path; otherwise use `session_message`.
- [x] 3.2 Implement v2 first-question extraction from the earliest `type=user` `session_message` `data.text`, with the existing injected-content filtering.
- [x] 3.3 Implement v2 message preview loading from `session_message` (`user.data.text`, `assistant.data.content[]` text blocks).
- [x] 3.4 Implement v2 timeline events: user messages, `assistant.data.content[]` tool start/end, `type=compaction` events, and per-message token request events using `assistant.data.tokens` as increments with `assistant.data.model.id` as model.
- [x] 3.5 Leave `ContextAfter`/`CumulativeTotal` unset for v2 events (no `step-finish` snapshot) and use v2-specific `SourceIdentity` prefixes.
- [x] 3.6 Add focused tests for v2 first question, preview, tool/compaction/token events, increment aggregation, unknown context snapshot, and legacy sessions still producing identical events.

## 4. TUI background sync status

- [x] 4.1 Render the background sync status as a bordered, high-contrast status block showing the real sync stage, spinner, and current-stage copy.
- [x] 4.2 Account for the status block's real line count in list height so the list never overflows.
- [x] 4.3 Show a completion notice with the real session-count delta after sync finishes, then auto-hide it; show a zero-change variant when nothing changed.
- [x] 4.4 Keep the existing failure status visible and distinct from the in-progress state.
- [x] 4.5 Add focused tests for stage display, line accounting, completion notice with delta, zero-change notice, auto-hide, and failure rendering.

## 5. Doctor storage report

- [x] 5.1 Report the detected OpenCode storage flavor (legacy `session` / v2 `session_v2`) and the discovered session count.
- [x] 5.2 Add focused tests for both flavors and for an unreadable database.

## 6. Documentation and verification

- [x] 6.1 Update `docs/formats/opencode.md` to the 2.0.12 measured schema, table routing, token semantics, and verification status.
- [x] 6.2 Update README environment facts for OpenCode v2 storage shape.
- [x] 6.3 Run `gofmt`, targeted package tests, `go vet`, `go test ./...`, and `golangci-lint`. (`gofmt`/`go vet`/`go test ./...` exit 0; `golangci-lint` v2.6.2 installed on authorization and reported `0 issues`, exit 0.)
- [x] 6.4 Verify `talea list` shows the previously missing OpenCode sessions and that `talea doctor` reports the storage flavor.

## 7. TUI session list

- [x] 7.1 Honor `general.include_subagents` in the TUI session list at the query layer, defaulting to hidden so the TUI matches `talea list`, while leaving all existing callers unchanged.
- [x] 7.2 Remove the hardcoded 500-session TUI truncation so every visible session is browsable via list pagination, and measure that full loading stays within noise of the previous capped load without introducing a new config option.
- [x] 7.3 Add focused tests for both subagent configurations, absence of truncation past the former limit, query-layer filtering, and the preserved `Limit` semantics.
