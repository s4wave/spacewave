---- MODULE Release ----
\* A finite release pass is split into bounded writer callbacks. A continuation
\* runs before another allocation job; each batch still yields to foreground
\* intent. Failed IO and new work arriving mid-pass are outside this finite
\* drain, while chunk eligibility is covered by Free/Pins and LogSpace.
EXTENDS Naturals
VARIABLES remaining, queued, waiting
vars == <<remaining, queued, waiting>>
Init == /\ remaining = 3 /\ queued = TRUE /\ waiting = FALSE
Request == /\ ~waiting /\ waiting' = TRUE /\ UNCHANGED <<remaining, queued>>
Finish == /\ waiting /\ waiting' = FALSE /\ UNCHANGED <<remaining, queued>>
Batch ==
 /\ queued /\ remaining > 0 /\ ~waiting
 /\ remaining' = remaining-1 /\ queued' = (remaining > 1)
 /\ UNCHANGED waiting
Next == Request \/ Finish \/ Batch \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ SF_vars(Batch) /\ WF_vars(Finish)
NoStrandedBatch == remaining > 0 => queued
Drains == remaining > 0 ~> remaining = 0
====
