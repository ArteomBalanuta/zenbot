# Whiskey admin parity: current architecture and next-slice decision

## Decision

[RECOMMENDED] **Do not make `whiskey` the next implementation slice as a user-visible command.** It is not policy-blocked like `mine`, `restart`, `shutdown`, or `sql`, but it is capability-blocked: Zenbot has neither proxy configuration nor a proxy-capable transport. A concrete implementation that accepts the command before those capabilities exist would either be a misleading direct replica (not Saturn parity) or another permanent failure route.

The next safe *preparatory* slice, if the roadmap permits an infrastructure-first slice, is bounded proxy capability plumbing only: typed configuration, a proxy-aware dialer boundary, validation, and tests that demonstrate no direct fallback. It must not expose `whiskey` from `RegisterUserUtilitiesWithDirectAgent` until the capability gate below passes.

## Evidence boundary

- [OBSERVED] Saturn authority is `../../projects/saturn/src/main/java/org/saturn/app/command/impl/admin/WhiskeyReplicaCommandImpl.java` (`WhiskeyReplicaCommandImpl`).
- [OBSERVED] Zenbot target is a dirty working tree. This analysis creates only this Markdown handoff and does not modify application or test source.
- [OBSERVED] Saturn has no `WhiskeyReplicaCommandImpl` test located under `../../projects/saturn/src/test`; exact behavior below is source-derived, not test-backed.
- [LIMITATION] Zenbot's `WhiskeyProxyOrder` unit test covers deterministic string selection only; it does not construct a proxy dialer, launch probe replicas, or reconnect a failed whiskey replica (`internal/command/replica.go` `WhiskeyProxyOrder`; `internal/command/replica_test.go` `TestMsgAndWhiskeyParsing`).

## Observed Saturn contract

### Catalog, authorization, invocation, and replies

[OBSERVED] `WhiskeyReplicaCommandImpl` has exactly the alias `whiskey`; the Zenbot catalog preserves canonical `whiskey`, alias `whiskey`, and `ADMIN` role (`../../projects/saturn/src/main/java/org/saturn/app/command/impl/admin/WhiskeyReplicaCommandImpl.java` `@CommandAliases`; `internal/command/registry.go` `RegisterAll`). `internal/command/admin_moderator_catalog_guard_test.go` `saturnAdminModeratorCatalog` guards that metadata.

[OBSERVED] Saturn subclasses `UserCommandBaseImpl` using `getAdminAndUserTrips(engine)`, so normal command authorization occurs through the shared user-command base before `execute`; the whiskey class itself does not add a separate role check (`WhiskeyReplicaCommandImpl` constructor).

[OBSERVED] Saturn takes the first two parsed arguments:

| Input condition | Saturn status and visible reply |
|---|---|
| fewer than two arguments | `FAILED`; `Error: Usage: whiskey <channel> <name>` |
| duplicate target channel in `replicasMappedByChannel` | `SUCCESSFUL`; `Replica already exists for channel: <channel>` |
| successful outer call after `registerReplica` returns | `SUCCESSFUL`; `Successfully started replica for channel: <channel>` |
| exception escaping `registerReplica` | `FAILED`; `Error: Failed to start replica: <exception message>` |

The inner direct path also sends `Started replica for channel: <channel>`, so a no-proxy success emits **two success replies**. The proxy success path can similarly emit its proxy-specific reply and the outer success reply. Replies use `OutService.enqueueMessageForSending(recipient, text, chatMessage.isWhisper())`; error replies prepend `Error: ` (`WhiskeyReplicaCommandImpl` `execute`, `startReplicaDirectly`, `connectToTargetChannel`, `sendMessage`, `sendErrorMessage`).

[OBSERVED] The first argument is trimmed as the target channel. The second is the custom name; a null/blank name is converted to `portal`, though normal parsed arguments make the null case unlikely (`WhiskeyReplicaCommandImpl` `execute`). Extra arguments are ignored. Saturn does not reject target equal to the host channel in this command; it only rejects an existing map key.

