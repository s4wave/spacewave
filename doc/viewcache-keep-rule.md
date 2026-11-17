# Viewcache Keep Rule

This note covers when `db/world/viewcache` keeps a computed value for later
readers. It proposes three changes. None of them is implemented yet.

## The rule today

A value is kept under its name in `Cache.entries`. Get reuses an entry when
`entry.at <= seqno <= c.seqno`, so a kept value counts as valid from the
revision it was computed at up to the head.

A value can be computed in one of three ways:

- **Shared** (`Get`, line 94): the value is computed at the reader's revision.
  The claim is kept before the compute runs, so readers who join while it runs
  wait on it. When the value finishes, `heldSinceLocked` checks it against the
  catch-ups that passed during the compute. If any of them touched the value's
  reads, the entry is dropped with cause `computing`.
- **Behind** (`computeBehind`): a part read inside another value's compute, at
  a revision the cache has already passed, while another value is kept under
  that name. Callers at the same name and revision share one claim in
  `c.behind`. The result is kept (line 197) only when nothing is kept under the
  name and `heldSinceLocked` holds from that revision to the head.
- **Recompute** (`recompute`): a facet part recomputed at a catch-up.

A reader who is behind and is not inside a compute goes to `getAtHead`, and its
watch reruns at the head.

## 1. Count a behind value that is not kept

A behind value is discarded silently at line 197 when:

- another value is kept under the name, or
- the history no longer reaches back to the value's revision, or
- a change since then touched its reads.

Each discard is a full compute whose result goes unused. Nothing counts it, so
the course-boards case, where nested readers each computed the same board, was
invisible on the dashboard.

Proposal: after the keep check, when `entry.err == nil` and the value was not
kept, call `c.countDrop(name, "behind")`. Add "behind" to the list of causes in
the `countDrop` doc. This is a value that was computed but never kept, not one
that was kept and then invalidated, so the `Observer.Dropped` doc should say
that too. The counter costs nothing while no observer is set.

## 2. Reuse a head value from where it is valid (validFrom)

Suppose a value was computed at revision `at` from reads R. If no change in
`(s, at]` touched R, then reading R at revision s returns the same results, so
the value is also valid at s. Get does not use this today: a nested reader at
`s < at` finds the kept entry, fails `entry.at <= seqno`, and goes to
computeBehind. That computes the value again, and line 197 then throws it away
because the name is already taken. The course-boards fix worked around this by
reading at the World head. This rule would make that the default.

Proposal: add `validFrom uint64` to `revisionEntry` and start it at `at`. In
Get's reuse check, when the entry is finished, not stale and not expired,
`computing(ctx)` is true, and `seqno < entry.validFrom`, check the history
between seqno and `entry.at` with the same per-step `entry.touched` test that
`heldSinceLocked` uses. If the reads hold, lower `validFrom` to seqno (under
mtx, so it only ever decreases) and reuse the entry.

- The check is lazy. It only runs on the behind path, which today pays a full
  compute, so it never makes that path slower.
- Changes that cannot say what changed make `touched` report true, so those
  values are not reused.
- `touched` checks the value's parts through `checkedSets`, so a value whose
  part changed is not reused either.
- A history that does not reach back to seqno gives false, as it does in
  `heldSinceLocked`.

Test: compute a value at the head, then read it as a part from a snapshot one
untouched revision behind. Expect one compute, not two.

## 3. Recover panics in a compute

If a compute panics, Get never closes `entry.done`, and the entry stays in
`c.entries` because it was kept before the compute ran. In computeBehind, the
claim stays in `c.behind`. Every waiter then blocks until its own context ends,
and every later reader at that name waits on the dead claim. `finish()` also
never runs. The site fix (`85d8d4639`) recovers panics in its own outer compute
and turns them into the build's error. It cannot fix a panic in a nested compute
that holds a claim.

Proposal: every compute path goes through `observe` (shared, stale, retry and
recompute), so recover there. A deferred recover turns the panic into an error
such as `viewcache: computing <family> panicked: <value>`, wrapped with the
stack. `observe` must recover even when no observer is set; today that case
returns `compute()` directly. The panic then follows the existing error path:

- The entry is dropped.
- `done` is closed.
- The claim in `c.behind` ends.
- `finish()` runs.

Waiters receive an error that is not a context error, so each one retries alone
through the "retry" path. A compute that panics every time therefore panics
once per waiter, and every one of those is recovered. That is acceptable.
Re-panicking would take down the server process.

Test: a nested compute that panics. Its waiter should return an error rather
than block, and a later read should compute again.
