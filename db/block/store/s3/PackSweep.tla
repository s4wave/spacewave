------------------------------ MODULE PackSweep ------------------------------
EXTENDS Naturals, Sequences, FiniteSets
\* A pack sweep cycle against a store whose writers never stop.
\*
\* The command (cmd/spacewave-pack-sweep) reads the write log's position R,
\* then the roots into the live set L, quarantines each snapshot pack whose
\* every key is outside L and W or placed in another registered pack, rechecks
\* it against the log since R, moves it to the trash, and removes it. The pack
\* store (PackStore) skips a quarantined pack on reads and falls back to it on
\* a miss, lifting it; a read that falls back to a trashing pack records the
\* miss, and the command restores that pack.
\*
\* Callers, from source and from the plan's cycle:
\* - Writers of the write-back store: a put of a parent that links a block
\*   (Link), a root change that drops one (Unlink), a put of a block's own data
\*   (RePut, which uploads unless locate places it) and a read that fills
\*   (Read). Each is a write log line while the log is on. A client may hold any
\*   block's hash from long ago, so any block may be linked or read at any time.
\* - The log: it breaks on a failed write, the cap or a backend crash, and the
\*   operator turns it on again in a new epoch (LogBreak, LogOn).
\* - The command: Position and Roots (step 3), Quarantine (steps 5 and 7),
\*   Recheck (step 8), BeginTrash and MoveFile (step 9), Remove (step 10),
\*   Restore, and Gap, which sends the cycle back to step 3.
\* Left out: the snapshot, since candidates are the snapshot's packs and a pack
\* never changes; the closure, which W abstracts as the blocks the log names,
\* and closure_test.go checks; a crash of the command, since each step commits
\* its scratch state before it calls the backend and every call is idempotent,
\* which Phase 6's run-through checks; compaction, whose adopted pack is the
\* duplicate holder p1 is to p2 here; the purge, an operator step after the
\* trash's retention. Each write here is one step, its log line included, and
\* each read takes its block in the step it reads; in the code a write spans
\* the dedup check and its log line, and a read operation may hold a block it
\* read before the quarantine. The recheck passes a pack only once the oldest
\* write and read operation open on the store began after its quarantine
\* (MarkingStore.OldestOperations), which makes each one atomic against it.
\*
\* A link made after every holder of the block passed its recheck, or a read
\* made after the command's last check before every holder's file moves, is
\* late: no log line the
\* cycle reads in time names it. The trash keeps the pack, and a late read
\* records the miss that restores it; the invariants cover every other block.

\* Start: the sets of blocks the roots may reach when the cycle begins.
CONSTANTS MaxWrites, MaxBreaks, Start

Blocks == {1, 2, 3}
Packs == {"p1", "p2"}
\* Block 2 is in both packs, so each is the other's holder elsewhere.
Holds(p) == IF p = "p1" THEN {1, 2} ELSE {2, 3}

VARIABLES
  reached,  \* blocks the roots and pinned refs reach now
  needed,   \* blocks read since the cycle's position R
  state,    \* each pack's state in the pack store
  moved,    \* each pack whose file is in the trash
  missed,   \* each trashing pack a fallback read missed
  fresh,    \* blocks uploaded again into a pack outside the snapshot
  log,      \* the open epoch's lines
  logOn,
  epoch,
  breaks,
  writes,
  phase,    \* the command's cycle: idle, pos, walked
  R, REpoch, L,
  cst,      \* the command's record of each pack
  late,
  failed    \* a read of a block the invariants cover found no copy

vars == <<reached, needed, state, moved, missed, fresh, log, logOn, epoch,
          breaks, writes, phase, R, REpoch, L, cst, late, failed>>
store == <<state, moved, missed, fresh>>
cmd == <<phase, R, REpoch, L, cst>>
logv == <<log, logOn, epoch, breaks>>

Init ==
  /\ reached \in Start
  /\ needed = {}
  /\ state = [p \in Packs |-> "reg"]
  /\ moved = [p \in Packs |-> FALSE]
  /\ missed = [p \in Packs |-> FALSE]
  /\ fresh = {}
  /\ log = <<>> /\ logOn = TRUE /\ epoch = 1 /\ breaks = 0 /\ writes = 0
  /\ phase = "idle" /\ R = 0 /\ REpoch = 0 /\ L = {}
  /\ cst = [p \in Packs |-> "none"]
  /\ late = {} /\ failed = FALSE

\* W: the blocks the log names since R, in the walk's epoch.
W == IF epoch = REpoch THEN {log[i] : i \in (R + 1)..Len(log)} ELSE {}

\* Holders of b other than p that a lookup places it in.
Elsewhere(b, p) ==
  (b \in fresh) \/ \E q \in Packs \ {p} : b \in Holds(q) /\ state[q] = "reg"

Removable(p) == \A b \in Holds(p) : b \notin (L \cup W) \/ Elsewhere(b, p)

\* The log covers the cycle from R on.
Covered == phase = "walked" /\ logOn /\ epoch = REpoch

