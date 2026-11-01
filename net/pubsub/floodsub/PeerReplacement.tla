---- MODULE PeerReplacement ----
EXTENDS Naturals
(* floodsub.go AddPeerStream, reconcile and session completion for one tuple.
   Callers: pubsub controller's negotiated mounted streams enter AddPeerStream;
   Execute/reconcile starts sessions; each session callback cleans up its map
   entry. Two generations can overlap after replacement cancels the old one.
   Skip stale pending sessions as reconcile does. Close and joins abstract to
   completion. Subscription packets from stale readers and message delivery
   are outside this cleanup-only model. No end-to-end delivery claim. *)
CONSTANT CheckCurrent
VARIABLES current, started, finished, installed, removed
vars == <<current, started, finished, installed, removed>>
Init == /\ current = 0 /\ started = {} /\ finished = {}
        /\ installed = 0 /\ removed = {}
AddPeerStream == /\ installed < 2 /\ installed' = installed + 1
                 /\ current' = installed + 1
                 /\ UNCHANGED <<started, finished, removed>>
Reconcile(a) == /\ current = a /\ a \notin started /\ a \notin finished
                /\ started' = started \cup {a}
                /\ UNCHANGED <<current, finished, installed, removed>>
Complete(a) ==
  /\ a \in started /\ a \notin finished
  /\ finished' = finished \cup {a}
  /\ current' = IF ~CheckCurrent \/ current = a THEN 0 ELSE current
  /\ removed' = IF current # 0 /\ (~CheckCurrent \/ current = a)
                  THEN removed \cup {current} ELSE removed
  /\ UNCHANGED <<started, installed>>
Next == AddPeerStream \/ (\E a \in {1, 2}: Reconcile(a) \/ Complete(a)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
OnlyFinishedRemoved == removed \subseteq finished
CurrentRetained == installed > 0 /\ installed \notin finished => current = installed
====
