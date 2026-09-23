---
status: current
system: orchestration
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-ASSIST-001
  - REQ-ORCHESTRATION-COORDINATOR-ASSIST-002
---

# Coordinator Assistance System Design

## Purpose and boundaries

This design covers how a coordinator is woken for, lists and resolves the
pending questions and permission requests of its delegated tasks, and how
assignment memory is stored and used.

The task system owns clarification requests (`internal/task` clarification
service and interaction projection) and live permission requests (held by
`agentctl`, per the
[live agent permission authority decision](../../../decisions/2026-08-11-live-agent-permission-authority.md)).
Orchestration never stores a copy of a pending request; every read and every
resolution goes to the owning service at call time.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-ORCHESTRATION-COORDINATOR-ASSIST-001` | [Relay callbacks](#relay-callbacks), [Listing pending requests](#listing-pending-requests), [Resolving requests](#resolving-requests) |
| `REQ-ORCHESTRATION-COORDINATOR-ASSIST-002` | [Memory](#memory) |

## Relay callbacks

The coordinator runtime subscribes to clarification and permission-request
events alongside task state events (see [callbacks](delegation.md#callbacks)).
When a new pending clarification or permission request appears on a session of
a delegated task, the runtime applies the same checks as a state callback
(delegation metadata present, assignment active in the task's workspace, not
the assignment's own conversation) and queues a `workspace_task_callback` turn.

The payload carries the task ID, title and state plus a bounded summary of each
pending request: kind (`question` or `permission`), session ID, request
identifiers and the question text or requested action. The idempotency key
includes the request identifier, so one request yields one callback however
often its events are redelivered. Answered, cancelled and stale-dismissed
events queue nothing.

A task that enters WAITING_FOR_INPUT or BLOCKED also produces a state callback.
Both paths read the current pending requests when the turn runs, so the
coordinator acts on live state, not on the payload alone. A session that is
idle without a pending request is never described as waiting for input.

## Listing pending requests

`task_permissions` (`GET /runtime/tasks/:id/permissions`, optional
`session_id`) returns, for a task in the token's workspace:

- live permission requests from `agentctl`, each with `session_id`,
  `request_id`, `pending_id`, the requested action and the provider's offered
  option IDs;
- pending clarification questions from the task's interaction projection, each
  with `session_id`, `pending_id`, the questions, their offered options and
  whether custom text is allowed.

Only live requests are listed. A clarification whose provider handle is gone is
not offered for answering.

## Resolving requests

`manage_task` action `answer_question` takes `session_id`, `pending_id` and
either `answers` (entries of `question_id` with `selected_options` and/or
`custom_text`) or `reject`. The handler loads the task, requires it to be in
the token's workspace and delegated to the calling assignment, and calls the
native clarification service's bundle resolution for that exact session and
pending ID. The answer is attributed to the coordinator agent in the task's
message history. A request that is already answered, expired or replaced
returns 409 and resolves nothing.

`manage_task` action `resolve_permission` takes the exact `session_id`,
`request_id`, `pending_id` and `option_id` from `task_permissions`. Like
`answer_question` and `session_mode`, it requires the task to be delegated to
the calling assignment (created or adopted by it). Only
`allow_once` and `reject_once` option kinds are accepted, so no persistent
rule is created. The strict live-permission path revalidates the request
against `agentctl`; a stale or replaced request returns a conflict.

`manage_task` action `session_mode` accepts `default`, `acceptEdits` and `auto`.
Permission-bypass modes are rejected. When a provider's automatic classifier
denies an action, the coordinator can switch the session to `default`, ask the
worker to retry and then review the resulting native request.

The coordinator prompt states the policy: answer or approve only what the
user's instructions in this conversation already cover, keep explicit denials,
and otherwise relay the request to the user in chat. A person can always
resolve the same request on the native task page; both paths call the same
native services, so the first resolution wins and the other receives a
conflict.

## Memory

`orchestration_memory` rows belong to one assignment (`agent_profile_id`) and
carry `id`, `layer`, `key`, `content`, `metadata` and timestamps, unique per
assignment, layer and key. The assignment belongs to one workspace, so memory
is workspace-scoped. Columns added to this table by earlier development builds
keep their defaults and are not read.

| Tool | Route | Behavior |
| --- | --- | --- |
| `memory` | `GET /runtime/memory` | Lists the calling assignment's entries; optional `memory_id`, `layer`, `key` |
| `remember` | `POST /runtime/memory` | Upserts `{layer, key, content}` for the calling assignment |
| `forget` | `DELETE /runtime/memory/:id` | Deletes one of the calling assignment's entries |

Every route derives the assignment from the runtime token and never from the
request, so a coordinator cannot address another assignment's memory. Writes
are bounded by entry count and content size, and a write over the limit is
rejected without changing stored entries.

Prompt assembly reads the assignment's entries directly and includes as many as
fit a fixed byte budget, then states how many were omitted so the coordinator
can call `memory` for the rest. Memory never changes authorization: every
broker call is authorized from the token and native state alone.

## Failure and recovery

Relay callbacks are idempotent and read live state when they run, so a
restart or a missed event cannot resolve a request twice. Resolution failures
return definite 4xx errors. Transport loss is an unknown outcome; the
coordinator re-reads `task_permissions` before trying again.

## Security

Both relay tools are workspace-scoped and delegation-scoped. The coordinator
cannot answer for tasks it did not delegate, cannot approve bypass modes,
cannot create persistent permission rules and cannot resolve authentication
prompts. Pending-request summaries in callbacks are bounded and exclude tool
payload bodies.

## Related decisions

- [One coordinator path](../../../decisions/2026-09-18-orchestrator-product-boundary.md)
- [Live agent permission authority](../../../decisions/2026-08-11-live-agent-permission-authority.md)
