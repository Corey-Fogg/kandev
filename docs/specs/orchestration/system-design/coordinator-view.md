---
status: current
system: orchestration
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-001
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-002
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-003
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-004
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-005
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-006
---

# Coordinator view system design

## Context and boundary

Orchestration owns this vertical view; canonical task state stays in
`internal/task`, including its `statussummary` projector. The coordinator's
own linked-task query (`OrchestratedTasks` in
`internal/orchestration/repository/sqlite`) returns only linked ID, title and
state rows and is not a source for the all-workspace view.

The view adds no schema and no plugin SDK surface. Conversation, runtime,
role and assignment contracts are described in the
[coordinator design](coordinators.md).

## Requirement mapping

| Requirement                            | Design sections                    |
| -------------------------------------- | ---------------------------------- |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-001 | Task data and paging; Presentation |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-002 | Group projection                   |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-003 | Conversation and navigation        |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-004 | Freshness and recovery             |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-005 | Presentation                       |
| REQ-ORCHESTRATION-COORDINATOR-VIEW-006 | Authority and scope                |

## Task data and paging

The view reads `GET /api/v1/workspaces/:id/tasks?view=kanban` through
`apps/web/lib/api/domains/kanban-api.ts:listTasksByWorkspace`. The handler has
bounded page/page_size (maximum 100), search, workflow/repository filters and
batched session/status enrichment. `view=kanban` selects a native
task-repository query that excludes hidden, configuration, conversation and
Office workflows before counting, searching and paging. Callers that omit
`view=kanban` keep their existing contract. Text search in this view does not
add the command palette's pull-request-number results. `include_ephemeral` and
`include_archived` stay false.

Read task `status_summary`, workflow/step, identifier, profile, repositories and
coordinator-link metadata. Reuse native task mapping and status presentation
helpers. PR/diff/last-activity values come from existing `TaskStatusSummary` fields;
never scrape chat or issue prompts. Do not request per-task transcripts, tool
payloads or full session timelines merely to draw rows.

The page-scoped hook loads 100 rows initially and exposes Load more. Dedupe by
task ID, cancel outstanding loads on workspace/filter change, and retain
server-supported deterministic sorting. Search/workflow/repository filters run
at the server. Selected-coordinator/group filters apply to loaded eligible rows,
with explicit partial coverage until all matching pages have been read. A small
result set can complete in the initial read; larger sets never silently stop
at 100 tasks.

The header distinguishes loaded eligible tasks from the underlying task-list
total and says “counts for loaded tasks” until complete. If canonical exclusions
reduce a page, do not mislabel the unfiltered total as a Kanban/group total.
Counts of open PRs or changed files are likewise sums for loaded rows with known
values, with unknown coverage shown. The view does not promise an
atomic workspace-wide statistics snapshot. It must not present complete-sounding
totals or a definitive empty group while unseen pages may contain matches.

## Group projection

`lib/orchestration/coordinator-task-groups.ts` is a pure, tested projection over
task DTOs plus the freshest canonical status summaries. Rows retain all relevant badges; this table only selects their
one primary group, evaluated in order:

| Order | Canonical signal                                                    | Group       |
| ----- | ------------------------------------------------------------------- | ----------- |
| 1     | Task-wide pending clarification or permission                       | Needs input |
| 2     | Active error, failed launch/interruption, FAILED or BLOCKED task    | Problems    |
| 3     | Live generating/background activity or active execution session     | Running     |
| 4     | REVIEW task, required review, or PR attention without higher signal | Review      |
| 5     | COMPLETED task with no higher signal                                | Done        |
| 6     | CREATED/TODO/SCHEDULING, queued step admission or pending prompts   | Queued      |
| 7     | Remaining states, including CANCELLED or insufficient evidence      | Other       |

Prefer the task-wide pending action projection over a primary-session shortcut:
another session may be waiting. Reuse native error acknowledgement/dismissal
semantics. WAITING_FOR_INPUT without a current canonical request is Other with
its raw status, not a fabricated question. An idle IN_PROGRESS task remains
Other unless a native activity signal explains it; parked background work keeps
its existing explanatory badge. Missing summaries permit documented coarse
fallbacks but never an inferred healthy/stalled state.

Display quiet time only from semantic `last_activity_at`, not projection
`updated_at` or wall-clock guesswork. Do not create a new inactivity threshold.
PR merged state remains a PR badge; native task completion determines Done.
This projection is presentation state and is not persisted.

## Conversation and navigation

