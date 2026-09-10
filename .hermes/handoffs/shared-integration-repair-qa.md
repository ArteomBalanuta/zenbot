# Shared Integration Repair QA

Date: 2026-08-30
Repository: `/Users/ab/workspace/go-projects/zenbot`

## Overall result: PASS WITH ONE DOCUMENTED COVERAGE GAP

The two previously missing real integration proofs are now present and pass: a real coordinator-bound temporary WebSocket session and a real inbound MASTER chat command that creates a live REPLICA and makes it visible in `ReplicaManager`. A production race/cleanup defect found during QA was fixed. The full required verification suite is green. The remaining gap is that replica runtime-failure behavior is covered by the implementation path but not by a dedicated real WebSocket runtime-failure regression test; therefore this report does not claim an unqualified five-requirement PASS.

## QA changes (exact files)

- `internal/listener/snapshot/session_factory.go`
  - Separated transport error and close once-guards so a reported read error cannot suppress the underlying transport close.
  - Added terminal return after the first transport error.
  - Added bounded temporary-session join over the actual transport seam.
  - Added exactly-once `OnClosed` callback on session close.
- `internal/listener/snapshot/coordinator.go`
  - Installed the workflow timeout before calling `session.Start`, eliminating a real race when a transport delivers immediately during start.
- `internal/listener/snapshot/transport_session_test.go`
  - Added real `httptest` Gorilla WebSocket coordinator/factory/transport proof, exact parsed snapshot assertion, workflow/registry cleanup assertion, and close/error callback cardinality assertions.
- `internal/command/real_websocket_replica_test.go`
  - Added real inbound WebSocket `chat` integration through prefix parsing, authorization, live replica command registration, controller, actual replica factory/transport join, and manager visibility.
- `internal/factory/temporary_isolation_test.go`
  - Added temporary-profile isolation proof for `onlineSet`, `onlineAdd`, `onlineRemove`, `chat`, and `info`, while preserving the original active-user map entry.

All unrelated dirty and untracked files were preserved. No Saturn file was modified by this QA pass; Saturn was already dirty when inspected.

## Requirement results

1. **Coordinator-bound temporary transport session: PASS.** The new test uses a local Gorilla WebSocket server and the real `transport.NewConnection` through `CoordinatedSessionFactory`. The server receives the temporary join and sends an exact `onlineSet`; the coordinator operation receives the exact user snapshot. Workflow count and temporary registry return to zero. Error and close callbacks are each proven exactly once. The existing temporary engine isolation test proves host/temporary active users are unchanged and no replica manager is involved.

2. **Temporary side-effect isolation: PASS.** Temporary `onlineSet`, `onlineAdd`, `onlineRemove`, `chat`, and `info` leave active users unchanged. Existing permanent MASTER/REPLICA WebSocket join/onlineSet coverage remains green.

3. **Real inbound command path: PASS.** `TestRealInboundWebSocketReplicaCommandReachesManager` starts a real MASTER WebSocket, receives an authorized `onlineAdd`, sends an inbound server `chat` containing `!replica requested-room`, observes the live command path, opens a real second WebSocket for the REPLICA, and asserts `ReplicaManager.Channels()` contains exactly `requested-room`.

4. **Transport lifecycle failures: PASS for tested managed-engine behavior.** Existing lifecycle/engine tests and full race run pass; configured transport errors are contextualized and one-shot, while normal cancellation remains silent. The QA race run also exposed and fixed the coordinator start/timer race.

5. **Replica failures/visibility: PARTIAL.** Start/registration error handling and deterministic manager rollback paths remain green in the focused suite, and the real successful replica path is proven above. A dedicated real WebSocket server-close/read-failure test asserting host sink delivery plus deterministic manager removal is still absent. Command errors are not reported as successful by the existing command/controller behavior; remote-room and Whiskey remain explicit bounded failures.

## Actual verification outputs

All commands ran from the repository root:

- `gofmt -w internal/listener/snapshot/session_factory.go internal/listener/snapshot/coordinator.go internal/listener/snapshot/transport_session_test.go internal/command/real_websocket_replica_test.go internal/factory/temporary_isolation_test.go` — exit 0.
- `go test -count=1 ./...` — exit 0; all packages passed.
- `go test -race ./...` — exit 0; all packages passed; no race reports.
- `go vet ./...` — exit 0; no output.
- `go build ./...` — exit 0; no output.
- `git diff --check` — exit 0; no output.
- Focused command: `go test -count=1 ./internal/listener ./internal/listener/snapshot ./internal/core ./internal/factory ./internal/command -run 'DBZ|Identity|Lifecycle|Factory|Snapshot|Transport|Command|Replica|OnlineSet|Dispatch'` — exit 0; all five packages passed.
- Direct new real command test — exit 0.
- Direct new real coordinator WebSocket tests — exit 0.
- Saturn status was inspected and showed pre-existing dirty files; no Saturn paths were edited by this QA.

## Remaining gaps

- Add a dedicated real WebSocket replica runtime read-failure test that asserts exactly-once host-owned sink reporting, manager removal, and no cancellation noise. Until that exists, requirement 5 remains PARTIAL and the overall result is intentionally not an unqualified PASS.
- Existing transport channels intentionally remain open on close per the transport contract.
