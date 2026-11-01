---- MODULE FreeQueue ----
\* Phase 5's proposed queue; current code still sweeps, so no Go deleter is
\* claimed here. The existing release resolver and ledger supply zero counts
\* and the adoption/oldest-reader watermark. Queue and deletion are separate
\* durable transactions. A crash loses the worker's acquisition, not the queue.
\* Two blocks, four clock values, two chunks, two revivals and one waiting
\* World writer. Two chunks suffice to violate a one-chunk bound; the model
\* does not claim a wall-clock scheduling bound.
\* Put is the publication writer's durable put/count update: it cancels a
\* queued free atomically. A live read starts only after that publication.
\* An uncommitted writer's memory is not a durable root; publication ordering
\* remains an assumption shared with the other reclamation specs.
\* Count recheck, grace and cancellation have separate observable contracts:
\* cancellation reclaims the queue entry even if recheck would prevent loss.
\* Reader pin/watermark prevents queue admission; time alone never substitutes
\* for that test. No fairness assumption bounds a real scheduler's latency:
\* WriterWait is the maximum number of chunks blocking a waiting writer.
EXTENDS Naturals, FiniteSets
CONSTANTS Recheck, Grace, CancelPut, YieldWriter
Blocks == 1..2
VARIABLES stored, count, names, deadAt, base, pin, queue, now, acquired,
          writer, waited, chunks, deletedAt, queuedAt, budget, restarted, selected
vars == <<stored, count, names, deadAt, base, pin, queue, now, acquired, writer,
      waited, chunks, deletedAt, queuedAt, budget, restarted, selected>>
Init ==
  /\ stored = Blocks /\ count = [b \in Blocks |-> 0] /\ names = {}
  /\ deadAt = [b \in Blocks |-> b] /\ base = 0 /\ pin = 0
  /\ queue = {} /\ now = 0 /\ acquired = 0
  /\ writer = "idle" /\ waited = 0 /\ chunks = 0
  /\ deletedAt = [b \in Blocks |-> 0] /\ queuedAt = [b \in Blocks |-> 0]
  /\ budget = 2 /\ restarted = FALSE /\ selected = {}
Adopt ==
  /\ base < 2 /\ base' = base + 1
  /\ UNCHANGED <<stored, count, names, deadAt, pin, queue, now, acquired, writer, waited,
      chunks, deletedAt, queuedAt, budget, restarted, selected>>
ReleasePin ==
  /\ pin = 0 /\ pin' = 2
  /\ UNCHANGED <<stored, count, names, deadAt, base, queue, now, acquired, writer, waited,
      chunks, deletedAt, queuedAt, budget, restarted, selected>>
Queue(b) ==
  /\ b \in stored \ queue /\ b \notin names /\ count[b] = 0
  /\ deadAt[b] <= base /\ deadAt[b] <= pin /\ deletedAt[b] = 0
  /\ queue' = queue \cup {b} /\ queuedAt' = [queuedAt EXCEPT ![b] = now]
  /\ UNCHANGED <<stored, count, names, deadAt, base, pin, now, acquired, writer, waited,
      chunks, deletedAt, budget, restarted, selected>>
Tick ==
  /\ now < 3 /\ now' = now + 1
  /\ UNCHANGED <<stored, count, names, deadAt, base, pin, queue, acquired, writer, waited,
      chunks, deletedAt, queuedAt, budget, restarted, selected>>
\* Put/adoption of a queued block adds its live holder and cancels its token.
Put(b) ==
  /\ b \in stored /\ b \notin names /\ budget > 0
  /\ count' = [count EXCEPT ![b] = @ + 1] /\ names' = names \cup {b}
  /\ queue' = IF CancelPut THEN queue \ {b} ELSE queue
  /\ budget' = budget - 1
  /\ UNCHANGED <<stored, deadAt, base, pin, now, acquired, writer, waited, chunks, deletedAt,
      queuedAt, restarted, selected>>
