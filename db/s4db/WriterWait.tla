---- MODULE WriterWait ----
\* writer.beginWait holds this process's reader-slot intent byte until the
\* foreground request acquires the writer lock (or cancels locally). Under
\* that lock, writer.locked reads every slot before running its callback.
\* Seeing intent makes it release both writer locks and wait on the intent
\* byte, or on requests.Broadcast for its own slot. Process death drops the
\* byte. A request beginning after the check can wait behind that step only.
\* One foreground process and one remote scheduler exercise the boundary;
\* each slot acts independently, so more foreground processes retain their
\* own intents rather than contending for one shared admission byte.
\* Each IO/lock span is one step. Wall-clock and memtable-ceiling waits are
\* outside the model. Strong fairness is assumed only for eventual service.
EXTENDS Naturals
CONSTANT IntentDescriptorOwned
VARIABLES holder, waiting, steps, remoteWait, failed
vars == <<holder, waiting, steps, remoteWait, failed>>
Init == /\ holder = "none" /\ waiting = FALSE /\ steps = 0
  /\ remoteWait = FALSE /\ failed = FALSE
Want ==
  /\ ~waiting /\ holder # "writer"
  /\ waiting' = TRUE /\ steps' = 0 /\ UNCHANGED holder
CancelOrDie ==
  /\ waiting /\ waiting' = FALSE
  /\ UNCHANGED <<holder, steps>>
Take ==
  /\ waiting /\ holder = "none"
  /\ holder' = "writer" /\ waiting' = FALSE /\ UNCHANGED <<steps, remoteWait, failed>>
CommitOrDiscard ==
  /\ holder = "writer" /\ holder' = "none"
  /\ UNCHANGED <<waiting, steps, remoteWait, failed>>
BackgroundStart ==
  /\ holder = "none" /\ holder' = "checking"
  /\ UNCHANGED <<waiting, steps>>
BackgroundCheck ==
  /\ holder = "checking"
  /\ holder' = IF waiting THEN "none" ELSE "background"
  /\ UNCHANGED <<waiting, steps>>
BackgroundEnd ==
  /\ holder = "background" /\ holder' = "none"
  /\ steps' = IF waiting THEN IF steps < 2 THEN steps + 1 ELSE 2 ELSE steps
  /\ UNCHANGED waiting
Next == Want \/ CancelOrDie \/ Take \/ CommitOrDiscard \/ BackgroundStart \/ BackgroundCheck \/ BackgroundEnd \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ SF_vars(Take)
  /\ WF_vars(CommitOrDiscard) /\ WF_vars(BackgroundEnd) /\ WF_vars(BackgroundCheck)
LockState == holder \in {"none", "writer", "checking", "background"}
BoundedWait == waiting => steps <= 1
RequestFinishes == waiting ~> ~waiting
====
