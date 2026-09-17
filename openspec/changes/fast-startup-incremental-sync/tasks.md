## 1. Local scan state and schema migration

- [x] 1.1 Define Talea-owned source scan state and OpenCode cursor models without introducing Agent-specific branching in core index code.
- [x] 1.2 Add an idempotent index schema migration for source scan state and FTS dirty tracking, preserving 0600 permissions and existing backup behavior.
- [x] 1.3 Add read-only source fingerprint helpers for database/WAL files, known session files, and parent directories, with conservative invalid/unknown results.
- [x] 1.4 Add unit tests for migration defaults, fingerprint changes, missing files, and fallback-required states.

## 2. Incremental discovery and indexing

- [x] 2.1 Add an optional incremental discovery capability with a complete-discovery fallback so Agent-specific cursor logic remains inside adapters.
- [x] 2.2 Implement OpenCode high-water discovery using `(time_updated, session_id)`, an overlap window, same-timestamp ordering, and read-only SQLite access.
- [x] 2.3 Integrate source fingerprints and optional incremental discovery into the indexer, committing scan state only after the corresponding source sync succeeds.
- [x] 2.4 Preserve unchanged-source skipping, handle new/removed sources conservatively, and keep per-Agent/per-session failures isolated.
- [x] 2.5 Batch working-directory and Git enrichment for changed sessions so repeated directories share one check before database upsert.
- [x] 2.6 Make activity refresh updates conditional on actual state changes for both inactive and possibly-active paths.

## 3. Incremental FTS and synchronization orchestration

- [x] 3.1 Mark changed session rows as FTS-dirty in the same local index transaction that upserts their metadata.
- [x] 3.2 Replace the unconditional missing-row FTS scan with dirty-row synchronization that updates changed FTS rows and clears dirty state only after commit.
- [x] 3.3 Detect an absent or incomplete FTS state and retain a safe one-time rebuild/backfill path.
- [x] 3.4 Return enough sync-change information for the syncer to skip FTS work when the local index is already complete and unchanged.

## 4. Cache-first TUI startup

- [x] 4.1 Load an existing cached session list before starting background synchronization, using persisted directory and Git fields without per-session refresh probes.
- [x] 4.2 Keep the existing loading card for an empty cache and add a non-blocking background-sync state for a displayed cache.
- [x] 4.3 Refresh the list after synchronization, preserve the selected session where possible, and keep cached data visible on synchronization failure.
- [x] 4.4 Add localized stale/syncing/failure feedback without changing the existing resume and quit behavior.

## 5. Verification and performance regression coverage

- [x] 5.1 Add indexer tests covering unchanged fast paths, changed OpenCode candidates, same-timestamp cursors, missing cursors, and complete-discovery fallback.
- [x] 5.2 Add tests proving activity and FTS synchronization are idempotent and that changed metadata remains searchable.
- [x] 5.3 Add TUI tests for cached startup, first-run loading, background completion, selection preservation, and failed refresh.
- [x] 5.4 Add a bounded no-change benchmark or timing test for repeated synchronization that does not read real user session content.
- [x] 5.5 Run gofmt, targeted tests, `go vet`, `go test ./...`, available lint checks, and linux/amd64 plus linux/arm64 builds; report any unavailable validation honestly.