\* A copy a read can reach: registered, or quarantined or trashing in place.
Serves(p) == state[p] \in {"reg", "quar"} \/ (state[p] = "trash" /\ ~moved[p])
Reachable(b) == (b \in fresh) \/ \E p \in Packs : b \in Holds(p) /\ Serves(p)
InPlace(b) == \E p \in Packs : b \in Holds(p) /\ state[p] = "trash" /\ ~moved[p]

\* Every live holder of b passed its recheck, or every one passed the
\* command's last check for a miss before its file moves.
Passed(b) ==
  /\ b \notin fresh
  /\ \A p \in Packs : (b \in Holds(p) /\ state[p] # "removed") =>
       cst[p] \in {"pass", "trash", "move", "done"}
Gone(b) ==
  /\ b \notin fresh
  /\ \A p \in Packs : (b \in Holds(p) /\ state[p] # "removed") =>
       cst[p] \in {"move", "done"}

Append1(b) == IF logOn THEN log' = Append(log, b) ELSE log' = log
Wrote == writes < MaxWrites /\ writes' = writes + 1

\* ---- Writers ----

Link(b) ==
  /\ Wrote /\ b \notin reached
  /\ reached' = reached \cup {b}
  /\ Append1(b)
  /\ late' = IF Passed(b) THEN late \cup {b} ELSE late
  /\ UNCHANGED <<needed, store, logOn, epoch, breaks, cmd, failed>>

Unlink(b) ==
  /\ Wrote /\ b \in reached
  /\ reached' = reached \ {b}
  /\ UNCHANGED <<needed, store, logv, cmd, late, failed>>

\* Locate places b only in a registered pack, so a put of a block that only
\* quarantined or trashing packs hold uploads it again.
RePut(b) ==
  /\ Wrote
  /\ reached' = reached \cup {b}
  /\ Append1(b)
  /\ fresh' = IF \E p \in Packs : b \in Holds(p) /\ state[p] = "reg"
              THEN fresh ELSE fresh \cup {b}
  /\ UNCHANGED <<needed, state, moved, missed, logOn, epoch, breaks, cmd,
                 late, failed>>

\* A read that misses the registered packs falls back: a quarantined holder
\* serves it and is lifted; a trashing holder records the miss, and serves it
\* while its file is in place.
Read(b) ==
  /\ Wrote
  /\ needed' = needed \cup {b}
  /\ Append1(b)
  /\ IF (b \in fresh) \/ \E p \in Packs : b \in Holds(p) /\ state[p] = "reg"
     THEN UNCHANGED <<state, missed, late, failed>>
     ELSE IF \E p \in Packs : b \in Holds(p) /\ state[p] = "quar"
     THEN /\ \E p \in Packs :
               /\ b \in Holds(p) /\ state[p] = "quar"
               /\ state' = [state EXCEPT ![p] = "reg"]
          /\ UNCHANGED <<missed, late, failed>>
     ELSE /\ missed' = [p \in Packs |->
                          missed[p] \/ (b \in Holds(p) /\ state[p] = "trash")]
          /\ late' = IF Gone(b) THEN late \cup {b} ELSE late
          /\ failed' = (failed \/ (~InPlace(b) /\ ~Gone(b)))
          /\ UNCHANGED state
  /\ UNCHANGED <<reached, moved, fresh, logOn, epoch, breaks, cmd>>

\* ---- The log ----

LogBreak ==
  /\ logOn /\ breaks < MaxBreaks
  /\ logOn' = FALSE /\ breaks' = breaks + 1
  /\ UNCHANGED <<reached, needed, store, log, epoch, writes, cmd, late, failed>>

LogOn ==
  /\ ~logOn
  /\ logOn' = TRUE /\ epoch' = epoch + 1 /\ log' = <<>>
  /\ UNCHANGED <<reached, needed, store, breaks, writes, cmd, late, failed>>

\* ---- The command ----

\* The admin service's Roots reads the position, then the roots.
Position ==
  /\ phase = "idle" /\ logOn
  /\ phase' = "pos"
  /\ R' = Len(log) /\ REpoch' = epoch /\ L' = L
  /\ needed' = {}
  /\ UNCHANGED <<reached, store, logv, writes, cst, late, failed>>

Roots ==
  /\ phase = "pos"
  /\ phase' = "walked"
  /\ L' = reached /\ R' = R /\ REpoch' = REpoch
  /\ UNCHANGED <<reached, needed, store, logv, writes, cst, late, failed>>

Quarantine(p) ==
  /\ Covered /\ state[p] = "reg" /\ cst[p] = "none" /\ Removable(p)
  /\ state' = [state EXCEPT ![p] = "quar"]
  /\ cst' = [cst EXCEPT ![p] = "quar"]
  /\ UNCHANGED <<reached, needed, moved, missed, fresh, logv, writes,
                 phase, R, REpoch, L, late, failed>>

Recheck(p) ==
  /\ cst[p] = "quar" /\ state[p] = "quar"
  /\ Covered /\ Removable(p)
  /\ cst' = [cst EXCEPT ![p] = "pass"]
  /\ UNCHANGED <<reached, needed, store, logv, writes, phase, R, REpoch, L,
                 late, failed>>

\* The command finds a lift and drops the pack from its cycle.
Dropped(p) ==
  /\ cst[p] \in {"quar", "pass"} /\ state[p] = "reg"
  /\ cst' = [cst EXCEPT ![p] = "none"]
  /\ UNCHANGED <<reached, needed, store, logv, writes, phase, R, REpoch, L,
                 late, failed>>

\* A gap sends the cycle back to step 3, and every quarantine stays. A pack
\* that already passed goes on: no log line after its recheck could keep it.
Gap ==
  /\ phase # "idle" /\ (~logOn \/ epoch # REpoch)
  /\ phase' = "idle"
  /\ UNCHANGED <<reached, needed, store, logv, writes, R, REpoch, L, cst,
                 late, failed>>

\* BeginTrash in the pack store moves only a pack still quarantined.
BeginTrash(p) ==
  /\ cst[p] = "pass" /\ state[p] = "quar"
  /\ state' = [state EXCEPT ![p] = "trash"]
  /\ cst' = [cst EXCEPT ![p] = "trash"]
  /\ UNCHANGED <<reached, needed, moved, missed, fresh, logv, writes,
                 phase, R, REpoch, L, late, failed>>

\* The command reads the pack's miss record, then renames its file over SFTP;
\* reads run between the two.
MoveCheck(p) ==
  /\ cst[p] = "trash" /\ state[p] = "trash" /\ ~missed[p]
  /\ cst' = [cst EXCEPT ![p] = "move"]
  /\ UNCHANGED <<reached, needed, store, logv, writes, phase, R, REpoch, L,
                 late, failed>>

MoveFile(p) ==
  /\ cst[p] = "move" /\ state[p] = "trash" /\ ~moved[p]
  /\ moved' = [moved EXCEPT ![p] = TRUE]
  /\ UNCHANGED <<reached, needed, state, missed, fresh, logv, writes, cmd,
                 late, failed>>

Remove(p) ==
  /\ cst[p] = "move" /\ state[p] = "trash" /\ moved[p] /\ ~missed[p]
  /\ state' = [state EXCEPT ![p] = "removed"]
  /\ cst' = [cst EXCEPT ![p] = "done"]
  /\ UNCHANGED <<reached, needed, moved, missed, fresh, logv, writes,
                 phase, R, REpoch, L, late, failed>>

\* A recorded miss restores the pack before anything else: the file comes
\* back, then the entry, and the pack is registered again. The command makes
\* these in three steps (packsweep.Restore), which one step abstracts: between
\* them the pack stays trashing with its miss, which the trash and the removal
\* both skip, and a read finds its file either gone, recording the miss again,
\* or back, and served. The pack store registers it only once both objects
\* are back. A removed pack the operator names comes back by adoption, which
\* adds a holder of its keys as compaction's adopted pack does, and is left out.
Restore(p) ==
  /\ missed[p] /\ state[p] = "trash"
  /\ moved' = [moved EXCEPT ![p] = FALSE]
  /\ state' = [state EXCEPT ![p] = "reg"]
  /\ missed' = [missed EXCEPT ![p] = FALSE]
  /\ cst' = [cst EXCEPT ![p] = "none"]
  /\ UNCHANGED <<reached, needed, fresh, logv, writes, phase, R, REpoch, L,
                 late, failed>>

\* Every write was made and the command has nothing left to do.
Done ==
  /\ writes = MaxWrites
  /\ UNCHANGED vars

Next ==
  \/ \E b \in Blocks : Link(b) \/ Unlink(b) \/ RePut(b) \/ Read(b)
  \/ LogBreak \/ LogOn
  \/ Position \/ Roots \/ Gap
  \/ \E p \in Packs : \/ Quarantine(p) \/ Recheck(p) \/ Dropped(p)
                      \/ BeginTrash(p) \/ MoveCheck(p) \/ MoveFile(p)
                      \/ Remove(p)
                      \/ Restore(p)
  \/ Done

Spec == Init /\ [][Next]_vars /\ \A p \in Packs : WF_vars(Restore(p))

\* ---- Invariants ----

\* Every block the roots reach, or someone read, keeps a copy a read reaches,
\* unless its link or read came late.
Served == \A b \in (reached \cup needed) \ late : Reachable(b)

\* No read of a block the invariants cover finds no copy.
NoFailedRead == ~failed

\* Remove runs only on a pack whose file is in the trash.
RemovedFromTrash == \A p \in Packs : state[p] = "removed" => moved[p]

\* A pack a read missed is never removed.
MissKept == \A p \in Packs : missed[p] => state[p] # "removed"

\* A recorded miss brings the pack back.
MissRestored == \A p \in Packs : missed[p] ~> state[p] = "reg"

TypeOK ==
  /\ state \in [Packs -> {"reg", "quar", "trash", "removed"}]
  /\ cst \in [Packs -> {"none", "quar", "pass", "trash", "move", "done"}]
  /\ phase \in {"idle", "pos", "walked"}
==============================================================================
