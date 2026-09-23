# Orchestrator vs. the workspace-coordinator feature study

As of 2026-09-23, against `feat/workspace-orchestration` at `1bc17b8c1` (the
tidy branch, six commits on upstream v0.95.1), deployed to production as
`0.95.1-orchestration.20260923.sh1bc17b8c1`.

This compares our Orchestrator with the external feature study *"Workspace
coordinator — maturity ladder, build surfaces, and competitive position"*
(author `nova28`, as of 2026-09-17, written against kdlbs/kandev#3752). That
study grades 22 products on a five-rung ladder, lists what Kandev needs per
rung, and recommends a plugin as the build surface.

Grades are derived from our own code, the same way the study graded vendors
from their documentation. Proven in production on 2026-09-23: chat replies,
turn settlement, callbacks and credential self-heal. Covered by tests but not
yet exercised live: tracker write-back, intake dedup and the metrics tool.

This revision replaces the pre-tidy comparison (same date, earlier build).
The "Before" column below records that earlier grading.

## Summary

The tidy branch reaches **L3 (4/4) plus 1/3 of L4** on the study's ladder. In
the study's field only Warp Factories arguably clears L3, and its gates are
contested as prompt conventions. The tidy traded some capability for a single
simpler path: it gained tracker write-back (L3b) and outcome metrics (L4a), but
lost the goal object, supervised self-repair proposals (L4c) and the per-action
approval dial (L2b is weaker).

## Ladder grading

| Rung | Criterion | Before | Now | Evidence |
|---|---|---|---|---|
| **L1** | Reads all work in scope | ✅ | ✅ | `workspace_tasks`, now with each task's source issue, PR and pending input |
| | Persistent conversation | ✅ | ✅ | One coordinator chat per workspace assignment. Replies took 6–10 s in production checks, down from about 90 s |
| | Grouped overview | ✅ | ✅ | Coordinator view, plus a nav badge for tasks waiting on you |
| **L2** | Creates, messages or stops work | ✅ | ✅ | `create_task`, `manage_task`, batched status and move, `repair_session` |
| | A human approval path exists | ⚠️ mode dial | ⚠️ weaker | Execution modes are gone. Remaining: engine-enforced workflow approval/review gates, a completion guard that refuses "done" while required reviews are pending, and workspace-manage access to configure or message a coordinator. No propose-then-approve step |
| | Woken when delegated work lands, fails or needs input | ✅ | ✅ stronger | Digest callbacks, question and permission relay, stall callbacks, and a login-failure hint that asks the coordinator to call `repair_session`. Verified in production |
| **L3** | Declared policy with named human gates | ✅ mostly | ✅ mostly | Engine-enforced workflow gates plus the completion guard; stronger than prompt-convention gates, but no declared autonomy tier per action class |
| | External intake with progress reported **back to the source** | ❌ | ✅ new | `update_source_issue` comments on or moves the Jira/Linear issue recorded on the task, never one the caller names. The coordinator invokes it; nothing fires it automatically on a state change |
| | Relays a child's question and routes the answer back | ✅ | ✅ | Question callback, then `answer_question`, limited to delegated tasks. `resolve_permission` offers only allow-once or reject-once |
| | Deduplicates repeated intake | ✅ | ✅ wider | Idempotent chat intake and callback keys; `create_task` with `source` returns the existing task for that issue with `duplicate: true` |
| **L4** | Outcome metrics | ❌ | ✅ new (tool only) | `metrics` broker tool: completions, failures, merged PRs, cycle time and cost over 7 or 30 days. No UI |
| | Automated grading | ⚠️ objective evidence gate | ❌ lost | Objectives and their acceptance-evidence gate were deleted with the assistant layer |
| | Proposes changes to its own process for review | ✅ | ❌ lost | Maintenance and self-repair proposals were deleted with the assistant layer |

Net: gained L3(b) and L4(a); lost L4(b) and L4(c); L2(b) is weaker.

## The study's gap list

| The study says Kandev needs | Before | Now |
|---|---|---|
| 1. Chat surface | Built | Built, with paging, retry by run id and polling only while visible |
| 2. Wake for a dormant coordinator | Built | Built, verified in production |
| 3. Promote the Needs-you Inbox | Own attention feed | Callback relay plus nav badge; still does not reuse upstream `needsYouInbox` |
| 4. Tracker write-back | Missing | Built (coordinator decides when) |
| 5. Intake dedup on every path | Chat and automations only | Wider: issue-sourced tasks reuse the existing one |
| 6. Outcome metrics | Missing | Built (tool, no dashboard) |

## The coordinator's own equipment

| Item | Before | Now |
|---|---|---|
| Goal / definition of done | Objectives | ❌ gone again (the study's "real hole") |
| Memory | Scoped, with provenance | ✅ workspace memory via `remember`/`forget`, injected into every turn |
| Budget ceiling | ❌ | ❌ |
| Escalation policy | Execution modes | ❌ implicit only (prompt guidance plus engine gates) |
| Decision record | Operations ledger | ❌ removed; chat history and task links remain |
| Failure vocabulary | Partial | ✅ better: stall outcomes `no_progress`/`never_started`/`orphaned` plus a login-failure hint |
| Identity | Runtime JWT per persona | ✅ same |
| Skills / instructions | Stored copies drifted | ✅ concise embedded instructions plus role instructions |

## Mechanisms from the study

**Have:** parent-surfaced questions (Eve), permission bubbling (Antigravity),
waking the existing session (Augment), bounded retries (Kiro), typed stall
outcomes (Cloudflare).

**Lack:** Approve/Reject/**Modify** checkpoints (GitLab Duo), attention rows with
entry and exit rules (Paperclip), autonomy % (Warp; our metrics report
completion rates, not how much happened without a human), self-improvement PRs
(ours were dropped).

## Build surface

Still in core, but the diff against v0.95.1 is half the size (454 files versus
882), split into stacked commits that each fit a 150-file PR. We still keep our
own versions of `agent_conversation` and the Needs-you inbox rather than reusing
upstream's; that remains the main upstreaming friction.

## Recommended next steps

1. **Proposal cards** (approve, edit, dismiss) on `create_task`: restores L2(b)
   without the mode dial.
2. **A light goal object**: acceptance criteria on the coordinator's delegated
   tasks, checked before it reports "done". Covers the study's "real hole" and
   most of L4(b) without the old objective/context-packet machinery.
3. **Automatic write-back**: post to the source issue when a task reaches review
   or done, without waiting for the coordinator to decide.
4. **UI for data we already collect**: metrics strip, stall badges and issue
   chips in the Coordinator view.
