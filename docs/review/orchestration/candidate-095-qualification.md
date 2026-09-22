# 0.95.0 Orchestrator candidate

Status: prepared for the private deployment review; the running service has not
been changed. This is a custom candidate based on the exact v0.95.0 release, not
an upstream stable release artifact.

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
| Live service | Still on its existing 0.94.0 candidate |

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

The private-data rehearsal copied the SQLite database through SQLite's Online
Backup API and left live records and service untouched. It did not copy
workspace directories, attachments, or external configuration. The final live
change window still needs the runbook's cold, verified backup of all required
data/config and service state. The receipt describes exactly what was tested;
it does not claim PostgreSQL, browser, provider, or attachment coverage.

The live cutover is pending the runbook's explicit instruction for this named
candidate. See the [dogfood runbook](../../plans/orchestration-delivery/dogfood-runbook.md)
for the final backup, quiesce, service switch, smoke and rollback sequence.
