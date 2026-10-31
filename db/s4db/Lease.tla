---- MODULE Lease ----
\* Leases, as lease.go runs them, for one lock byte. A file lock belongs to
\* the process, so a second lock in the same process succeeds; the handle
\* therefore claims the byte under bcast before it locks it, and only one
\* caller per handle tries. TryLease drops the claim when another process
\* holds the byte. WaitLease locks in the background; when its context ends
\* first, the abandoned lock keeps the claim until the system grants it, then
\* unlocks and drops the claim. Release unlocks and drops the claim together.
\*
\* Callers a and b run in process P1, c in P2. Not modeled: Close, which
\* drops the process's locks, and two names sharing one byte, which only
\* excludes them from each other.

Callers == {"a", "b", "c"}
Proc == [g \in Callers |-> IF g = "c" THEN "P2" ELSE "P1"]
Procs == {"P1", "P2"}

VARIABLES
  owner,   \* the process holding the file lock, or "none"
  claimed, \* the processes whose handle claims the byte
  pc       \* each caller: "idle", "try", "wait", "held", "abandoned", "granted"
vars == <<owner, claimed, pc>>

Init == owner = "none" /\ claimed = {} /\ pc = [g \in Callers |-> "idle"]

Free(g) == owner \in {"none", Proc[g]}

\* claimLeaseLocked: claim the byte for TryLease or WaitLease.
Claim(g) ==
  /\ pc[g] = "idle" /\ Proc[g] \notin claimed
  /\ claimed' = claimed \cup {Proc[g]}
  /\ \E next \in {"try", "wait"} : pc' = [pc EXCEPT ![g] = next]
  /\ UNCHANGED owner

\* TryLease locks without waiting, or drops the claim.
Try(g) ==
  /\ pc[g] = "try"
  /\ IF Free(g)
     THEN owner' = Proc[g] /\ pc' = [pc EXCEPT ![g] = "held"] /\ UNCHANGED claimed
     ELSE /\ claimed' = claimed \ {Proc[g]}
          /\ pc' = [pc EXCEPT ![g] = "idle"] /\ UNCHANGED owner

\* WaitLease's background lock returns once the byte is free.
Grant(g) ==
  /\ pc[g] = "wait" /\ Free(g)
  /\ owner' = Proc[g] /\ pc' = [pc EXCEPT ![g] = "held"]
  /\ UNCHANGED claimed

\* WaitLease's context ends first; the claim stays.
Abandon(g) ==
  /\ pc[g] = "wait"
  /\ pc' = [pc EXCEPT ![g] = "abandoned"]
  /\ UNCHANGED <<owner, claimed>>

\* The abandoned lock is granted, then unlocked with the claim dropped.
AbandonedGrant(g) ==
  /\ pc[g] = "abandoned" /\ Free(g)
  /\ owner' = Proc[g] /\ pc' = [pc EXCEPT ![g] = "granted"]
  /\ UNCHANGED claimed

AbandonedRelease(g) ==
  /\ pc[g] = "granted"
  /\ owner' = "none" /\ claimed' = claimed \ {Proc[g]}
  /\ pc' = [pc EXCEPT ![g] = "idle"]

Release(g) ==
  /\ pc[g] = "held"
  /\ owner' = "none" /\ claimed' = claimed \ {Proc[g]}
  /\ pc' = [pc EXCEPT ![g] = "idle"]

Next ==
  \/ \E g \in Callers :
       Claim(g) \/ Try(g) \/ Grant(g) \/ Abandon(g) \/ AbandonedGrant(g)
       \/ AbandonedRelease(g) \/ Release(g)
  \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars

\* At most one caller in any process holds the lease, and its process holds
\* the lock.
OneHolder ==
  \A g, h \in Callers : pc[g] = "held" /\ pc[h] = "held" => g = h
HolderLocked == \A g \in Callers : pc[g] = "held" => owner = Proc[g]
====
