---
title: "Workspace orchestration"
description: "Configure workspace coordinators using your existing agent profiles, roles and Kanban workflows."
---

# Workspace orchestration

Coordinators are conversational agents assigned to an existing workspace. Create as many as you need; each has its own execution profile, workspace context, memory and persistent conversation, with identity and instructions supplied by its global role. Tasks continue to use their own assigned agents and normal Kanban workflows.

## Enable and configure

1. Complete Kanban onboarding and configure your agent profiles, provider logins, executors and workflows.
2. Enable **Workspace orchestration** in **Settings > System > Feature toggles**, then restart Kandev. The experimental flag is `KANDEV_FEATURES_ORCHESTRATION`; it defaults off and is independent of Office.
3. Configure the role's name, icon and instructions in **Settings > Orchestration**. Then open **Settings > Workspaces > your workspace > Orchestration**, or follow the Orchestration link on the workspace overview card or sidebar.
4. Select **Add orchestrator**. Choose the global role, an existing agent profile and executor, and workspace context/delegation guidance. No separate worker personas or model-routing setup is required.
5. Open **Coordinator** from the workspace sidebar or overview card. Select an assignment to see its conversation beside the workspace tasks. On mobile, use **Tasks** and **Chat** to switch panes. Configuration and conversation pages link back to the workspace; coordinated tasks link back to their coordinator.

**Settings > Orchestration** manages global roles. Administrators configure each role's name, icon and instructions once. Workspace assignments inherit that configuration; saved role edits apply to all assignments on their next turn. A running turn keeps the instructions it started with. Roles in use cannot be deleted. The built-in Chief of staff role is editable but cannot be deleted.

Choose **Persona icon** in the global role settings to use name initials or a built-in emoji icon. Initials use the first and last name words, so Chief of staff displays CS. The same identity appears in chat, workspace lists and sidebar navigation.

Edit orchestrators with the shared **Save changes** and **Reset** controls. Pause blocks new turns; it does not interrupt a running turn. Delete removes the coordinator and stops its conversation sessions, while delivery tasks remain on their board. Opening a conversation does not create another workspace or workflow.

Every conversation belongs to its workspace. Anyone with access to the workspace can read it. Sending messages, like adding or changing an orchestrator, needs workspace manage access, because the coordinator acts with that authority.

## Work from the Coordinator view

Open `/workspaces/<workspace-id>/coordinator`. The task view includes ordinary,
non-archived board tasks across the workspace's visible workflows. Office tasks,
hidden system workflows and coordinator conversations stay out of this list.
Opening or refreshing the view starts no agent work.

Select a coordinator to use its configured profile and persistent conversation.
A workspace with one assignment selects it automatically; with several, choose
one. The page remembers a valid choice during your browser session. A missing
assignment or profile offers configuration instead of selecting another account.
Drafts remain separate for each conversation while switching coordinators or
mobile tabs. Drafts are held in browser memory and do not survive a full reload.

Tasks are grouped by current native status: input needed, problems, running,
review, done, queued and other. Follow a task to answer its native question or
permission request, inspect execution, or review its changes. Review and an open
or merged pull request do not imply that a task is completed. Unknown or idle
activity is not classified as stalled.

Use search, workflow and repository filters, or choose **Selected coordinator’s
tasks**. Status groups and totals describe the loaded tasks; load additional
pages for more coverage. The initial page contains up to 100 tasks. Pull request
and changed-file totals show how many loaded tasks have known data. A stale
notice means refresh failed; retry before relying on the displayed state.

On desktop, hide or show chat without changing the task selection. On a touch
device, Tasks and Chat share the same workspace and conversation. Expand
**Filters** for workflow, repository and coordinator scope controls. Live task
summaries update the groups, and reconnecting refreshes the loaded rows.

## Control workspace tasks from chat

The coordinator uses your configured execution profile and Kandev's injected
run credentials. It does not need a separate Kandev API key. Its only tools are
Kandev's coordinator tools; it has no shell, plugin tools or other MCP servers.
Backend task and workflow checks apply to every call.

Ask it to create a task, edit its title, description, priority or parent, assign
an execution profile, start or stop work, send a worker a message, move a task
between workflow steps, change its status, archive it, or delete it. Changes use
the native task services and appear on the board. For example: "Create a task
called Update the sample README, then move it to the backlog."

