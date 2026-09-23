---
status: active
system: orchestration
specification_version: 1
migration: complete
owners:
  - Kandev
---

# Orchestration

## Purpose

Orchestration gives a workspace one or more conversational coordinators. A
coordinator is a workspace assignment of a reusable role that runs under an
existing execution profile. People talk to it in a persistent chat, and it
delegates, monitors and steers ordinary Kanban tasks through a restricted
broker. The central Coordinator view shows the workspace's tasks beside that
chat.

Every coordinator conversation uses the same runtime path: the
`orchestrator-broker-v1` MCP surface, a runtime credential with the
`workspace_coordinator` audience, and broker tools scoped to the assignment's
workspace. The whole feature is gated by `features.orchestration`.

## Ownership

Orchestration owns:

- Global coordinator roles and workspace coordinator assignments.
- Persistent coordinator conversations, durable message intake and turn
  scheduling for those conversations.
- The coordinator runtime: prompt assembly, runtime credentials, broker tool
  catalog, automatic turn recovery and session repair.
- Delegation links between coordinators and the tasks they create or adopt,
  and the callbacks that wake a coordinator about those tasks.
- Relay of pending questions and permission requests from delegated tasks.
- Workspace-scoped coordinator memory.
- The Coordinator view, which combines task observations with the selected
  coordinator's conversation.
- The automation delivery target that queues a prompt into a coordinator
  conversation.

## Exclusions

- [Tasks](../tasks/README.md) owns task and session state, workflow moves,
  completion gates, pending questions and permission requests. Orchestration
  reads and requests changes through native task services and never keeps a
  second copy of task state.
- [Workspaces](../workspaces/README.md) owns workspace membership and access.
- [Agents](../agents/README.md) owns execution profiles, providers, accounts
  and executors. A coordinator selects an existing profile; it does not define
  one.
- [Office](../office/README.md) owns its autonomous agent fleet. Orchestration
  does not depend on Office packages, tables or routes.
- [Plugins](../plugins/README.md) owns plugin tools. Coordinator sessions do
  not receive plugin or external MCP tools.
- Automations owns schedules and delivery history. Orchestration supplies only
  the coordinator delivery target.

## Find specifications

Use the catalog command to list this system's current documents:

    python3 scripts/list-docs.py specs --system orchestration --format markdown
    python3 scripts/list-docs.py specs --system orchestration --kind requirement --format paths
    python3 scripts/list-docs.py specs --system orchestration --kind system-design --format paths

Do not copy the command output into this README. Keep this file focused on the
system boundary, migration record, and related systems.

## Migration record

The free-form workspace-orchestrators specification, the Office-era chief of
staff and workspace-agents specifications, and the unified workspace
orchestration notes were replaced by the requirement and system-design
documents in this directory. No editable legacy source remains.

## Related systems

- [Tasks](../tasks/README.md): the source of truth for every delegated task,
  including the managed-parent completion guard.
- [Agents](../agents/README.md): execution profiles used by coordinators and
  delegated tasks.
- [Workspaces](../workspaces/README.md): the scope of every coordinator call.
- [Integrations](../integrations/README.md): issue trackers used by
  tracker write-back and intake deduplication.

Related decisions:

- [Workspace orchestration owns its coordination runtime](../../decisions/2026-09-07-workspace-orchestration.md).
- [One coordinator path](../../decisions/2026-09-18-orchestrator-product-boundary.md).
