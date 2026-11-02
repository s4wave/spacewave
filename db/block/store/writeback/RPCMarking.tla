---- MODULE RPCMarking ----
(* NewStore chooses transaction marking only for atomic local stores. The
   local Volume commits block and marker together; an RPC proxy commits the
   block first, then queue writes its marker through the existing KV API.
   Both paths acknowledge only after the marker persists. A failure can leave
   an unacknowledged block, which retry marks again. One write, one retry and
   one crash cover the acknowledgement and caller-lifetime boundary; upload
   and GC follow the separate queue/ownership contracts. Choosing transaction
   marking for RPC causes a write to fail despite a healthy marker store.
   SelectByCapability=FALSE reproduces that unreachable marking request. *)
CONSTANT SelectByCapability
VARIABLES atomicLocal, phase, stored, marked, acknowledged, retried
vars == <<atomicLocal, phase, stored, marked, acknowledged, retried>>
Init == /\ atomicLocal \in BOOLEAN /\ phase = "writing" /\ stored = FALSE
        /\ marked = FALSE /\ acknowledged = FALSE /\ retried = FALSE
Write == /\ phase = "writing" /\ stored' = TRUE
         /\ marked' = atomicLocal
         /\ phase' = IF atomicLocal THEN "reply"
                      ELSE IF SelectByCapability THEN "mark" ELSE "unsupported"
         /\ UNCHANGED <<atomicLocal, acknowledged, retried>>
Mark == /\ phase = "mark" /\ marked' = TRUE /\ phase' = "reply"
        /\ UNCHANGED <<atomicLocal, stored, acknowledged, retried>>
FailMark == /\ phase = "mark" /\ ~retried /\ phase' = "retry"
            /\ UNCHANGED <<atomicLocal, stored, marked, acknowledged, retried>>
Retry == /\ phase = "retry" /\ retried' = TRUE /\ phase' = "writing"
         /\ UNCHANGED <<atomicLocal, stored, marked, acknowledged>>
Reply == /\ phase = "reply" /\ acknowledged' = TRUE /\ phase' = "done"
         /\ UNCHANGED <<atomicLocal, stored, marked, retried>>
Crash == /\ phase \notin {"done", "crashed"} /\ phase' = "crashed"
         /\ UNCHANGED <<atomicLocal, stored, marked, acknowledged, retried>>
Next == Write \/ Mark \/ FailMark \/ Retry \/ Reply \/ Crash \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Write) /\ WF_vars(Mark) /\ WF_vars(Retry) /\ WF_vars(Reply)
SuccessHasMarker == acknowledged => (stored /\ marked)
HealthyWriteFinishes == <> (acknowledged \/ phase = "crashed")
====
