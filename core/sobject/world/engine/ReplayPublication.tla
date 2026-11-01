---- MODULE ReplayPublication ----
\* applyHold tries capacity under the writer; a full queue coalesces a size
\* request. executeReplayPublications waits outside it, then rebuilds the
\* current replay under the writer. A larger save releases its claim and retries.
\* The installed World/replayer retain the data; the request carries only size.
\* At most one update arrives while the first save is deferred. Physical receipts
\* eventually settle; caller cancellation is represented by engine retirement.
EXTENDS Naturals
CONSTANTS ReleaseWriter, Rebuild, GrowSave
VARIABLES pending, phase, lock, claim, request, revision, captured, saved, retired, bad
vars == <<pending, phase, lock, claim, request, revision, captured, saved, retired, bad>>
Init == /\ pending \in 0..2 /\ phase = "prepare" /\ lock = TRUE
        /\ claim = 0 /\ request = 0 /\ revision = 1 /\ captured = 1
        /\ saved = 0 /\ retired = FALSE /\ bad = FALSE
Size == IF GrowSave THEN revision ELSE 1
Prepare == /\ phase = "prepare" /\ ~retired
           /\ phase' = "wait" /\ request' = Size /\ captured' = revision
           /\ lock' = ~ReleaseWriter
           /\ UNCHANGED <<pending, claim, revision, saved, retired, bad>>
Update == /\ revision = 1 /\ ~lock /\ ~retired
          /\ revision' = 2
          /\ UNCHANGED <<pending, phase, lock, claim, request, captured, saved, retired, bad>>
Reserve == /\ phase = "wait" /\ pending < 2 /\ ~retired
           /\ claim' = request /\ request' = 0
           /\ phase' = "locked" /\ lock' = TRUE
           /\ UNCHANGED <<pending, revision, captured, saved, retired, bad>>
Retry == /\ phase = "locked" /\ claim < Size /\ ~retired
         /\ claim' = 0 /\ request' = Size /\ captured' = revision
         /\ phase' = "wait" /\ lock' = FALSE
         /\ UNCHANGED <<pending, revision, saved, retired, bad>>
Publish == /\ phase = "locked" /\ claim >= Size /\ ~retired
           /\ saved' = IF Rebuild THEN revision ELSE captured
           /\ bad' = (~Rebuild /\ captured # revision)
           /\ pending' = pending + 1 /\ claim' = 0 /\ phase' = "done" /\ lock' = FALSE
           /\ UNCHANGED <<request, revision, captured, retired>>
Again == /\ phase = "done" /\ saved < revision /\ ~retired
         /\ phase' = "prepare" /\ lock' = TRUE
         /\ UNCHANGED <<pending, claim, request, revision, captured, saved, retired, bad>>
Settle == /\ pending > 0 /\ pending' = pending - 1
          /\ UNCHANGED <<phase, lock, claim, request, revision, captured, saved, retired, bad>>
Retire == /\ ~retired /\ retired' = TRUE /\ claim' = 0 /\ lock' = FALSE
          /\ phase' = "closed" /\ request' = 0
          /\ UNCHANGED <<pending, revision, captured, saved, bad>>
Next == Prepare \/ Update \/ Reserve \/ Retry \/ Publish \/ Again \/ Settle \/ Retire \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Prepare) /\ WF_vars(Reserve) /\ WF_vars(Retry)
            /\ WF_vars(Publish) /\ WF_vars(Again) /\ WF_vars(Settle)
NoWaitUnderLock == phase = "wait" => ~lock
Bounded == pending + (IF claim = 0 THEN 0 ELSE 1) <= 2 /\ request <= 2
CurrentSave == ~bad
Released == retired => (claim = 0 /\ ~lock /\ request = 0)
Finishes == (saved < revision /\ ~retired) ~> (saved = revision \/ retired)
====
