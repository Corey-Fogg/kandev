# Orchestrator vs. the workspace-coordinator feature study

As of 2026-09-23, against production build
`0.95.1-orchestration.20260923.sh1fa3f47ef` (branch `fix/0951-production`,
rebased on upstream v0.95.1).

This compares our Orchestrator with the external feature study *"Workspace
coordinator — maturity ladder, build surfaces, and competitive position"*
(author `nova28`, as of 2026-09-17, written against kdlbs/kandev#3752). That
study grades 22 products on a five-rung ladder, lists what Kandev needs per
rung, and recommends a plugin as the build surface.

Grades here are derived from our own code and behaviour, the same way the study
graded vendors from their documentation. Several pieces only started working in
production on 2026-09-23 (task callbacks, reply bridging, task creation), after
the 0.95 rebase regressions were repaired.

## Summary

Graded on the study's ladder, the Orchestrator is **L2+ (3/4)**: the top of the
crowded middle, level with Paperclip, Factory.ai and GitLab Duo. It also ships
one L4 criterion no graded product has in this form: supervised self-repair
proposals. Its missing L3 piece is reporting progress back to the intake
source (Jira, Linear), which the study rates cheap because the outbound clients
already exist.

## Ladder grading

| Rung | Criterion | Us | Evidence |
|---|---|---|---|
| **L1** | Reads all work in scope | ✅ | Broker tools `workspace_tasks`, `task_details`, `task_content`, `capabilities` |
| | Persistent conversation | ✅ | Coordinator chat is a durable task-backed conversation |
| | Overview grouped by state | ✅ | Coordinator view: seven groups based on evidence (Needs input, Problems, Running, Review, Queued, Done, Other) |
| **L2** | Creates, messages or stops work | ✅ | `create_task`; `manage_task` with message, start, stop, move, archive, delete, `repair_session` |
| | A human approval path exists | ⚠️ partial | Execution mode acts as a policy dial (Inspect is read-only, Design is plan-only, Execute acts). Maintenance and linked workspaces require human grants. No propose-then-approve card for individual dispatches |
| | Woken when delegated work lands, fails or needs input | ✅ | Callbacks on REVIEW, COMPLETED, FAILED, WAITING_FOR_INPUT and BLOCKED queue a durable turn in the **same** conversation, plus attention wakes. The study identifies this rung as where the field stalls |
| **L3** | Declared policy with named human gates | ✅ mostly | Engine-enforced workflow gates, plus objectives with acceptance criteria, plus execution modes and grants. The study rates engine gates above prompt-convention gates |
| | External intake with progress reported **back to the source** | ❌ | Automations can target the coordinator as intake; nothing writes progress back to Jira or Linear |
| | Relays a child's question to a human and routes the answer back | ✅ | Attention feed plus native input resolution; `task_permissions` and `resolve_permission` for approval requests. The coordinator answers only questions marked delegable, using confirmed memory |
| | Deduplicates repeated intake | ✅ | Idempotent chat intake (client message ID), operation IDs, callback idempotency keys, automation dedup keys |
| **L4** | Outcome metrics | ❌ | Nothing computes autonomy %, cycle time or cost per PR |
| | Automated grading | ⚠️ | Objective completion requires evidence for every acceptance criterion; this is a gate, not a quality grade |
| | Proposes changes to its own process for review | ✅ | Friction incidents become improvement candidates. With a human grant, a scoped repair gets a sandboxed patch, checks and a local commit, then human review. It cannot approve its own change or publish |

The study found only two products that relay questions (L3(c)) and deduplicate
intake (L3(d)). We do both. Our L3 gap is (b), write-back to the intake source.

## The study's gap list vs. what we built

| The study says Kandev needs | Us |
|---|---|
| 1. A chat surface for the conversation | **Built** in core, with desktop and phone layouts |
| 2. A wake that reaches a dormant coordinator | **Built**: core subscribes to agent, task, clarification and permission events. The study calls this the item that decides "better than polling" |
| 3. Promote the Needs-you Inbox | **Built our own** attention projection instead; it does not reuse upstream `features.needsYouInbox` (divergence risk for upstreaming) |
| 4. Automatic progress write-back to Jira and Linear | **Missing** |
| 5. Dedup on every task-creating intake path | Chat and automation paths only |
| 6. Outcome metrics | **Missing** |
| Goal object ("the real hole") | **Built**: objectives with acceptance criteria and evidence |
| Coordinator memory | **Built**: scoped memory with provenance, edit and forget |
| Budget ceiling | **Missing** |
| Decision record | Partial: operation ledger and objective evidence; no verdict record |
| Failure vocabulary | Partial: attention kinds and a closed list of friction causes; no "still working vs stopped watching vs dead" signal |
| Proposal cards (approve, edit, dismiss) | **Missing** |

## Where we go further than the study scores

- **Durability when the coordinator dies mid-action.** The study says no product
  documents this. We keep a ledger of stable operation IDs whose unknown outcomes
  are never blindly replayed, and interrupted runs are settled at startup. The
  2026-09-23 title-length incident showed the cost: an error misreported as
  "unknown outcome" made the agent stop instead of retrying. Definite validation
  errors are now rejected before the operation is recorded.
- **Permission scope**, which the study explicitly does not score and expects the
  maintainer to ask about:
  - private conversation ownership
  - a restricted tool broker (no shell, only Kandev tools)
  - authority re-checked on every request
  - explicit grants for linked workspaces
- **Self-repair.** `repair_session` recovers worker sessions that stopped on a
  provider login or OAuth refresh failure (clears a stale account lock, resumes
  the same session).

## Build surface: the main disagreement

The study recommends a **plugin**, to prove the workflow on shipped primitives
before asking upstream for a platform API. Two sibling studies it cites partly
reverse that and argue for a core Coordinator page.

We built in core, which is why we have the two things the study says plugins
lack: a chat surface and session-level wakes. The cost is what the maintainer
warned about on #3752:

- a large core diff with frequent rebase conflicts (the 0.95.0 and 0.95.1
  rebases each dropped runtime wiring)
- parallel versions of primitives upstream already ships (`agent_conversation`
  from #3672, `features.needsYouInbox`)

Our 2026-09-18 plugin review found the plugin route blocked because v0.94.0
lacked the needed host APIs. **v0.95.1 may now have them; re-check before
opening an upstream PR.**

## Mechanisms from the study

**Have:**

- parent-surfaced questions answered on the parent (Eve)
- permission bubbling from child to parent (Antigravity)
- waking the existing session on delegated events (Augment)
- bounded retries: automatic recovery stops after five, a light version of
  Kiro's loop cap

**Lack:**

- Approve/Reject/**Modify** checkpoints (GitLab Duo)
- attention rows with explicit entry and exit rules (Paperclip)
- typed stall outcomes such as `no-progress` and `budget-exceeded` (Cloudflare)
- autonomy % and self-improvement PRs linked to motivating runs (Warp)

## Recommended next steps

1. **Tracker write-back**: the cheapest route to full L3. On completion or
   review, post to the Jira or Linear issue the task came from.
2. **Proposal cards**: close L2(b) properly instead of relying on the mode dial.
3. **Outcome metrics** from stored data: autonomy %, cycle time, cost per merged
   PR.
4. **Re-evaluate the plugin surface** against v0.95.1 host APIs before any
   upstream PR.
