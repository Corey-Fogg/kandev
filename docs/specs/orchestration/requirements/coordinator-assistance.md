---
status: active
system: orchestration
created: 2026-09-23
owners:
  - Kandev
---

# Coordinator Assistance Requirements

## Overview

Delegated work often stops on a question or a permission request. The person
who asked for the work is talking to the coordinator, not watching every task.
The coordinator therefore relays those requests into its conversation and can
resolve them within the authority the person gave it. It also keeps a small,
workspace-scoped memory of standing preferences so each turn starts with them.

The task system owns every pending question and permission request. This
document owns how a coordinator learns about them, lists them and answers them,
and how coordinator memory is written, forgotten and used.

## Terminology

- **Pending question:** A clarification request from a delegated task's agent
  that is still waiting for an answer.
- **Pending permission:** A live tool-permission request that a delegated
  task's agent is waiting on.
- **Relay:** Waking the coordinator with the pending request and letting it
  answer, resolve or ask the person in chat.
- **Memory entry:** A short keyed note stored for one assignment and injected
  into its turns.

## Requirements

### REQ-ORCHESTRATION-COORDINATOR-ASSIST-001: Question and permission relay

**Intent:** Keep delegated work moving without the person opening every task.

**User story:** As a workspace member, I want the coordinator to tell me when a
delegated task is waiting on a decision, and to handle the ones I already
covered, so that work does not sit blocked.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.1:** When a delegated task's
  session raises a question or a permission request, the system shall wake the
  delegating coordinator with a callback that identifies the task and summarizes
  the pending request.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.2:** The coordinator shall be able to
  list a delegated task's pending questions and pending permissions, with the
  session, request identifiers and offered options needed to resolve each one.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.3:** When the coordinator answers a
  pending question with `manage_task` action `answer_question`, the system shall
  resolve only that exact session and request through the native clarification
  service, and the task history shall attribute the answer to the coordinator.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.4:** When the coordinator resolves a
  pending permission, the system shall accept only `allow_once` or `reject_once`
  for that exact live request and shall create no persistent allow rule.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.5:** The coordinator shall not be
  able to select a permission-bypass mode for any session.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.6:** When a question or permission
  was already answered, expired or replaced, a coordinator response shall return
  a conflict and shall not resolve a newer request.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.7:** An idle or quiet session with no
  pending request shall not be reported as waiting for input.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-001.8:** A person shall always be able to
  answer the same request on the native task page, and both paths shall converge
  on one resolution.

### REQ-ORCHESTRATION-COORDINATOR-ASSIST-002: Workspace coordinator memory

**Intent:** Carry standing preferences between turns without a transcript.

#### Acceptance criteria

- **AC-ORCHESTRATION-COORDINATOR-ASSIST-002.1:** The coordinator shall be able to
  write a keyed memory entry and forget an entry through broker tools. Entries
  shall belong to the calling assignment in its workspace.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-002.2:** Each turn's prompt shall include
  the assignment's memory within a fixed size budget, and shall say how many
  entries were left out when the budget is exceeded.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-002.3:** A coordinator shall not read,
  write or forget another assignment's memory, including an assignment in the
  same workspace.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-002.4:** Forgetting an entry shall remove
  it from later turns. It does not remove text already sent to a provider or
  recorded in conversation history.
- **AC-ORCHESTRATION-COORDINATOR-ASSIST-002.5:** Memory shall be treated as
  context, not authorization. A memory entry shall not widen what any broker
  call is allowed to do.

## Out of scope

- A separate attention ledger or inbox outside the coordinator conversation.
- Automatic answers that the coordinator's instructions and the conversation do
  not support.
- Resolving authentication or sign-in prompts on a person's behalf.
- User-, project- or task-scoped memory, memory confirmation workflows and
  memory shared between assignments.
