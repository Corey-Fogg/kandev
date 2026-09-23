# ADR-2026-09-18-orchestrator-product-boundary: One coordinator path for every Orchestrator conversation

**Status:** accepted
**Date:** 2026-09-18
**Area:** backend, frontend, agentctl, security

## Context

Workspace coordinators and a per-user "personal assistant" were developed as
two paths over the same conversation runtime. The assistant path added a user
binding, owner-private conversations, execution modes, objectives, context
packets, an operations ledger, linked-workspace grants, a maintenance sandbox,
credential descriptors, a capability directory and a separately pinned
provider runtime, with its own broker surface and token audience.

Two paths meant two authorization models, two tool catalogs and two sets of
launch rules for what users experience as the same chat. Most of the assistant
machinery guarded capabilities that coordinators did not need. The parts users
did rely on (being told when delegated work is blocked, and remembering
standing preferences) fit on the workspace assignment directly.

## Decision

Every Orchestrator conversation is a workspace coordinator conversation and
runs one path:

- The session uses the `orchestrator-broker-v1` MCP surface. Its only MCP server
  is the managed `kandev_orchestrator` broker started by `agentctl`. Kandev's
  shell tool, ask-question tool, automatic permission approval, profile MCP
  servers and plugin tools are removed. Claude sessions also disable built-in
  tools and allow only `mcp__kandev_orchestrator__*`. Other providers get the
  same single broker attachment.
- The broker policy is stored on the session's metadata when the session is
  created, and launch, resume and steering paths require it, so a resumed
  session keeps the restriction. Sessions stored under the earlier assistant
  policy key are treated as the same policy.
- Runtime JWTs use the `workspace_coordinator` audience and are bound to the
  assignment, workspace, run and session. Every broker route rechecks the
  claimed run and the workspace of every referenced resource.
- `models.WorkspaceBrokerTools()` is the single tool catalog.
- Conversations are workspace-scoped. Any member with workspace access can
  read them, including conversations that recorded an owning user.

Two assistant capabilities move onto the coordinator assignment:

- **Question and permission relay.** A delegated task's pending question or
  permission request wakes the delegating coordinator. It can list pending
  requests, answer a question with `manage_task` `answer_question`, and resolve
  a permission once with `resolve_permission`. It cannot select a bypass
  permission mode or create persistent rules.
- **Memory.** Each assignment has workspace-scoped memory that it writes and
  forgets through broker tools and that is injected into its prompt.

Everything else from the assistant path is removed: the `/assistant` page,
user bindings, owner-private conversations, execution modes,
`assistant-broker-v1`, provider version pinning, objectives, context packets,
the operations ledger and its operation IDs, linked-workspace grants and
exports, the maintenance sandbox, friction and improvement proposals,
credential descriptors, the capability directory and the
`assistant_delegable` question flag. The Office "workspace chief" bridge and
the `conversation` MCP surface are removed with them.

`features.orchestration` remains the only gate. Office stays a separate
product with its own flag and is not required by Orchestration.

## Persistence

Schema changes are replayable and idempotent. Fresh installs no longer create
the assistant-only tables. Existing installs keep those tables, and no code
reads them. An existing user binding is ignored, so the conversation it pointed
to continues as an ordinary coordinator conversation.

## Consequences

- One authorization model, one tool catalog and one launch rule cover every
  coordinator chat. Security review has one broker to reason about.
- A coordinator can act only inside its assignment's workspace. There is no
  cross-workspace access and no private per-user coordinator.
- Coordinators can use any execution profile. Provider-native tools of
  non-Claude providers are not disabled by Kandev; they remain behind that
  provider's own permission prompts, which are never auto-approved.
- Retained legacy tables cost disk space until a later cleanup drops them.

## Alternatives considered

- **Keep both paths.** Rejected: duplicate authority models for one product.
- **Keep the assistant path and retire coordinators.** Rejected: owner-private
  scope and execution modes do not fit shared workspace work, and the pinned
  runtime blocks ordinary execution profiles.
- **Drop the legacy tables in the same change.** Rejected: a rollback to an
  older build would lose data. Dropping them is a separate cleanup.
