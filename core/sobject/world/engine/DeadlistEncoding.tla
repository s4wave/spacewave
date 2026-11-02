---- MODULE DeadlistEncoding ----
\* pendingNet.poll encodes each record after its naming receipt resolves.
\* The encoding can finish after executeWatchSOState's last pass, while the
\* SharedObject stays unchanged. Its completion must wake the state watcher
\* so deadlistLedger.resolve consumes it and a save names the record.
\* DeadlistLedger models save generations, adoption and durable names, but its
\* Resolve action omits this scheduling boundary and does not check liveness.
\* Two receipts and two encoders cover a coalesced wake and completion during
\* a watch pass. Snapshot changes cancel an edge receiver; it must exit
\* before the next pass or it could steal a later completion. Publication
\* persistence and GC counts are outside this model.
EXTENDS Naturals
CONSTANTS NotifyEncoding, JoinNotifier
VARIABLES state, phase, wake, retired, oldListener, stateChanged
vars == <<state, phase, wake, retired, oldListener, stateChanged>>
Records == 1..2
Init == /\ state = [r \in Records |-> "receipt"]
        /\ phase = "waiting" /\ wake = FALSE /\ retired = FALSE
        /\ oldListener = FALSE /\ stateChanged = FALSE
ReceiptCompletes(r) == /\ state[r] = "receipt" /\ ~retired
                       /\ state' = [state EXCEPT ![r] = "ready"]
                       /\ wake' = TRUE
                       /\ UNCHANGED <<phase, retired, oldListener, stateChanged>>
TakeInput == /\ phase = "waiting" /\ wake /\ ~retired
             /\ phase' = "processing" /\ wake' = FALSE
             /\ UNCHANGED <<state, retired, oldListener, stateChanged>>
StateChange == /\ phase = "waiting" /\ ~stateChanged /\ ~retired
               /\ phase' = IF JoinNotifier THEN "joining" ELSE "processing"
               /\ oldListener' = TRUE /\ stateChanged' = TRUE
               /\ UNCHANGED <<state, wake, retired>>
ListenerEnds == /\ oldListener /\ oldListener' = FALSE
                /\ phase' = IF phase = "joining" THEN "processing" ELSE phase
                /\ UNCHANGED <<state, wake, retired, stateChanged>>
ListenerConsumes == /\ oldListener /\ wake
                    /\ oldListener' = FALSE /\ wake' = FALSE
                    /\ phase' = IF phase = "joining" THEN "processing" ELSE phase
                    /\ UNCHANGED <<state, retired, stateChanged>>
Resolve == /\ phase = "processing" /\ ~retired
           /\ state' = [r \in Records |->
                CASE state[r] = "ready" -> "encoding"
                  [] state[r] = "encoded" -> "stored"
                  [] OTHER -> state[r]]
           /\ phase' = "waiting"
           /\ UNCHANGED <<wake, retired, oldListener, stateChanged>>
Encode(r) == /\ state[r] = "encoding" /\ ~retired
             /\ state' = [state EXCEPT ![r] = "encoded"]
             /\ wake' = (wake \/ NotifyEncoding)
             /\ UNCHANGED <<phase, retired, oldListener, stateChanged>>
Retire == /\ ~retired /\ retired' = TRUE
          /\ UNCHANGED <<state, phase, wake, oldListener, stateChanged>>
Next == (\E r \in Records : ReceiptCompletes(r) \/ Encode(r))
        \/ TakeInput \/ StateChange \/ ListenerEnds \/ ListenerConsumes
        \/ Resolve \/ Retire \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(TakeInput) /\ WF_vars(Resolve)
            /\ WF_vars(ListenerEnds) /\ WF_vars(ListenerConsumes)
            /\ (\A r \in Records : WF_vars(ReceiptCompletes(r)) /\ WF_vars(Encode(r)))
TypeOK == /\ state \in [Records -> {"receipt", "ready", "encoding", "encoded", "stored"}]
          /\ phase \in {"waiting", "joining", "processing"} /\ wake \in BOOLEAN /\ retired \in BOOLEAN
          /\ oldListener \in BOOLEAN /\ stateChanged \in BOOLEAN
Finishes == <> ((\A r \in Records : state[r] = "stored") \/ retired)
====
