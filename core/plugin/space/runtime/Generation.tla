----------------------------- MODULE Generation -----------------------------
EXTENDS Naturals

(* The runtime withdraws a generation before releasing it. A route can finish
   its readiness wait during withdrawal, so it rechecks the published instance
   before sending its first response. Host changes do not replace a generation.
   The caller stop signals compete with readiness and release without priority. *)
VARIABLES phase, route, current, published, stale
vars == <<phase, route, current, published, stale>>
Init == /\ phase = "starting"
        /\ route = "absent"
        /\ current = FALSE
        /\ published = FALSE
        /\ stale = FALSE
Start == /\ phase = "starting"
         /\ phase' = "ready"
         /\ current' = TRUE
         /\ UNCHANGED <<route, published, stale>>
FailStart == /\ phase = "starting"
             /\ phase' = "done"
             /\ UNCHANGED <<route, current, published, stale>>
Install == /\ phase = "ready" /\ route = "absent"
           /\ route' = "starting"
           /\ UNCHANGED <<phase, current, published, stale>>
RouteReady == /\ route = "starting" /\ phase # "done"
              /\ route' = "ready"
              /\ UNCHANGED <<phase, current, published, stale>>
Withdraw == /\ phase = "ready"
            /\ phase' = "replaced"
            /\ current' = FALSE
            /\ UNCHANGED <<route, published, stale>>
Release == /\ phase = "replaced"
           /\ phase' = "done"
           /\ UNCHANGED <<route, current, published, stale>>
Respond == /\ route = "ready" /\ current /\ ~published
           /\ published' = TRUE
           /\ stale' = ~current
           /\ UNCHANGED <<phase, route, current>>
StopRoute == /\ route # "absent"
             /\ route' = "absent"
             /\ UNCHANGED <<phase, current, published, stale>>
HostChange == /\ phase = "ready" /\ UNCHANGED vars
Next == Start \/ FailStart \/ Install \/ RouteReady \/ Withdraw \/ Release
        \/ Respond \/ StopRoute \/ HostChange
Spec == Init /\ [][Next]_vars
TypeOK == /\ phase \in {"starting", "ready", "replaced", "done"}
          /\ route \in {"absent", "starting", "ready"}
          /\ current \in BOOLEAN /\ published \in BOOLEAN /\ stale \in BOOLEAN
WithdrawnBeforeDone == phase = "done" => ~current
NoStaleResponse == ~stale
=============================================================================
