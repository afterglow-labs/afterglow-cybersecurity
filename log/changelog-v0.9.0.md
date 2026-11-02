# ScopeWeaver v0.9.0 — Delete a finished worker's output

A worker that finished with hallucinated output could not be deleted at all: the delete action was
restricted to pending, running and paused intents, and removing `workspace/tasks/<taskID>/i<intentID>`
only deleted working files while the output stayed in PostgreSQL ([#62](https://github.com/cskwork/scopeweaver/issues/62)).
This release adds that capability and fixes the concurrency and planner-context defects an adversarial
review found in it. See [#73](https://github.com/cskwork/scopeweaver/pull/73).

## Added

- Delete a **finished** worker's output from the task's session list, in either mode, with a required
  reason. Terminal intents (`done`, `blocked`, `exhausted`, `stopped`) join pending, running and paused.
  **Retract** keeps the facts, findings and activity trace and tells the planner the output is withdrawn
  and must not be relied on. **Permanent delete** removes the intent plus the output it exclusively
  supports, preserving goals, origin facts, shared nodes and digest-covered output. Inherited intents
  stay read-only and an already-deleted intent is still refused.
- A soft-deleted worker's retained output now stays marked withdrawn, with the user's reason, on every
  later planner round rather than only the round the deletion fired. The marking reaches
  `graph_overview` (including the planner's prefetched snapshot), `list_facts`, `list_findings`,
  `node_detail`, `get_worker_output`, `expand_digest`, `get_worker_trace` and
  `search_all_worker_traces`. Retained audit content is unchanged, and a surviving worker's output is
  presented exactly as before.

## Fixed

- **Behavior change: intent deletion now returns HTTP 409 when the worker's state changed before the
  deletion took its row lock.** The delete path decided whether to stop a worker from an unlocked read,
  so a rerun that moved a `blocked` intent to `running` in that gap produced a successful delete with
  the worker never cancelled, and two concurrent requests could both succeed — the permanent one
  physically removing the row the soft one had just marked deleted. `SoftDeleteIntent` and
  `CancelIntent` now revalidate the caller's state under the lock; permanent deletion also refuses a
  locked `deleted` row. Clients that retry on conflict are unaffected; clients that assumed success
  must handle 409.
- **Behavior change: permanent deletion refuses a cascade containing a running intent**, returning
  HTTP 409 naming the worker, so the operator stops it first. Previously the cascade removed a running
  descendant's records while its execution stayed live, letting a late write land on removed rows.
- The cascade no longer decides exclusivity from a stale snapshot. It locks the exploration's nodes
  before reading edges, so support a detached compaction commits mid-cascade is honoured and protects
  its member; a digest insert that loses the race fails on the foreign key and rolls back whole, leaving
  no partial digest. This briefly serializes writers on that exploration's nodes during cleanup.
- A failed rerun admission no longer resurrects a deleted intent. The rollback restores the reopened row
  only while it is still `open`, restores state and blocked reason in one conditional update, and reports
  an unapplied rollback instead of a silent success.
- A late side-conversation snapshot can no longer recreate a soft-deleted worker's session; the parent
  guard now rejects the soft-delete state as well as `stopped`.
- The deletion dialog no longer promises an instruction the server does not send: withdrawal wording is
  shown for a finished worker, while an unfinished one is described as cancelled and replanned. Complete
  in all six shipped languages.

## Known limit

Output that a cold digest has already folded into a summary survives a **permanent** deletion unmarked:
the cascade spares it because the surviving digest is a parent, and the producing intent and its lineage
edges are gone, so the withdrawal lookup has nothing left to find. Retract is unaffected, so prefer it
for a hallucinating worker in a task that has run long enough to compact.

## Known security issue

The Zod advisory disclosed in v0.8.0 is unchanged and still tracked in [issue #67](https://github.com/cskwork/scopeweaver/issues/67). See the [v0.8.0 note](../log/changelog-v0.8.0.md#known-security-issue).

## Upgrade and verification

v0.9.0 adds no database migration. Back up the PostgreSQL database and data directory before
upgrading, and stop the previous server before replacing it.

Verified before release: the `db`, `server` and `agent` Go suites against a disposable
`scopeweaver_test_*` database, the frontend suite and static export, and all seven required checks on
[#73](https://github.com/cskwork/scopeweaver/pull/73) plus `checks` and `e2e` on `dev` and on `main`.
The new deletion, cascade-fencing, rerun-rollback and withdrawal behaviours each ship with tests that
fail without their change.
