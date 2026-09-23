---
status: current
system: orchestration
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-001
  - REQ-ORCHESTRATION-COORDINATOR-002
  - REQ-ORCHESTRATION-COORDINATOR-003
  - REQ-ORCHESTRATION-COORDINATOR-004
  - REQ-ORCHESTRATION-COORDINATOR-005
---

# Coordinator System Design

## Purpose and boundaries

`internal/orchestration` owns coordinator roles, workspace assignments,
conversations, the coordinator turn runtime and the broker HTTP surface. It
reuses core infrastructure and does not import `internal/office`:

- `internal/runs` queues, claims, retries and finishes coordinator runs.
- `internal/agent/runtimeauth` mints and validates runtime JWTs.
- `internal/task` owns conversation tasks, sessions, messages and every
  delegated task.
- Execution profiles, executors and provider launch belong to the agents
  system and `agentctl`.

Delegation operations and callbacks are described in
[delegation](delegation.md); question relay and memory in
[coordinator assistance](coordinator-assistance.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-ORCHESTRATION-COORDINATOR-001` | [Roles and assignments](#roles-and-assignments) |
| `REQ-ORCHESTRATION-COORDINATOR-002` | [Conversations and intake](#conversations-and-intake) |
| `REQ-ORCHESTRATION-COORDINATOR-003` | [Turn lifecycle and recovery](#turn-lifecycle-and-recovery) |
| `REQ-ORCHESTRATION-COORDINATOR-004` | [Runtime security](#runtime-security) |
| `REQ-ORCHESTRATION-COORDINATOR-005` | [Feature gate](#feature-gate) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `orchestration.Handler` (`handler.go`) | Human API for roles, assignments, conversation open, pause/resume and persona import |
| `orchestration/personas` | Registers an assignment's core agent profile and pins its execution profile |
| `orchestration/runtime.Service` | Turn queueing, prompt assembly, runtime binding, event subscription, callbacks, recovery |
| `orchestration/runtime.Handler` | Conversation comments and retry, plus the signed `/runtime/*` broker routes |
| `orchestration/repository/sqlite` | Roles, assignments, conversations, intake, intents, instructions and memory |
| `cmd/agentctl` broker | The `kandev_orchestrator` stdio MCP server that forwards named tools to `/runtime/*` |
| `apps/web/app/settings/orchestration` | Role and assignment settings and the conversation route |
| `apps/web/app/coordinator` | The Coordinator view ([design](coordinator-view.md)) |

## Roles and assignments

`orchestration_roles` stores `id`, `name`, `icon` and `instructions`. The
built-in `chief-of-staff` row is inserted with `ON CONFLICT DO NOTHING`, so
edits survive restarts. Deleting a role referenced by `workspace_orchestrators`
is rejected.

`workspace_orchestrators` maps a core agent profile (`agent_id`) to a
`workspace_id` and `role_id`. The profile's settings hold the execution profile
ID, executor preference and workspace context. The prompt reads the role at
turn start, so saved role edits apply from the next turn.

Human routes, all under `/api/v1/orchestration`:

| Route | Behavior |
| --- | --- |
| `GET/POST /roles`, `PUT/DELETE /roles/:roleId` | Global role CRUD |
| `GET /workspaces/:wsId/profiles` | Execution profiles available to assignments |
| `GET/POST /workspaces/:wsId/orchestrators` | List and create assignments |
| `GET/PUT/DELETE /workspaces/:wsId/orchestrators/:id` | Read, update, delete one assignment |
| `GET /workspaces/:wsId/orchestrators/:id/tasks` | The assignment's delegated tasks |
| `POST /workspaces/:wsId/orchestrators/:id/conversation` | Ensure and return the conversation |
| `POST /workspaces/:wsId/orchestrators/:id/status` | Pause or resume |
| `POST /workspaces/:wsId/import/:id` | Register an existing agent persona as an assignment, keeping its identity and history |

Every route authorizes the caller against `:wsId`. Deleting an assignment
stops its conversation sessions; delegated tasks keep their metadata and stay
on their boards.

## Conversations and intake

`orchestration_conversations` maps an assignment to one hidden conversation
task. `EnsureAgentConversation` is idempotent, so opening a conversation never
queues a turn. The legacy `owner_user_id` column is ignored: every read
authorizes by the conversation's workspace, so older conversations stay
readable by workspace members.

`POST /tasks/:id/comments` accepts `{body, client_message_id}`. The comment and
an `orchestration_intake` outbox row commit together. The identical retry
returns 200 with the original receipt; a changed body under the same
`client_message_id` returns 409. `DispatchIntake` drains the outbox from the
run scheduler and queues a `task_comment` turn keyed by the comment ID.

`orchestration_conversation_intents` holds a per-conversation revision that
increments on each accepted message. A run records the revision it was queued
for. Broker writes from a run whose revision is older than the conversation's
current revision return 409 `intent_superseded`, so a turn cannot act on
instructions a newer message replaced.

`bridgeReply` copies the coordinator's final agent message into the
conversation as a comment whose ID is derived from the turn, session and body
hash, so redelivered events write it once. `GET /tasks/:id/comments` pages with
`before`.

## Turn lifecycle and recovery

Every turn is a core run queued by `Service.QueueTurn(agent, conversation,
reason, idempotencyKey, payload)`. Reasons are `task_comment`,
`workspace_task_callback` and `automation`. The run dispatcher claims runs in
order per conversation, so a later message waits behind a queued, running or
recovering turn.

Launch resolves the assignment's pinned execution profile and executor
(`executionSelection`), builds the prompt, and starts a session on the hidden
conversation task. `OnSessionPrepared` calls `bindRuntimeSession`, which mints a
runtime JWT for the new session and records the run's runtime snapshot before
the provider starts.

The prompt contains, in order: entry instruction files, the role name and
instructions, workspace/assignment/conversation identity and routing context,
budgeted memory, the last four conversation comments (1,000 bytes each), the
triggering message or callback payload, and broker tool guidance. Each turn
uses fresh provider context.

`finishTurn` settles the claimed run on `AgentCompleted`, `AgentStopped` or
`AgentFailed` after matching run ID, session ID and claim time. Failure handling
runs under the execution owner's session guard (`HandleFailure`) before terminal
UI state is published:

- A failure with known evidence, no observed output or effect, a prompt
  generation and no dynamic routing attempt is classified by `routingerr`.
  Transient classes schedule a retry of the same run at 15, 30, 60, 60 and 60
  seconds, up to five times. The retry claim is conditional on run, session,
  status and retry count.
- Other failures record the error and finish the run as failed. The
  conversation shows a retry entry; `POST /tasks/:id/retry` queues a new run
  with fresh credentials.
- Stop or Cancel retires a queued retry.
- `RecoverInterrupted` marks runs that were running at restart as failed. Core
  stale-run recovery excludes coordinator profiles. Scheduled retries are rows
  in the runs table and survive restart.

## Runtime security

Every coordinator session uses the MCP surface `orchestrator-broker-v1`
(`internal/mcp/profile`). When `agentctl` configures an instance on that
surface it:

- disables Kandev's shell tool, native ask-question tool and automatic
  permission approval;
- replaces every profile and ambient MCP attachment with one stdio server named
  `kandev_orchestrator`, launched as `agentctl kandev orchestrator-mcp` from the
  managed `agentctl` binary;
- passes the broker only the Kandev API URL, runtime token, run, task, agent,
  workspace and runtime API prefix (`KANDEV_RUNTIME_API_PREFIX`, used by SSH
  executors) environment values.

For Claude ACP sessions the adapter also sends session options that disable
built-in tools, setting sources, plugins, hooks and file checkpointing, set
`strictMcpConfig`, and allow only `mcp__kandev_orchestrator__*`. ACP host file
and terminal methods fail closed. Other providers receive the same single
broker attachment; their provider-native tools remain subject to the provider's
own permission prompts, which are never auto-approved for a coordinator.

The broker policy is written to the conversation session's metadata when the
session is created. Launch, resume and steering paths check that metadata and
the current claimed run before reusing a session, so a resumed session keeps
the broker-only restriction. Sessions written with the earlier
assistant policy key are read as the same broker policy.

The runtime JWT carries the assignment's agent profile, conversation task,
workspace, run and session, with audience and capability
`workspace_coordinator`. Every `/runtime/*` handler validates the token,
requires the run to be claimed and the session to match, reloads the
assignment and its role, and checks that every referenced task, workflow,
repository or conversation belongs to the token's workspace. Scope mismatches
return not found. A finished run's token cannot mutate anything. Writes also
carry `X-Kandev-Run-Id`.

## Feature gate

`features.orchestration` is registered in `internal/runtimeflags` as an
experimental, restart-required flag with environment lock
`KANDEV_FEATURES_ORCHESTRATION`, off in `prod`, `dev` and `e2e`.
`backendapp` starts the orchestration runtime (`startOrchestrationRuntime`),
the intake dispatcher, event subscriptions and the automation target only when
the flag is on. Human and runtime route groups reject requests while it is off;
stored data is untouched. The web navigation, settings sections and Coordinator
route are hidden behind the same flag.

## Persistence

Orchestration migrations are idempotent and replay on every start. Live tables:
`orchestration_roles`, `workspace_orchestrators`, `orchestration_conversations`,
`orchestration_instructions`, `orchestration_memory`, `orchestration_intake`,
`orchestration_conversation_intents` and `orchestration_legacy_imports`. Fresh
installs create no other orchestration tables. Tables left by earlier
development builds are not dropped and are never read.

## Observability

Coordinator runs appear in the core runs table with their reason, retry count,
scheduled retry time and failure message. Turn failures and retries are shown
in the conversation. Logs carry run, session, task and workspace IDs, never
message bodies or tokens.

## Related decisions

- [Workspace orchestration owns its coordination runtime](../../../decisions/2026-09-07-workspace-orchestration.md)
- [One coordinator path](../../../decisions/2026-09-18-orchestrator-product-boundary.md)
- [Replayable schema migrations](../../../decisions/0027-replayable-schema-migrations.md)
