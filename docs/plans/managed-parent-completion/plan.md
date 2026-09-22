---
created: 2026-09-22
status: completed
requirements:
  - ../../specs/tasks/requirements/task-completion.md
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Plan: Guard Orchestrator-managed parent completion

## Overview

Prevent an Orchestrator-managed parent task from reaching `COMPLETED` while
direct child work remains unsettled. The task repository owns the state
transition, so enforce the rule transactionally in every completion write path.
This is a separate fix and commit from Claude profile compatibility.

## Work order

1. [Task 01: Guard managed parent completion](task-01-managed-parent-completion.md)

## Exclusions

Do not close, cancel, restart, retry, or otherwise mutate existing child tasks.
Do not change completion behavior for unmanaged parents. FAILED and CANCELLED
children are settled terminal outcomes and remain visible to users.

## Verification

Run the focused and full task SQLite repository tests, the assistant authority
tests, and documentation catalog/spec validation.
