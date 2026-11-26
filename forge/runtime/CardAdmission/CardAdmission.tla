---- MODULE CardAdmission ----
\* Two container Executions ask one Worker for its one graphics card. The
\* Worker's admission reserves the whole card for one Execution at a time and
\* credits it back only once that Execution's container is known to be stopped.
\* A request no card on the Worker can meet is refused at once; one that waits
\* only for a busy card waits, and the Execution viewer says what it waits for.
\* Design "fail" is the admission Forge had before, which failed an Execution
\* whose request did not fit right now; no config selects it.
EXTENDS Naturals

CONSTANTS
  Design,  \* "wait" (built) or "fail" (the earlier admission)
  BigB     \* TRUE when Execution b asks for more than the Worker's card offers

Execs == {"a", "b"}
TooBig == IF BigB THEN {"b"} ELSE {}

VARIABLES
  st,     \* each Execution's state
  holder  \* the Execution the card is reserved for, or "none"
vars == <<st, holder>>

States == {"pending", "waiting", "running", "stopping", "uncertain", "done", "refused"}

TypeOK == st \in [Execs -> States] /\ holder \in Execs \cup {"none"}

Init == st = [e \in Execs |-> "pending"] /\ holder = "none"

\* The card is free and the request fits it: reserve it and start.
Reserve(e) == st[e] = "pending" /\ e \notin TooBig /\ holder = "none"
  /\ holder' = e /\ st' = [st EXCEPT ![e] = "running"]
\* The request fits the card, but another Execution holds it.
Busy(e) == st[e] = "pending" /\ e \notin TooBig /\ holder \notin {"none", e}
  /\ st' = [st EXCEPT ![e] = IF Design = "wait" THEN "waiting" ELSE "refused"]
  /\ UNCHANGED holder
\* No card on the Worker can meet the request, so waiting would never end.
Refuse(e) == st[e] = "pending" /\ e \in TooBig
  /\ st' = [st EXCEPT ![e] = "refused"] /\ UNCHANGED holder
\* The credit wakes a waiting Execution, which reserves the card.
Wake(e) == st[e] = "waiting" /\ holder = "none"
  /\ holder' = e /\ st' = [st EXCEPT ![e] = "running"]
\* The container exits; the Worker records the stop before the credit.
Exit(e) == st[e] = "running" /\ st' = [st EXCEPT ![e] = "stopping"] /\ UNCHANGED holder
\* The confirmed stop credits the card back.
Credit(e) == st[e] = "stopping" /\ holder = e
  /\ holder' = "none" /\ st' = [st EXCEPT ![e] = "done"]
\* The Worker restarts while the container may still run: the reservation is
\* uncertain and keeps the card.
Crash(e) == st[e] = "running" /\ st' = [st EXCEPT ![e] = "uncertain"] /\ UNCHANGED holder
\* The restarted Worker stops the named container and so confirms the stop.
Recover(e) == st[e] = "uncertain" /\ st' = [st EXCEPT ![e] = "stopping"] /\ UNCHANGED holder

Finished == \A e \in Execs : st[e] \in {"done", "refused"}
Done == Finished /\ UNCHANGED vars

Next ==
  \E e \in Execs :
    Reserve(e) \/ Busy(e) \/ Refuse(e) \/ Wake(e) \/ Exit(e) \/ Credit(e) \/ Crash(e) \/ Recover(e)
  \/ Done

Spec == Init /\ [][Next]_vars /\ \A e \in Execs :
  WF_vars(Reserve(e)) /\ WF_vars(Busy(e)) /\ WF_vars(Refuse(e)) /\ WF_vars(Wake(e))
  /\ WF_vars(Exit(e)) /\ WF_vars(Credit(e)) /\ WF_vars(Recover(e))

\* Two containers never share the card.
OneHolder == \A e \in Execs :
  st[e] \in {"running", "stopping", "uncertain"} => holder = e
\* A request that fits the card is never refused because the card is busy.
FitsNeverRefused == \A e \in Execs : st[e] = "refused" => e \in TooBig
\* An Execution waits only for a card that can meet its request.
WaitEnds == \A e \in Execs : st[e] = "waiting" => e \notin TooBig

\* Every Execution eventually completes or is refused.
EveryoneFinishes == <>Finished
====
