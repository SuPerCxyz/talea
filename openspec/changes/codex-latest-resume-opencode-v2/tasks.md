## 1. Codex latest policy regression

- [x] 1.1 Add a rollout fixture/test where an ordinary `turn_context` is followed by full yolo-equivalent policy and assert the later policy wins.
- [x] 1.2 Verify the persisted resume-argument path still feeds dry-run and shared restore command construction without changing the whitelist.

## 2. OpenCode v2 source compatibility

- [x] 2.1 Resolve `OPENCODE_DB`, `opencode debug paths db`, and legacy fallback paths with shell-free argument execution.
- [x] 2.2 Make OpenCode session metadata scanning tolerate v2 nullable fields and distinguish unknown default-zero Token data from known usage.
- [x] 2.3 Extend the OpenCode fixture with v2 projection tables and nullable metadata, and add override-path coverage.

## 3. Documentation and verification

- [x] 3.1 Update OpenCode format/README documentation for v2.0.7 paths, tables, CLI recovery, and compatibility boundaries; update Codex version/policy notes from current local evidence.
- [x] 3.2 Run gofmt, targeted adapter/index/CLI tests, `go vet`, `go test ./...`, available lint checks, and linux/amd64 plus linux/arm64 builds.
