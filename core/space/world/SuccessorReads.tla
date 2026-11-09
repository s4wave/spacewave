---- MODULE SuccessorReads ----
\* What the successor B reads of one operation the departing owner A wrote
\* above the installed checkpoint, during a handover: the operation's payload
\* p, which A wrote and B fetches, and the head World page g that B's replay
\* computes from the operation and keeps in memory.
\* Callers: Adapter.Apply on B, spaceBackfill.execute through copyAndHold
\* while it follows the head during REPLICA_STEP_BACKFILL and SERVE_PROOF and
\* at the checkpoint after, the reconciler's transfer, an engine reopen,
\* Install, the test's cut-off, and the Volume collector. The collector
\* abstracts gc/Journal: it frees a stored block that no root holds once its
\* release passed the grace, which any interleaving here may outlast. A cursor
\* save also holds p and g, but only after MinCheckpointOperations positions,
\* so a short tail above the checkpoint never reaches it and it is left out.
\* A reopen loses the kept pages, and the replay after it computes g again
\* before any read, so it leaves gKept as it was.
EXTENDS Naturals
CONSTANTS HoldOnApply, KeepOnCopy
VARIABLES applied, pStored, gKept, gStored, follow, backfillHolds,
          payloadsHold, covered, owner, peer
vars == <<applied, pStored, gKept, gStored, follow, backfillHolds,
          payloadsHold, covered, owner, peer>>

Init == /\ applied = FALSE /\ pStored = FALSE /\ gKept = FALSE
        /\ gStored = FALSE /\ follow = TRUE /\ backfillHolds = FALSE
        /\ payloadsHold = FALSE /\ covered = FALSE /\ owner = "A"
        /\ peer = TRUE

\* B replays A's operation, keeping g in memory. With the hold, Apply holds
\* p under replay-payloads, whether or not p is stored yet.
Apply == /\ ~applied
         /\ applied' = TRUE /\ gKept' = TRUE
         /\ payloadsHold' = (payloadsHold \/ HoldOnApply)
         /\ UNCHANGED <<pStored, gStored, follow, backfillHolds, covered, owner, peer>>

\* A read on demand stores p unheld while A serves it.
Fetch == /\ ~pStored /\ peer
         /\ pStored' = TRUE
         /\ UNCHANGED <<applied, gKept, gStored, follow, backfillHolds, payloadsHold, covered, owner, peer>>

\* The copy that follows the head stores p and g and moves the backfill root
\* onto the head World, which names both. Moved through the kept pages, the
\* move takes g out of memory.
FollowCopy == /\ follow /\ applied /\ (pStored \/ peer)
              /\ pStored' = TRUE /\ gStored' = TRUE /\ backfillHolds' = TRUE
              /\ gKept' = (gKept /\ KeepOnCopy)
              /\ UNCHANGED <<applied, follow, payloadsHold, covered, owner, peer>>

\* The copy at the checkpoint moves the backfill root onto the checkpoint
\* World, which names p and g only once a checkpoint covers them.
CheckpointCopy == /\ ~follow /\ backfillHolds /\ ~covered
                  /\ backfillHolds' = FALSE
                  /\ UNCHANGED <<applied, pStored, gKept, gStored, follow, payloadsHold, covered, owner, peer>>

\* A takes B's proof of a complete copy of its head, and B owns. The
\* reconciler stops following once it no longer proves.
Transfer == /\ owner = "A" /\ follow /\ applied /\ pStored /\ gStored /\ backfillHolds
            /\ owner' = "B" /\ follow' = FALSE
            /\ UNCHANGED <<applied, pStored, gKept, gStored, backfillHolds, payloadsHold, covered, peer>>

\* B's engine reopens: the named roots stay, and the new backfill does not
\* follow the head.
Reopen == /\ follow
          /\ follow' = FALSE
          /\ UNCHANGED <<applied, pStored, gKept, gStored, backfillHolds, payloadsHold, covered, owner, peer>>

\* The reconciler follows the head again while A still owns.
Refollow == /\ ~follow /\ owner = "A"
            /\ follow' = TRUE
            /\ UNCHANGED <<applied, pStored, gKept, gStored, backfillHolds, payloadsHold, covered, owner, peer>>

\* A checkpoint covers the operation: its World, held by replay-base, names
\* p and g, the move stores g, and the payloads reset drops p from
\* replay-payloads.
Install == /\ applied /\ ~covered /\ (pStored \/ peer) /\ (gKept \/ gStored)
           /\ covered' = TRUE /\ pStored' = TRUE /\ gStored' = TRUE
           /\ payloadsHold' = FALSE
           /\ UNCHANGED <<applied, gKept, follow, backfillHolds, owner, peer>>

\* A leaves: nothing serves p but B's store.
CutOff == /\ owner = "B" /\ peer
          /\ peer' = FALSE
          /\ UNCHANGED <<applied, pStored, gKept, gStored, follow, backfillHolds, payloadsHold, covered, owner>>

\* The collector frees p or g once nothing holds it.
CollectPayload == /\ pStored /\ ~backfillHolds /\ ~payloadsHold /\ ~covered
                  /\ pStored' = FALSE
                  /\ UNCHANGED <<applied, gKept, gStored, follow, backfillHolds, payloadsHold, covered, owner, peer>>
CollectPage == /\ gStored /\ ~backfillHolds /\ ~covered
               /\ gStored' = FALSE
               /\ UNCHANGED <<applied, pStored, gKept, follow, backfillHolds, payloadsHold, covered, owner, peer>>

\* The handover ends once A is gone and a checkpoint covers the operation.
Done == /\ owner = "B" /\ ~peer /\ covered /\ UNCHANGED vars

Next == Apply \/ Fetch \/ FollowCopy \/ CheckpointCopy \/ Transfer \/ Reopen
        \/ Refollow \/ Install \/ CutOff \/ CollectPayload \/ CollectPage \/ Done

Spec == Init /\ [][Next]_vars

TypeOK == /\ applied \in BOOLEAN /\ pStored \in BOOLEAN /\ gKept \in BOOLEAN
          /\ gStored \in BOOLEAN /\ follow \in BOOLEAN
          /\ backfillHolds \in BOOLEAN /\ payloadsHold \in BOOLEAN
          /\ covered \in BOOLEAN /\ owner \in {"A", "B"} /\ peer \in BOOLEAN

\* The owner reads p from its own store once no peer serves it.
OwnerReadsPayload == (owner = "B" /\ ~peer /\ applied) => pStored

\* The owner reads g from memory or its own store once no peer serves it.
OwnerReadsPage == (owner = "B" /\ ~peer /\ applied) => (gKept \/ gStored)
====
