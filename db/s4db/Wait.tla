---- MODULE Wait ----
\* A writer waits behind at most one background step, as the scheduler runs
\* it: a background job (the flusher, the merger or the cleaner) takes the
\* writer lock for one step, a record append, and before taking it again
\* checks WritersWaiting, yielding while a writer waits. Writers take the lock
\* for a transaction and release it at commit or discard.
\*
\* Callers found from source (master db/s4db at aa2254798 and the design):
\* Tx.Commit and Tx.Discard release the lock a write transaction holds, and
\* the three jobs' steps; a job's step is modeled as one lock span. Not
\* modeled: the quiet gate, which only delays steps further, and the lock's
\* fcntl across processes, which orders holders the same way.

EXTENDS Naturals

Writers == {"w1", "w2"}
Bound == 1

VARIABLES
  holder,  \* who holds the writer lock: "none", "bg", or a writer
  waiting, \* the writers waiting for the lock
  steps    \* for each waiting writer, the background steps since it began waiting
vars == <<holder, waiting, steps>>

Init ==
  /\ holder = "none"
  /\ waiting = {}
  /\ steps = [w \in Writers |-> 0]

\* A writer asks for the lock.
Want(w) ==
  /\ w \notin waiting /\ holder # w
  /\ waiting' = waiting \cup {w}
  /\ steps' = [steps EXCEPT ![w] = 0]
  /\ UNCHANGED holder

\* A waiting writer takes the free lock.
Take(w) ==
  /\ w \in waiting /\ holder = "none"
  /\ holder' = w
  /\ waiting' = waiting \ {w}
  /\ UNCHANGED steps

\* A writer commits or discards.
Release(w) ==
  /\ holder = w
  /\ holder' = "none"
  /\ UNCHANGED <<waiting, steps>>

\* A background job takes the lock for one step only while no writer waits:
\* the WritersWaiting check between steps.
StepStart ==
  /\ holder = "none" /\ waiting = {}
  /\ holder' = "bg"
  /\ UNCHANGED <<waiting, steps>>

\* The step ends; every writer that waited through it counts one step.
StepEnd ==
  /\ holder = "bg"
  /\ holder' = "none"
  /\ steps' = [w \in Writers |-> IF w \in waiting THEN steps[w] + 1 ELSE steps[w]]
  /\ UNCHANGED waiting

Next ==
  \/ \E w \in Writers : Want(w) \/ Take(w) \/ Release(w)
  \/ StepStart \/ StepEnd
  \/ UNCHANGED vars

\* The lock is fair among writers: csync.Mutex queues them in order, and a
\* writer that is enabled again and again gets its turn (strong fairness).
Fairness ==
  /\ \A w \in Writers : SF_vars(Take(w)) /\ WF_vars(Release(w))
  /\ WF_vars(StepEnd)

Spec == Init /\ [][Next]_vars /\ Fairness

\* A waiting writer never sees more than Bound background steps.
BoundedWait == \A w \in Writers : w \in waiting => steps[w] <= Bound

\* Every waiting writer is served.
Served == \A w \in Writers : w \in waiting ~> holder = w
====
