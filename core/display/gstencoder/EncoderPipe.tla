---- MODULE EncoderPipe ----
\* One source owns one helper. Model the oldest unread picture as one slot.
\* Pipe EOF and the GStreamer stop event represent cancellation.
\* Reaping releases both pipe ends and the writer.

VARIABLES state, request, picture, stalled
vars == <<state, request, picture, stalled>>

Init ==
  /\ state = "running"
  /\ request = "none"
  /\ picture = "none"
  /\ stalled = FALSE

Request ==
  /\ state = "running"
  /\ request = "none"
  /\ request' = "queued"
  /\ UNCHANGED <<state, picture, stalled>>

Deliver ==
  /\ state = "running"
  /\ request = "queued"
  /\ request' = "armed"
  /\ UNCHANGED <<state, picture, stalled>>

Encode ==
  /\ state = "running"
  /\ picture = "none"
  /\ picture' = IF request = "armed" THEN "IDR" ELSE "delta"
  /\ UNCHANGED <<state, request, stalled>>

ReadDelta ==
  /\ state = "running"
  /\ picture = "delta"
  /\ request = "none"
  /\ picture' = "none"
  /\ UNCHANGED <<state, request, stalled>>

SkipDelta ==
  /\ state = "running"
  /\ picture = "delta"
  /\ request # "none"
  /\ picture' = "none"
  /\ UNCHANGED <<state, request, stalled>>

ReadIDR ==
  /\ state = "running"
  /\ picture = "IDR"
  /\ picture' = "none"
  /\ request' = "none"
  /\ UNCHANGED <<state, stalled>>

Cancel ==
  /\ state = "running"
  /\ state' = "stopping"
  /\ request' = "none"
  /\ picture' = "none"
  /\ UNCHANGED stalled

Stall ==
  /\ state \in {"running", "stopping"}
  /\ ~stalled
  /\ stalled' = TRUE
  /\ UNCHANGED <<state, request, picture>>

Stop ==
  /\ state = "stopping"
  /\ ~stalled
  /\ state' = "exited"
  /\ UNCHANGED <<request, picture, stalled>>

Kill ==
  /\ state = "stopping"
  /\ stalled
  /\ state' = "killed"
  /\ UNCHANGED <<request, picture, stalled>>

FinishStop == Stop \/ Kill

Crash ==
  /\ state = "running"
  /\ state' = "exited"
  /\ request' = "none"
  /\ picture' = "none"
  /\ UNCHANGED stalled

Reap ==
  /\ state \in {"exited", "killed"}
  /\ state' = "reaped"
  /\ UNCHANGED <<request, picture, stalled>>

Done ==
  /\ state = "reaped"
  /\ UNCHANGED vars

Next == Request \/ Deliver \/ Encode \/ ReadDelta \/ SkipDelta \/ ReadIDR
        \/ Cancel \/ Stall \/ FinishStop \/ Crash \/ Reap \/ Done
Spec == Init /\ [][Next]_vars /\ WF_vars(FinishStop) /\ WF_vars(Reap)

TypeOK ==
  /\ state \in {"running", "stopping", "exited", "killed", "reaped"}
  /\ request \in {"none", "queued", "armed"}
  /\ picture \in {"none", "IDR", "delta"}
  /\ stalled \in BOOLEAN

NoCaptureAfterStop ==
  state # "running" => /\ request = "none"
                       /\ picture = "none"

StopReaps == (state = "stopping") ~> (state = "reaped")
CrashReaps == (state = "exited") ~> (state = "reaped")
====
