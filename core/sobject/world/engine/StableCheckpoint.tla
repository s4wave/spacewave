---- MODULE StableCheckpoint ----
\* SQL-primary stable checkpoints without the page view (checkpoint.go).
\* nameWorld names the last World receipt; waitNamed waits outside both writer
\* locks. adoptStable reserves queue capacity before taking the provider state
\* lock, submits a head-only publication on the admitted state and releases
\* that lock before waiting for its receipt. Failed predecessors invalidate
\* the head CAS, so the provider drops their admitted state and successors.
\* World admission's separate reservation model covers full-queue waits.
\* Set ReserveBeforeLock and PublishState FALSE to reproduce the old -24 path;
\* set PrepareBusy FALSE to reproduce -23's skip under continuing writes.
\* Queue is bounded, including the checkpoint's unconsumed claim. Page-view
\* naming, state contents and store durability belong to their own models.
EXTENDS Naturals
CONSTANTS Queue, ReserveBeforeLock, PublishState, PrepareBusy, PageView
VARIABLES pend, nr, ck, slot, open, adopted, fell
vars == <<pend, nr, ck, slot, open, adopted, fell>>
Init == /\ pend \in 0..Queue /\ nr = 0 /\ ck = "idle" /\ slot = 0
        /\ open = (pend > 0) /\ adopted = FALSE /\ fell = FALSE
Write == /\ ck # "locked" /\ pend + slot < Queue
         /\ pend' = pend + 1 /\ open' = TRUE
         /\ UNCHANGED <<nr, ck, slot, adopted, fell>>
Settle == /\ pend > 0 /\ pend' = pend - 1
          /\ nr' = IF nr > 0 THEN nr - 1 ELSE 0
          /\ UNCHANGED <<ck, slot, open, adopted, fell>>
Fail == /\ pend > 0 /\ ~fell /\ pend' = 0 /\ nr' = 0 /\ fell' = TRUE
        /\ ck' = IF ck = "published" \/ (ck = "named" /\ nr > 0) THEN "idle" ELSE ck
        /\ UNCHANGED <<slot, open, adopted>>
Prepare == /\ ck = "idle" /\ open /\ (PrepareBusy \/ pend = 0)
           /\ ck' = "named" /\ nr' = pend /\ open' = FALSE /\ adopted' = FALSE
           /\ UNCHANGED <<pend, slot, fell>>
Reserve == /\ ReserveBeforeLock /\ ck = "named" /\ nr = 0 /\ pend < Queue
           /\ slot' = 1 /\ ck' = "reserved"
           /\ UNCHANGED <<pend, nr, open, adopted, fell>>
Lock == /\ ((ck = "reserved") \/ (~ReserveBeforeLock /\ ck = "named" /\ nr = 0))
        /\ ck' = "locked"
        /\ UNCHANGED <<pend, nr, slot, open, adopted, fell>>
Adopt == /\ ck = "locked"
         /\ IF PublishState THEN (slot = 1 \/ pend < Queue) ELSE pend = 0
         /\ pend' = IF PublishState THEN pend + 1 ELSE pend
         /\ nr' = IF PublishState THEN pend + 1 ELSE 0
         /\ slot' = 0 /\ ck' = "published"
         /\ UNCHANGED <<open, adopted, fell>>
Finish == /\ ck = "published" /\ nr = 0
          /\ ck' = "idle" /\ adopted' = TRUE
          /\ UNCHANGED <<pend, nr, slot, open, fell>>
Next == Write \/ Settle \/ Fail \/ Prepare \/ Reserve \/ Lock \/ Adopt \/ Finish \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
\* A scheduled capacity waiter eventually wins a slot as receipts settle;
\* writes may continue forever. The one modeled failure does not recur.
FairSpec == Spec /\ WF_vars(Settle) /\ WF_vars(Prepare) /\ SF_vars(Reserve)
                 /\ WF_vars(Lock) /\ WF_vars(Adopt) /\ WF_vars(Finish)
Named == open ~> ~open
Finishes == (ck # "idle") ~> (ck = "idle")
Admits == []<>(ck # "locked")
\* Page-view naming has its own publication and is outside this model.
CoveredMode == ~PageView
Bounded == pend + slot <= Queue
NoWaitUnderLock == ck = "locked" =>
  IF PublishState THEN (slot = 1 \/ pend < Queue) ELSE pend = 0
====
