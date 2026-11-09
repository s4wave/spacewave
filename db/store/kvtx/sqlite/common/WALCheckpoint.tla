---- MODULE WALCheckpoint ----
EXTENDS Naturals
(* The SQLite store's write lock between its writers and the WAL checkpointer,
   with time. A write takes the writer turn (Store.writeMtx) in this process,
   then BEGIN IMMEDIATE takes SQLite's write lock. runCheckpoints copies the
   log with a passive checkpoint, which takes no lock; for a fully copied log
   past its bound, restartWAL holds SQLite's write lock for Hold ticks while it
   copies the frames committed since and waits for readers. A write that finds
   SQLite's lock held waits in the busy handler for BusyTimeout ticks, then
   fails with "database is locked". The store has no busy retry, so that
   failure reaches the caller, such as writeback's marker clear, whose upload
   routine then exits and sends the pack again.

   Hold > BusyTimeout is the measured case: 2 to 15 s on a busy disk against a
   5 s busy wait. RestartTakesTurn is the guard: the restart takes the writer
   turn before the lock, so a write waits for it in the turn and never in the
   busy handler. Without the guard, the restart is a lock holder outside the
   writer queue, and the store comment's claim that writers never wait in the
   busy handler is false; the absent busy retry then fails the write.
   Restarts bounds the restarts one write can meet. *)
CONSTANTS BusyTimeout, Hold, Restarts

\* RestartTakesTurn is restartWAL taking writeMtx before the checkpoint.
RestartTakesTurn == TRUE

VARIABLES
  turn,     \* "free" or "ckpt": the writer turn
  lock,     \* "free" or "ckpt": SQLite's write lock
  holdLeft, \* ticks left in the restart
  left,     \* restarts still to come
  clear,    \* the write: "wait", "done" or "failed"
  waited    \* ticks the write has waited in SQLite's busy handler

vars == <<turn, lock, holdLeft, left, clear, waited>>

Init == /\ turn = "free" /\ lock = "free" /\ holdLeft = 0 /\ left = Restarts
        /\ clear = "wait" /\ waited = 0

\* InBusyHandler holds while the write has the turn and SQLite's lock is
\* taken.
InBusyHandler == clear = "wait" /\ turn = "free" /\ lock = "ckpt"

\* Restart is restartWAL taking the lock for a fully copied log.
Restart ==
  /\ turn = "free" /\ lock = "free" /\ left > 0
  /\ left' = left - 1 /\ lock' = "ckpt" /\ holdLeft' = Hold
  /\ turn' = IF RestartTakesTurn THEN "ckpt" ELSE turn
  /\ UNCHANGED <<clear, waited>>

\* Commit is the write taking the turn and the lock, and committing.
Commit ==
  /\ clear = "wait" /\ turn = "free" /\ lock = "free"
  /\ clear' = "done" /\ waited' = 0
  /\ UNCHANGED <<turn, lock, holdLeft, left>>

\* Busy is the busy wait expiring with the lock still held.
Busy ==
  /\ InBusyHandler /\ waited = BusyTimeout
  /\ clear' = "failed" /\ waited' = 0
  /\ UNCHANGED <<turn, lock, holdLeft, left>>

\* Tick is one unit of time: the restart's hold runs down and a busy wait
\* runs up.
Tick ==
  /\ lock = "ckpt"
  /\ ~(InBusyHandler /\ waited = BusyTimeout)
  /\ holdLeft' = holdLeft - 1
  /\ lock' = IF holdLeft = 1 THEN "free" ELSE lock
  /\ turn' = IF holdLeft = 1 THEN "free" ELSE turn
  /\ waited' = IF InBusyHandler THEN waited + 1 ELSE waited
  /\ UNCHANGED <<left, clear>>

Finished == clear \in {"done", "failed"} /\ UNCHANGED vars

Next == Restart \/ Commit \/ Busy \/ Tick \/ Finished

Spec == Init /\ [][Next]_vars /\ WF_vars(Commit) /\ WF_vars(Busy)
        /\ WF_vars(Tick)

TypeOK == /\ turn \in {"free", "ckpt"} /\ lock \in {"free", "ckpt"}
          /\ holdLeft \in 0..Hold /\ left \in 0..Restarts
          /\ clear \in {"wait", "done", "failed"} /\ waited \in 0..BusyTimeout

\* No write waits in SQLite's busy handler past its timeout.
WritersNeverBusy == ~(InBusyHandler /\ waited = BusyTimeout)
\* No write fails for a lock it could have waited out.
NoLostClear == clear # "failed"
ClearCommits == clear = "wait" ~> clear = "done"
====
