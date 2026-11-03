---- MODULE Pacing ----
\* Pace.Admit gates batches by pause, byte rate and sampled storage pressure.
\* Cancellation models a wait observing its context before admission.
\* It controls timing only. The writeback store separately proves eligibility.
EXTENDS Naturals
VARIABLES phase, cancelled, paused, due, pressure, waited
vars == <<phase,cancelled,paused,due,pressure,waited>>
Init == /\ phase = "ready" /\ cancelled = FALSE /\ paused \in BOOLEAN
        /\ due \in BOOLEAN /\ pressure \in BOOLEAN /\ waited = 0
Start == /\ phase = "ready" /\ phase' = "serial"
         /\ UNCHANGED <<cancelled,paused,due,pressure,waited>>
Acquire == /\ phase = "serial"
         /\ phase' = IF paused THEN "paused" ELSE IF due THEN "rate" ELSE "pressure"
         /\ UNCHANGED <<cancelled,paused,due,pressure,waited>>
RatePass == /\ phase = "rate" /\ ~cancelled /\ due' = FALSE /\ phase' = "pressure"
            /\ UNCHANGED <<paused,pressure,waited,cancelled>>
Backoff == /\ phase = "pressure" /\ pressure /\ waited < 3 /\ ~cancelled
           /\ waited' = waited+1 /\ UNCHANGED <<phase,cancelled,paused,due,pressure>>
Recover == /\ pressure /\ pressure' = FALSE
           /\ UNCHANGED <<phase,cancelled,paused,due,waited>>
Admit == /\ phase = "pressure" /\ (~pressure \/ waited = 3) /\ ~cancelled
         /\ phase' = "admitted" /\ UNCHANGED <<cancelled,paused,due,pressure,waited>>
Cancel == /\ phase \notin {"admitted","cancelled"} /\ cancelled' = TRUE
          /\ phase' = "cancelled" /\ UNCHANGED <<paused,due,pressure,waited>>
Next == Start \/ Acquire \/ RatePass \/ Backoff \/ Recover \/ Admit \/ Cancel
Spec == Init /\ [][Next]_vars
PauseHolds == phase = "admitted" => ~paused
RateHeld == phase = "admitted" => ~due
CancelledNeverAdmits == cancelled => phase # "admitted"
=============================================================================