[OBSERVED] Zenbot help advertises `whiskey <channel> <name> - starts an agent replica with a custom nick` (`internal/command/help.go` `adminCommands`), but production registration intentionally omits it: `RegisterUserUtilitiesWithDirectAgent` conditionally registers only generic replica lifecycle commands when the engine satisfies `ReplicaController` (`internal/command/dispatch_adapter.go` `RegisterUserUtilitiesWithDirectAgent`). If a caller directly uses the catalog definition, `saturnCommand.Execute` returns `FAILED` with `whiskey proxy configuration is unavailable` and no chat reply (`internal/command/registry.go` `saturnCommand.Execute`). `TestAdminModeratorCatalogGenericFallbackIsExplicitlyBounded` explicitly allows this generic fallback (`internal/command/admin_moderator_catalog_guard_test.go`).

### Saturn lifecycle and proxy behavior

[OBSERVED] `WhiskeyReplicaCommandImpl.initializeProxyMapping` parses `engine.proxies` entries by `:` and admits only two-part values into static `PORT_MAPPED_BY_IP`, keyed by host/IP. Reconstructing the static map occurs in each command constructor and is neither cleared nor synchronized beyond `ConcurrentHashMap`; malformed values are discarded and duplicate hosts overwrite earlier values (`WhiskeyReplicaCommandImpl` `PORT_MAPPED_BY_IP`, `initializeProxyMapping`).

[OBSERVED] With no admitted proxy, Saturn creates an `EngineType.AGENT` replica, opens a new database connection using the parent `dbPath`, sets channel and custom nick, calls `start()`, then records it in the parent map. It does **not** copy the host password in this branch (`WhiskeyReplicaCommandImpl` `createReplica`, `startReplicaDirectly`).

[OBSERVED] With proxies, Saturn launches one asynchronous test engine per proxy. Each receives a synthetic `<target>_test_<index>_<millis>` channel and `<name>_test_<index>` nick, starts using the supplied proxy, sleeps a fixed 5 seconds, and is healthy only if `isConnected()`. Failed test engines are stopped; connected test engines are kept (`WhiskeyReplicaCommandImpl` `startReplicaWithProxyTesting`, `testProxyConnection`, `awaitHealthyProxies`).

[OBSERVED] It awaits futures in proxy-list order, not completion order. The first healthy result becomes primary. The target is a newly created AGENT engine started with that proxy; after a second fixed 5-second wait it is added to the map only if connected. Remaining healthy results are placed in `engine.backupProxiesByChannel`; the apparently retained primary test replica is logged but not stored in a lifecycle collection (`WhiskeyReplicaCommandImpl` `connectToTargetChannel`).

[OBSERVED] On a primary target failure, Saturn sends an error. If the primary test engine remains connected, it changes that engine's channel and adds it as the target fallback, without changing its test nick or sending a new join shown in this class (`WhiskeyReplicaCommandImpl` `connectToTargetChannel`).

[OBSERVED] Saturn `EngineImpl.stopReplicas` clears every mapped replica, stops it, then asynchronously calls `reconnectWithBackupProxy(this, "system", channel, "portal")`. Reconnect removes index zero from the backup list, starts a new AGENT with that proxy, sleeps 5 seconds, adds it on connectivity, otherwise stops/recurse; it also recursively retries after exceptions. The backup map is a non-concurrent `HashMap` (`../../projects/saturn/src/main/java/org/saturn/app/facade/impl/EngineImpl.java` fields, `stopReplicas`; `WhiskeyReplicaCommandImpl` `reconnectWithBackupProxyInternal`). This means Saturn's stop path can restart replicas, which is not an operational model Zenbot should copy without an explicit lifecycle design.

[OBSERVED] Saturn's proxy actually reaches the WebSocket connection: `EngineImpl.start(Proxy)` passes `Proxy` to `Connection`, while `start()` passes null; both start non-blocking and the join is written through the connection (`../../projects/saturn/src/main/java/org/saturn/app/facade/impl/EngineImpl.java` `start`, `start(Proxy)`, `sendJoinMessage`).

## Observed Zenbot architecture

### Existing managed replica lifecycle

[OBSERVED] Zenbot has concrete admin parity for generic replicas: aliases `replica|bot|agent`, `replicaoff|offline|botoff|agentoff`, and `replicastatus|status`, all `ADMIN`. `newCommand` selects concrete command types for those three canonicals (`internal/command/registry.go` `RegisterAll`, concrete `Execute` methods; `internal/command/handlers.go` `newCommand`).

