----------------------------- MODULE QueueFloor -----------------------------
\* MarkBlocks lowers floor before committing. clear reads under the same
\* storage writer, then applies its new floor after commit under bcast.
\* One later writer with an earlier timestamp exposes the post-commit
\* publication race. A batch clears in transactions of a bounded size, so the
\* batch here, the older blocks 1 and 2, clears in two: each takes the writer,
\* removes its part, reads the head and commits, then publishes its floor, and
\* the producer may run before, between or inside them. Real key prefixes and
\* discarded transactions are covered by the upload/restart run-throughs.
EXTENDS Naturals, FiniteSets, Sequences
\* End is the head of an empty queue, past every key.
End == 3
\* Parts are the batch's transactions, in order.
Parts == <<{1}, {2}>>
VARIABLES queue, floor, lowered, writer, producer, clearer, part, snap, head
vars == <<queue, floor, lowered, writer, producer, clearer, part, snap, head>>
Min(S) == CHOOSE k \in S : \A j \in S : k <= j
Init == /\ queue = {1, 2} /\ floor = 1 /\ lowered = 0 /\ writer = "none"
        /\ producer = "ready" /\ clearer = "ready" /\ part = 1
        /\ snap = 0 /\ head = End
ProducerBegin == /\ producer = "ready" /\ writer = "none"
                 /\ producer' = "lower" /\ writer' = "producer"
                 /\ UNCHANGED <<queue, floor, lowered, clearer, part, snap, head>>
Lower == /\ producer = "lower" /\ floor' = 0 /\ lowered' = lowered + 1
         /\ producer' = "commit"
         /\ UNCHANGED <<queue, writer, clearer, part, snap, head>>
ProducerCommit == /\ producer = "commit" /\ queue' = queue \cup {0}
                  /\ writer' = "none" /\ producer' = "done"
                  /\ UNCHANGED <<floor, lowered, clearer, part, snap, head>>
\* A transaction removes its part and reads the head that remains.
ClearBegin == /\ clearer = "ready" /\ writer = "none"
              /\ writer' = "clear" /\ clearer' = "commit"
              /\ queue' = queue \ Parts[part] /\ snap' = lowered
              /\ head' = IF queue' = {} THEN End ELSE Min(queue')
              /\ UNCHANGED <<floor, lowered, producer, part>>
ClearCommit == /\ clearer = "commit" /\ writer' = "none"
               /\ clearer' = "apply"
               /\ UNCHANGED <<queue, floor, lowered, producer, part, snap, head>>
\* Each transaction publishes its floor, then the next part queues for the
\* writer again.
ClearApply == /\ clearer = "apply"
              /\ floor' = IF lowered = snap \/ head < floor THEN head ELSE floor
              /\ IF part < Len(Parts)
                 THEN clearer' = "ready" /\ part' = part + 1
                 ELSE clearer' = "done" /\ part' = part
              /\ UNCHANGED <<queue, lowered, writer, producer, snap, head>>
Next == ProducerBegin \/ Lower \/ ProducerCommit \/ ClearBegin \/ ClearCommit
        \/ ClearApply \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
VisibleQueue == \A key \in queue : floor <= key
VisibleInFlight == producer = "commit" => floor <= 0
=============================================================================
