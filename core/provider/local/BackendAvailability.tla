----------------------- MODULE BackendAvailability -----------------------
EXTENDS Naturals, FiniteSets, TLC
CONSTANTS Readers, MaxChanges, UseWatch, Notify
VARIABLES backend, revision, waiter, sampled
vars == <<backend, revision, waiter, sampled>>
States == {"unplaced", "opening", "ready", "retrying", "closed"}
WaitingStates == {"opening", "retrying"}

(* Backend readiness for one mounted-store tracker lifetime.
   setPlacementBackend and swapRemote publish through remoteBcast;
   readBackend captures the state and its wait channel under that same lock.
   Closing the mount context wakes waiters through placementState.done.
   Publish overapproximates those transitions, including removal during open.
   UseWatch=FALSE retains the displaced polling wait as a comparison model.
   Readers exclude acquired read scopes, protocol IO and upload receipts. *)
Init == /\ backend = "unplaced"
        /\ revision = 0
        /\ waiter = [r \in Readers |-> "idle"]
        /\ sampled = [r \in Readers |-> 0]

Publish(next) ==
  /\ backend # "closed"
  /\ revision < MaxChanges
  /\ next \in States \ {backend}
  /\ backend' = next
  /\ revision' = revision + 1
  /\ waiter' = [r \in Readers |->
       IF UseWatch /\ Notify /\ waiter[r] = "waiting"
       THEN "awake" ELSE waiter[r]]
  /\ UNCHANGED sampled

(* Observe captures current state and the next notification under one lock.
   The implementation releases that lock before returning or using a store. *)
Observe(r) ==
  /\ waiter[r] \in {"idle", "awake"}
  /\ waiter' = [waiter EXCEPT ![r] =
       IF backend \in WaitingStates THEN "waiting" ELSE "done"]
  /\ sampled' = [sampled EXCEPT ![r] = revision]
  /\ UNCHANGED <<backend, revision>>

Poll(r) == /\ ~UseWatch
           /\ waiter[r] = "waiting"
           /\ waiter' = [waiter EXCEPT ![r] = "awake"]
           /\ UNCHANGED <<backend, revision, sampled>>

(* Cancel and Expire preserve terminal outcomes even if a publication races.
   The bounded model abstracts duration; it models the timer eventually firing. *)
Cancel(r) == /\ waiter[r] # "done"
             /\ waiter' = [waiter EXCEPT ![r] = "done"]
             /\ UNCHANGED <<backend, revision, sampled>>
Expire(r) == /\ waiter[r] \in {"waiting", "awake"}
             /\ waiter' = [waiter EXCEPT ![r] = "done"]
             /\ UNCHANGED <<backend, revision, sampled>>
Next == \/ \E next \in States : Publish(next)
        \/ \E r \in Readers : Observe(r) \/ Poll(r) \/ Cancel(r) \/ Expire(r)
        \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
        /\ \A r \in Readers : WF_vars(Observe(r)) /\ WF_vars(Poll(r)) /\ WF_vars(Expire(r))
TypeOK == /\ backend \in States
          /\ revision \in 0..MaxChanges
          /\ waiter \in [Readers -> {"idle", "waiting", "awake", "done"}]
          /\ sampled \in [Readers -> 0..MaxChanges]
NoMissedWake == \A r \in Readers : waiter[r] = "waiting" => sampled[r] = revision
WaitEnds == \A r \in Readers : (waiter[r] = "waiting") ~> (waiter[r] = "done")
=============================================================================
