---- MODULE AdmissionPreparation ----
\* A caller holding SQLite discards before Prepare. Preparation owns the
\* candidate lease before capacity, then reruns the body with both available.
\* A second ordinary candidate may already own the lease while it waits for
\* the empty queue an oversized publication needs. Receipts eventually settle.
EXTENDS Naturals, FiniteSets
CONSTANTS DiscardBeforeWait, LeaseBeforeCapacity, CheckEpoch
VARIABLES pc, sql, engine, lease, claims, pending, failed, admitted, canceled
vars == <<pc, sql, engine, lease, claims, pending, failed, admitted, canceled>>
Init == /\ pc = "discard" /\ sql = TRUE /\ engine = TRUE
        /\ lease = 1 /\ claims = {} /\ pending \in 0..2
        /\ failed = FALSE /\ admitted = FALSE /\ canceled = FALSE
Discard == /\ pc = "discard" /\ pc' = "lease" /\ lease' \in {0, 2}
           /\ sql' = ~DiscardBeforeWait /\ engine' = ~DiscardBeforeWait
           /\ UNCHANGED <<claims, pending, failed, admitted, canceled>>
TakeLease == /\ pc = "lease" /\ lease = 0
             /\ (LeaseBeforeCapacity \/ 1 \in claims)
             /\ lease' = 1 /\ pc' = IF 1 \in claims THEN "body" ELSE "capacity"
             /\ UNCHANGED <<sql, engine, claims, pending, failed, admitted, canceled>>
BadCapacity == /\ ~LeaseBeforeCapacity /\ pc = "lease" /\ pending < 2 /\ claims = {}
               /\ claims' = {1}
               /\ UNCHANGED <<pc, sql, engine, lease, pending, failed, admitted, canceled>>
Reserve == /\ pc = "capacity" /\ pending < 2 /\ 1 \notin claims
           /\ claims' = claims \cup {1} /\ pc' = "body"
           /\ UNCHANGED <<sql, engine, lease, pending, failed, admitted, canceled>>
OtherCandidate == /\ lease = 2 /\ pending = 0 /\ claims = {}
                  /\ lease' = 0 /\ pending' = 1
                  /\ UNCHANGED <<pc, sql, engine, claims, failed, admitted, canceled>>
Body == /\ pc = "body" /\ (~CheckEpoch \/ ~failed)
        /\ sql' = TRUE /\ engine' = TRUE /\ pc' = "publish"
        /\ UNCHANGED <<lease, claims, pending, failed, admitted, canceled>>
Publish == /\ pc = "publish" /\ (~CheckEpoch \/ ~failed)
           /\ admitted' = TRUE /\ sql' = FALSE /\ engine' = FALSE
           /\ lease' = 0 /\ claims' = {} /\ pending' = pending + 1 /\ pc' = "done"
           /\ UNCHANGED <<failed, canceled>>
Fail == /\ ~failed /\ ~admitted /\ failed' = TRUE
        /\ UNCHANGED <<pc, sql, engine, lease, claims, pending, admitted, canceled>>
Cancel == /\ ~admitted /\ ~canceled /\ canceled' = TRUE
          /\ claims' = {} /\ lease' = IF lease = 1 THEN 0 ELSE lease
          /\ pc' = "closed" /\ sql' = FALSE /\ engine' = FALSE
          /\ UNCHANGED <<pending, failed, admitted>>
Reject == /\ CheckEpoch /\ failed /\ pc \in {"body", "publish"}
          /\ claims' = {} /\ lease' = 0 /\ pc' = "closed" /\ sql' = FALSE /\ engine' = FALSE
          /\ UNCHANGED <<pending, failed, admitted, canceled>>
Settle == /\ pending > 0 /\ pending' = pending - 1
          /\ UNCHANGED <<pc, sql, engine, lease, claims, failed, admitted, canceled>>
Next == Discard \/ TakeLease \/ BadCapacity \/ Reserve \/ OtherCandidate \/ Body \/ Publish \/ Fail \/ Cancel \/ Reject \/ Settle \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
NoWriterWait == pc \in {"lease", "capacity"} => (~sql /\ ~engine)
NoPreparationCycle == ~(lease = 2 /\ 1 \in claims /\ pc = "lease" /\ pending = 0)
NoFailedHistory == ~admitted \/ ~failed
Bounded == pending + Cardinality(claims) <= 2
Released == pc \in {"done", "closed"} => (1 \notin claims /\ lease # 1 /\ ~sql /\ ~engine)
====
