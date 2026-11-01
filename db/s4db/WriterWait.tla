---- MODULE WriterWait ----
\* Actual cross-process writer locking: writer.lock and writer.locked both
\* take a local csync.Mutex followed by the shared blocking file lock. There
\* is no cross-process writer-waiting bit and locked checks waiting only
\* AFTER fn, for its metric. The OS file lock does not promise FIFO priority
\* to a foreground request in another process. A background process may
\* therefore reacquire between that request's wake and successful lock.
\* One foreground writer in process A and one scheduler in process B suffice.
\* Each IO/lock span is a single step; wall-clock duration, memtable ceiling
\* waits, IO contention and arbitrary OS starvation are outside the model.
\* Strong fairness is assumed ONLY for eventual service, never for a bound.
EXTENDS Naturals
VARIABLES holder, waiting, steps
vars == <<holder, waiting, steps>>
Init == /\ holder = "none" /\ waiting = FALSE /\ steps = 0
Want ==
  /\ ~waiting /\ holder # "writer"
  /\ waiting' = TRUE /\ steps' = 0 /\ UNCHANGED holder
Take ==
  /\ waiting /\ holder = "none"
  /\ holder' = "writer" /\ waiting' = FALSE /\ UNCHANGED steps
CommitOrDiscard ==
  /\ holder = "writer" /\ holder' = "none"
  /\ UNCHANGED <<waiting, steps>>
BackgroundStart ==
  /\ holder = "none" /\ holder' = "background"
  /\ UNCHANGED <<waiting, steps>>
BackgroundEnd ==
  /\ holder = "background" /\ holder' = "none"
  /\ steps' = IF waiting THEN IF steps < 2 THEN steps + 1 ELSE 2 ELSE steps
  /\ UNCHANGED waiting
Next == Want \/ Take \/ CommitOrDiscard \/ BackgroundStart \/ BackgroundEnd \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars /\ SF_vars(Take)
  /\ WF_vars(CommitOrDiscard) /\ WF_vars(BackgroundEnd)
LockState == holder \in {"none", "writer", "background"}
BoundedWait == waiting => steps <= 1
Served == waiting ~> holder = "writer"
====
