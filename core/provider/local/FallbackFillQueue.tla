-------------------------- MODULE FallbackFillQueue --------------------------
\* The fill queue of a placed store's read fallback (bstore-fallback.go), for
\* several placed stores of one Provider that share one byte budget.
\*
\* A read that the fallback alone answers starts a fill of its block. The fill
\* is skipped when the block is already queued or filling, when the store's
\* mount has ended, when the store's queue holds QueueMax blocks, or when the
\* block's bytes do not fit the shared budget. Otherwise it is queued, its
\* bytes are charged to the budget, and a worker starts if the store runs
\* fewer than Slots. A worker takes the oldest fill and returns its bytes to
\* the budget. While the mount lives it runs the fill, which stores the block
\* or fails; once the mount has ended it drops the fill unrun. A worker that
\* finds the queue empty exits; the check and the exit are one step under the
\* queue's lock, as the spawn decision is under the same lock in start.
\*
\* A fill asserts its block's refs, from a decoded read, or only lists them,
\* and the Volume may drop a listed fill but stores or fails an asserted one.
\* A hit asserting refs of a block queued with listed refs upgrades the
\* waiting fill in place; behind a running listed fill it waits as again, and
\* the worker queues it once the running fill ends, or counts a skip when
\* nothing has room. want names the blocks an asserted hit was accepted for
\* and whose asserted fill has not run or been skipped yet.
\*
\* Abstraction and bounds: blocks are 1..NBlocks, the last two bytes and the
\* rest one; stores are 1..NStores. A fill's outcome is chosen freely, and a
\* fill already running when the mount ends may still store its block, as a
\* write may finish before its context is checked. Counters are not modeled.
\* A stored block is served by the local store and never reaches the
\* fallback again, so it is not read again.
EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS NStores, NBlocks, Slots, QueueMax, Budget

Stores == 1..NStores
Blocks == 1..NBlocks
Size(b) == IF b = NBlocks THEN 2 ELSE 1

VARIABLES mount, queue, queued, filling, running, stored, budget, late,
          asserted, again, want
vars == <<mount, queue, queued, filling, running, stored, budget, late,
          asserted, again, want>>

RECURSIVE SeqBytes(_)
SeqBytes(q) == IF q = <<>> THEN 0 ELSE Size(Head(q)) + SeqBytes(Tail(q))

RECURSIVE QueuedBytes(_)
QueuedBytes(S) ==
  IF S = {} THEN 0
  ELSE LET s == CHOOSE x \in S : TRUE IN SeqBytes(queue[s]) + QueuedBytes(S \ {s})

InQueue(s) == {queue[s][i] : i \in 1..Len(queue[s])}

TypeOK ==
  /\ mount \in [Stores -> {"live", "ended"}]
  /\ queue \in [Stores -> Seq(Blocks)]
  /\ queued \in [Stores -> SUBSET Blocks]
  /\ filling \in [Stores -> SUBSET Blocks]
  /\ running \in [Stores -> 0..Slots]
  /\ stored \in [Stores -> SUBSET Blocks]
  /\ budget \in Nat
  /\ late \in SUBSET Stores
  /\ asserted \in [Stores -> SUBSET Blocks]
  /\ again \in [Stores -> SUBSET Blocks]
  /\ want \in [Stores -> SUBSET Blocks]

Init ==
  /\ mount = [s \in Stores |-> "live"]
  /\ queue = [s \in Stores |-> <<>>]
  /\ queued = [s \in Stores |-> {}]
  /\ filling = [s \in Stores |-> {}]
  /\ running = [s \in Stores |-> 0]
  /\ stored = [s \in Stores |-> {}]
  /\ budget = 0
  /\ late = {}
  /\ asserted = [s \in Stores |-> {}]
  /\ again = [s \in Stores |-> {}]
  /\ want = [s \in Stores |-> {}]

\* A fallback hit of b on s, asserting its refs when a: queue its fill, or
\* skip it.
Start(s, b, a) ==
  /\ b \notin stored[s]
  /\ b \notin queued[s]
  /\ mount[s] = "live"
  /\ Len(queue[s]) < QueueMax
  /\ budget + Size(b) <= Budget
  /\ queue' = [queue EXCEPT ![s] = Append(@, b)]
  /\ queued' = [queued EXCEPT ![s] = @ \cup {b}]
  /\ budget' = budget + Size(b)
  /\ running' = [running EXCEPT ![s] = IF @ < Slots THEN @ + 1 ELSE @]
  /\ asserted' = [asserted EXCEPT ![s] = IF a THEN @ \cup {b} ELSE @]
  /\ want' = [want EXCEPT ![s] = IF a THEN @ \cup {b} ELSE @]
  /\ UNCHANGED <<mount, filling, stored, late, again>>

\* A hit asserting the refs of b, queued or filling on s with listed refs:
\* the waiting fill takes the asserted refs, or the asserted fill waits as
\* again behind the running one.
Upgrade(s, b) ==
  /\ b \in queued[s]
  /\ b \notin asserted[s]
  /\ want' = [want EXCEPT ![s] = @ \cup {b}]
  /\ IF b \in filling[s]
       THEN /\ again' = [again EXCEPT ![s] = @ \cup {b}]
            /\ UNCHANGED asserted
       ELSE /\ asserted' = [asserted EXCEPT ![s] = @ \cup {b}]
            /\ UNCHANGED again
  /\ UNCHANGED <<mount, queue, queued, filling, running, stored, budget, late>>

