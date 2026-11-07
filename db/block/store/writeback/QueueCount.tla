----------------------------- MODULE QueueCount -----------------------------
\* The upload queue's kept count and the walk that starts it on a store that
\* never kept one. The store keeps one row beside the queue: the count n, the
\* walk's cursor, the last key it counted, its end, the first key of the writes
\* after it started, and done once it counted every entry between. A key is
\* covered once done, at or below the cursor, or at or past the end. Every
\* transaction that inserts or deletes a queue entry adjusts n for a covered
\* key alone, in the same transaction, and each walk slice counts the next
\* keys past the cursor below the end and moves the cursor over them in one
\* transaction, so writes after the start count as they commit. So
\* n plus the uncovered entries is the queue's size in every committed state,
\* through crashes and restarts, and n alone once done.
\*
\* The status is the in-memory projection of n: each transaction's change to
\* n is owed to it after commit and applied later, in any order, and a crash
\* drops it with what is owed; an open reads n again. Keys are 1..MaxKey, each
\* entry of size one, and a slice counts at most Slice keys. A clear that
\* empties the queue sets the count to zero and ends the walk, which mends a
\* count a writer without it left off. RestartRecount is the runtime switch
\* that resets the row at an open so the walk counts again.
EXTENDS Integers, FiniteSets

CONSTANTS MaxKey, Slice, MaxOps

Keys == 1..MaxKey

VARIABLES
    queue,   \* the committed queue entries
    n,       \* the row's count
    cursor,  \* the row's cursor: every key at or below it is counted
    end,     \* the row's end: every key at or past it is counted
    done,    \* the row's walk reached the queue's end
    walker,  \* the walk's step: "slice", or "cursor" between the two halves
             \* of a slice that persists its cursor apart
    upTo,    \* the cursor that slice will persist
    up,      \* the process is running
    status,  \* the in-memory count
    owed,    \* the changes committed and not yet applied to status, a bag as
             \* a function from change to how many are owed
    ops      \* transactions so far, bounding the state space

vars == <<queue, n, cursor, end, done, walker, upTo, up, status, owed, ops>>

Changes == -MaxKey..MaxKey

Ends == 1..(MaxKey + 1)

Covered(k) == done \/ k <= cursor \/ k >= end

\* EndPast(q) is the end a walk over queue q starts with: just past its last
\* key, so every entry the walk starts with lies below it.
EndPast(q) == IF q = {} THEN 1 ELSE (CHOOSE k \in q : \A j \in q : j <= k) + 1

Uncovered == {k \in queue : ~Covered(k)}

\* Owe adds change d to what the status is owed.
Owe(d) == owed' = [owed EXCEPT ![d] = @ + 1]

NoneOwed == [d \in Changes |-> 0]

Init ==
    /\ queue = {}
    /\ n = 0
    /\ cursor = 0
    /\ end = 1
    /\ done = FALSE
    /\ walker = "slice"
    /\ upTo = 0
    /\ up = TRUE
    /\ status = 0
    /\ owed = NoneOwed
    /\ ops = 0

\* A store that never kept a count starts with entries and a fresh row.
InitExisting ==
    /\ queue \in SUBSET Keys
    /\ n = 0
    /\ cursor = 0
    /\ end = EndPast(queue)
    /\ done = FALSE
    /\ walker = "slice"
    /\ upTo = 0
    /\ up = TRUE
    /\ status = 0
    /\ owed = NoneOwed
    /\ ops = 0

Step == ops' = ops + 1

\* A write queues k, adjusting the count only when k is covered.
Insert(k) ==
    /\ up /\ ops < MaxOps
    /\ k \notin queue
    /\ queue' = queue \cup {k}
    /\ IF Covered(k)
       THEN n' = n + 1 /\ Owe(1)
       ELSE UNCHANGED <<n, owed>>
    /\ Step
    /\ UNCHANGED <<end, cursor, done, walker, upTo, up, status>>

\* A clear deletes k, adjusting the count only when k is covered. One that
\* empties the queue sets the count to zero and ends the walk.
Delete(k) ==
    /\ up /\ ops < MaxOps
    /\ k \in queue
    /\ queue' = queue \ {k}
    /\ IF queue' = {}
       THEN n' = 0 /\ done' = TRUE /\ Owe(-n)
       ELSE /\ UNCHANGED done
            /\ IF Covered(k)
               THEN n' = n - 1 /\ Owe(-1)
               ELSE UNCHANGED <<n, owed>>
    /\ Step
    /\ UNCHANGED <<end, cursor, walker, upTo, up, status>>

\* Next(c) is the next Slice keys of the queue past cursor c.
NextKeys(c) ==
    LET past == {k \in queue : k > c /\ k < end}
        Below(k) == Cardinality({j \in past : j < k})
    IN {k \in past : Below(k) < Slice}