Workflow moves respect configured manual-move rules, active-session restrictions
and required review decisions. Marking a task done uses the native completion
gates. Deletion uses native cleanup and does not discard uncommitted work. Other
workspaces and Office tasks are outside these controls. After an uncertain
result, the coordinator inspects the task before deciding whether to retry.

Task details contain compact status and result previews. The coordinator can
read full requirements, worker replies, answered questions and command failures
in bounded pages, so long conversations do not require shell or file access.

If a delegated task's session stopped because a provider login or token refresh
failed, the coordinator can repair it: Kandev clears the stale account lock and
resumes the same session. Other failures are not repaired this way.

## Questions and permissions from delegated tasks

When a task the coordinator created or adopted asks a question or waits on a
tool permission, the coordinator is woken in its conversation with a summary
of the request. It can list the task's pending questions and permission
requests, answer a question, or approve or reject one exact permission request
using the option the provider offered.

The coordinator answers or approves only what your instructions in the
conversation already cover, keeps explicit denials, and otherwise asks you in
chat. Answers it gives are attributed to the coordinator in the task history.
It cannot enable permission-bypass modes, create persistent allow rules or
complete a sign-in for you. You can always answer the same request on the task
page yourself; whichever answer arrives first is used.

For an authorized task blocked by a provider's automatic permission classifier,
the coordinator can switch that session to manual permission review, ask the
worker to retry, and review the resulting request.

## Memory

Each coordinator keeps a short memory for its workspace. Ask it to remember a
standing preference ("always use the backlog column for new bugs") or to forget
one. Remembered entries are included in each later turn, within a size limit;
the coordinator can look up entries that did not fit. Memory is context, not
permission: it never lets the coordinator do something its tools would refuse.

Forgetting removes an entry from later turns. It does not erase text already
sent to a provider or kept in chat history and backups. Keep secret values out
of memory.

## Updates from delegated tasks

When a delegated task reaches review, completes, fails, waits for input or is
blocked, Kandev queues one update into the owning coordinator's conversation.
Other coordinators in the workspace do not receive it. Duplicate events for one
transition are merged, and updates for separate tasks stay separate. The
coordinator inspects the current result and posts only new information. Review
means ready for review, not completed.

Kandev also wakes the coordinator when a delegated task stalls: the agent never
started, is making no progress, or the task has no live execution. The update
names which of these happened so the coordinator can repair, stop or message
the task. Quiet time alone never counts as a stall.

Paused or disabled coordinators receive no updates, and resuming does not
replay updates that arrived while paused.

The configuration page lists the latest 100 coordinated tasks. Follow a task to
manage its profile and workflow, then use **Coordinating orchestrator** to
return. Conversations show chat and execution feedback without task properties
such as labels, priority, blockers or reviewers.

## Manage the workspace

The coordinator can also manage its assigned workspace. Ask it to update
workspace settings, register an existing local Git checkout or remote
repository, create or edit a delivery workflow, or add, configure, reorder and
remove workflow columns.

Workspace defaults include the executor, environment and agent profiles.
Workflow configuration includes instructions and default agent profiles;
column configuration includes start columns, events, WIP limits and automatic
progression. Changes appear through the same live updates as edits in settings.
Deleting a workflow archives its remaining tasks. Move tasks before deleting
an occupied column if they should remain assigned to a column. Repository
removal uses Kandev's normal active-session and cleanup checks.

Configuration access stays within the assigned workspace. Workspace membership,
global settings and hidden system workflows remain separate administrative
controls. GitHub-synced workflow definitions must be edited at their source.

## Automatic conversation recovery

If a temporary provider failure occurs before the coordinator has produced a
response or called a tool, Kandev automatically restarts the execution and
retries the pending request. This includes Claude account-token refresh
contention after an idle period. The conversation shows a retry notice; use
**Cancel** or **Stop** to prevent the pending retry.

Recovery tries up to five times, waiting 15 seconds, 30 seconds, then 60 seconds
between the remaining attempts. Each attempt uses the same configured account
and freshly scoped Kandev access. Later messages wait behind the recovering turn.

