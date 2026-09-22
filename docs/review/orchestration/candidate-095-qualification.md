# 0.95.0 Orchestrator candidate

Status: deployed to the private service on 2026-09-22. This is a custom
candidate based on the exact v0.95.0 release, not an upstream stable release
artifact.

| Review item | Candidate |
| --- | --- |
| Version | `0.95.0-orchestration.20260922.shabc82422f83a4` |
| Source commit | `bc82422f83a4f78b2b90097701260a2674baa2d3` |
| v0.95.0 base | `f92877b4be2724c0bfa1f1cdbdd35edd68a24fa6` |
| Platform | Linux x86-64 |
| Bundle | Six immutable binaries; [SHA-256 receipt](candidate-095-receipt.json) |
| Web assets | 577 embedded production assets |
| Feature smoke | Orchestrator on; Office off; synthetic network-isolated startup passed |
| Private database rehearsal | SQLite online backup, migration/replay, restart and integrity checks passed in network-isolated copies |
| Rollback rehearsal | Exact running 0.94.0 bundle started against a separate pre-candidate database copy |
| Live service | Running this candidate; `/health` reports `ok` and this exact version |
| Live data | Existing database retained; SQLite integrity check passed after startup |
| Rollback | Cold local copy of data, service configuration/drop-ins and old bundle verified |

The four implementation commits are separate: Claude model/effort profile
support, the managed-parent completion guard, required-store startup ordering,
and the rollback-writer guard. They are present on the public fork's
`feat/workspace-orchestration` branch. No issue or PR was created.

The complete task SQLite repository tests passed, as did focused assistant,
startup, race-detector, SQL guard, documentation and changed-package lint checks.
The full backendapp package has four existing failures; all four reproduce at
the pre-change commit `c1af7fddc`. Repository-wide Go lint also reaches an
unrelated compile error in the untouched lifecycle SSH test; lint on the changed
packages reports zero issues. The environment had no PostgreSQL test DSN, so
PostgreSQL conformance was not run. Browser tests were not run because this
candidate changes backend behavior only.

The live service was stopped before the cold copy. The copy includes the local
Kandev data tree (database, attachments, repositories/workspaces, and plugins),
service unit/drop-ins, and the exact previous bundle. The backup manifest was
verified against every copied file; the database, service configuration and old
executable hashes were also checked directly. SQLite integrity passed both
before cutover (on the copy) and after candidate startup (live). No task, session,
or run rows were changed by the deployment. Four existing worker processes were
stopped with the service; their sessions were waiting for input or still marked
created, with no queued/running runs in the queue at cutover.

The candidate started with authentication and Orchestrator enabled and Office
disabled. The public health endpoint returned `ok` and the candidate version,
and the web root returned HTTP 200. The deployment receipt does not claim
PostgreSQL, browser, or provider coverage.

See the [dogfood runbook](../../plans/orchestration-delivery/dogfood-runbook.md)
for the cutover and rollback procedure.