The SPA route `/workspaces/:workspaceId/coordinator` (`app/coordinator`) is
reachable from workspace navigation and gated by `features.orchestration`. The
selected assignment is carried as `orchestratorId` in the route query and
validated against the workspace's assignments by `use-coordinator-selection`.
The view uses a previously valid selection for that workspace, or the sole
assignment; otherwise it shows an explicit selector. It never falls back to
another account when an assignment becomes unavailable.

The `/workspace/conversations/:taskId` conversation route and the Orchestration
settings pages keep working and link into the central view with the same
workspace and assignment identity. Global role settings and workspace
assignment settings remain the configuration owners.

`coordinator-chat.tsx` reuses `ConversationContent` from
`app/settings/orchestration/conversation-route.tsx`, `TaskChat`, identity,
comment/recovery transports, active-session context and streaming
reconciliation, without a second routed page, header or scroll owner.

Opening an assignment may ensure its deterministic existing conversation mapping;
it must not queue a turn. Hide/show chat and mobile tab changes keep the composer
mounted or preserve drafts in memory keyed by workspace and assignment. Switching
assignment clears the visible old content before loading the next. Do not persist
draft text in a new localStorage key or expose it to task filters/report generation.

The view offers observation, chat and links to existing task controls.
It has no new Sweep now/Run build action. Existing automation Run now remains
available in Automations, with its established dispatch semantics.

## Freshness and recovery

The view integrates with the canonical task cache and WebSocket
status/lifecycle handlers and does not assume the active board cache contains
every workflow on this page. Task create/update/delete, status summary, pending
input, PR and connection events refresh or invalidate its loaded pages.
Read invalidations are coalesced; no agent is polled and no model is launched
for status.

Use `pickFreshestStatusSummary`/`isNewerStatusSummary` for HTTP/WS races, preserving
equal-revision queue-count refresh behavior. Keep an explicit workspace generation
with abort signals and stale-response rejection; the plugin's workspace guard is
a useful pattern, not a reason to copy its separate state store.

On reconnect, refresh the loaded window and reconcile changed/deleted tasks.
On a failed read, retain only same-workspace rows marked stale and offer Retry;
do not turn the failure into zero counts. Scope/access changes clear rows/chat.
Refresh is a read and never retries a coordinator message or automation dispatch.

## Presentation

Desktop has a workspace header/assignment selector, task summary/filter bar and
two panes. Task groups occupy the main pane; chat is a resizable or fixed-width
side pane using existing layout primitives. Group headings expose count/coverage,
and rows link to native task pages and known PRs. Keyboard focus survives refresh.

At narrow widths, Tasks/Chat tabs replace the split view. Use one scroll owner per
visible pane, accessible selected-tab labels, readable task cards, safe-area
padding and existing touch target conventions. No essential action depends on
hover. Empty state differentiates no assignments, no tasks, no filter matches and
unavailable data. Hidden chat must not send or lose a draft.

Illustrative layout (design only; not a feature screenshot):

```text
Workspace / Coordinator                 [assignment] [settings]
[counts for loaded tasks] [coverage / stale indicator]
[search] [workflow] [repository] [all / coordinated]
+--------------------------------------+----------------------+
| Needs input / Problems / Running ... | Selected coordinator |
| Task · Step · Activity · PR · Diff    | Persistent chat      |
| [load more]                          | [message composer]   |
+--------------------------------------+----------------------+
Mobile: [Tasks] [Chat], same selection and state
```

## Authority and scope

Workspace authorization stays in the backend task and orchestration services.
Hidden conversation records are excluded at the task query boundary, not
masked in the browser. Regression tests cover conversation exclusion,
cross-workspace access, pagination and feature-off paths. No unscoped task
query is used to obtain summary totals.

Task observation does not set `orchestration_chief_id`, export context to a
different profile or grant cross-workspace access. Pending-input links open the
native task interface. The coordinator's own question and permission relay is
described in [coordinator assistance](coordinator-assistance.md).

## Persistence, observability and validation

No durable tables or migrations. Local view preferences use existing
preference patterns for non-content values only.

Failures use existing task/API diagnostics; UI diagnostics can include
workspace/task IDs and summary revisions, never chat bodies, prompts or secrets.
Unit tests cover classification, paging and races
(`lib/orchestration/coordinator-task-groups.test.ts`,
`coordinator-task-totals.test.ts`, `app/coordinator/coordinator-page.test.tsx`);
native task authorization tests cover server exclusions; browser tests cover
tasks plus conversation on desktop and mobile, navigation, reconnect and
feature gates.

## Related decisions

- [Workspace orchestration owns its coordination runtime](../../../decisions/2026-09-07-workspace-orchestration.md).
- [One coordinator path](../../../decisions/2026-09-18-orchestrator-product-boundary.md).
