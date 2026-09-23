---
status: active
system: orchestration
created: 2026-09-23
owners:
  - Kandev
---

# Delegation Requirements

## Overview

A coordinator turns a conversation into ordinary Kanban work. It creates or
adopts tasks, assigns them to existing execution profiles, steers them, and is
woken when a delegated task needs attention. Orchestration owns the broker
operations the coordinator uses and the callbacks it receives. The task system
remains the owner of every task, session, workflow move and completion gate.

## Terminology

- **Delegated task:** A task whose orchestration metadata names a coordinator
  assignment, because the coordinator created or adopted it.
- **Callback:** A turn queued into a coordinator's conversation because one of
  its delegated tasks changed.
- **Stall:** A delegated task's session that the task system reports as making
  no progress, never started, or orphaned.

## Requirements

### REQ-ORCHESTRATION-DELEGATION-001: Task delegation through the broker

**Intent:** Let the coordinator do what a person does on the board, through
native task services.

**User story:** As a workspace member, I want to ask the coordinator to create
and run tasks, so that I can steer work from one conversation.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-001.1:** The coordinator shall be able to read
  the workspace's workflows, steps, repositories and execution profiles, list
  workspace tasks in pages, and read a task's details and bounded content.
- **AC-ORCHESTRATION-DELEGATION-001.2:** When the coordinator creates a task, the
  system shall create an ordinary board task in the assignment's workspace,
  record the coordinator as its delegating assignment and use the execution
  profile the coordinator named. A title longer than 60 characters shall be
  rejected as a definite error.
- **AC-ORCHESTRATION-DELEGATION-001.3:** When a create request repeats an
  `external_id` already used in the workspace, the system shall return the
  existing task instead of creating a second one.
- **AC-ORCHESTRATION-DELEGATION-001.4:** The coordinator shall be able to edit,
  move, assign, adopt, start, stop, message, archive and delete a workspace task,
  and change its status. Each action shall use the native task service and its
  workflow, manual-move, active-session and review rules.
- **AC-ORCHESTRATION-DELEGATION-001.5:** When the coordinator adopts an existing
  task, the system shall keep the task's assigned execution profile. The profile
  shall change only through an explicit assign action.
- **AC-ORCHESTRATION-DELEGATION-001.6:** When the coordinator sets a task to
  done, the task system's completion gates, including the managed-parent
  completion guard, shall apply unchanged.
- **AC-ORCHESTRATION-DELEGATION-001.7:** When a broker write loses its response,
  the coordinator shall receive an unknown-outcome error and the broker shall
  not retry the write.

### REQ-ORCHESTRATION-DELEGATION-002: Session repair

**Intent:** Recover delegated work that stopped on a provider login failure.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-002.1:** When a delegated session stopped on a
  provider login or OAuth refresh failure, the `repair_session` action shall
  clear the stale account lock and resume the same session.
- **AC-ORCHESTRATION-DELEGATION-002.2:** When the session stopped for any other
  reason, `repair_session` shall refuse and change nothing.

### REQ-ORCHESTRATION-DELEGATION-003: Workspace administration

**Intent:** Let the coordinator maintain the configuration of its own workspace.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-003.1:** The coordinator shall be able to update
  the assigned workspace's settings, create, update, delete and reorder its
  delivery workflows and workflow steps, and register, update and remove its
  repositories, through the native services that own them.
- **AC-ORCHESTRATION-DELEGATION-003.2:** The system shall reject administration
  of other workspaces, workspace membership, installation-wide settings, hidden
  system workflows and workflow definitions synced from a source repository.

### REQ-ORCHESTRATION-DELEGATION-004: Task callbacks

**Intent:** Wake only the coordinator that owns a task, only when there is
something to act on.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-004.1:** When a delegated task enters REVIEW,
  COMPLETED, FAILED, WAITING_FOR_INPUT or BLOCKED, the system shall queue one
  callback turn in the delegating assignment's conversation.
- **AC-ORCHESTRATION-DELEGATION-004.2:** Other coordinators in the same
  workspace shall not receive that task's callbacks. A coordinator's own
  conversation task shall never produce a callback.
- **AC-ORCHESTRATION-DELEGATION-004.3:** Duplicate events for one committed
  transition shall produce one callback. Callbacks for different tasks shall
  remain separate turns.
- **AC-ORCHESTRATION-DELEGATION-004.4:** The callback shall identify the task,
  its title and its state. The coordinator's instructions shall say that review
  is not completion.
- **AC-ORCHESTRATION-DELEGATION-004.5:** When the assignment is paused or the
  feature is off, the system shall not queue callbacks.

