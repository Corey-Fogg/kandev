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

The page supplies `page_size=100`; `page`, `query`, `workflow_id` and
`repository_id` use the existing task-list query contract. Text search in this
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
| `capabilities` | `GET /runtime/capabilities` | The tools available to this coordinator |
| `memory` | `GET /runtime/memory` | This coordinator's memory (optional `memory_id`, `layer`, `key`) |
| `remember` | `POST /runtime/memory` | Save `layer`, `key`, `content` |
| `forget` | `DELETE /runtime/memory/:id` | Delete one memory entry |
| `create_task` | `POST /runtime/tasks` | Create a delegated task |
| `manage_task` | `POST /runtime/tasks/:id/manage` | Task actions (below) |
| `task_status` | `POST /runtime/tasks/:id/status` | `status`: `todo`, `in_progress`, `in_review`, `done` |
| `comment` | `POST /runtime/comments` | Add an internal receipt to this conversation |

`create_task` requires `title` (60 characters or fewer; longer titles are
rejected) and accepts `description`, `workflow_id`, `workflow_step_id`,
`repository_id`, `parent_id`, `assignee` (execution profile ID),
`execution_mode` and `external_id`. A repeated `external_id` in the same
workspace returns the existing task.

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

Use `move` for board progression; `task_status` changes native status under the
normal completion gates. Read `task_details` to verify the result. Bypass
permission modes are unavailable, and `resolve_permission` accepts only
`allow_once` or `reject_once` options. `session_mode`, `resolve_permission` and
`answer_question` work only on tasks the coordinator created or adopted.
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
