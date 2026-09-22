---
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-001
  - REQ-ORCHESTRATION-ASSISTANT-004
system_design:
  - ../../specs/orchestration/system-design/personal-assistant.md
created: 2026-09-22
status: completed
---

# Plan: Assistant execution reliability

## Overview

Allow the restricted Claude assistant to accept supported model and effort
selections without weakening its broker boundary. This change stays on the
existing Kandev fork and is committed separately from other fixes; this plan
does not include creating a dedicated issue or PR.

## Work orders

1. [Task 01: Allow Claude model and effort settings](task-01-claude-model-effort.md)

## Dependencies

This work order must land as its own commit.

## Risks and exclusions

Task 01 must not expand the restricted provider boundary to arbitrary config
options, CLI flags, environment variables, fallback, tools, MCP servers or
executor hooks. Only the profile model and provider-supported effort setting
are in scope.


## Verification strategy

Run the exact focused Go test commands in each work order, then run
`python3 scripts/list-docs.py validate` and
`python3 scripts/lint-spec-files.py --all`. The existing unrelated local
changes in `apps/backend/internal/orchestration/repository/sqlite/intake.go`
and its two validation test files are outside this plan and must remain intact.