### REQ-ORCHESTRATION-DELEGATION-005: Stall callbacks

**Intent:** Tell the coordinator when a delegated task stopped moving, and why.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-005.1:** When the task system reports a stalled
  agent or task for a delegated task, the system shall queue one callback with
  state `STALLED` and an outcome of `no_progress`, `never_started` or
  `orphaned`.
- **AC-ORCHESTRATION-DELEGATION-005.2:** One stall episode shall produce one
  callback, however many times its event is delivered.
- **AC-ORCHESTRATION-DELEGATION-005.3:** Quiet time alone shall not produce a
  stall callback. Only a stall signal published by the task system shall.
- **AC-ORCHESTRATION-DELEGATION-005.4:** The system shall record the latest
  stall episode (outcome, duration, session and detection time) on the task,
  also while the assignment is paused, so the Coordinator view can show it.

### REQ-ORCHESTRATION-DELEGATION-006: Automation delivery

**Intent:** Run recurring prompts through an existing coordinator.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-006.1:** When an automation targets a
  coordinator assignment, the system shall queue its prompt in that
  assignment's conversation, using the assignment's current role, execution
  profile, context and memory.
- **AC-ORCHESTRATION-DELEGATION-006.2:** A `dispatched` automation run shall mean
  the prompt was queued, not that the requested work finished. The run shall
  link to the conversation, and deleting automation history shall not delete
  the conversation.
- **AC-ORCHESTRATION-DELEGATION-006.3:** When the target is paused, deleted or
  disabled, the automation run shall record a delivery failure and shall not
  fall back to another assignment or account. A repeated firing shall be
  deduplicated.

### REQ-ORCHESTRATION-DELEGATION-007: Task proposals

**Intent:** Let a person approve the tasks a coordinator wants to create.

**User story:** As a workspace member, I want the coordinator to propose tasks
in chat when I ask it to, so that nothing starts without my approval.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-007.1:** When the assignment asks before
  creating tasks, `create_task` shall store a proposal, show it in the
  conversation and create no task. When the setting is off, `create_task` shall
  behave as before.
- **AC-ORCHESTRATION-DELEGATION-007.2:** The same request in the same turn shall
  return the same proposal. A pending proposal for the same source issue shall
  be returned instead of a new one, and a source issue that already has a task
  shall return that task.
- **AC-ORCHESTRATION-DELEGATION-007.3:** Approving shall create the task exactly
  as proposed, or with the person's edits, and start it as `create_task` would.
  Approving again shall return the same task and create no other.
- **AC-ORCHESTRATION-DELEGATION-007.4:** Dismissing shall record the decision
  and an optional reason. A dismissed proposal cannot be approved and an
  approved one cannot be dismissed. An invalid edit or a failed create shall
  leave the proposal pending.
- **AC-ORCHESTRATION-DELEGATION-007.5:** After each decision the coordinator
  shall be woken with the outcome: approved with the task, approved with
  edits, or dismissed with the reason. A paused coordinator shall not be woken
  and the decision shall stand.
- **AC-ORCHESTRATION-DELEGATION-007.6:** Reading proposals shall need workspace
  read access; deciding shall need workspace-manage access.

### REQ-ORCHESTRATION-DELEGATION-008: Acceptance criteria

**Intent:** Make "done" mean the agreed outcomes were checked.

#### Acceptance criteria

- **AC-ORCHESTRATION-DELEGATION-008.1:** A delegated task, or a proposal, shall
  accept up to 10 acceptance criteria of 1 to 300 characters each.
- **AC-ORCHESTRATION-DELEGATION-008.2:** The coordinator shall record, per
  criterion, whether it is met with evidence of at most 1,000 characters. A
  request with an unknown criterion, a missing verdict or missing evidence
  shall change nothing. Only the task's delegating coordinator can set or
  verify its criteria.
- **AC-ORCHESTRATION-DELEGATION-008.3:** Setting a task done through the broker
  shall be refused, listing the unmet criteria, until every criterion is met.
- **AC-ORCHESTRATION-DELEGATION-008.4:** Task updates, task details and the
  Coordinator view shall show the criteria and their status.

## Out of scope

- A second task engine, board or copy of task state.
- Automatic adoption of tasks the coordinator did not create or adopt.
- Permission modes that bypass native permission review.
- Tracker write-back and cross-path intake deduplication; see
  [tracker intake](tracker-intake.md).
- Gating workflow-driven completion on acceptance criteria. Only the
  coordinator's `task_status done` is gated.
