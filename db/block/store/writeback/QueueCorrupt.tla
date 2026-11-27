---- MODULE QueueCorrupt ----
EXTENDS Naturals, FiniteSets
(* A queued block whose local bytes fail their hash is known corrupt. Upload
   reads the queue oldest first; a read that fails its hash marks the block
   known corrupt, counts it in the status the person sees, and keeps its
   marker. The queue's head, and the linger deadline taken from the head's
   oldest entry, read only eligible entries, so a known corrupt block neither
   blocks the blocks behind it nor holds the linger at its old deadline.
   A slow timer, which runs only while a block is known corrupt, puts every
   known corrupt block back for one more read: a block repaired in the
   meantime, by a rewrite or a refill, then uploads and leaves the count.

   Blocks are ordered by age. state is the queue state of each block, good
   whether its local bytes hash to its ref, local whether the local store
   holds it, remote what the remote holds for it. count is the corrupt count
   the status shows, reads the corrupt reads of each block since the timer
   last fired. Upload takes one eligible block a step, in any order: a
   batch reads the head and the entries behind it, and a corrupt read skips
   its block without failing the rest of the batch. *)
CONSTANTS Blocks
VARIABLES state, good, local, remote, count, reads, timer
vars == <<state, good, local, remote, count, reads, timer>>

States == {"queued", "corrupt", "retry", "done"}
Known == {b \in Blocks : state[b] \in {"corrupt", "retry"}}
Eligible == {b \in Blocks : state[b] \in {"queued", "retry"}}

Init == /\ state = [b \in Blocks |-> "queued"]
        /\ good \in [Blocks -> BOOLEAN]
        /\ local = [b \in Blocks |-> TRUE]
        /\ remote = [b \in Blocks |-> "none"]
        /\ count = 0 /\ reads = [b \in Blocks |-> 0] /\ timer = "idle"

\* Upload reads an eligible block. Good bytes upload and clear the marker;
\* bytes that fail their hash leave the marker and count the block.
Upload(b) == /\ b \in Eligible
             /\ IF good[b]
                THEN /\ remote' = [remote EXCEPT ![b] = "good"]
                     /\ state' = [state EXCEPT ![b] = "done"]
                     /\ count' = IF b \in Known THEN count - 1 ELSE count
                     /\ UNCHANGED <<good, local, reads, timer>>
                ELSE /\ state' = [state EXCEPT ![b] = "corrupt"]
                     /\ count' = IF b \in Known THEN count ELSE count + 1
                     /\ reads' = [reads EXCEPT ![b] = IF @ < 2 THEN @ + 1 ELSE 2]
                     /\ UNCHANGED <<good, local, remote, timer>>

\* A rewrite or a refill gives a corrupt block good local bytes again.
Repair(b) == /\ ~good[b] /\ good' = [good EXCEPT ![b] = TRUE]
             /\ UNCHANGED <<state, local, remote, count, reads, timer>>

\* The slow timer arms while a block is known corrupt and fires once its
\* delay passes; firing puts every known corrupt block back for one read.
\* It arms again only once that pass read every block it put back, so an
\* older block that stays corrupt cannot starve a younger one repaired.
Arm == /\ timer = "idle" /\ Known # {} /\ \A b \in Blocks : state[b] # "retry"
       /\ timer' = "armed"
       /\ UNCHANGED <<state, good, local, remote, count, reads>>
Fire == /\ timer = "armed"
        /\ state' = [b \in Blocks |-> IF state[b] = "corrupt" THEN "retry" ELSE state[b]]
        /\ reads' = [b \in Blocks |-> 0] /\ timer' = "idle"
        /\ UNCHANGED <<good, local, remote, count>>

\* Eviction drops a local copy only once its marker is gone and the remote
\* holds the block.
Evict(b) == /\ local[b] /\ state[b] = "done" /\ remote[b] # "none"
            /\ local' = [local EXCEPT ![b] = FALSE]
            /\ UNCHANGED <<state, good, remote, count, reads, timer>>

Next == (\E b \in Blocks : Upload(b)) \/ Arm \/ Fire \/ (\E b \in Blocks : Repair(b) \/ Evict(b)) \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ (\A b \in Blocks : WF_vars(Upload(b)))
            /\ WF_vars(Arm) /\ WF_vars(Fire)

TypeOK == /\ state \in [Blocks -> States] /\ good \in [Blocks -> BOOLEAN]
          /\ local \in [Blocks -> BOOLEAN]
          /\ remote \in [Blocks -> {"none", "good", "bad"}]
          /\ count \in 0..Cardinality(Blocks) /\ reads \in [Blocks -> 0..2]
          /\ timer \in {"idle", "armed"}
\* The remote never holds bytes that fail their hash.
NoWrongBytes == \A b \in Blocks : remote[b] # "bad"
\* A local copy goes only after the remote holds the block.
KeptUntilUploaded == \A b \in Blocks : ~local[b] => remote[b] = "good"
\* The status counts exactly the known corrupt blocks, which keep their marker.
CountShown == count = Cardinality(Known)
\* A known corrupt block is read at most once per slow timer interval.
NoSpin == \A b \in Blocks : reads[b] <= 1
\* Every block with good bytes uploads, whatever stands ahead of it.
HealthyDrains == \A b \in Blocks : good[b] ~> state[b] = "done"
====
