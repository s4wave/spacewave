---- MODULE DistReclaim ----
\* Native dist checkouts and their reclaimer.
\* An execution acquires its checkout's reference, syncs the checkout and runs
\* from it until it releases the reference.
\* The reclaimer lists the checkouts without the lock.
\* For each listed one it checks the reference count and claims the root in one
\* step under the lock, then renames the checkout to a trash name and drops the
\* claim without the lock.
\* An acquisition of a claimed root waits for the claim to drop.
\* Trash is removed later without the lock.
\* A checkout acquired after the listing must survive the pass.
EXTENDS Naturals, FiniteSets
Artifacts == {"a1", "a2"}
VARIABLES refs, acquired, running, disk, listed, claimed, pass, trash
vars == <<refs, acquired, running, disk, listed, claimed, pass, trash>>

Init == /\ refs = [a \in Artifacts |-> 0]
        /\ acquired = [a \in Artifacts |-> 0]
        /\ running = [a \in Artifacts |-> 0]
        /\ disk = [a \in Artifacts |-> "absent"]
        /\ listed = {}
        /\ claimed = {}
        /\ pass = "idle"
        /\ trash = 0

\* Acquire counts a reference before the execution syncs its checkout.
Acquire(a) == /\ acquired[a] + running[a] < 2
              /\ a \notin claimed
              /\ refs' = [refs EXCEPT ![a] = @ + 1]
              /\ acquired' = [acquired EXCEPT ![a] = @ + 1]
              /\ UNCHANGED <<running, disk, listed, claimed, pass, trash>>

\* Sync materializes the checkout and starts the process from it.
Sync(a) == /\ acquired[a] > 0
           /\ disk' = [disk EXCEPT ![a] = "present"]
           /\ acquired' = [acquired EXCEPT ![a] = @ - 1]
           /\ running' = [running EXCEPT ![a] = @ + 1]
           /\ UNCHANGED <<refs, listed, claimed, pass, trash>>

\* Fail ends an execution whose sync failed.
Fail(a) == /\ acquired[a] > 0
           /\ refs' = [refs EXCEPT ![a] = @ - 1]
           /\ acquired' = [acquired EXCEPT ![a] = @ - 1]
           /\ UNCHANGED <<running, disk, listed, claimed, pass, trash>>

\* Release drops the reference after the process has exited.
Release(a) == /\ running[a] > 0
              /\ refs' = [refs EXCEPT ![a] = @ - 1]
              /\ running' = [running EXCEPT ![a] = @ - 1]
              /\ UNCHANGED <<acquired, disk, listed, claimed, pass, trash>>

\* List reads the directory without the lock.
List == /\ pass = "idle"
        /\ listed' = {a \in Artifacts : disk[a] = "present"}
        /\ pass' = "moving"
        /\ UNCHANGED <<refs, acquired, running, disk, claimed, trash>>

\* Claim checks the reference and claims the root in one locked step.
Claim(a) == /\ pass = "moving"
            /\ a \in listed
            /\ listed' = listed \ {a}
            /\ claimed' = IF refs[a] = 0 /\ disk[a] = "present"
                             THEN claimed \cup {a}
                             ELSE claimed
            /\ UNCHANGED <<refs, acquired, running, disk, pass, trash>>

\* Rename moves a claimed checkout to trash and drops the claim without the lock.
Rename(a) == /\ a \in claimed
             /\ disk' = [disk EXCEPT ![a] = "absent"]
             /\ trash' = trash + 1
             /\ claimed' = claimed \ {a}
             /\ UNCHANGED <<refs, acquired, running, listed, pass>>

\* Remove deletes the trash without the lock and ends the pass.
Remove == /\ pass = "moving"
          /\ listed = {}
          /\ claimed = {}
          /\ trash' = 0
          /\ pass' = "idle"
          /\ UNCHANGED <<refs, acquired, running, disk, listed, claimed>>

Next == \/ \E a \in Artifacts : Acquire(a) \/ Sync(a) \/ Fail(a) \/ Release(a)
                               \/ Claim(a) \/ Rename(a)
        \/ List
        \/ Remove
Spec == Init /\ [][Next]_vars

\* RunningHasFiles holds when every running execution's checkout is on disk.
RunningHasFiles == \A a \in Artifacts : running[a] > 0 => disk[a] = "present"
\* RefsCountExecutions holds when each reference belongs to one execution.
RefsCountExecutions == \A a \in Artifacts : refs[a] = acquired[a] + running[a]
=============================================================================
