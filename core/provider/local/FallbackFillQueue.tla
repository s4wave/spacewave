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

VARIABLES mount, queue, queued, filling, running, stored, budget, late
vars == <<mount, queue, queued, filling, running, stored, budget, late>>

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

Init ==
  /\ mount = [s \in Stores |-> "live"]
  /\ queue = [s \in Stores |-> <<>>]
  /\ queued = [s \in Stores |-> {}]
  /\ filling = [s \in Stores |-> {}]
  /\ running = [s \in Stores |-> 0]
  /\ stored = [s \in Stores |-> {}]
  /\ budget = 0
  /\ late = {}

\* A fallback hit of b on s: queue its fill, or skip it.
Start(s, b) ==
  /\ b \notin stored[s]
  /\ b \notin queued[s]
  /\ mount[s] = "live"
  /\ Len(queue[s]) < QueueMax
  /\ budget + Size(b) <= Budget
  /\ queue' = [queue EXCEPT ![s] = Append(@, b)]
  /\ queued' = [queued EXCEPT ![s] = @ \cup {b}]
  /\ budget' = budget + Size(b)
  /\ running' = [running EXCEPT ![s] = IF @ < Slots THEN @ + 1 ELSE @]
  /\ UNCHANGED <<mount, filling, stored, late>>

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
                  /\ UNCHANGED queued
             ELSE /\ queued' = [queued EXCEPT ![s] = @ \ {b}]
                  /\ UNCHANGED filling
        /\ late' = IF run /\ mount[s] = "ended" THEN late \cup {s} ELSE late
  /\ UNCHANGED <<mount, running, stored>>

\* A running fill of b on s ends: it stored the block, or failed.
Finish(s, b) ==
  /\ b \in filling[s]
  /\ filling' = [filling EXCEPT ![s] = @ \ {b}]
  /\ queued' = [queued EXCEPT ![s] = @ \ {b}]
  /\ \E ok \in BOOLEAN : stored' = [stored EXCEPT ![s] = IF ok THEN @ \cup {b} ELSE @]
  /\ UNCHANGED <<mount, queue, running, budget, late>>

\* An idle worker of s finds the queue empty and frees its slot.
Exit(s) ==
  /\ running[s] > Cardinality(filling[s])
  /\ queue[s] = <<>>
  /\ running' = [running EXCEPT ![s] = @ - 1]
  /\ UNCHANGED <<mount, queue, queued, filling, stored, budget, late>>

\* The mount of s ends: its context is cancelled.
End(s) ==
  /\ mount[s] = "live"
  /\ mount' = [mount EXCEPT ![s] = "ended"]
  /\ UNCHANGED <<queue, queued, filling, running, stored, budget, late>>

Next ==
  \/ \E s \in Stores, b \in Blocks : Start(s, b) \/ Finish(s, b)
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

\* A queued fill always has a worker, idle or filling, that will take it:
\* a worker exits only on an empty queue.
QueueHasWorker == \A s \in Stores : queue[s] # <<>> => running[s] > 0

\* No store runs more than Slots fills, nor queues more than QueueMax.
Bounded == \A s \in Stores : Cardinality(filling[s]) <= running[s] /\ Len(queue[s]) <= QueueMax

\* No fill begins after its store's mount has ended.
NoFillAfterEnd == late = {}

\* Every queued fill leaves the queue.
FillsLeave == \A s \in Stores, b \in Blocks : b \in queued[s] ~> b \notin queued[s]

\* Once a mount ends, its queue empties and its workers exit.
EndedStops ==
  \A s \in Stores : mount[s] = "ended" ~> (queue[s] = <<>> /\ running[s] = 0 /\ filling[s] = {})
=============================================================================