If recovery is exhausted, the normal failure and manual retry controls appear.
Authentication that requires sign-in, turns with prior output or tool activity,
and interrupted turns whose effects are unknown are not automatically replayed.
After a backend restart, interrupted turns are marked failed; already scheduled
safe retries survive the restart. Inspect the latest task results before
retrying manually.

## Profiles and routing

The coordinator's execution profile determines its provider and account. For example, a personal Claude coordinator can direct Jira work to your existing work Claude profile and personal development to a personal Claude or Codex profile. Describe these choices in its routing context, including when to use each profile and where account-specific setup instructions live.

Profiles supply their configured environment. For separate Claude subscriptions, configure the appropriate `CLAUDE_CONFIG_DIR` in each profile and authenticate it on the chosen execution host. A profile name alone does not establish which subscription is authenticated. Keep credentials in the existing secrets/account setup, rather than in role instructions or routing context.

A coordinator does not automatically fall back to another account. Tasks keep their assigned profiles when adopted. Explicit reassignment is supported, including changes between Claude and Codex; task profiles are not pinned to the coordinator. Executors and remote-host access use Kandev's existing execution configuration.

Any provider can run a coordinator. On Claude, Kandev also turns off the provider's built-in tools. Other providers keep their own built-in tools behind their normal permission prompts, which Kandev never approves automatically for a coordinator.

## Context and compatibility

Each turn uses fresh provider context with bounded recent conversation excerpts: up to four comments at 1,000 bytes each and up to 6,000 bytes from the triggering message. Routing includes at most 12 profile names/IDs and 2,000 bytes of guidance. Memory is included within a byte budget. Instructions and anything the coordinator retrieves also consume context; these bounds are not a total token budget.

Orchestration owns its runtime, conversation API, instructions, memory and conversation registry. It uses core execution profiles, task sessions, authentication, run queue and chat rendering. Office is independently feature flagged and is not required to configure or run a coordinator. Office APIs cannot operate on registered coordinators or their conversations. Disabling Orchestration hides its navigation and rejects its API and run paths without deleting configuration. The scoped `POST /api/v1/orchestration/workspaces/:wsId/import/:id` endpoint registers an existing agent persona as a coordinator while keeping its identity and conversation history.

Upgrading from the earlier shared implementation copies registered personas' instructions, memory and conversation mappings into Orchestration's own tables. Existing persona IDs, conversation URLs and comment history are preserved. Back up the database before upgrading. Disabling the flag keeps the stored data.

### Global roles and workspace assignments

Define identity and behavior in global role settings, then add that role to a workspace. The workspace form contains only role selection, execution profile, execution environment and local context/delegation guidance. It does not duplicate the name, icon or instructions. Assignments keep separate conversations, memory, task ownership and pause controls. The same role can run under a personal profile in one workspace and a work profile in another.

Upgrading preserves existing assignment identities and history. If assignments previously had different names, icons or instruction snapshots, those configurations become distinct global roles. Repeated upgrades do not overwrite later global role edits.

The [orchestration API reference](orchestration-api.md) lists the coordinator's tools and the HTTP routes behind them.

## Scheduled orchestrator prompts

In workspace **Automations**, create an automation, choose **every day** (or another existing schedule) and its time zone, then select your workspace's code reviewer under **Run with**. Enter the recurring instruction, for example:

> Check open PRs assigned to me or requesting my review. Identify what needs to move forward, delegate detailed reviews to the appropriate task agents, and report actionable findings with PR links. Reuse existing review tasks and avoid duplicate comments or reviews.

The target is a workspace assignment, so it inherits the role's current instructions, its selected account/execution environment, local context and memory. No duplicate profile or repository selection is needed. **Run now** uses the same delivery path as the schedule.

Each firing records **Delivered to orchestrator** once the prompt is queued and links to the existing conversation. This status confirms delivery, not completion of the review. Follow chat for execution progress, results and errors. Turns are processed in order. Paused, deleted, unavailable or feature-disabled targets record a delivery failure; there is no fallback to another account. A repeated scheduled firing is deduplicated. Deleting automation history does not delete the shared conversation. Kandev must be running when the schedule is due, and the configured execution account must have access to the PR provider.
