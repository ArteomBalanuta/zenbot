# Saturn agent migration completeness audit

**Verdict: NOT COMPLETE.**

**Scope and method.** Read-only comparison of Saturn `src/main/java/org/saturn/app/agent/**` with Zenbot `internal/agent/**`, resource trees, and required live wiring locations. “Mapped” means a demonstrable target owner exists; it does **not** claim behavioral equivalence unless runtime wiring and focused test evidence are shown. Target worktree was already dirty; this audit changes only this handoff artifact.

## Executive finding

Zenbot contains substantial private agent building blocks and an exact copy of the 27 Saturn `resources/agent/**` files. It is not a complete migration or live integration. `cmd/zenbot/main.go` imports no `internal/agent` package; the message chain’s `RelayAgentMessage.Handle` and `AgentParticipation.Handle` are no-ops; the `l` command is catalogued but has no `case "l"` execution path; and target memory is `turn.InMemoryStore`, not an H2-backed store. These facts independently prevent completion.

## Exact source inventory

- **Java source units:** 141 files, including `package-info.java`.
- **Source agent resources:** 27 files under `src/main/resources/agent/**`.
- **Source agent tests located:** 83 files under `src/test/java/org/saturn/app/agent/**` (not executed in this read-only audit).

| Saturn category | Java units | Count | Target owner / disposition |
|---|---|---:|---|
| `api` | `AgentCapability.java`<br>`AgentContext.java`<br>`AgentConversationContextProvider.java`<br>`AgentExecutionLimits.java`<br>`AgentInvocation.java`<br>`AgentInvocationMode.java`<br>`AgentMemoryStore.java`<br>`AgentParticipationConfig.java`<br>`AgentResult.java`<br>`AgentRoomAutomation.java`<br>`AgentRouter.java`<br>`AgentRoutingException.java`<br>`AgentTool.java`<br>`AgentToolDescriptor.java`<br>`AgentToolResult.java`<br>`AgentUserIdentity.java`<br>`ToolAccess.java`<br>`ToolEffect.java`<br>`ToolExample.java`<br>`ToolResponseEnvelope.java`<br>`ToolResultMode.java` | 21 | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `config` | `AgentConfig.java`<br>`AgentConfigLoader.java`<br>`AgentConfigValueReader.java`<br>`AgentSqlConfig.java` | 4 | `internal/config/{agent_config.go,agent_sql_config.go}` — PARTIAL: resolve/validation exists, but not wired from setup/main. |
| `llm` | `LlmClient.java`<br>`LlmException.java`<br>`LlmMessage.java`<br>`LlmRequest.java`<br>`LlmResponse.java`<br>`LlmToolCall.java`<br>`UnsupportedResponseFormatException.java` | 7 | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/provider/openai` | `OpenAiCompatibleClient.java` | 1 | `internal/agent/llm/openai/client.go` — MAPPED: OpenAI-compatible client adapter exists; no live construction evidence. |
| `moderation` | `AgentModerationConfig.java`<br>`EngineModerationActionExecutor.java`<br>`ModerationAction.java`<br>`ModerationActionExecutor.java`<br>`ModerationDecision.java`<br>`RoomModerationMonitor.java` | 6 | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `.` | `package-info.java` | 1 | `—` — NO TARGET: Java package documentation has no Go package documentation owner. |
| `persistence` | `AgentDatabaseSchema.java`<br>`AgentPersistenceException.java`<br>`AgentQueryRepository.java`<br>`AgentSchemaRepository.java`<br>`AgentSqlRepository.java`<br>`AgentSqlResult.java`<br>`H2AgentMemoryStore.java`<br>`H2AgentQueryRepository.java`<br>`H2AgentSchemaRepository.java`<br>`H2AgentSqlRepository.java`<br>`H2ReadOnlyConnectionFactory.java`<br>`H2TransactionExecutor.java`<br>`RepositoryAgentConversationContextProvider.java` | 13 | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `room` | `AgentMentionParser.java`<br>`AgentQuietRegistry.java`<br>`AgentRoomAutomationFactory.java`<br>`AgentRoomMessagePipeline.java`<br>`AgentSessionLockManager.java`<br>`DefaultAgentRoomAutomation.java`<br>`ProtectedPrincipalPolicy.java` | 7 | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `routing` | `AgentCommandChannelPolicy.java`<br>`AgentCommandIntentPolicy.java`<br>`AgentCommandProseGuard.java`<br>`AgentContextProjection.java`<br>`AgentInfrastructure.java`<br>`AgentInfrastructureFactory.java`<br>`AgentInvocationFactory.java`<br>`AgentMessageProjector.java`<br>`AgentPreparedRequest.java`<br>`AgentPromptCatalog.java`<br>`AgentRequestAssembler.java`<br>`AgentRequestClassifier.java`<br>`AgentRequestInput.java`<br>`AgentRequestKind.java`<br>`AgentResponseCorrector.java`<br>`AgentResponseFinalizer.java`<br>`AgentResponseSanitizer.java`<br>`AgentRouterFactory.java`<br>`AgentRuntimeFactory.java`<br>`AgentSystemPrompt.java`<br>`AgentTextBounds.java`<br>`DefaultAgentRouter.java`<br>`VerifiedQuoteCatalog.java` | 23 | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `sql` | `AgentSqlErrorCode.java`<br>`AgentSqlPolicy.java`<br>`AgentSqlPolicyException.java`<br>`JSqlParserAgentSqlPolicy.java`<br>`ValidatedAgentSql.java` | 5 | `internal/agent/sql/policy.go` — PARTIAL: policy exists; no repository execution integration. |
| `tool` | `AgentRoomDirectory.java`<br>`AgentToolArgumentReader.java`<br>`DatabaseQueryTool.java`<br>`DatabaseSchemaTool.java`<br>`DatabaseSqlTool.java`<br>`EngineAgentRoomDirectory.java`<br>`EngineSaturnCommandGateway.java`<br>`RoomUsersTool.java`<br>`RunCommandTool.java`<br>`SaturnCommandGateway.java`<br>`SaturnCommandTool.java`<br>`SaturnCommandToolCatalog.java`<br>`UserMessageHistoryTool.java` | 13 | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/contract` | `AgentToolDefinitionFactory.java`<br>`AgentToolDefinitionJson.java`<br>`AgentToolSchemaValidator.java`<br>`AgentToolSchemas.java` | 4 | `internal/agent/tool/contract/{definition.go,schema.go}` — PARTIAL: definition/schema types exist; validator/resource factory parity not established. |
| `tool/execution` | `AgentModelVisibleToolResultRenderer.java`<br>`AgentScheduledToolCall.java`<br>`AgentToolBatchContext.java`<br>`AgentToolBudgetPolicy.java`<br>`AgentToolCallScheduler.java`<br>`AgentToolCallValidator.java`<br>`AgentToolExecutionContext.java`<br>`AgentToolExecutionHooks.java`<br>`AgentToolExecutionLedger.java`<br>`AgentToolExecutionMiddleware.java`<br>`AgentToolExecutionMode.java`<br>`AgentToolExecutionObserver.java`<br>`AgentToolExecutionPolicy.java`<br>`AgentToolExecutor.java`<br>`AgentToolInvoker.java`<br>`AgentToolRegistry.java`<br>`AgentToolRegistryFactory.java`<br>`AgentToolResultCoordinator.java`<br>`CancellationToken.java`<br>`ValidatedToolCall.java` | 20 | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `turn` | `AgentExecutionState.java`<br>`AgentFreshDataCoordinator.java`<br>`AgentFreshDataFinalValidator.java`<br>`AgentFreshDataPolicy.java`<br>`AgentFreshDataTurnPolicy.java`<br>`AgentFreshnessPolicy.java`<br>`AgentMessageHistory.java`<br>`AgentNickNormalizer.java`<br>`AgentToolEvidence.java`<br>`AgentTurnMemory.java`<br>`AgentTurnPolicy.java`<br>`AgentTurnPolicyChain.java`<br>`AgentTurnPolicyInput.java`<br>`AgentTurnPolicyResult.java`<br>`AgentTurnState.java`<br>`AgentUnverifiedActionPolicy.java` | 16 | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |

