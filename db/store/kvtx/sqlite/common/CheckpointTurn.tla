---------------------------- MODULE CheckpointTurn ----------------------------
\* The checkpointer bounds a store's write-ahead log in two steps. A passive
\* checkpoint copies the log into the database file and syncs it without the
\* writer turn or SQLite's write lock, so no writer waits on the copy or the
\* fsync. Then it takes the writer turn, in the order writers take it, so the
\* write lock is free, and restarts the log only if no writer ended since the
\* passive checkpoint began: the log is fully copied, and the restart only
\* waits for readers. Otherwise it gives the turn back for a later round.
\* Writers that follow each other with no gap end a turn during every passive
\* checkpoint, so that alone never restarts the log; with Force, a log past
\* its force bound (MaxLog here) has the remaining frames copied under the
\* turn and then restarts. Without Force, Restarts fails. Readers are short,
\* so the restart's short busy wait outlasts a reader on the log.
EXTENDS Naturals
CONSTANTS Writers, MaxLog, Force
VARIABLES turn, locked, log, copied, dirty, reading, ckpt
vars == <<turn, locked, log, copied, dirty, reading, ckpt>>
\* turn: who holds the writer turn, "none", a writer, or "ckpt".
\* locked: who holds SQLite's write lock, the same values.
\* log: frames in the write-ahead log, saturating at MaxLog.
\* copied: frames of the log copied into the database file and synced.
\* dirty: a writer ended its turn since the last copy began, as writeTurns
\* tells the checkpointer.
\* reading: a reader holds a snapshot on the log.
\* ckpt: the checkpointer's step, "idle", "copied" after a passive
\* checkpoint, "queued" for the turn, or "holding" it.
Holders == Writers \cup {"none", "ckpt"}
Init == /\ turn = "none" /\ locked = "none" /\ log = 0 /\ copied = 0
        /\ dirty = FALSE /\ reading = FALSE /\ ckpt = "idle"

\* A writer takes the free turn and begins at once, taking the write lock. The
\* turn goes in arrival order, so no writer passes a queued checkpointer.
Begin(w) == /\ turn = "none" /\ locked = "none" /\ ckpt /= "queued"
            /\ turn' = w /\ locked' = w
            /\ UNCHANGED <<log, copied, dirty, reading, ckpt>>
\* It commits, adding to the log and counting its turn, and hands the turn
\* and lock to the next writer with no gap, or leaves both free. A queued
\* checkpointer gets the turn next; the writers queued before it, finitely
\* many, are folded into this one.
Commit(w) == /\ turn = w /\ locked = w
             /\ log' = IF log < MaxLog THEN log + 1 ELSE log
             /\ dirty' = TRUE
             /\ IF ckpt = "queued"
                THEN turn' = "ckpt" /\ locked' = "none" /\ ckpt' = "holding"
                ELSE /\ \/ \E v \in Writers \ {w} : turn' = v /\ locked' = v
                        \/ turn' = "none" /\ locked' = "none"
                     /\ UNCHANGED ckpt
             /\ UNCHANGED <<copied, reading>>

\* A reader opens and closes a snapshot on the log.
Read == /\ reading' = ~reading
        /\ UNCHANGED <<turn, locked, log, copied, dirty, ckpt>>

\* The passive checkpoint notes the turns ended and copies the whole log,
\* holding neither the turn nor the write lock; writers go on meanwhile, and
\* their commits after it set dirty.
Passive == /\ ckpt = "idle" /\ log > 0
           /\ copied' = log /\ dirty' = FALSE /\ ckpt' = "copied"
           /\ UNCHANGED <<turn, locked, log, reading>>
\* It queues for the writer turn, which it takes at once when the turn is free.
Ask == /\ ckpt = "copied"
       /\ IF turn = "none" THEN turn' = "ckpt" /\ ckpt' = "holding"
          ELSE ckpt' = "queued" /\ UNCHANGED turn
       /\ UNCHANGED <<locked, log, copied, dirty, reading>>
\* Holding the turn, it gives the turn back when a writer ended since the
\* copy began, unless Force and the log is past its force bound.
GiveUp == /\ ckpt = "holding" /\ dirty /\ ~(Force /\ log = MaxLog)
          /\ turn' = "none" /\ ckpt' = "idle"
          /\ UNCHANGED <<locked, log, copied, dirty, reading>>
\* Past the force bound it copies and syncs the remaining frames under the
\* turn.
CopyHeld == /\ ckpt = "holding" /\ dirty /\ Force /\ log = MaxLog
            /\ copied' = log /\ dirty' = FALSE
            /\ UNCHANGED <<turn, locked, log, reading, ckpt>>
\* With the log fully copied it restarts once the reader has left the log,
\* copying nothing, and gives the turn back.
Restart == /\ ckpt = "holding" /\ ~dirty /\ ~reading
           /\ log' = 0 /\ copied' = 0 /\ turn' = "none" /\ ckpt' = "idle"
           /\ UNCHANGED <<locked, dirty, reading>>

Next == \/ \E w \in Writers : Begin(w) \/ Commit(w)
        \/ Read \/ Passive \/ Ask \/ GiveUp \/ CopyHeld \/ Restart
\* Writers keep writing and readers keep reading; the checkpointer keeps
\* copying and trying, and restarts whenever it can infinitely often.
Spec == /\ Init /\ [][Next]_vars
        /\ \A w \in Writers : WF_vars(Commit(w))
        /\ WF_vars(Read)
        /\ SF_vars(Passive) /\ SF_vars(Ask) /\ WF_vars(GiveUp)
        /\ WF_vars(CopyHeld) /\ SF_vars(Restart)

TypeOK == /\ turn \in Holders /\ locked \in Holders
          /\ log \in 0..MaxLog /\ copied \in 0..MaxLog /\ copied <= log
          /\ dirty \in BOOLEAN /\ reading \in BOOLEAN
          /\ ckpt \in {"idle", "copied", "queued", "holding"}
\* No writer holds the write lock while the checkpointer holds the turn.
TurnGuardsLock == turn = "ckpt" => locked = "none"
\* With no writer ended since the copy began, the whole log is copied.
CleanIsCopied == ~dirty => copied = log
\* A restart resets only a fully copied log, so it copies and syncs nothing.
RestartCopiesNothing == [][log > 0 /\ log' = 0 => copied = log]_vars
\* The checkpointer copies under the turn only past the force bound.
HeldCopyPastForce == [][turn = "ckpt" /\ copied' > copied => log = MaxLog]_vars
\* The log restarts again and again under steady writes.
Restarts == []<>(log = 0)
=============================================================================
