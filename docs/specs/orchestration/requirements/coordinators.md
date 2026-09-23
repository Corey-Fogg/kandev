---
status: active
system: orchestration
created: 2026-09-23
owners:
  - Kandev
---

# Coordinator Requirements

## Overview

A person running several Kanban tasks in a workspace needs a conversational
coordinator that can plan, delegate and follow up without a separate board,
workflow engine or worker persona set. Orchestration owns coordinator roles,
their workspace assignments, the persistent conversation with each assignment
and the security boundary that every coordinator turn runs inside.

## Terminology

- **Role:** A global, reusable coordinator template with a name, icon and
  behavioral instructions. A role contains no model, account or credential.
- **Assignment:** A role placed in one workspace with an execution profile, an
  executor and workspace context. People often call an assignment "the
  coordinator".
- **Conversation:** The persistent chat between workspace members and one
  assignment. It is stored as a hidden conversation task, not a delivery task.
- **Turn:** One coordinator run triggered by a user message, a task callback or
  an automation delivery.
- **Broker:** The Kandev-owned MCP server that is the coordinator's only tool
  source.

## Requirements

### REQ-ORCHESTRATION-COORDINATOR-001: Roles and workspace assignments

**Intent:** Configure coordinator behavior once and reuse it across workspaces
and accounts.

**User story:** As a workspace administrator, I want to add a coordinator by
choosing a role and an existing execution profile, so that I do not duplicate
agent configuration.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-001.1:** When an administrator saves a role,
  the system shall store its name, icon and instructions globally, and every
  assignment of that role shall use the saved instructions from its next turn.
  A turn that is already running shall keep the instructions it started with.
- **AC-ORCHESTRATION-COORDINATOR-001.2:** When a role has at least one
  assignment, the system shall reject deleting that role. The built-in Chief of
  staff role shall be editable and shall not be deletable.
- **AC-ORCHESTRATION-COORDINATOR-001.3:** When an administrator adds an
  assignment, the system shall require a role, an enabled execution profile and
  an executor, and shall accept workspace context and delegation guidance. It
  shall not create a workspace, workflow or delivery task.
- **AC-ORCHESTRATION-COORDINATOR-001.4:** A workspace shall accept any number of
  assignments. The same role can run under different execution profiles in
  different workspaces, and each assignment shall keep its own conversation,
  memory, delegated tasks and pause state.
- **AC-ORCHESTRATION-COORDINATOR-001.5:** When an assignment is paused, the
  system shall start no new turn for it and shall not interrupt a running turn.
  Resuming shall not replay callbacks that were rejected while paused.
- **AC-ORCHESTRATION-COORDINATOR-001.6:** When an assignment is deleted, the
  system shall stop its conversation sessions and keep its delegated tasks on
  their boards.

### REQ-ORCHESTRATION-COORDINATOR-002: Persistent coordinator conversation

**Intent:** Keep one continuous, workspace-visible conversation per assignment.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-002.1:** Opening an assignment's conversation
  shall return the same conversation every time and shall not send a prompt or
  start an agent.
- **AC-ORCHESTRATION-COORDINATOR-002.2:** Every member with read access to the
  workspace shall be able to read and post in the conversation. Conversations
  created before this contract, including ones that recorded an owning user,
  shall remain readable by workspace members.
- **AC-ORCHESTRATION-COORDINATOR-002.3:** When a client posts a message with a
  `client_message_id`, the system shall accept it durably before any run exists.
  An identical retry shall return the original receipt, and a different body
  under the same identifier shall be rejected as a conflict.
- **AC-ORCHESTRATION-COORDINATOR-002.4:** Accepted messages shall start turns in
  acceptance order. A later message shall not overtake a turn that is queued,
  running or waiting for automatic recovery.
- **AC-ORCHESTRATION-COORDINATOR-002.5:** The coordinator's final reply of each
  turn shall appear in the conversation exactly once, including after event
  redelivery.
- **AC-ORCHESTRATION-COORDINATOR-002.6:** The conversation shall show chat and
  execution feedback, and shall not show delivery-task properties such as
  labels, reviewers, blockers or workflow step.

### REQ-ORCHESTRATION-COORDINATOR-003: Turn recovery

**Intent:** Recover coordinator turns from transient provider failures without
repeating effects.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-003.1:** When a turn fails with a transient
  provider error before it produced output or called a tool, the system shall
  retry it automatically up to five times, waiting 15, 30, 60, 60 and 60
  seconds, with fresh runtime credentials and the same execution profile.
- **AC-ORCHESTRATION-COORDINATOR-003.2:** When a failed turn produced output,
  called a tool, has unknown effects, or failed on authentication that needs a
  person, the system shall not retry it automatically and shall show the
  failure with a manual retry control.
- **AC-ORCHESTRATION-COORDINATOR-003.3:** When a person selects Cancel or Stop
  on a turn with a pending retry, the system shall retire that retry.
- **AC-ORCHESTRATION-COORDINATOR-003.4:** After a backend restart, the system
  shall mark interrupted turns failed and shall not replay them. Retries that
  were already scheduled shall survive the restart.
- **AC-ORCHESTRATION-COORDINATOR-003.5:** A coordinator turn shall never fall
  back to a different execution profile or account.

### REQ-ORCHESTRATION-COORDINATOR-004: Restricted coordinator runtime

**Intent:** Bound what a coordinator can do to the workspace it is assigned to.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-004.1:** Every coordinator session shall
  receive the Kandev broker as its only MCP server. Plugin tools, profile MCP
  servers and Kandev's shell tool shall not be attached, whichever provider the
  execution profile uses.
- **AC-ORCHESTRATION-COORDINATOR-004.2:** When a coordinator runs on Claude, the
  session shall also disable provider built-in tools and allow only the
  broker's tools.
- **AC-ORCHESTRATION-COORDINATOR-004.3:** Every broker call shall be authorized
  by a runtime credential bound to the assignment, its workspace, the current
  run and the current session. A credential from a finished run, another
  session or another workspace shall be rejected.
- **AC-ORCHESTRATION-COORDINATOR-004.4:** When a broker call names a task,
  workflow, repository or conversation outside the assignment's workspace, the
  system shall reject it without revealing whether the resource exists.
- **AC-ORCHESTRATION-COORDINATOR-004.5:** When a coordinator session is resumed
  or steered, the system shall apply the same broker-only restriction that the
  session started with.

### REQ-ORCHESTRATION-COORDINATOR-005: Feature gate

**Intent:** Keep the experimental feature off until an operator opts in.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-005.1:** `features.orchestration`
  (`KANDEV_FEATURES_ORCHESTRATION`) shall default off in every shipped runtime
  profile and shall require a restart when changed.
- **AC-ORCHESTRATION-COORDINATOR-005.2:** When the feature is off, the system
  shall hide coordinator navigation, reject orchestration and runtime routes,
  and start no coordinator turn, callback or automation delivery.
- **AC-ORCHESTRATION-COORDINATOR-005.3:** Turning the feature off shall keep
  saved roles, assignments, conversations and memory unchanged.
- **AC-ORCHESTRATION-COORDINATOR-005.4:** The feature shall work with Office
  disabled, and Office routes shall not operate on coordinator assignments or
  conversations.

## Out of scope

- Per-user private coordinators, execution modes or objective tracking. Every
  conversation is workspace-scoped.
- Cross-workspace access from one coordinator.
- A scheduler. Recurring prompts use Automations with a coordinator target.
- Provider or account selection beyond the assignment's execution profile.
