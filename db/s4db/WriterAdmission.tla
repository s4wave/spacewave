---- MODULE WriterAdmission ----
\* Exploration only: extend writer.locked's per-call aged bit to two marked
\* transaction callers sharing writer.mtx. No production change implements it.
\* The FIFO here models csync.Mutex; the foreground announcement and enqueue
\* are merged, which removes races and still permits the reported violation.
\* W holds writer initially. B1 and B2 are queued. F1/F2 may arrive repeatedly.
\* A background check releases writer before waiting on the next local
\* endWait; Wake then ages that call and requeues it at the FIFO tail.
\* Every foreground admission/cancellation wakes each yielded local call.
\* Cross-process locks, refresh failures and pre-enqueue announcements are
\* omitted: this local FIFO counterexample already refutes the global bound.
\* Remote slots retain independent intents (WriterWait.tla); that model has
\* one background actor and proves its bound per scheduler, not across calls.
EXTENDS Naturals, Sequences, FiniteSets
Foreground == {"F1", "F2"}
Background == {"B1", "B2"}
Actors == Foreground \cup Background
VARIABLES holder, queue, phase, aged, wake, passed, yields
vars == <<holder, queue, phase, aged, wake, passed, yields>>
Waiting == {f \in Foreground : phase[f] = "queued"}
Init ==
  /\ holder = "W" /\ queue = <<"B1", "B2">>
  /\ phase = [a \in Actors |-> IF a \in Background THEN "queued" ELSE "idle"]
  /\ aged = [b \in Background |-> FALSE]
  /\ wake = [b \in Background |-> FALSE]
  /\ passed = [f \in Foreground |-> 0]
  /\ yields = [b \in Background |-> 0]
Want(f) ==
  /\ phase[f] = "idle"
  /\ phase' = [phase EXCEPT ![f] = "queued"]
  /\ queue' = Append(queue, f)
  /\ passed' = [passed EXCEPT ![f] = 0]
  /\ UNCHANGED <<holder, aged, wake, yields>>
Take ==
  /\ holder = "none" /\ Len(queue) > 0
  /\ LET a == Head(queue) IN
       /\ holder' = a /\ queue' = Tail(queue)
       /\ phase' = [phase EXCEPT ![a] = IF a \in Foreground THEN "holding" ELSE "checking"]
       /\ wake' = [b \in Background |-> wake[b] \/ (a \in Foreground /\ phase[b] = "yielded")]
  /\ UNCHANGED <<aged, passed, yields>>
Check(b) ==
  /\ holder = b /\ phase[b] = "checking"
  /\ IF Waiting # {} /\ ~aged[b]
     THEN /\ holder' = "none"
          /\ phase' = [phase EXCEPT ![b] = "yielded"]
          /\ yields' = [yields EXCEPT ![b] = IF @ < 2 THEN @ + 1 ELSE 2]
          /\ UNCHANGED passed
     ELSE /\ phase' = [phase EXCEPT ![b] = "holding"]
          /\ passed' = [f \in Foreground |-> IF f \in Waiting THEN IF passed[f] < 2 THEN passed[f] + 1 ELSE 2 ELSE passed[f]]
          /\ UNCHANGED <<holder, yields>>
  /\ UNCHANGED <<queue, aged, wake>>
Wake(b) ==
  /\ phase[b] = "yielded" /\ wake[b]
  /\ phase' = [phase EXCEPT ![b] = "queued"]
  /\ aged' = [aged EXCEPT ![b] = TRUE]
  /\ wake' = [wake EXCEPT ![b] = FALSE]
  /\ queue' = Append(queue, b)
  /\ UNCHANGED <<holder, passed, yields>>
Release ==
  /\ holder # "none"
  /\ IF holder = "W"
     THEN UNCHANGED phase
     ELSE /\ phase[holder] = "holding"
          /\ phase' = [phase EXCEPT ![holder] = IF holder \in Background THEN "done" ELSE "idle"]
  /\ holder' = "none"
  /\ UNCHANGED <<queue, aged, wake, passed, yields>>
Cancel(f) ==
  /\ phase[f] = "queued"
  /\ phase' = [phase EXCEPT ![f] = "idle"]
  /\ queue' = SelectSeq(queue, LAMBDA a : a # f)
  /\ wake' = [b \in Background |-> wake[b] \/ (phase[b] = "yielded")]
  /\ UNCHANGED <<holder, aged, passed, yields>>
Next == Take \/ Release \/ (\E f \in Foreground : Want(f) \/ Cancel(f))
  \/ (\E b \in Background : Check(b) \/ Wake(b))
Spec == Init /\ [][Next]_vars /\ WF_vars(Take) /\ WF_vars(Release)
  /\ (\A b \in Background : WF_vars(Check(b)) /\ WF_vars(Wake(b)))
\* Requested file-wide bound. Expected to FAIL for the proposed direct reuse.
ForegroundPassedAtMostOnce == \A f \in Waiting : passed[f] <= 1
\* The reused policy's actual bound: each individual call yields at most once.
BackgroundYieldBound == \A b \in Background : yields[b] <= 1
BackgroundAdmitted == \A b \in Background : <>(phase[b] = "done")
====
