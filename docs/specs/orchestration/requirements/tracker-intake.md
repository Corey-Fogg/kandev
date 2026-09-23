---
status: active
system: orchestration
created: 2026-09-23
owners:
  - Kandev
---

# Tracker Intake Requirements

## Overview

Work often starts as a Jira or Linear issue. The same issue can reach a
workspace through an integration watch, an automation and a coordinator
conversation. Each path must converge on one task, and the coordinator must be
able to report progress back on the issue it is working from.

Orchestration owns the coordinator broker operations. The
[integrations system](../../integrations/README.md) owns the tracker clients,
watches and issue metadata that those operations use.

## Terminology

- **Source issue:** The tracker issue recorded in a task's integration
  metadata.
- **Tracker write-back:** A comment or state change that the coordinator posts
  on a task's source issue.

## Requirements

### REQ-ORCHESTRATION-TRACKER-001: Tracker write-back

**Intent:** Report delegated progress where the request came from.

#### Acceptance criteria

- **AC-ORCHESTRATION-TRACKER-001.1:** When the coordinator requests write-back
  for a delegated task, the system shall post a comment of at most 4,000
  characters on that task's source issue, and can move the issue to a
  `started`, `review` or `done` state category.
- **AC-ORCHESTRATION-TRACKER-001.2:** The system shall resolve the source issue
  only from the task's own metadata. A request shall not be able to name an
  arbitrary issue.
- **AC-ORCHESTRATION-TRACKER-001.3:** When the task has no source issue, or the
  tracker offers no transition to the requested state category, the system
  shall reject the request, list the available transitions when known, and
  change nothing.
- **AC-ORCHESTRATION-TRACKER-001.4:** Apart from the automatic write-back in
  `REQ-ORCHESTRATION-TRACKER-003`, write-back shall happen only when the
  coordinator requests it.

### REQ-ORCHESTRATION-TRACKER-002: Intake deduplication

**Intent:** One tracker issue yields one task, whichever path saw it first.

#### Acceptance criteria

- **AC-ORCHESTRATION-TRACKER-002.1:** When the coordinator creates a task with a
  source issue, the system shall record the same tracker metadata that an
  integration watch records, so branch naming and write-back behave the same.
- **AC-ORCHESTRATION-TRACKER-002.2:** When a non-archived task in the workspace
  already has the same source issue, the system shall return that task and
  mark the response as a duplicate instead of creating a second task.

### REQ-ORCHESTRATION-TRACKER-003: Automatic write-back

**Intent:** Keep the source issue current without the coordinator spending a
turn on it.

#### Acceptance criteria

- **AC-ORCHESTRATION-TRACKER-003.1:** When a delegated task with a source issue
  enters review or completes, and the assignment's automatic comment setting is
  on (the default), the system shall post one short comment naming the task,
  its pull request when known and its acceptance criteria progress.
- **AC-ORCHESTRATION-TRACKER-003.2:** When such a task completes and the
  assignment's automatic move setting is on (off by default), the system shall
  also move the issue to the done category.
- **AC-ORCHESTRATION-TRACKER-003.3:** Each transition shall write at most once.
  A redelivered event shall write nothing; leaving review and returning shall
  write again. Turning a setting on shall not post past transitions. A task
  that was already in review or complete when the system first observes it
  (for example, after an upgrade) shall not be written until it changes state.
- **AC-ORCHESTRATION-TRACKER-003.4:** When the write fails, the coordinator
  shall be told once, including when the tracker call times out. When the
  tracker integration is not configured, the
  system shall skip silently. A paused assignment's tasks shall not be written.

## Out of scope

- Automatic tracker comments on state changes other than review and
  completion.
- Trackers other than Jira and Linear.
- Creating tracker issues from Kandev tasks.
