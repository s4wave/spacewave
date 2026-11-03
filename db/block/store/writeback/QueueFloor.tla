----------------------------- MODULE QueueFloor -----------------------------
\* MarkBlocks lowers floor before committing. clear reads under the same
\* storage writer, then applies its new floor after commit under bcast.
\* One older queued block and one later writer with an earlier timestamp
\* expose the post-commit publication race. Real key prefixes and discarded
\* transactions are covered by the upload/restart run-throughs.
EXTENDS Naturals, FiniteSets
VARIABLES queue, floor, lowered, writer, producer, clearer, snap, head
vars == <<queue, floor, lowered, writer, producer, clearer, snap, head>>
Init == /\ queue = {1} /\ floor = 1 /\ lowered = 0 /\ writer = "none"
        /\ producer = "ready" /\ clearer = "ready" /\ snap = 0 /\ head = 2
ProducerBegin == /\ producer = "ready" /\ writer = "none"
                 /\ producer' = "lower" /\ writer' = "producer"
                 /\ UNCHANGED <<queue, floor, lowered, clearer, snap, head>>
Lower == /\ producer = "lower" /\ floor' = 0 /\ lowered' = lowered + 1
         /\ producer' = "commit"
         /\ UNCHANGED <<queue, writer, clearer, snap, head>>
ProducerCommit == /\ producer = "commit" /\ queue' = queue \cup {0}
                  /\ writer' = "none" /\ producer' = "done"
                  /\ UNCHANGED <<floor, lowered, clearer, snap, head>>
ClearBegin == /\ clearer = "ready" /\ writer = "none"
              /\ writer' = "clear" /\ clearer' = "commit"
              /\ queue' = queue \ {1} /\ snap' = lowered
              /\ head' = IF 0 \in queue THEN 0 ELSE 2
              /\ UNCHANGED <<floor, lowered, producer>>
ClearCommit == /\ clearer = "commit" /\ writer' = "none"
               /\ clearer' = "apply"
               /\ UNCHANGED <<queue, floor, lowered, producer, snap, head>>
ClearApply == /\ clearer = "apply" /\ clearer' = "done"
              /\ floor' = IF lowered = snap \/ head < floor THEN head ELSE floor
              /\ UNCHANGED <<queue, lowered, writer, producer, snap, head>>
Next == ProducerBegin \/ Lower \/ ProducerCommit \/ ClearBegin \/ ClearCommit
        \/ ClearApply \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
VisibleQueue == \A key \in queue : floor <= key
VisibleInFlight == producer = "commit" => floor <= 0
=============================================================================
