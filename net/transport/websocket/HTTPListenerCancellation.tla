------------------------- MODULE HTTPListenerCancellation -------------------------
EXTENDS TLC
\* ListenHTTP must stop its owned server when Execute's context ends.
\* Cancellation may precede listener startup. net/http.Server.Close prevents a
\* later ListenAndServe as well as closing an existing listener. HTTP handlers
\* may continue accepting requests until the canceled server is closed.
\* The old implementation calls Shutdown only after ListenAndServe returns.
Managed == TRUE
VARIABLES phase, canceled
vars == <<phase, canceled>>
Init == /\ phase = "created" /\ canceled \in BOOLEAN
Register == /\ phase = "created" /\ phase' = "armed" /\ UNCHANGED canceled
\* A binding error returns directly without a live listener.
BindFailed == /\ phase = "armed" /\ phase' = "stopped" /\ UNCHANGED canceled
Serve == /\ phase = "armed" /\ phase' = "serving" /\ UNCHANGED canceled
Cancel == /\ ~canceled /\ canceled' = TRUE /\ UNCHANGED phase
Stop == /\ Managed /\ canceled /\ phase \in {"armed", "serving"}
        /\ phase' = "stopped" /\ UNCHANGED canceled
ServeRequests == /\ phase = "serving" /\ UNCHANGED vars
Done == /\ phase = "stopped" /\ UNCHANGED vars
Next == Register \/ BindFailed \/ Serve \/ Cancel \/ Stop \/ ServeRequests \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(Register) /\ WF_vars(Serve) /\ WF_vars(Stop)
CanceledEventuallyStops == canceled ~> (phase = "stopped")
===============================================================================
