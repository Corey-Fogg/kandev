---
title: "Orchestration API"
description: "Coordinator conversation, runtime broker and task-observation API reference."
status: experimental
---

# Orchestration API

This reference describes the HTTP routes and broker tools behind
[workspace orchestration](orchestration-personas.md). The feature is
experimental. `features.orchestration` (`KANDEV_FEATURES_ORCHESTRATION`) enables
it; it defaults off in every shipped profile and requires a restart after a
change in **Settings > System > Feature Toggles**. An explicit environment value
overrides and locks the setting. While it is off, the routes below are rejected
and no coordinator turn starts. Stored configuration and history are kept.

Routes use the `/api/v1/orchestration` prefix unless stated otherwise. Human
routes use Kandev's authenticated identity and the caller's workspace access.

## Orchestrators

A workspace has one orchestrator. It follows a global role template and has its
own instance name: `display_name` (1 to 60 characters, no control characters;
empty inherits the role name). Every response reports the effective name as
`name`, the template name as `role_name`, and three behavior settings:
`ask_before_create` (default off), `auto_comment_source` (default on) and
`auto_move_source_done` (default off).

| Route | Access | Result |
| --- | --- | --- |
| `GET /workspaces/:wsId/orchestrators` | read | `{"orchestrators": [...]}` |
| `POST /workspaces/:wsId/orchestrators` | manage | 201 with the orchestrator; 409 `{"error":"orchestrator_exists","orchestrator_id"}` when the workspace has one |
| `PUT /workspaces/:wsId/orchestrators/:id` | manage | Full configuration; the name and settings are optional. 409 while a turn runs |
| `PATCH /workspaces/:wsId/orchestrators/:id` | manage | Any of `display_name` and the three settings; allowed while a turn runs; 400 when empty or invalid |
| `POST /workspaces/:wsId/import/:id` | manage | Registers a legacy assistant; 409 `orchestrator_exists` when another is registered |
| `GET /workspaces/:wsId/orchestrators/:id/metrics?days=7\|30` | read | Delegated outcomes, the same fields as the `metrics` tool; 422 for other windows |
| `GET /workspaces/:wsId/orchestrators/:id/proposals?status=pending\|all&limit=1..50` | read | `{"proposals": [...]}`, newest first |
| `GET /workspaces/:wsId/orchestrators/:id/proposals/:proposalId` | read | One proposal |
| `POST /workspaces/:wsId/orchestrators/:id/proposals/:proposalId/approve` | manage | Optional `edits` (`title`, `description`, `workflow_id`, `workflow_step_id`, `repository_id`, `assignee`, `execution_mode`, `acceptance_criteria`); returns `{proposal, task_id, duplicate}` |
| `POST /workspaces/:wsId/orchestrators/:id/proposals/:proposalId/dismiss` | manage | Optional `reason` (500 characters); returns `{proposal}` |

Approving is idempotent: a repeated approval returns the same task. Approving a
dismissed proposal, or dismissing an approved one, returns 409
`proposal_already_decided` with the current proposal. An invalid edit or a
failed create returns 422 and leaves the proposal pending. A workspace that
already has several orchestrators from an earlier build keeps them all working.

## Conversations

- `POST /workspaces/:wsId/orchestrators/:id/conversation` returns the
  assignment's conversation, creating it on first use. It never starts a turn.
- `POST /tasks/:id/comments` accepts `body` and a stable `client_message_id`.
  First acceptance returns 201; an identical retry returns 200 with the same
  receipt; a changed body under the same ID returns 409. Acceptance is durable
  before any run exists, and accepted messages start turns in order.
- `GET /tasks/:id/comments` returns history; pass `before` for older pages.
- `POST /tasks/:id/retry` retries a failed turn with fresh credentials.

Conversations are workspace-scoped: any member with access to the workspace can
read them. Posting and retrying, like creating, updating, pausing or deleting an
orchestrator, need workspace manage access, because the coordinator acts with
that authority.

## Coordinator task observations

`GET /api/v1/workspaces/:workspaceId/tasks?view=kanban` is the read-only task
source for the Coordinator page. It requires the normal workspace read
permission and applies ordinary-task, archive, ephemeral, configuration, hidden
workflow and Office-workflow exclusions before totals, filtering and pagination.
Existing callers that omit `view=kanban` keep their existing behavior.

The page supplies `page_size=100`; `page`, `query`, `workflow_id`,
`repository_id`, `include_archived` and `only_archived` use the existing
task-list query contract. Text search in this
view does not add command-palette pull-request-number results. Native
`task.status_summary.updated` events update only the matching workspace;
lifecycle events and reconnects refresh the loaded window. No conversation
history or worker transcript is fetched to classify task rows.

## Runtime credentials

Each coordinator turn receives a runtime JWT with the `workspace_coordinator`
audience, bound to the assignment, its workspace, the run and the session.
Tokens expire with the run. Every runtime route rechecks the claimed run and
session, and requires every referenced task, workflow, repository and
conversation to be in the token's workspace; anything else returns not found.
Payload identifiers cannot override the signed scope. Writes carry the
`X-Kandev-Run-Id` header, and a write from a run whose conversation has since
accepted a newer message returns 409 `intent_superseded`.

The coordinator reaches these routes only through the `kandev_orchestrator` MCP
server that `agentctl` attaches to its session. It has no shell, plugin tools
or other MCP servers. Tool arguments are `id` for routes with `:id`, `query` for
query parameters and `request` for the JSON body.

## Broker tools

