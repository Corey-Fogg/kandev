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
  - REQ-ORCHESTRATION-DELEGATION-007
  - REQ-ORCHESTRATION-DELEGATION-008
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
| `REQ-ORCHESTRATION-DELEGATION-007` | [Task proposals](#task-proposals) |
| `REQ-ORCHESTRATION-DELEGATION-008` | [Acceptance criteria](#acceptance-criteria) |

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

## Task proposals

When the caller's assignment has `ask_before_create`, the `create_task`
handler runs its usual title, mode, source and criteria validation, including
the existing-task check for a source issue, and then calls
`Service.ProposeTask` instead of `CreateWorkspaceTask`. One transaction inserts
an `orchestration_task_proposals` row and a conversation comment whose ID is
the proposal ID and whose `source` is `proposal`; the web renders that comment
as a card. The row is unique on `(agent_id, run_id, request_hash)`, so a replay
in the same run returns the stored proposal (200) instead of a new one (202).
Inside the same transaction, after the insert, a pending or approving proposal
for the same source key rolls the insert back and is returned with
`duplicate_proposal: true`. A partial unique index on `(agent_id, source_key)`
over undecided rows backs this; it is created only when no duplicates exist,
so parallel `create_task` calls for one issue store one card.

`DecideProposal` backs the human routes. Approval claims the row
(`pending -> approving`) under a random `claim_token` with `claimed_at`; an
`approving` row is claimed again only once its claim is more than five minutes
old (an interrupted approval). A concurrent approve therefore gets 409
`proposal_approval_in_progress` with the current proposal instead of creating
a second task, and only the claim holder can release or complete the claim.
Approval then applies the edits, validates the final spec
(human titles are never shortened) and creates the task with
`ChiefID` = the assignment and the external ID of the spec, the source issue or
`orchestration-proposal:<id>`. A `DuplicateTaskError` counts as success, so a
retried approval creates one task. Validation or create failures release the
claim and answer 422. Dismissal is a `pending -> dismissed` update. Each
decision queues a `proposal_decision` turn keyed `proposal-decision:<id>` with
a `proposal_decisions` payload; a queue failure is logged and the decision
stands. The prompt lists the decisions and tells the coordinator not to
propose a dismissed task again. `task_proposals` lets the coordinator read its
own proposals.

## Acceptance criteria

`create_task` and proposals accept `acceptance_criteria`; the runtime builds a
goal with IDs `c1…cN` and the adapter stores it as `orchestration_goal` task
metadata. `manage_task` handles `set_criteria` and `verify_criteria` before
the native task action. Both require the task in the token's workspace,
delegated to the caller (403 otherwise) and not archived; neither is
batchable. Verification is all or nothing, redacts evidence and records the
run. Criteria actions on one task are serialized through a striped in-process
lock, so parallel verifications never overwrite each other. `set_criteria`
with an empty list is refused with 409 `acceptance_criteria_unmet` while any
current criterion is unmet, since clearing would lift the completion gate. The
runtime writes metadata only through `TaskMetadataWriter`, which
accepts only `orchestration_goal` and `orchestration_stall` and publishes
`task.updated`. `task_status` with `done` or `COMPLETED` answers 409
`acceptance_criteria_unmet` with the unmet list before calling the native
update, and a `move` into a step with `complete_task_on_enter` is refused the
same way (the adapter returns `ErrCriteriaUnmet`, answered as 409). Digests
carry the criteria outside the digest identity, and the prompt
reminds the coordinator to verify before reporting a task in review or
completed as done.

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
publishers already emit once per episode. Before queueing, the episode is
written to the task as `orchestration_stall` metadata (outcome, duration,
session, detection time), also while the assignment is paused.

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
