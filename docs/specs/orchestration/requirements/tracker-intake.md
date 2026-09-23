---
status: draft
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
able to report progress back on the issue it is working from. This document is
a draft: the behavior is planned and not yet accepted.

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
- **AC-ORCHESTRATION-TRACKER-001.4:** Write-back shall happen only when the
  coordinator requests it. Task callbacks shall not post tracker comments by
  themselves.

### REQ-ORCHESTRATION-TRACKER-002: Intake deduplication

**Intent:** One tracker issue yields one task, whichever path saw it first.

#### Acceptance criteria

- **AC-ORCHESTRATION-TRACKER-002.1:** When the coordinator creates a task with a
  source issue, the system shall record the same tracker metadata that an
  integration watch records, so branch naming and write-back behave the same.
- **AC-ORCHESTRATION-TRACKER-002.2:** When a non-archived task in the workspace
  already has the same source issue, the system shall return that task and
  mark the response as a duplicate instead of creating a second task.

## Out of scope

- Automatic tracker comments on every state change.
- Trackers other than Jira and Linear.
- Creating tracker issues from Kandev tasks.
