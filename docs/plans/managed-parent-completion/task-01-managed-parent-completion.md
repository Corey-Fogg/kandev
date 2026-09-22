---
id: "01-managed-parent-completion"
title: "Guard managed parent completion"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-COMPLETION-001.14
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 01: Guard managed parent completion

## Summary

Make it impossible for an Orchestrator-managed parent to become `COMPLETED`
while any eligible direct child is still active, waiting, or under review.

## Acceptance

1. Every task-state write path that can set `COMPLETED` enforces the child
   invariant within the state-write transaction.
2. The rejected write returns an actionable error naming one unsettled child
   and its current state.
3. Archived and ephemeral children do not block completion; terminal children
   in `COMPLETED`, `FAILED`, or `CANCELLED` do not block it.
4. Unmanaged parents retain existing completion behavior.

## Verification

```sh
(cd apps/backend && go test -tags fts5 -count=1 ./internal/task/repository/sqlite)
```
