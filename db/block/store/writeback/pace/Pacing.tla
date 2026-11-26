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
         /\ phase' = (IF paused THEN "paused" ELSE IF due THEN "rate" ELSE "pressure")
         /\ UNCHANGED <<cancelled,paused,due,pressure,waited>>
RatePass == /\ phase = "rate" /\ ~cancelled /\ due' = FALSE /\ phase' = "pressure"
            /\ UNCHANGED <<paused,pressure,waited,cancelled>>
\* RateChange is SetBytesPerSecond during a wait: the batch reads the settings
\* again, and the new cap may make it due now or later.
RateChange == /\ phase \in {"paused","rate","pressure"} /\ due' \in BOOLEAN
              /\ phase' = "serial" /\ waited' = 0
              /\ UNCHANGED <<cancelled,paused,pressure>>
\* PauseChange is SetPaused before admission; a batch waiting reads the
\* settings again.
PauseChange == /\ phase \notin {"admitted","cancelled"} /\ paused' = ~paused
               /\ phase' = (IF phase = "ready" THEN "ready" ELSE "serial")
               /\ waited' = 0 /\ UNCHANGED <<cancelled,due,pressure>>
Backoff == /\ phase = "pressure" /\ pressure /\ waited < 3 /\ ~cancelled
           /\ waited' = waited+1 /\ UNCHANGED <<phase,cancelled,paused,due,pressure>>
Recover == /\ pressure /\ pressure' = FALSE
           /\ UNCHANGED <<phase,cancelled,paused,due,waited>>
Admit == /\ phase = "pressure" /\ (~pressure \/ waited = 3) /\ ~cancelled
         /\ phase' = "admitted" /\ UNCHANGED <<cancelled,paused,due,pressure,waited>>
Cancel == /\ phase \notin {"admitted","cancelled"} /\ cancelled' = TRUE
          /\ phase' = "cancelled" /\ UNCHANGED <<paused,due,pressure,waited>>
Next == Start \/ Acquire \/ RatePass \/ RateChange \/ PauseChange \/ Backoff \/ Recover \/ Admit \/ Cancel
Spec == Init /\ [][Next]_vars
PauseHolds == phase = "admitted" => ~paused
RateHeld == phase = "admitted" => ~due
CancelledNeverAdmits == cancelled => phase # "admitted"
=============================================================================
