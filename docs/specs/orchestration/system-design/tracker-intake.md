---
status: draft
system: orchestration
requirements:
  - REQ-ORCHESTRATION-TRACKER-001
  - REQ-ORCHESTRATION-TRACKER-002
---

# Tracker Intake System Design

## Purpose and boundaries

This draft design adds tracker write-back and cross-path intake deduplication
to the coordinator broker. The Jira and Linear clients, their per-workspace
credentials and the issue metadata that watches write stay in
`internal/integrations`, as described in that package's `AGENTS.md`.
Orchestration adds one broker tool, one `create_task` field and the
authorization around them.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-ORCHESTRATION-TRACKER-001` | [Write-back](#write-back) |
| `REQ-ORCHESTRATION-TRACKER-002` | [Intake deduplication](#intake-deduplication) |

## Write-back

A new broker tool `update_source_issue` maps to
`POST /runtime/tasks/:id/source-issue` with request
`{comment?: string, state?: "started" | "review" | "done"}`. At least one field
is required and `comment` is at most 4,000 characters.

The handler loads the task, requires `task.WorkspaceID` to equal the token's
workspace, and reads the source issue only from the task's metadata
(`jira_issue_key` or `linear_issue_identifier`). A task without either key
returns 422.

- **Comment.** Each tracker client gains `AddComment`. Jira Cloud posts to
  `/issue/{key}/comment` (an ADF body on API v3, plain text on v2); the
  Atlassian MCP client uses its comment tool. Linear uses the `commentCreate`
  mutation with the issue's UUID from `GetIssue`.
- **State.** Linear maps the category to the issue team's workflow state type
  and calls `SetIssueState`. Jira reads the issue's transitions, including each
  target's status category, and applies the matching transition. When nothing
  matches, the handler returns 422 with the available transition names.

The services expose `AddCommentForWorkspace` and
`TransitionToCategoryForWorkspace`, following the existing `*ForWorkspace`
pattern, so the workspace's own credentials are used. Write-back is
coordinator-initiated only; callbacks carry the task's source so the
coordinator can decide to report.

## Intake deduplication

`create_task` accepts an optional `source: {tracker, key, url}`. The handler
normalizes it to `external_id` `jira:<KEY>` or `linear:<ID>` and writes the
same metadata keys that integration watches write (`jira_issue_key` and
`jira_issue_url`, or the Linear equivalents). Branch naming and write-back then
treat coordinator-created and watch-created tasks the same way.

Before creating, the handler looks for a non-archived task in the workspace
whose metadata has the same issue key. When one exists, it returns
`{id, duplicate: true}` and creates nothing. The existing `external_id`
idempotency still covers retries of the same request.

## Failure and recovery

Tracker failures return the provider's status as a definite error and change
nothing in Kandev. A lost response is an unknown outcome; the coordinator reads
the issue before posting again, because a comment cannot be deduplicated by
the tracker.

## Security

The tool cannot name an issue: the issue comes from task metadata, and the task
must be in the token's workspace. Credentials never leave the integration
service. Comment text is not logged.

## Related decisions

- [Workspace-scoped integration settings](../../../decisions/0030-workspace-scoped-integration-settings.md)
