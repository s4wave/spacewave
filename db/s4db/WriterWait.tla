---- MODULE WriterWait ----
\* Transitional per-call background yielding inside the sole writable handle.
\* This checks one scheduler step, not the file-wide multi-caller bound.
\* WriterAdmission replaces this model when shared local admission is wired.
EXTENDS Naturals
VARIABLES holder, waiting, steps, yielded, aged
vars == <<holder, waiting, steps, yielded, aged>>
Init == /\ holder = "none" /\ waiting = FALSE /\ steps = 0
  /\ yielded = FALSE /\ aged = FALSE
Want ==
  /\ ~waiting /\ holder # "writer"
  /\ waiting' = TRUE /\ steps' = 0 /\ UNCHANGED <<holder, yielded, aged>>
CancelOrDie ==
  /\ waiting /\ waiting' = FALSE
  /\ UNCHANGED <<holder, steps, yielded, aged>>
Take ==
  /\ waiting /\ holder = "none"
  /\ holder' = "writer" /\ waiting' = FALSE /\ UNCHANGED <<steps, yielded, aged>>
CommitOrDiscard ==
  /\ holder = "writer" /\ holder' = "none"
  /\ UNCHANGED <<waiting, steps, yielded, aged>>
BackgroundStart ==
  /\ holder = "none" /\ ~yielded /\ holder' = "checking"
  /\ UNCHANGED <<waiting, steps, yielded, aged>>
BackgroundCheck ==
  /\ holder = "checking"
  /\ holder' = IF waiting /\ ~aged THEN "none" ELSE "background"
  /\ yielded' = (waiting /\ ~aged)
  /\ aged' = (waiting \/ aged)
  /\ UNCHANGED <<waiting, steps>>
BackgroundEnd ==
  /\ holder = "background" /\ holder' = "none"
  /\ steps' = IF waiting THEN IF steps < 2 THEN steps + 1 ELSE 2 ELSE steps
  /\ aged' = FALSE
  /\ UNCHANGED <<waiting, yielded>>
\* A yielded local step wakes when its announced request clears.
WakeBackground ==
  /\ yielded /\ ~waiting /\ yielded' = FALSE
  /\ UNCHANGED <<holder, waiting, steps, aged>>
Next == Want \/ CancelOrDie \/ Take \/ CommitOrDiscard \/ BackgroundStart \/ BackgroundCheck \/ BackgroundEnd
  \/ WakeBackground \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ SF_vars(Take) /\ SF_vars(BackgroundStart)
  /\ WF_vars(CommitOrDiscard) /\ WF_vars(BackgroundEnd) /\ WF_vars(BackgroundCheck)
  /\ SF_vars(WakeBackground)
LockState == holder \in {"none", "writer", "checking", "background"}
BoundedWait == waiting => steps <= 1
RequestFinishes == waiting ~> ~waiting
\* The scheduler always has a step to run, so each step must eventually run.
BackgroundRuns == []<>(holder = "background")
====
