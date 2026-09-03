## 1. Progress signaling

- [x] 1.1 Add optional indexer progress notifications for local Agent detection and session indexing without changing existing callers.
- [x] 1.2 Add the progress-aware sync entry point and map index/search work to the three user-facing stages while keeping `Sync` compatible.

## 2. TUI loading card

- [x] 2.1 Route background sync stage events through Bubble Tea's message loop and track the current loading stage.
- [x] 2.2 Render the localized three-row loading card with completed, active, and pending markers, current-stage copy, spinner, and quit hint.
- [x] 2.3 Preserve normal list title, quit behavior, and synchronization failure feedback.

## 3. Verification

- [x] 3.1 Add focused tests for progress stage ordering and TUI stage rendering/transitions.
- [x] 3.2 Run gofmt, targeted package tests, `go vet`, `go test ./...`, and the available repository build checks.
- [ ] 3.3 Run the repository lint check with `golangci-lint` when the tool is available.
