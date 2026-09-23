---
status: current
system: orchestration
requirements:
  - REQ-ORCHESTRATION-DELEGATION-001
  - REQ-ORCHESTRATION-DELEGATION-002
  - REQ-ORCHESTRATION-DELEGATION-003
  - REQ-ORCHESTRATION-DELEGATION-004
  - REQ-ORCHESTRATION-DELEGATION-005
  - REQ-ORCHESTRATION-DELEGATION-006
---

# Delegation System Design

## Purpose and boundaries

This design covers the broker operations a coordinator uses to delegate and
steer work, the callbacks that wake it, and the automation delivery target.
Every mutation is performed by the owning native service: task creation,
editing, moves, sessions and completion by `internal/task` and the workflow
controller; workspace, workflow and repository configuration by their existing
services. Orchestration adds authorization, scoping and the delegation link.

Runtime credentials and the broker attachment are described in the
[coordinator design](coordinators.md#runtime-security).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-ORCHESTRATION-DELEGATION-001` | [Broker tool catalog](#broker-tool-catalog), [Task operations](#task-operations) |
| `REQ-ORCHESTRATION-DELEGATION-002` | [Session repair](#session-repair) |
| `REQ-ORCHESTRATION-DELEGATION-003` | [Workspace administration](#workspace-administration) |
| `REQ-ORCHESTRATION-DELEGATION-004` | [Callbacks](#callbacks) |
| `REQ-ORCHESTRATION-DELEGATION-005` | [Callbacks](#callbacks) |
| `REQ-ORCHESTRATION-DELEGATION-006` | [Automation target](#automation-target) |

## Broker tool catalog

`models.WorkspaceBrokerTools()` is the single tool catalog. The `agentctl`
broker registers exactly these tools, and the `capabilities` tool returns the
same list. Each tool maps to one route under `/api/v1/orchestration`:

| Tool | Route | Purpose |
| --- | --- | --- |
| `workspace` | `GET /runtime/workspace` | Workflows, steps, repositories, execution profiles, templates |
| `manage_workspace` | `POST /runtime/workspace/manage` | Workspace configuration |
| `workspace_tasks` | `GET /runtime/tasks` | Paged task summaries (`after`, `limit`) |
| `task_details` | `GET /runtime/tasks/:id/details` | Task, sessions and result previews |
| `task_content` | `GET /runtime/tasks/:id/content` | Paged description, messages and tool failures |
| `task_permissions` | `GET /runtime/tasks/:id/permissions` | Pending permissions and questions ([relay](coordinator-assistance.md)) |
| `comments` | `GET /tasks/:id/comments` | The coordinator's own conversation history |
| `capabilities` | `GET /runtime/capabilities` | This catalog |
| `memory`, `remember`, `forget` | `/runtime/memory` | Assignment memory ([memory](coordinator-assistance.md#memory)) |
| `create_task` | `POST /runtime/tasks` | Create a delegated task |
| `manage_task` | `POST /runtime/tasks/:id/manage` | Task actions |
| `task_status` | `POST /runtime/tasks/:id/status` | `todo`, `in_progress`, `in_review`, `done` |
| `comment` | `POST /runtime/comments` | Internal receipt in the coordinator's own conversation |

Read tools carry the MCP read-only hint. The broker passes `id`, `query` and
`request` through unchanged; it never adds scope. Resource IDs containing path
separators, `%`, `?` or `#` are rejected before the request is sent. `start`
and `message` actions use a two-minute client deadline because session start
can outlast the normal deadline. A lost response is reported as an unknown
outcome and is never retried by the broker.

## Task operations

`create_task` requires `title` (60 characters or fewer; longer titles return a
definite validation error) and accepts `description`, `workflow_id`,
`workflow_step_id`, `repository_id`, `parent_id`, `assignee` (execution profile
ID), `execution_mode` and `external_id`. The handler creates the task through
`CreateWorkspaceTask` in the token's workspace and writes
`orchestration_chief_id` (the assignment's agent profile ID) and
`orchestration_managed` metadata. `DirectProfile` is resolved server-side from
the assignment; a request cannot select it. Task creation is idempotent on
`external_id` within the workspace.

`manage_task` actions:

| Action | Fields | Native effect |
| --- | --- | --- |
| `edit` | `title`, `description`, `priority`, `parent_id` | Task update; empty `parent_id` removes nesting |
| `move` | `workflow_step_id`, optional `workflow_id`, `position` | Workflow controller move with manual-move and review rules |
| `assign` | `assignee` | Changes the execution profile |
| `adopt` | none | Writes the delegation metadata; keeps the assigned profile |
| `start`, `stop` | optional `session_id` | Session start or stop |
| `message` | `prompt`, optional `session_id` | Sends a prompt to the worker session |
| `repair_session` | optional `session_id` | See [session repair](#session-repair) |
| `session_mode` | `session_id`, `mode` | `default`, `acceptEdits` or `auto`; bypass modes rejected |
| `resolve_permission`, `answer_question` | see [relay](coordinator-assistance.md) | Native permission and clarification services |
| `archive`, `delete` | none | Native archive; delete with safe worktree cleanup |

`task_status` delegates to the native status update, so completion gates,
including the managed-parent completion guard owned by the task system, apply
unchanged. Every handler loads the task and rejects it unless
`task.WorkspaceID` equals the token's workspace. Legacy Office task profile
pins are not applied to `orchestration_managed` tasks.

## Session repair

`repair_session` selects the given session or the task's latest session. It
proceeds only when the session stopped with a provider authentication or OAuth
refresh failure classified by `routingerr`. It clears the stale account lock
held for that session and resumes the same session through the native resume
path. Any other stop reason returns a definite refusal with no side effect.

## Workspace administration

`manage_workspace` accepts `resource` (`workspace`, `workflow`, `step`,
`repository`), `action` (`create`, `update`, `delete`, `reorder`), optional `id`
and `configuration`. The handler dispatches to the native workspace, workflow,
step and repository services, and step events use the shared publisher. It
rejects foreign IDs, hidden system workflows, workflow definitions synced from
a source repository, workspace membership and installation-wide settings.
Deleting a workflow archives its remaining tasks; deleting a repository uses
the native active-session checks.

## Callbacks

`Service.Subscribe` listens on the event bus for task state, task move, agent
lifecycle, session state, message, clarification, permission-request and stall
events. Every event is resolved to a task ID (directly or through its session)
and routed as follows.

**State callbacks.** On `TaskStateChanged` or `TaskMoved`, `taskCallback` loads
the task and proceeds only when it has `orchestration_chief_id`, is in REVIEW,
COMPLETED, FAILED, WAITING_FOR_INPUT or BLOCKED, the assignment still exists in
the same workspace, and the task is not the assignment's own conversation. It
queues a `workspace_task_callback` turn with idempotency key
`workspace-task-callback:<assignment>:<task>:<state>:<updated_at>`, so one
committed transition yields one turn.

**Relay callbacks.** New pending clarifications and permission requests on a
delegated task queue a `workspace_task_callback` keyed by the request ID; see
[coordinator assistance](coordinator-assistance.md#relay-callbacks).

**Stall callbacks.** On `AgentStalled` or `TaskStalled` for a delegated task,
`stallCallback` applies the same assignment and conversation checks and queues
a callback with state `STALLED`, an outcome (`never_started` when the agent
never started, `no_progress` for other agent stalls, `orphaned` for task
stalls), the stall duration and the session. Its key is
`workspace-task-stall:<assignment>:<task>:<session>:<episode>`; the stall
publishers already emit once per episode.

The callback payload carries task ID, title and state, plus the pending
request or stall details when present. The prompt tells the coordinator to
inspect the task, post only new information, treat review as not complete, and
use `repair_session`, `stop` or `message` as remedies for a failed or stalled
session. `QueueTurn` refuses callbacks for a paused assignment or while the
feature is off.

## Automation target

`backendapp` injects the runtime service into Automations as
`automation.OrchestratorTarget`. Delivery validates that the target assignment
exists in the automation's workspace and is active, posts the prompt as a
conversation comment and queues an `automation` turn keyed by the delivery ID.
The automation run records `dispatched` and stores the conversation in
`conversation_task_id`, never `task_id`, so automation cleanup cannot delete
the shared conversation. Trigger producers start only after the orchestration
queue is ready.

## Failure and recovery

Broker handlers return definite 4xx errors for validation and scope failures
and never partially apply a task action. Transport loss surfaces to the model
as an unknown outcome; the prompt instructs it to inspect `task_details`
before retrying. Callback queueing is idempotent, so event redelivery and
restarts do not duplicate turns.

## Related decisions

- [Workspace orchestration owns its coordination runtime](../../../decisions/2026-09-07-workspace-orchestration.md)
- [One coordinator path](../../../decisions/2026-09-18-orchestrator-product-boundary.md)
- [Live agent permission authority](../../../decisions/2026-08-11-live-agent-permission-authority.md)