| Tool | Route | Purpose |
| --- | --- | --- |
| `workspace` | `GET /runtime/workspace` | Workflows, steps, repositories, execution profiles and templates |
| `manage_workspace` | `POST /runtime/workspace/manage` | Workspace, workflow, step and repository configuration |
| `workspace_tasks` | `GET /runtime/tasks` | Paged task summaries (`after`, `limit`; follow `next_cursor`) |
| `task_details` | `GET /runtime/tasks/:id/details` | Task, sessions and result previews (`include_result=false` omits previews) |
| `task_content` | `GET /runtime/tasks/:id/content` | Paged description, messages and tool failures |
| `task_permissions` | `GET /runtime/tasks/:id/permissions` | Pending permission requests and questions (optional `session_id`) |
| `comments` | `GET /tasks/:id/comments` | The coordinator's conversation history |
| `metrics` | `GET /runtime/metrics` | Delegated outcomes over 7 or 30 days (`days`) |
| `task_proposals` | `GET /runtime/proposals` | This coordinator's task proposals (`status`: `pending` or `all`) |
| `capabilities` | `GET /runtime/capabilities` | The tools available to this coordinator |
| `memory` | `GET /runtime/memory` | This coordinator's memory (optional `memory_id`, `layer`, `key`) |
| `remember` | `POST /runtime/memory` | Save `layer`, `key`, `content` |
| `forget` | `DELETE /runtime/memory/:id` | Delete one memory entry |
| `create_task` | `POST /runtime/tasks` | Create a delegated task |
| `manage_task` | `POST /runtime/tasks/:id/manage` | Task actions (below) |
| `task_status` | `POST /runtime/tasks/:id/status` | `status`: `todo`, `in_progress`, `in_review`, `done` |
| `update_source_issue` | `POST /runtime/tasks/:id/source-issue` | Comment on or move the task's Jira or Linear issue |
| `comment` | `POST /runtime/comments` | Add an internal receipt to this conversation |

`create_task` requires `title` (60 characters or fewer; longer titles are
rejected) and accepts `description`, `workflow_id`, `workflow_step_id`,
`repository_id`, `parent_id`, `assignee` (execution profile ID),
`execution_mode`, `external_id`, `source` and `acceptance_criteria` (up to 10
short outcomes of up to 300 characters). A repeated `external_id` in the same
workspace returns the existing task.

When the orchestrator asks before creating tasks, `create_task` creates nothing.
It stores a proposal, posts it in the conversation (the comment id is the
proposal id and its `source` is `proposal`) and returns 202 `{proposal_id,
status: "pending"}`. The same request in the same turn returns the same
proposal, and a pending proposal for the same source issue is returned with
`duplicate_proposal: true`. The coordinator is woken with the user's decision.

`manage_task` actions:

| Action | Fields |
| --- | --- |
| `edit` | Optional `title`, `description`, `priority`, `parent_id` (empty removes nesting) |
| `move` | `workflow_step_id`, optional `workflow_id`, `position` |
| `assign` | `assignee` execution profile ID |
| `adopt`, `start`, `stop`, `archive`, `delete` | None; `start` and `stop` accept `session_id` |
| `message` | `prompt`, optional `session_id` |
| `repair_session` | Optional `session_id`; only for a session stopped on a provider login or OAuth refresh failure |
| `session_mode` | `session_id`, `mode`: `default`, `acceptEdits` or `auto` |
| `resolve_permission` | `session_id`, `request_id`, `pending_id`, `option_id` from `task_permissions` |
| `answer_question` | `session_id`, `pending_id`, and `answers` (`question_id` with `selected_options` and/or `custom_text`) or `reject` |
| `set_criteria` | `acceptance_criteria`; replaces the list and resets every criterion to unverified; an empty list clears it only when every current criterion is met (409 `acceptance_criteria_unmet` otherwise) |
| `verify_criteria` | `criteria`: `id`, `met` and `evidence` (up to 1000 characters) per checked criterion; all or nothing |

Use `move` for board progression; `task_status` changes native status under the
normal completion gates. Read `task_details` to verify the result. Bypass
permission modes are unavailable, and `resolve_permission` accepts only
`allow_once` or `reject_once` options. `session_mode`, `resolve_permission` and
`answer_question`, `set_criteria` and `verify_criteria` work only on tasks the
coordinator created or adopted. `task_status done` on a task with acceptance
criteria returns 409 `acceptance_criteria_unmet` with the `unmet` list until
every criterion is recorded as met; a `move` into a step that completes its task
is refused with the same 409.
Answers and permission decisions apply only to the exact live request; a stale
or replaced request returns 409.

`manage_workspace` takes `resource` (`workspace`, `workflow`, `step`,
`repository`), `action` (`create`, `update`, `delete`, `reorder`), optional `id`
and `configuration`. It is limited to the assigned workspace; membership, global
settings, hidden system workflows and GitHub-synced workflow definitions are
rejected.

Writes are never retried automatically. After a transport failure the tool
reports an unknown outcome; inspect native state before trying again. An empty
HTTP error is reported with its status code.

## Callbacks

A delegated task queues one turn in its coordinator's conversation when it
enters REVIEW, COMPLETED, FAILED, WAITING_FOR_INPUT or BLOCKED, when it raises a
question or permission request, and when it stalls. The turn's prompt contains a
`callback` object with `task_id`, `title` and `state`, plus the pending request
summary, or `state: "STALLED"` with an `outcome` of `no_progress`,
`never_started` or `orphaned`. Callbacks are deduplicated per transition,
request or stall episode, and only the delegating coordinator receives them.

A turn also carries `proposal_decisions` after the user approves (optionally
with edits) or dismisses a task proposal, and a `source_writeback_error` on a
task update when an automatic tracker comment or move failed. Automatic
write-back happens once per transition into REVIEW or COMPLETED for a task with
a source issue, following the orchestrator's settings. The latest stall episode
and the acceptance criteria are stored on the task as `orchestration_stall` and
`orchestration_goal` metadata.
