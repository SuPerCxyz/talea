## 1. Progress signaling

- [x] 1.1 Add optional indexer progress notifications for local Agent detection and session indexing without changing existing callers.
- [x] 1.2 Add the progress-aware sync entry point and map index/search work to the three user-facing stages while keeping `Sync` compatible.

## 2. TUI loading card

- [x] 2.1 Route background sync stage events through Bubble Tea's message loop and track the current loading stage.
- [x] 2.2 Render the localized three-row loading card with completed, active, and pending markers, current-stage copy, spinner, and quit hint.
- [x] 2.3 Preserve normal list title, quit behavior, and synchronization failure feedback.
- [x] 2.4 Refine the loading card hierarchy with responsive width, border/padding, high-contrast active-stage styling, and color-independent markers without changing sync semantics.
- [x] 2.5 Add focused tests for active-stage emphasis, narrow-terminal line widths, centered max-width behavior, and localized loading copy.
- [x] 2.6 Center and strengthen the `Talea` title inside the loading card for both normal and failure states without changing terminal font configuration.
- [x] 2.7 Add focused tests for title centering, clean title hierarchy, and narrow-card rendering.
- [x] 2.8 Replace the block wordmark and underline with a clean centered single-line `Talea` title.
- [x] 2.9 Add focused tests for the clean title rendering and absence of decorative block bars.
- [x] 2.10 Render high-contrast keyboard hints with responsive wrapping so all list actions remain visible.
- [x] 2.11 Add focused tests for keyboard hint contrast, complete action coverage, wrapping, and list height accounting.

## 3. Verification

- [x] 3.1 Add focused tests for progress stage ordering and TUI stage rendering/transitions.
- [x] 3.2 Run gofmt, targeted package tests, `go vet`, `go test ./...`, and the available repository build checks.
- [ ] 3.3 Run the repository lint check with `golangci-lint` when the tool is available.