\* An idle worker of s takes the oldest fill: it runs it while the mount
\* lives, and drops it unrun once the mount has ended.
Take(s) ==
  /\ running[s] > Cardinality(filling[s])
  /\ queue[s] # <<>>
  /\ LET b == Head(queue[s])
         run == mount[s] = "live"
     IN /\ queue' = [queue EXCEPT ![s] = Tail(@)]
        /\ budget' = budget - Size(b)
        /\ IF run
             THEN /\ filling' = [filling EXCEPT ![s] = @ \cup {b}]
                  /\ UNCHANGED <<queued, asserted, want>>
             ELSE /\ queued' = [queued EXCEPT ![s] = @ \ {b}]
                  /\ asserted' = [asserted EXCEPT ![s] = @ \ {b}]
                  /\ want' = [want EXCEPT ![s] = @ \ {b}]
                  /\ UNCHANGED filling
        /\ late' = IF run /\ mount[s] = "ended" THEN late \cup {s} ELSE late
  /\ UNCHANGED <<mount, running, stored, again>>

\* The asserted fill of b waiting on s has room in the queue.
AgainFits(s, b) == mount[s] = "live" /\ Len(queue[s]) < QueueMax /\ budget + Size(b) <= Budget

\* A running fill of b on s ends: it stored the block, the Volume dropped a
\* fill with listed refs, or it failed. Its worker then queues the asserted
\* fill that waited on it, or skips it when nothing has room.
Finish(s, b) ==
  /\ b \in filling[s]
  /\ filling' = [filling EXCEPT ![s] = @ \ {b}]
  /\ \E outcome \in {"stored", "dropped", "failed"} :
       /\ outcome = "dropped" => b \notin asserted[s]
       /\ stored' = [stored EXCEPT ![s] = IF outcome = "stored" THEN @ \cup {b} ELSE @]
  /\ again' = [again EXCEPT ![s] = @ \ {b}]
  /\ IF b \in again[s] /\ AgainFits(s, b)
       THEN /\ queue' = [queue EXCEPT ![s] = Append(@, b)]
            /\ budget' = budget + Size(b)
            /\ asserted' = [asserted EXCEPT ![s] = @ \cup {b}]
            /\ UNCHANGED <<queued, want>>
       ELSE /\ queued' = [queued EXCEPT ![s] = @ \ {b}]
            /\ asserted' = [asserted EXCEPT ![s] = @ \ {b}]
            /\ want' = [want EXCEPT ![s] = @ \ {b}]
            /\ UNCHANGED <<queue, budget>>
  /\ UNCHANGED <<mount, running, late>>

\* An idle worker of s finds the queue empty and frees its slot.
Exit(s) ==
  /\ running[s] > Cardinality(filling[s])
  /\ queue[s] = <<>>
  /\ running' = [running EXCEPT ![s] = @ - 1]
  /\ UNCHANGED <<mount, queue, queued, filling, stored, budget, late, asserted, again, want>>

\* The mount of s ends: its context is cancelled.
End(s) ==
  /\ mount[s] = "live"
  /\ mount' = [mount EXCEPT ![s] = "ended"]
  /\ UNCHANGED <<queue, queued, filling, running, stored, budget, late, asserted, again, want>>

Next ==
  \/ \E s \in Stores, b \in Blocks, a \in BOOLEAN : Start(s, b, a)
  \/ \E s \in Stores, b \in Blocks : Upgrade(s, b) \/ Finish(s, b)
  \/ \E s \in Stores : Take(s) \/ Exit(s) \/ End(s)

Spec ==
  /\ Init /\ [][Next]_vars
  /\ \A s \in Stores : WF_vars(Take(s)) /\ WF_vars(Exit(s))
  /\ \A s \in Stores, b \in Blocks : WF_vars(Finish(s, b))

\* The queued bytes of every store fit the one budget, and the budget counts
\* exactly them.
BudgetBound == budget = QueuedBytes(Stores) /\ budget <= Budget

\* A block is queued or filling at most once per store, and queued names
\* exactly those.
NoDuplicate ==
  \A s \in Stores :
    /\ \A i, j \in 1..Len(queue[s]) : i # j => queue[s][i] # queue[s][j]
    /\ InQueue(s) \cap filling[s] = {}
    /\ queued[s] = InQueue(s) \cup filling[s]
    /\ asserted[s] \subseteq queued[s]
    /\ again[s] \subseteq filling[s]

\* A queued fill always has a worker, idle or filling, that will take it:
\* a worker exits only on an empty queue.
QueueHasWorker == \A s \in Stores : queue[s] # <<>> => running[s] > 0

\* No store runs more than Slots fills, nor queues more than QueueMax.
Bounded == \A s \in Stores : Cardinality(filling[s]) <= running[s] /\ Len(queue[s]) <= QueueMax

\* An accepted asserted hit is never lost to the listed fill it met: its
\* block waits with asserted refs, or runs with them, or runs listed with
\* the asserted fill waiting behind it, until the asserted fill runs or its
\* skip is counted.
AssertedKept ==
  \A s \in Stores : \A b \in want[s] :
    \/ b \in InQueue(s) /\ b \in asserted[s]
    \/ b \in filling[s] /\ (b \in asserted[s] \/ b \in again[s])

\* No fill begins after its store's mount has ended.
NoFillAfterEnd == late = {}

\* Every queued fill leaves the queue.
FillsLeave == \A s \in Stores, b \in Blocks : b \in queued[s] ~> b \notin queued[s]

\* Once a mount ends, its queue empties and its workers exit.
EndedStops ==
  \A s \in Stores : mount[s] = "ended" ~> (queue[s] = <<>> /\ running[s] = 0 /\ filling[s] = {})
=============================================================================
