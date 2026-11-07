---- MODULE WriterWait ----
\* writer.beginWait holds this process's reader-slot intent byte until the
\* foreground request acquires the writer lock (or cancels locally). Under
\* that lock, writer.locked reads every slot before running its callback.
\* Seeing intent makes it release both writer locks and wait on the intent
\* byte, or on requests.Broadcast for its own slot. Process death drops the
\* byte. A request beginning after the check can wait behind that step only.
\* Two foreground processes and one remote scheduler exercise the boundary;
\* each slot acts independently, so more foreground processes retain their
\* own intents rather than contending for one shared admission byte.
\* Darwin intent bytes use descriptor ownership to avoid a false kernel
\* deadlock when a scheduler thread waits beside its own foreground writer.
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
  /\ waiting' = TRUE /\ steps' = 0 /\ UNCHANGED <<holder, remoteWait, failed>>
CancelOrDie ==
  /\ waiting /\ waiting' = FALSE
  /\ UNCHANGED <<holder, steps, remoteWait, failed>>
Take ==
  /\ waiting /\ holder = "none"
  /\ holder' = "writer" /\ waiting' = FALSE /\ UNCHANGED <<steps, remoteWait, failed>>
CommitOrDiscard ==
  /\ holder = "writer" /\ holder' = "none"
  /\ UNCHANGED <<waiting, steps, remoteWait, failed>>
BackgroundStart ==
  /\ holder = "none" /\ ~remoteWait /\ holder' = "checking"
  /\ UNCHANGED <<waiting, steps, remoteWait, failed>>
BackgroundCheck ==
  /\ holder = "checking"
  /\ holder' = IF waiting THEN "none" ELSE "background"
  /\ remoteWait' = waiting
  /\ UNCHANGED <<waiting, steps, failed>>
BackgroundEnd ==
  /\ holder = "background" /\ holder' = "none"
  /\ steps' = IF waiting THEN IF steps < 2 THEN steps + 1 ELSE 2 ELSE steps
  /\ UNCHANGED <<waiting, remoteWait, failed>>
\* A foreground thread of the scheduler's process can take writer while its
\* background thread waits on the first process's intent. Process-owned locks
\* turn this into a false kernel deadlock; descriptor-owned intent does not.
OtherTake ==
  /\ holder = "none" /\ holder' = "otherWriter"
  /\ UNCHANGED <<waiting, steps, remoteWait, failed>>
OtherEnd ==
  /\ holder = "otherWriter" /\ holder' = "none"
  /\ UNCHANGED <<waiting, steps, remoteWait, failed>>
WakeBackground ==
  /\ remoteWait /\ ~waiting /\ remoteWait' = FALSE
  /\ UNCHANGED <<holder, waiting, steps, failed>>
KernelDeadlock ==
  /\ ~IntentDescriptorOwned /\ remoteWait /\ waiting /\ holder = "otherWriter"
  /\ failed' = TRUE
  /\ UNCHANGED <<holder, waiting, steps, remoteWait>>
Next == Want \/ CancelOrDie \/ Take \/ CommitOrDiscard \/ BackgroundStart \/ BackgroundCheck \/ BackgroundEnd
  \/ OtherTake \/ OtherEnd \/ WakeBackground \/ KernelDeadlock \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ SF_vars(Take)
  /\ WF_vars(CommitOrDiscard) /\ WF_vars(BackgroundEnd) /\ WF_vars(BackgroundCheck)
  /\ WF_vars(OtherEnd) /\ WF_vars(WakeBackground)
LockState == holder \in {"none", "writer", "checking", "background", "otherWriter"}
BoundedWait == waiting => steps <= 1
NoKernelDeadlock == ~failed
RequestFinishes == waiting ~> ~waiting
====
