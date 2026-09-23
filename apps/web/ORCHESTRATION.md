# Workspace orchestration

Experimental Orchestration lives in `app/settings/orchestration` and `app/coordinator`, with shared API helpers in `lib/api/domains/orchestration-api.ts`. Instances are workspace scoped; roles are global settings. `features.orchestration` is the only gate: keep workspace cards/tabs, settings discovery, the Coordinator sidebar entry, coordinator conversations and task return links connected under it. Mobile task headers are fixed: put task-to-orchestrator navigation inside `SessionMobileTopBar`, not in a sibling strip it covers. Configuration uses the settings save contributor; roles own global names, icons and live instructions and profiles supply execution identity.

Every coordinator chat is one orchestrator assignment's conversation task, read through `/api/v1/orchestration/tasks/:id` and its paged `/comments`. `useConversationChat` owns the paged comment window and polling (only while the panel is visible); messages post through `createConversationSender`, which reuses one `client_message_id` per message across retries. Links to a conversation go through `conversationHref`.

Orchestration renders with the shared Office chat renderer and may use its task and comment types. Do not import Office pages, APIs or stores for behavior: comment posting, retry and persona identity reach the renderer through `CommentTransportContext`, `RecoveryTransportContext` and `ChatIdentityContext`, whose defaults keep Office behavior.

Automation orchestrator destinations inherit execution configuration from their workspace assignment. Keep stale task profile/executor/repository fields out of those payloads. Delivery history links to `conversation_task_id` and labels dispatch separately from work completion.