[TEST-BACKED] The commands validate first channel argument, deny the host/duplicates, preserve exact replies, preserve whisper delivery, require authorization, and do not claim success when lifecycle calls fail (`internal/command/replica_test.go` `TestReplicaCommandIsBlockedForUnauthorizedAuthor`, `TestReplicaAliasesAndFirstArgumentUseSaturnContract`, `TestReplicaSaturnFailuresReplyAndFail`, `TestReplicaOffAliasesRepliesAndFailures`, `TestReplicaStatusExactReplyForNoneAndSortedChannels`, `TestReplicaLifecycleFailureDoesNotClaimSuccess`).

[OBSERVED] `ReplicaManager` owns a mutex-protected map. `ManagedReplicaController.AddReplica` constructs, starts, then atomically registers a `ManagedEngine`; registration failure stops the started engine. Runtime transport failure reports one channel-qualified error and removes/stops the manager entry. Removal deletes the entry before it stops the engine; failed stopping therefore removes it nevertheless (`internal/core/replica_manager.go` `ReplicaManager`; `internal/core/replica_controller.go` `ManagedReplicaController`).

[TEST-BACKED] End-to-end tests prove ordinary replicas open a separate real WebSocket and that a runtime close reports once and removes the manager entry (`internal/command/real_websocket_replica_test.go` `TestRealInboundWebSocketReplicaCommandReachesManager`; `internal/command/real_websocket_replica_failure_test.go` `TestRealWebSocketReplicaRuntimeFailureReportsOnceAndRemovesManagerEntry`).

[OBSERVED] The production composition root constructs one `ReplicaFactory` from master config and creates replicas with the same `EngineOptions`. `ReplicaFactory.NewReplica` copies config, changes only `Channel`, and leaves `Name` unchanged. `cmd/zenbot/main.go` installs the controller and starts/stops managed replicas; `internal/factory/replica_factory.go` `NewReplica` gives every replica `model.REPLICA`, not Saturn's `AGENT` (`cmd/zenbot/main.go` production composition; `internal/factory/replica_factory.go` `ReplicaFactory.NewReplica`).

### Configuration and transport gap

[OBSERVED] `config.Config` has no proxy field (`internal/config/config.go` `Config`). `factory.EngineOptions` passes only `transport.Config`; `transport.Config` has URL, `Dialer`, and timing fields but no proxy descriptor or per-replica dialer selector (`internal/factory/engine_factory.go` `EngineOptions`, `NewEngineWithOptions`; `internal/transport/connection.go` `Config`, `Connection.start`).

[OBSERVED] `transport.Connection.start` calls the configured `Dialer.DialContext(ctx, URL, nil)`. The default is Gorilla's default direct dialer. There is no `Proxy` configuration or invocation in Zenbot (`internal/transport/connection.go` `NewConnection`, `Connection.start`).

[OBSERVED] The sole whiskey-named target helper, `WhiskeyProxyOrder`, only calls a provided probe over strings and returns the first successful proxy plus remaining strings. It is unused by production code, does not apply `ProxyRetryPolicy`, and does not attach a chosen proxy to a transport (`internal/command/replica.go` `ProxyRetryPolicy`, `WhiskeyProxyOrder`).

## Target mapping and gaps

| Saturn responsibility | Zenbot mapping | Gap / outcome |
|---|---|---|
| ADMIN catalog identity and help text | `RegisterAll`, `adminCommands`, catalog guard | Metadata/help exist; command is not registered in production. |
| Custom nick + AGENT engine | `ReplicaFactory.NewReplica` | Factory takes channel only, preserves host name, builds `REPLICA`; needs a distinct request/factory contract. |
| Proxy config parse | `config.Config` | No field/schema/validation. |
| Proxy WebSocket dialing | `transport.Config` / `Connection.start` | No proxy-capable dialer integration. |
| Probe every proxy with temporary engines and 5s connection check | `WhiskeyProxyOrder` | Helper is not an engine probe and has no timeout application, temporary lifecycle, or cleanup. |
| Ordered backup ownership and reconnect | `ReplicaManager` + runtime failure handler | Manager intentionally removes failed replicas; no metadata/backup state or reconnect policy. |
| Register only after live target connection | `ManagedReplicaController.AddReplica` | Ordinary path has correct start-before-add ordering; custom/proxy construction is missing. |
| Stop semantics | `manager.StopAll` in main | Zenbot stops replicas; it does not restart them during host shutdown. Preserve that safer behavior unless separately approved. |

