---- MODULE FollowerReplay ----
\* What a member that does not own a SharedObject shows when one owner, the
\* leader, both orders and checkpoints its operations. The member's body
\* (body-driver.go step) installs each checkpoint the state holds and, with
\* Replay, applies the operations the state orders above it. Without Replay
\* it shows the checkpoint alone, as members that did not own once did.
\*
\* The leader orders operations into the state and checkpoints once Interval
\* of them sit above the last checkpoint (MinCheckpointOperations); Interval =
\* 1 is a leader that checkpoints every batch. The leader may stop at any
\* time, such as between ordering an operation and its checkpoint, and not
\* come back; the checkpoint deadline does not close that gap, since the
\* leader can stop before the deadline passes. The follower may restart once,
\* losing its body's memory, and reload the checkpoint.
\*
\* Shown: once the order stops growing, the follower comes to show every
\* ordered operation. Showing the checkpoint alone fails it for every
\* Interval, since the operations above the last checkpoint stay unshown
\* while the leader is stopped (testdata/FollowerReplay-follow.cfg); a leader
\* that checkpoints every batch only narrows the gap to one batch
\* (testdata/FollowerReplay-follow-batch.cfg).
\*
\* Outside the model: the member's own edits, group control, refusals, a
\* block the follower must fetch before it applies (the body waits and
\* retries), and the order's agreement, which StableCheckpoint covers.
EXTENDS Naturals

CONSTANTS N, Interval, Replay

VARIABLES order, ckpt, leader, shown, up, restarts
vars == <<order, ckpt, leader, shown, up, restarts>>

TypeOK == /\ order \in 0..N /\ ckpt \in 0..N /\ leader \in BOOLEAN
          /\ shown \in 0..N /\ up \in BOOLEAN /\ restarts \in 0..1

Init == /\ order = 0 /\ ckpt = 0 /\ leader = TRUE
        /\ shown = 0 /\ up = TRUE /\ restarts = 0

\* The leader orders one more operation into the state.
Order == /\ leader /\ order < N
         /\ order' = order + 1
         /\ UNCHANGED <<ckpt, leader, shown, up, restarts>>

\* The leader stores and adopts a checkpoint over the ordered operations.
Checkpoint == /\ leader /\ order - ckpt >= Interval
              /\ ckpt' = order
              /\ UNCHANGED <<order, leader, shown, up, restarts>>

\* The leader stops and does not come back.
Stop == /\ leader /\ leader' = FALSE
        /\ UNCHANGED <<order, ckpt, shown, up, restarts>>

\* The follower installs a checkpoint it has not reached. Without Replay it
\* also drops what it applied above the checkpoint, as the follow path the
\* body once had did.
Install == /\ up
           /\ IF Replay THEN shown < ckpt ELSE shown # ckpt
           /\ shown' = ckpt
           /\ UNCHANGED <<order, ckpt, leader, up, restarts>>

\* Replaying, the follower applies the next operation above what it shows.
Apply == /\ Replay /\ up /\ ckpt <= shown /\ shown < order
         /\ shown' = shown + 1
         /\ UNCHANGED <<order, ckpt, leader, up, restarts>>

\* The follower restarts once, losing its body's memory.
Crash == /\ up /\ restarts = 0
         /\ up' = FALSE /\ shown' = 0 /\ restarts' = 1
         /\ UNCHANGED <<order, ckpt, leader>>

Recover == /\ ~up /\ up' = TRUE /\ shown' = ckpt
           /\ UNCHANGED <<order, ckpt, leader, restarts>>

\* The order stops growing once the leader stops or orders the last
\* operation; the follower may then wait with nothing to do.
Quiet == /\ ~leader \/ order = N
         /\ UNCHANGED vars

Next == Order \/ Checkpoint \/ Stop \/ Install \/ Apply \/ Crash
        \/ Recover \/ Quiet

Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Install) /\ WF_vars(Apply) /\ WF_vars(Recover)
            /\ WF_vars(Checkpoint \/ Stop)

\* The follower never shows an operation the state does not order.
Bounded == /\ ckpt <= order /\ shown <= order

\* Once the order stops growing, the follower comes to show all of it.
Shown == <>[](up /\ shown = order)
====