\* A revived block can die again. That zero-holder transition installs a
\* new retirement stamp and grace deadline, even if an old acquired chunk
\* still contains its key. The initial reader has already released before
\* the first queue admission; this model has no later old-version reader.
Release(b) ==
  /\ b \in names /\ count[b] = 1 /\ pin >= base /\ base > 0
  /\ names' = names \ {b} /\ count' = [count EXCEPT ![b] = 0]
  /\ deadAt' = [deadAt EXCEPT ![b] = base]
  /\ queue' = queue \cup {b} /\ queuedAt' = [queuedAt EXCEPT ![b] = now]
  /\ UNCHANGED <<stored, base, pin, now, acquired, writer, waited, chunks, deletedAt, budget,
      restarted, selected>>
\* Worker acquisition precedes a writer request; it covers at most one chunk.
Acquire ==
  /\ acquired = 0 /\ queue # {} /\ writer # "waiting" /\ chunks < 2
  /\ ~Grace \/ \E b \in queue : now >= queuedAt[b] + 2
  /\ acquired' = 1 /\ selected' = {b \in queue : ~Grace \/ now >= queuedAt[b] + 2}
  /\ UNCHANGED <<stored, count, names, deadAt, base, pin, queue, now, writer, waited, chunks,
      deletedAt, queuedAt, budget, restarted>>
RequestWriter ==
  /\ writer = "idle" /\ writer' = "waiting"
  /\ UNCHANGED <<stored, count, names, deadAt, base, pin, queue, now, acquired, waited,
      chunks, deletedAt, queuedAt, budget, restarted, selected>>
\* Acquire keeps a bounded candidate chunk across writer transactions. A put
\* can cancel its durable queue token after acquisition, so recheck is needed
\* even with correct cancellation. One delete per chunk. The count is re-read in the deleting transaction;
\* seeing a positive count removes a stale token instead of deleting the page.
DeleteChunk(b) ==
  /\ acquired = 1 /\ b \in selected /\ chunks < 2
  /\ ~Grace \/ now >= queuedAt[b] + 2
  /\ LET frees == ~Recheck \/ count[b] = 0 IN
       /\ stored' = IF frees THEN stored \ {b} ELSE stored
       /\ deletedAt' = IF frees THEN [deletedAt EXCEPT ![b] = now + 1] ELSE deletedAt
  /\ queue' = queue \ {b} /\ selected' = selected \ {b} /\ chunks' = chunks + 1
  /\ waited' = IF writer = "waiting" THEN waited + 1 ELSE waited
  /\ acquired' = IF (YieldWriter /\ writer = "waiting") \/ selected \ {b} = {}
                    \/ chunks + 1 = 2 THEN 0 ELSE acquired
  /\ UNCHANGED <<count, names, deadAt, base, pin, now, writer, queuedAt, budget, restarted>>
ServeWriter ==
  /\ writer = "waiting" /\ acquired = 0 /\ writer' = "done"
  /\ UNCHANGED <<stored, count, names, deadAt, base, pin, queue, now, acquired, waited,
      chunks, deletedAt, queuedAt, budget, restarted, selected>>
Crash ==
  /\ ~restarted /\ acquired = 1
  /\ acquired' = 0 /\ restarted' = TRUE /\ selected' = {}
  /\ UNCHANGED <<stored, count, names, deadAt, base, pin, queue, now, writer, waited, chunks,
      deletedAt, queuedAt, budget>>
Next == Adopt \/ ReleasePin \/ Tick \/ Acquire \/ RequestWriter \/ ServeWriter \/ Crash
  \/ (\E b \in Blocks : Queue(b) \/ Put(b) \/ Release(b) \/ DeleteChunk(b))
  \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
NoFreedRead == names \subseteq stored
PutCancels == names \cap queue = {}
NoEarlyDelete == \A b \in Blocks : deletedAt[b] # 0 =>
                   deletedAt[b] - 1 >= queuedAt[b] + 2
WriterWait == waited <= 1
Watermark == \A b \in queue : deadAt[b] <= base /\ deadAt[b] <= pin
====