## Capability gate

[RECOMMENDED] Register a concrete `whiskey` command only when all gates are true:

1. **Configuration:** an explicit, validated ordered proxy configuration exists; malformed/empty entries are surfaced deterministically, and secrets are not emitted in replies/logs.
2. **Transport:** a tested proxy-aware `transport.Dialer` (or equivalent factory) performs WebSocket dialing through the selected proxy with context timeout and cancellation.
3. **Construction:** a dedicated request/factory accepts channel, custom name, engine type, and dialer/proxy selection without mutating master config or accidentally sharing connection instances.
4. **Readiness:** target registration follows a defined readiness signal, not only successful TCP/WebSocket dial. The production command must not announce success until it is registered and ready.
5. **Lifecycle ownership:** backup metadata has synchronization, bounded attempts, cleanup, and an explicit distinction between runtime recovery and intentional remove/host shutdown. No recursive, unbounded reconnect and no orphan probe engines.
6. **Dispatch:** the canonical is conditionally registered only when the above capability object is installed; otherwise it remains unavailable and help must not advertise a usable command.
7. **Policy:** confirm with the owning roadmap that this scope excludes the explicitly policy-blocked `mine`, `restart`, `shutdown`, and `sql` work.

## Risks and test map

| Risk | Required test evidence before enabling command |
|---|---|
| Direct fallback silently changes Saturn proxy behavior | No-proxy config: command remains unregistered/returns explicit capability failure; no replica WebSocket is opened. |
| Proxy credentials/config mishandled | Parser validation and redaction tests; ordered duplicates/malformed values tested. |
| Target reply before usable join | Fake proxy WebSocket verifies target join/readiness before map registration and success reply. |
| Test-probe leaks | One healthy/one failed proxy integration test asserts failed probe close and healthy probe ownership/cleanup. |
| Incorrect ordering | Slow/later success cannot supersede earlier configured healthy proxy; verify backup order. |
| Runtime loop / shutdown resurrection | Closing a replica triggers at most configured bounded recovery; explicit `replicaoff` and host shutdown trigger none. |
| Cross-room behavior regression | Re-run existing generic replica WebSocket and runtime failure tests. |
| Catalog exposure before capability is installed | Registry/dispatch test proves exact aliases and conditional absence/presence. |

## First RED and file map

[RECOMMENDED] Start with a command-level RED test, not implementation: add `TestWhiskeyIsNotRegisteredWithoutWhiskeyCapability` beside `internal/command/replica_test.go` (or the existing registration test) and assert that a normal `ReplicaController` alone does not enable `whiskey`. Pair it with a future capability-present test asserting ADMIN role, sole alias, exact `<channel> <name>` validation, and that no success reply occurs until factory/proxy readiness succeeds. This confirms the intended gate before changing behavior.

[RECOMMENDED] Expected implementation surface after that RED test:

- `internal/command/registry.go` — concrete whiskey handler selection; remove `whiskey` from generic fallback only after behavioral tests exist.
- `internal/command/dispatch_adapter.go` — capability-gated registration.
- `internal/command/replica.go` or a dedicated `internal/command/whiskey.go` — request validation and reply contract; avoid growing generic replica code without a dedicated interface.
- `internal/common/...` — small capability/request interface owned by command boundary.
- `internal/config/config.go` — explicit proxy configuration only if configuration policy approves it.
- `internal/transport/connection.go` and focused transport tests — proxy-aware dialing boundary.
- `internal/factory/replica_factory.go` — named/proxied AGENT construction without changing ordinary replica behavior.
- `internal/core/...` — only if a separately approved bounded recovery owner is required.
- `internal/command/*whiskey*_test.go`, `internal/factory/*whiskey*_test.go`, `internal/transport/*proxy*_test.go` — unit/integration coverage listed above.

## No-scope

[RECOMMENDED] Do not modify, register, or otherwise implement `mine`, `restart`, `shutdown`, or `sql`; all four are explicitly policy-blocked. Do not refactor ordinary `replica`, `replicaoff`, or `replicastatus` while preparing whiskey. Do not copy Saturn's static mutable proxy map, 5-second sleeps as readiness, retained unowned test engines, asynchronous restart-on-stop behavior, recursive reconnect, or unsynchronized backup map. Do not add a direct no-proxy whiskey fallback merely to make the help entry executable.
