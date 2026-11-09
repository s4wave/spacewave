---------------------------- MODULE CheckpointTurn ----------------------------
\* The checkpoint that restarts a store's write-ahead log needs SQLite's write
\* lock and no reader on the log. The store's writers take the writer turn and
\* then the write lock, and a waiting writer begins as soon as the last one
\* commits, so under steady writes the lock is never free between them. With
\* TakeTurn the checkpointer takes the writer turn first, in the order writers
\* take it, so it holds the free lock and waits only for readers. Without it,
\* it asks SQLite for the lock beside the writers, finds it held each time, and
\* the log grows without a restart: Restarts fails. Readers are short, so the
\* checkpointer's short busy wait outlasts a reader on the log; a held write
\* lock makes it give up for a later round, as restartWAL does.
EXTENDS Naturals, FiniteSets
CONSTANTS Writers, MaxLog, TakeTurn
VARIABLES turn, locked, log, reading, ckpt
vars == <<turn, locked, log, reading, ckpt>>
\* turn: who holds the writer turn, "none", a writer, or "ckpt".
\* locked: who holds SQLite's write lock, the same values.
\* log: frames in the write-ahead log, saturating at MaxLog.
\* reading: a reader holds a snapshot on the log.
\* ckpt: the checkpointer's step, "idle", "queued" for the turn, or "asking".
Holders == Writers \cup {"none", "ckpt"}
Init == turn = "none" /\ locked = "none" /\ log = 0 /\ reading = FALSE /\ ckpt = "idle"

\* A writer takes the free turn and begins at once, taking the write lock. The
\* turn goes in arrival order, so no writer passes a queued checkpointer.
Begin(w) == /\ turn = "none" /\ locked = "none" /\ ckpt /= "queued"
            /\ turn' = w /\ locked' = w
            /\ UNCHANGED <<log, reading, ckpt>>
\* It commits, adding to the log, and hands the turn and lock to the next
\* writer with no gap, or leaves both free. A queued checkpointer gets the
\* turn next; the writers queued before it, finitely many, are folded into
\* this one.
Commit(w) == /\ turn = w /\ locked = w
             /\ log' = IF log < MaxLog THEN log + 1 ELSE log
             /\ IF ckpt = "queued"
                THEN turn' = "ckpt" /\ locked' = "none" /\ ckpt' = "asking"
                ELSE /\ \/ \E v \in Writers \ {w} : turn' = v /\ locked' = v
                        \/ turn' = "none" /\ locked' = "none"
                     /\ UNCHANGED ckpt
             /\ UNCHANGED reading

\* A reader opens and closes a snapshot on the log.
Read == /\ reading' = ~reading /\ UNCHANGED <<turn, locked, log, ckpt>>

\* The checkpointer starts a restart of a log past its bound. With TakeTurn it
\* queues for the writer turn, which it takes at once when the turn is free;
\* without, it asks SQLite for the lock straight away.
Ask == /\ ckpt = "idle" /\ log > 0
       /\ IF ~TakeTurn THEN ckpt' = "asking" /\ UNCHANGED turn
          ELSE IF turn = "none" THEN turn' = "ckpt" /\ ckpt' = "asking"
          ELSE ckpt' = "queued" /\ UNCHANGED turn
       /\ UNCHANGED <<locked, log, reading>>
\* It restarts the log once the write lock is free and the reader has left
\* it, and gives up for a later round when a writer holds the lock.
Restart == /\ ckpt = "asking" /\ locked = "none" /\ ~reading
           /\ log' = 0 /\ ckpt' = "idle"
           /\ turn' = IF TakeTurn THEN "none" ELSE turn
           /\ UNCHANGED <<locked, reading>>
GiveUp == /\ ckpt = "asking" /\ locked /= "none"
          /\ ckpt' = "idle"
          /\ turn' = IF TakeTurn THEN "none" ELSE turn
          /\ UNCHANGED <<locked, log, reading>>

Next == \/ \E w \in Writers : Begin(w) \/ Commit(w)
        \/ Read \/ Ask \/ Restart \/ GiveUp
\* Writers keep writing and readers keep reading; the checkpointer keeps
\* trying and restarts whenever it can infinitely often.
Spec == /\ Init /\ [][Next]_vars
        /\ \A w \in Writers : WF_vars(Commit(w))
        /\ WF_vars(Read)
        /\ SF_vars(Ask) /\ SF_vars(Restart)

TypeOK == /\ turn \in Holders /\ locked \in Holders /\ log \in 0..MaxLog
          /\ reading \in BOOLEAN /\ ckpt \in {"idle", "queued", "asking"}
\* No writer holds the write lock while the checkpointer holds the turn.
TurnGuardsLock == turn = "ckpt" => locked = "none"
\* The log restarts again and again under steady writes.
Restarts == []<>(log = 0)
=============================================================================