### Full unit-level source-to-target ledger

| Saturn unit | Target evidence / status |
|---|---|
| `api/AgentCapability.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentContext.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentConversationContextProvider.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentExecutionLimits.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentInvocation.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentInvocationMode.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentMemoryStore.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentParticipationConfig.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentResult.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentRoomAutomation.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentRouter.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentRoutingException.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentTool.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentToolDescriptor.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentToolResult.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/AgentUserIdentity.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/ToolAccess.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/ToolEffect.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/ToolExample.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/ToolResponseEnvelope.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `api/ToolResultMode.java` | `internal/agent/api/{api.go,identity.go,result.go}` — PARTIAL: core contracts exist; missing source-specific interfaces/configuration contracts remain unproven. |
| `config/AgentConfig.java` | `internal/config/{agent_config.go,agent_sql_config.go}` — PARTIAL: resolve/validation exists, but not wired from setup/main. |
| `config/AgentConfigLoader.java` | `internal/config/{agent_config.go,agent_sql_config.go}` — PARTIAL: resolve/validation exists, but not wired from setup/main. |
| `config/AgentConfigValueReader.java` | `internal/config/{agent_config.go,agent_sql_config.go}` — PARTIAL: resolve/validation exists, but not wired from setup/main. |
| `config/AgentSqlConfig.java` | `internal/config/{agent_config.go,agent_sql_config.go}` — PARTIAL: resolve/validation exists, but not wired from setup/main. |
| `llm/LlmClient.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/LlmException.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/LlmMessage.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/LlmRequest.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/LlmResponse.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/LlmToolCall.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/UnsupportedResponseFormatException.java` | `internal/agent/llm/client.go` — PARTIAL: neutral LLM contracts exist. |
| `llm/provider/openai/OpenAiCompatibleClient.java` | `internal/agent/llm/openai/client.go` — MAPPED: OpenAI-compatible client adapter exists; no live construction evidence. |
| `moderation/AgentModerationConfig.java` | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `moderation/EngineModerationActionExecutor.java` | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `moderation/ModerationAction.java` | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `moderation/ModerationActionExecutor.java` | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `moderation/ModerationDecision.java` | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `moderation/RoomModerationMonitor.java` | `internal/agent/moderation/action.go` — PARTIAL: action type only; monitor/executor/config not evidenced. |
| `package-info.java` | `—` — NO TARGET: Java package documentation has no Go package documentation owner. |
| `persistence/AgentDatabaseSchema.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/AgentPersistenceException.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/AgentQueryRepository.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/AgentSchemaRepository.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/AgentSqlRepository.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/AgentSqlResult.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/H2AgentMemoryStore.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/H2AgentQueryRepository.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/H2AgentSchemaRepository.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/H2AgentSqlRepository.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/H2ReadOnlyConnectionFactory.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/H2TransactionExecutor.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `persistence/RepositoryAgentConversationContextProvider.java` | `internal/agent/persistence/schema.go; internal/repository/h2/schema-h2.sql` — PARTIAL: schema DTO and SQL table text exist; no H2 agent repositories/memory store. |
| `room/AgentMentionParser.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `room/AgentQuietRegistry.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `room/AgentRoomAutomationFactory.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `room/AgentRoomMessagePipeline.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `room/AgentSessionLockManager.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `room/DefaultAgentRoomAutomation.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `room/ProtectedPrincipalPolicy.java` | `internal/agent/room/protected_principal.go; internal/agent/participation/policies.go` — PARTIAL: protected principal and parser/policies only; automation/pipeline/session/quiet are absent. |
| `routing/AgentCommandChannelPolicy.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentCommandIntentPolicy.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentCommandProseGuard.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentContextProjection.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentInfrastructure.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentInfrastructureFactory.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentInvocationFactory.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentMessageProjector.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentPreparedRequest.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentPromptCatalog.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentRequestAssembler.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentRequestClassifier.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentRequestInput.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentRequestKind.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentResponseCorrector.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentResponseFinalizer.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentResponseSanitizer.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentRouterFactory.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentRuntimeFactory.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentSystemPrompt.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/AgentTextBounds.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/DefaultAgentRouter.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `routing/VerifiedQuoteCatalog.java` | `internal/agent/{routing/routing.go,assemble/assemble.go,prompt/prompt.go,runtime/*.go}` — PARTIAL: bounded assembly/prompt/runtime seams exist; no live router factory/service wiring. |
| `sql/AgentSqlErrorCode.java` | `internal/agent/sql/policy.go` — PARTIAL: policy exists; no repository execution integration. |
| `sql/AgentSqlPolicy.java` | `internal/agent/sql/policy.go` — PARTIAL: policy exists; no repository execution integration. |
| `sql/AgentSqlPolicyException.java` | `internal/agent/sql/policy.go` — PARTIAL: policy exists; no repository execution integration. |
| `sql/JSqlParserAgentSqlPolicy.java` | `internal/agent/sql/policy.go` — PARTIAL: policy exists; no repository execution integration. |
| `sql/ValidatedAgentSql.java` | `internal/agent/sql/policy.go` — PARTIAL: policy exists; no repository execution integration. |
| `tool/AgentRoomDirectory.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/AgentToolArgumentReader.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/DatabaseQueryTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/DatabaseSchemaTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/DatabaseSqlTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/EngineAgentRoomDirectory.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/EngineSaturnCommandGateway.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/RoomUsersTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/RunCommandTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/SaturnCommandGateway.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/SaturnCommandTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/SaturnCommandToolCatalog.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/UserMessageHistoryTool.java` | `internal/agent/tool/tool.go` — PARTIAL: generic Tool/Registry exists; no Saturn concrete database/room/command/history tools or engine gateways. |
| `tool/contract/AgentToolDefinitionFactory.java` | `internal/agent/tool/contract/{definition.go,schema.go}` — PARTIAL: definition/schema types exist; validator/resource factory parity not established. |
| `tool/contract/AgentToolDefinitionJson.java` | `internal/agent/tool/contract/{definition.go,schema.go}` — PARTIAL: definition/schema types exist; validator/resource factory parity not established. |
| `tool/contract/AgentToolSchemaValidator.java` | `internal/agent/tool/contract/{definition.go,schema.go}` — PARTIAL: definition/schema types exist; validator/resource factory parity not established. |
| `tool/contract/AgentToolSchemas.java` | `internal/agent/tool/contract/{definition.go,schema.go}` — PARTIAL: definition/schema types exist; validator/resource factory parity not established. |
| `tool/execution/AgentModelVisibleToolResultRenderer.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentScheduledToolCall.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolBatchContext.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolBudgetPolicy.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolCallScheduler.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolCallValidator.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionContext.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionHooks.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionLedger.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionMiddleware.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionMode.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionObserver.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutionPolicy.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolExecutor.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolInvoker.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolRegistry.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolRegistryFactory.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/AgentToolResultCoordinator.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/CancellationToken.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `tool/execution/ValidatedToolCall.java` | `internal/agent/tool/execution/execution.go` — PARTIAL: bounded execution components exist; source 20-unit scheduler/ledger/middleware/observer model is not one-for-one. |
| `turn/AgentExecutionState.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentFreshDataCoordinator.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentFreshDataFinalValidator.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentFreshDataPolicy.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentFreshDataTurnPolicy.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentFreshnessPolicy.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentMessageHistory.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentNickNormalizer.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentToolEvidence.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentTurnMemory.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentTurnPolicy.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentTurnPolicyChain.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentTurnPolicyInput.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentTurnPolicyResult.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentTurnState.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |
| `turn/AgentUnverifiedActionPolicy.java` | `internal/agent/turn/{budget.go,coordinator.go,freshness.go,memory.go,policy.go,state.go,state_results.go}` — PARTIAL: turn seams and in-memory store exist; source policies/evidence persistence not fully present. |

## Zenbot target inventory

- **Go files under `internal/agent/**`: 44 total = 29 production + 15 test files.**
- **Production files by directory:**
  - `api`: 3
  - `assemble`: 1
  - `llm`: 1
  - `llm/openai`: 1
  - `moderation`: 1
  - `participation`: 2
  - `persistence`: 1
  - `prompt`: 1
  - `room`: 1
  - `routing`: 1
  - `runtime`: 4
  - `sql`: 1
  - `tool`: 1
  - `tool/contract`: 2
  - `tool/execution`: 1
  - `turn`: 7

### Production target files

- `internal/agent/api/api.go`
- `internal/agent/api/identity.go`
- `internal/agent/api/result.go`
- `internal/agent/assemble/assemble.go`
- `internal/agent/llm/client.go`
- `internal/agent/llm/openai/client.go`
- `internal/agent/moderation/action.go`
- `internal/agent/participation/invocation.go`
- `internal/agent/participation/policies.go`
- `internal/agent/persistence/schema.go`
- `internal/agent/prompt/prompt.go`
- `internal/agent/room/protected_principal.go`
- `internal/agent/routing/routing.go`
- `internal/agent/runtime/adapters.go`
- `internal/agent/runtime/api_bridge.go`
- `internal/agent/runtime/contracts.go`
- `internal/agent/runtime/runtime.go`
- `internal/agent/sql/policy.go`
- `internal/agent/tool/contract/definition.go`
- `internal/agent/tool/contract/schema.go`
- `internal/agent/tool/execution/execution.go`
- `internal/agent/tool/tool.go`
- `internal/agent/turn/budget.go`
- `internal/agent/turn/coordinator.go`
- `internal/agent/turn/freshness.go`
- `internal/agent/turn/memory.go`
- `internal/agent/turn/policy.go`
- `internal/agent/turn/state.go`
- `internal/agent/turn/state_results.go`

### Target test files

- `internal/agent/api/api_test.go`
- `internal/agent/assemble/assemble_test.go`
- `internal/agent/llm/client_test.go`
- `internal/agent/llm/openai/client_test.go`
- `internal/agent/llm/openai/qa_test.go`
- `internal/agent/participation/policies_test.go`
- `internal/agent/prompt/prompt_test.go`
- `internal/agent/runtime/runtime_test.go`
- `internal/agent/sql/policy_test.go`
- `internal/agent/tool/contract/contract_test.go`
- `internal/agent/tool/execution/execution_extra_test.go`
- `internal/agent/tool/execution/execution_test.go`
- `internal/agent/turn/coordinator_test.go`
- `internal/agent/turn/parity_red_test.go`
- `internal/agent/turn/turn_test.go`

## Resources

- **Exact resource-file parity:** 27 Saturn files versus 27 Zenbot files; set comparison found **no missing and no extra paths**. Examples: `agent/persona/vaelen-system-prompt.txt`, `agent/tool-copy.json`, `agent/verified-quotes.json`, all correction/input/system templates.
- **Limitation:** resource presence is not resource loading or live use. `internal/agent/prompt/prompt.go` is a bounded loader/contract owner; no `main.go` construction connects it to a running router.

### Complete resource ledger

| Saturn resource | Zenbot target resource | Status |
|---|---|---|
| `src/main/resources/agent/correction/router-command-not-executed-correction.txt` | `resources/agent/correction/router-command-not-executed-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-command-output-correction.txt` | `resources/agent/correction/router-command-output-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-command-tool-correction.txt` | `resources/agent/correction/router-command-tool-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-failure-placeholder-correction.txt` | `resources/agent/correction/router-failure-placeholder-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-fresh-synthesis-correction.txt` | `resources/agent/correction/router-fresh-synthesis-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-fresh-tool-correction.txt` | `resources/agent/correction/router-fresh-tool-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-internal-evidence-correction.txt` | `resources/agent/correction/router-internal-evidence-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-non-command-correction.txt` | `resources/agent/correction/router-non-command-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-quote-only-correction.txt` | `resources/agent/correction/router-quote-only-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-stale-response-correction.txt` | `resources/agent/correction/router-stale-response-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-unavailable-action-response.txt` | `resources/agent/correction/router-unavailable-action-response.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-unverified-action-correction.txt` | `resources/agent/correction/router-unverified-action-correction.txt` | Exact relative path present |
| `src/main/resources/agent/correction/router-unverified-action-final-correction.txt` | `resources/agent/correction/router-unverified-action-final-correction.txt` | Exact relative path present |
| `src/main/resources/agent/input/command-executed-result.txt` | `resources/agent/input/command-executed-result.txt` | Exact relative path present |
| `src/main/resources/agent/input/router-contextualized-prompt.txt` | `resources/agent/input/router-contextualized-prompt.txt` | Exact relative path present |
| `src/main/resources/agent/input/router-room-delivery.txt` | `resources/agent/input/router-room-delivery.txt` | Exact relative path present |
| `src/main/resources/agent/persona/vaelen-system-prompt.txt` | `resources/agent/persona/vaelen-system-prompt.txt` | Exact relative path present |
| `src/main/resources/agent/system/database-policy-disabled.txt` | `resources/agent/system/database-policy-disabled.txt` | Exact relative path present |
| `src/main/resources/agent/system/database-policy-enabled.txt` | `resources/agent/system/database-policy-enabled.txt` | Exact relative path present |
| `src/main/resources/agent/system/participation-ambient.txt` | `resources/agent/system/participation-ambient.txt` | Exact relative path present |
| `src/main/resources/agent/system/participation-direct.txt` | `resources/agent/system/participation-direct.txt` | Exact relative path present |
| `src/main/resources/agent/system/participation-mention.txt` | `resources/agent/system/participation-mention.txt` | Exact relative path present |
| `src/main/resources/agent/system/participation-moderation.txt` | `resources/agent/system/participation-moderation.txt` | Exact relative path present |
| `src/main/resources/agent/system/router-finalize.txt` | `resources/agent/system/router-finalize.txt` | Exact relative path present |
| `src/main/resources/agent/system/system-policy.txt` | `resources/agent/system/system-policy.txt` | Exact relative path present |
| `src/main/resources/agent/tool-copy.json` | `resources/agent/tool-copy.json` | Exact relative path present |
| `src/main/resources/agent/verified-quotes.json` | `resources/agent/verified-quotes.json` | Exact relative path present |

## Demonstrable mappings and bounded test evidence

| Saturn capability / symbols | Zenbot evidence | Audit assessment |
|---|---|---|
| API identities/results/invocations (`api/AgentContext`, `AgentInvocation`, `AgentResult`, `AgentUserIdentity`) | `internal/agent/api/{api.go,identity.go,result.go}` and `api_test.go` | Partial contract migration; no live caller from `main.go`. |
| LLM contracts and OpenAI provider (`llm/Llm*`, `provider/openai/OpenAiCompatibleClient`) | `internal/agent/llm/client.go`, `internal/agent/llm/openai/client.go` with tests | Adapter code is tested in package scope; not constructed by application wiring. |
| Request assembly/prompt/routing (`routing/AgentRequestAssembler`, `AgentPromptCatalog`, `DefaultAgentRouter`) | `internal/agent/assemble/assemble.go`, `prompt/prompt.go`, `routing/routing.go` | Partial private seams; no `AgentRouterFactory`/equivalent wired from target startup. |
| SQL policy (`sql/JSqlParserAgentSqlPolicy`) | `internal/agent/sql/policy.go` and tests | Bounded policy migration; no concrete database SQL tool/repository path invokes it. |
| Tool contracts/execution (`tool/contract/**`, `tool/execution/**`) | `internal/agent/tool/{tool.go,contract/**,execution/execution.go}` and focused tests | Generic registry/execution code exists; no concrete tool catalog/gateways or persistence integration. |
| Turn/freshness/memory (`turn/**`, `persistence/H2AgentMemoryStore`) | `internal/agent/turn/{coordinator.go,freshness.go,memory.go,...}` | Partial. `memory.go` provides `InMemoryStore`; no H2 implementation or runtime injection. |
| Mention/protected-principal behavior (`room/AgentMentionParser`, `ProtectedPrincipalPolicy`) | `participation/policies.go`, `room/protected_principal.go`, participation tests | Bounded parser policy evidence; it is not connected to listener handling. |
| Agent schema tables (`persistence/AgentDatabaseSchema`, H2 repositories) | `internal/repository/h2/schema-h2.sql`, `resources/schema-h2.sql` mention `agent_memory` and `agent_tool_memory`; `internal/agent/persistence/schema.go` only data types | SQL DDL presence alone is not persistence implementation or real-H2 agent behavior. |

## Live integration status

| Integration surface | Observed target evidence | Status |
|---|---|---|
| Startup composition | `cmd/zenbot/main.go` imports command/config/core/factory/model/h2/transport but no `internal/agent`; `factory.NewEngineWithOptions` receives no agent option there. | **UNWIRED** |
| Agent command (`l`) | `internal/command/registry.go:134` declares `def("l", []string{"l"}, ...)`; `saturnCommand.Execute` has no `case "l"`, so its default returns “no Saturn implementation”. | **UNWIRED / not implemented** |
| Agent relay | `internal/listener/message/handlers.go:40–42`: `RelayAgentMessage.Handle` returns `true, nil` without acting. | **NO-OP** |
| Agent participation | `handlers.go:128–130`: `AgentParticipation.Handle` returns `true, nil`; it is placed in `DefaultChain` at line 157 but does not invoke automation. | **NO-OP** |
| Memory persistence | `internal/agent/turn/memory.go:26–95`: `InMemoryStore` holds a mutex/map. No `H2AgentMemoryStore` equivalent exists in `internal/repository/h2/**`. | **NOT PERSISTED** |
| Tool persistence | Target H2 schema names agent tables, but no target H2 agent query/schema/SQL repositories or tool-evidence writer was located. | **NOT IMPLEMENTED / NOT WIRED** |
| Room lifecycle | No target equivalents located for `AgentRoomAutomationFactory`, `DefaultAgentRoomAutomation`, `AgentRoomMessagePipeline`, `AgentQuietRegistry`, or `AgentSessionLockManager`; listener join path has no agent call. | **ABSENT** |
| Moderation monitor | Only `internal/agent/moderation/action.go` exists; no `RoomModerationMonitor`/engine executor target was located. | **ABSENT** |

## Missing, unwired, or untested capability inventory

- **Missing implementation groups:** H2 agent memory/query/schema/SQL repositories and transactions; repository conversation provider; concrete database query/schema/SQL tools; room-user/history/command tools; Saturn command/room gateway adapters; room automation/message pipeline/quiet/session locking; moderation configuration/monitor/executor; complete router infrastructure/factories/classifier/finalizer/corrector/sanitizer; and the source execution scheduler/ledger/middleware/observer structure.
- **Unwired existing groups:** config resolution, LLM client, prompt catalog, runtime, routing/assembly, policy, tools, turn coordination, and resources have no demonstrated composition from `cmd/zenbot/main.go`/factory into command or listener execution.
- **Untested target production directories in the focused command:** `internal/agent/moderation`, `persistence`, `room`, `routing`, and `tool` reported “[no test files]”. The command did pass for package tests that exist, including agent packages, message listener, command, H2 repository, and service.
- **Source test gap:** Saturn has 83 agent tests versus target’s 15 agent test files; file counts are not a parity metric, but the target lacks required real-H2 memory/query/schema/tool integration tests and any command/listener end-to-end agent test.

## Verification performed

- Ran: `go test ./internal/agent/... ./internal/listener/message ./internal/command ./internal/repository/h2 ./internal/service` — **exit 0**. This verifies currently selected target tests, not migration completeness.
- Read-only source/target path inventories, resource set comparison, source/target symbols, command registry, listener chain, startup code, H2 schema references, and `MIGRATION_PLAN.md`.
- `MIGRATION_PLAN.md:19`, `:177–183`, and `:235–244` independently record that agent boundaries remain unwired and that completion requires all units plus real-H2/live integration evidence.

## Final verdict

**NOT COMPLETE.** The source package contains **141 Java units** plus **27 resources**. Zenbot has **29 production Go files** under `internal/agent/**` (44 including tests) and exact resource-file copies, but essential Saturn units are absent or only partial, and every required live surface—agent command, relay/participation, persistent memory, and tool persistence—is demonstrably unwired or no-op. Concise evidence: `cmd/zenbot/main.go`, `internal/listener/message/handlers.go:40–42,128–130,157`, `internal/command/registry.go:30–106,134`, `internal/agent/turn/memory.go:26–95`, `internal/repository/h2/schema-h2.sql`, and `MIGRATION_PLAN.md:19,177–183`.