\* A slice counts the next keys below the end and moves the cursor over them,
\* or ends the walk once it reaches the end, in one transaction.
WalkSlice ==
    /\ up /\ ~done /\ walker = "slice"
    /\ LET walked == NextKeys(cursor)
           ended == Cardinality(walked) < Slice
           last == IF walked = {} THEN cursor
                   ELSE CHOOSE k \in walked : \A j \in walked : j <= k
       IN /\ n' = n + Cardinality(walked)
          /\ Owe(Cardinality(walked))
          /\ cursor' = IF ended THEN cursor ELSE last
          /\ done' = ended
          /\ UNCHANGED <<walker, upTo>>
    /\ UNCHANGED <<end, queue, up, status, ops>>

\* The second half of a slice that persisted its cursor apart, which the
\* correct slice never reaches.
PersistCursor ==
    /\ up /\ walker = "cursor"
    /\ cursor' = upTo
    /\ walker' = "slice"
    /\ UNCHANGED <<end, queue, n, done, upTo, up, status, owed, ops>>

\* The status applies one owed change.
Apply(d) ==
    /\ up /\ owed[d] > 0
    /\ status' = status + d
    /\ owed' = [owed EXCEPT ![d] = @ - 1]
    /\ UNCHANGED <<end, queue, n, cursor, done, walker, upTo, up, ops>>

\* Disabling uploads drops the queue and ends the walk with nothing counted;
\* the status drops what it was owed, since the change of enabled orders
\* every projection before it out.
Disable ==
    /\ up /\ ops < MaxOps /\ walker = "slice"
    /\ queue' = {} /\ n' = 0 /\ done' = TRUE /\ status' = 0
    /\ owed' = NoneOwed
    /\ Step
    /\ UNCHANGED <<end, cursor, walker, upTo, up>>

\* A crash loses the status and what it is owed, and a half-done slice.
Crash ==
    /\ up /\ ops < MaxOps
    /\ up' = FALSE
    /\ owed' = NoneOwed
    /\ walker' = "slice"
    /\ Step
    /\ UNCHANGED <<end, queue, n, cursor, done, upTo, status>>

\* An open reads the row into the status and resumes the walk.
Restart ==
    /\ ~up
    /\ up' = TRUE
    /\ status' = n
    /\ UNCHANGED <<end, queue, n, cursor, done, walker, upTo, owed, ops>>

\* An open with the switch that recounts at open on resets the row in its
\* transaction, so the walk counts the queue again.
RestartRecount ==
    /\ ~up /\ ops < MaxOps
    /\ up' = TRUE
    /\ n' = 0 /\ cursor' = 0 /\ done' = FALSE /\ status' = 0
    /\ end' = EndPast(queue)
    /\ Step
    /\ UNCHANGED <<queue, walker, upTo, owed>>

\* The bounded run ends with every transaction spent.
Idle == ops = MaxOps /\ UNCHANGED vars

Next ==
    \/ \E k \in Keys : Insert(k) \/ Delete(k)
    \/ WalkSlice
    \/ PersistCursor
    \/ \E d \in Changes : Apply(d)
    \/ Disable
    \/ Crash
    \/ Restart
    \/ RestartRecount
    \/ Idle

\* The walk runs while the process does, an open follows a crash, and the
\* status applies what it is owed. Transactions, crashes and recounts are
\* bounded by MaxOps; slices are not, since each moves the cursor or ends
\* the walk.
Fairness ==
    /\ WF_vars(WalkSlice)
    /\ WF_vars(PersistCursor)
    /\ WF_vars(Restart)
    /\ \A d \in Changes : WF_vars(Apply(d))

Spec == InitExisting /\ [][Next]_vars /\ Fairness

TypeOK ==
    /\ queue \subseteq Keys
    /\ n \in Int
    /\ cursor \in 0..MaxKey
    /\ end \in Ends
    /\ done \in BOOLEAN
    /\ walker \in {"slice", "cursor"}

\* The count plus the entries the walk has yet to reach is the queue's size.
CountedPlusUncovered == n + Cardinality(Uncovered) = Cardinality(queue)

\* Once done, the count is the queue's size.
DoneExact == done => n = Cardinality(queue)

\* OwedSum is the sum of the changes owed to the status.
OwedSum ==
    LET Sum[i \in Changes] ==
          IF i = -MaxKey THEN i * owed[i] ELSE Sum[i - 1] + i * owed[i]
    IN Sum[MaxKey]

\* The status plus what it is owed is the count, so once nothing is owed the
\* status is the count.
StatusFollows == up => status + OwedSum = n

\* The walk ends, whatever crashes and writes come first.
WalkEnds == <>[]done

\* The status settles at the count.
StatusSettles == <>[](up /\ status = n)
=============================================================================
