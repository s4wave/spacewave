---- MODULE WorldRegen ----
\* Regeneration of one World into the newest format while writers commit:
\* the contract of Regenerator in regen.go. The regenerator pins a root,
\* builds a new-format World from it, catches the new World up from the root
\* diff, and swaps the head through the Authority; the old root stays held
\* until a comparison proves the two roots equal, and until then a switch back
\* returns to the old format. The switch back mirrors the swap: rounds off the
\* writer turn carry the old root forward by the diff the new format's
\* commits made, and only the last bounded diff runs under the turn.
\*
\* Every diff applies strictly, as ApplyDeltas does with no ignore options: a
\* delete of an item the target lacks or an add of one it holds means the two
\* roots diverged, and the regenerator stops, keeping both roots held.
\*
\* A World's content is a set of items: object records and quads alike. A
\* commit adds or deletes one item, so a catch-up that drops deletes or a swap
\* that drops a commit shows as a head that differs from history, the content
\* the acknowledged commits define.
\*
\* Actors, from the Engine (db/world/block/engine.go):
\* - Local writers: modeled. A write turn holds the Engine's wmtx, so a local
\*   commit cannot run while the regenerator holds the turn for its swap.
\* - Remote writers: modeled. Another Device commits through the same
\*   Authority without the local turn; the Authority accepts a candidate only
\*   on the head it was built from, so a remote commit between the swap's
\*   proposal and its decision makes the Authority refuse the swap.
\* - The regenerator: modeled, one per World. Its phase, cursor, partial World
\*   and candidate live in memory; its state file holds what a restart
\*   resumes: the stage, the build position with the pages written up to it,
\*   the new World and the old content it reflects, and the swap's intent.
\* - The walk's own frontier persistence: left out, WalkState.tla in
\*   db/block/packcopy/world checks it. Here the build position advances one
\*   item at a time and saves with the pages before it.
\* - Block GC: left out as an actor. held is the named hold on the old roots
\*   the regenerator reads, and the invariants say when it may go.
\*
\* A crash can happen in every phase and loses everything but the head, the
\* hold and the state file. Recover derives the phase from the head's format
\* and the file: the Authority's head is the truth about whether a swap took,
\* since a crash can fall between its decision and the file's record of it.
\*
\* The code keeps the state file as the named roots base and built, and moves
\* them in the same SetRoots that installs a swap's World root, so it has no
\* window between the decision and its record: the model checks the looser
\* protocol, which the code refines. The model's swap.old is base, swap.new is
\* built, and a stopped regeneration is a Run that returns its error with both
\* roots held.
\*
\* With Faults, the build may skip an item, standing for a defect in the new
\* format's writer. The comparison must then refuse to release the old root,
\* and history is checked only while no fault happened.

EXTENDS Naturals, FiniteSets

CONSTANTS N, MaxCommits, MaxCrashes, Faults

Items == 1..N
Writers == {"local", "remote"}
NoRoot == [fmt |-> "none", items |-> {}]
NoCand == [base |-> 0, root |-> NoRoot]
NoSwap == [old |-> NoRoot, new |-> NoRoot]
Terminal == {"verified", "switchedBack", "stopped"}

\* Apply moves set by the change from one content to another: the root diff.
Apply(set, from, to) == (set \ (from \ to)) \cup (to \ from)

\* Clean holds when the diff from one content to another applies to set
\* strictly: every item it deletes is in set and every item it adds is not.
Clean(set, from, to) ==
  /\ (from \ to) \subseteq set
  /\ (to \ from) \cap set = {}

\* Toggle adds an item a set lacks or deletes one it holds.
Toggle(set, i) == IF i \in set THEN set \ {i} ELSE set \cup {i}

VARIABLES
  head,     \* the Authority's head: format, content, seqno
  history,  \* ghost: the content the acknowledged commits define
  commits,  \* commits so far
  crashes,  \* crashes so far
  faulty,   \* ghost: the build skipped an item
  phase,    \* the regenerator's phase, "down" after a crash
  turn,     \* the regenerator holds the local write turn
  pin,      \* the pinned root's content
  cursor,   \* the build position: items up to it are copied
  built,    \* the new World's content
  base,     \* the old content the new World reflects
  cand,     \* the head proposed to the Authority: its base seqno and root
  swap,     \* the swap's two roots: the old head and the new root
  held,     \* the named hold on the old roots
  file      \* the state file

vars == <<head, history, commits, crashes, faulty, phase, turn, pin, cursor,
          built, base, cand, swap, held, file>>

NoFile == [stage |-> "none", pin |-> {}, cursor |-> 0, built |-> {},
           base |-> {}, swap |-> NoSwap]

Init ==
  /\ head = [fmt |-> "old", items |-> {1}, seq |-> 1]
  /\ history = {1}
  /\ commits = 0
  /\ crashes = 0
  /\ faulty = FALSE
  /\ phase = "idle"
  /\ turn = FALSE
  /\ pin = {}
  /\ cursor = 0
  /\ built = {}
  /\ base = {}
  /\ cand = NoCand
  /\ swap = NoSwap
  /\ held = FALSE
  /\ file = NoFile

\* A writer commits a change to one item, in the head's format. Local writers
\* wait for the regenerator's turn; the turn lasts one bounded last diff.
Commit(w, i) ==
  /\ commits < MaxCommits
  /\ w = "local" => ~turn
  /\ head' = [head EXCEPT !.items = Toggle(@, i), !.seq = @ + 1]
  /\ history' = Toggle(history, i)
  /\ commits' = commits + 1
  /\ UNCHANGED <<crashes, faulty, phase, turn, pin, cursor, built, base, cand,
                 swap, held, file>>

\* Pin holds the head's root by name and records it before the build reads it.
Pin ==
  /\ phase = "idle"
  /\ head.fmt = "old"
  /\ pin' = head.items
  /\ base' = head.items
  /\ held' = TRUE
  /\ phase' = "pinned"
  /\ file' = [NoFile EXCEPT !.stage = "building", !.pin = head.items,
                            !.base = head.items]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, turn, cursor, built,
                 cand, swap>>

\* Build copies the pinned root's next item into the new World. It reads the
\* pinned root, never the head, so commits since the pin do not reach it.
Build ==
  /\ phase \in {"pinned", "building"}
  /\ cursor < N
  /\ cursor' = cursor + 1
  /\ built' = IF cursor + 1 \in pin THEN built \cup {cursor + 1} ELSE built
  /\ phase' = "building"
  /\ UNCHANGED <<head, history, commits, crashes, faulty, turn, pin, base,
                 cand, swap, held, file>>

\* BuildSkip is a defect in the new format's writer: the item is lost.
BuildSkip ==
  /\ Faults
  /\ phase \in {"pinned", "building"}
  /\ cursor < N
  /\ cursor + 1 \in pin
  /\ cursor' = cursor + 1
  /\ faulty' = TRUE
  /\ phase' = "building"
  /\ UNCHANGED <<head, history, commits, crashes, turn, pin, built, base, cand,
                 swap, held, file>>

\* SavePosition records the position with the pages written before it.
SavePosition ==
  /\ phase = "building"
  /\ file' = [file EXCEPT !.cursor = cursor, !.built = built]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, phase, turn, pin,
                 cursor, built, base, cand, swap, held>>

\* BuildDone ends the build: the new World reflects the pinned root.
BuildDone ==
  /\ phase = "building"
  /\ cursor = N
  /\ phase' = "catchingUp"
  /\ file' = [file EXCEPT !.stage = "catchingUp", !.cursor = N, !.built = built]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, turn, pin, cursor,
                 built, base, cand, swap, held>>

\* CatchUp applies the root diff from the content the new World reflects to
\* the head, off the writer turn, and records the new World with that content.
CatchUp ==
  /\ phase = "catchingUp"
  /\ head.fmt = "old"
  /\ base # head.items
  /\ Clean(built, base, head.items)
  /\ built' = Apply(built, base, head.items)
  /\ base' = head.items
  /\ file' = [file EXCEPT !.built = Apply(built, base, head.items),
                          !.base = head.items]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, phase, turn, pin,
                 cursor, cand, swap, held>>

\* SwapBegin takes the local write turn, applies the last diff, and records
\* the swap's two roots in the state file before it proposes the new one.
SwapBegin ==
  /\ phase = "catchingUp"
  /\ head.fmt = "old"
  /\ ~turn
  /\ Clean(built, base, head.items)
  /\ LET new == [fmt |-> "new", items |-> Apply(built, base, head.items)]
         s == [old |-> [fmt |-> "old", items |-> head.items], new |-> new]
     IN /\ cand' = [base |-> head.seq, root |-> new]
        /\ swap' = s
        /\ file' = [file EXCEPT !.swap = s]
  /\ turn' = TRUE
  /\ phase' = "swapping"
  /\ UNCHANGED <<head, history, commits, crashes, faulty, pin, cursor, built,
                 base, held>>

\* SwapDecide is the Authority's decision: it installs the new root only on
\* the head the candidate was built from. Either way the turn ends.
SwapDecide ==
  /\ phase = "swapping"
  /\ IF head.seq = cand.base
       THEN /\ head' = [fmt |-> "new", items |-> cand.root.items,
                        seq |-> head.seq + 1]
            /\ phase' = "switched"
            /\ UNCHANGED swap
       ELSE /\ phase' = "catchingUp"
            /\ swap' = NoSwap
            /\ UNCHANGED head
  /\ cand' = NoCand
  /\ turn' = FALSE
  /\ UNCHANGED <<history, commits, crashes, faulty, pin, cursor, built, base,
                 held, file>>

\* RecordSwitch writes the switch to the state file after the decision.
RecordSwitch ==
  /\ phase = "switched"
  /\ file.stage = "catchingUp"
  /\ file' = [file EXCEPT !.stage = "switched"]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, phase, turn, pin,
                 cursor, built, base, cand, swap, held>>

\* Verify streams the swap's old and new roots side by side; only equal roots
\* release the hold. Unequal roots leave the switch back as the next step.
Verify ==
  /\ phase = "switched"
  /\ file.stage = "switched"
  /\ swap.old.items = swap.new.items
  /\ held' = FALSE
  /\ phase' = "verified"
  /\ file' = [file EXCEPT !.stage = "verified"]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, turn, pin, cursor,
                 built, base, cand, swap>>

\* BackCatchUp is a switch back round, off the writer turn: it applies the
\* diff from the new root the old one reflects to the head onto the old root,
\* and records the head as the content the old root now reflects, both in one
\* write of the state file. The pair stays equal in content unless the build
\* was faulty, so Verify can still prove it.
BackCatchUp ==
  /\ phase = "switched"
  /\ file.stage = "switched"
  /\ head.fmt = "new"
  /\ swap.new.items # head.items
  /\ Clean(swap.old.items, swap.new.items, head.items)
  /\ LET s == [old |-> [fmt |-> "old",
                        items |-> Apply(swap.old.items, swap.new.items,
                                        head.items)],
               new |-> [fmt |-> "new", items |-> head.items]]
     IN /\ swap' = s
        /\ file' = [file EXCEPT !.swap = s]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, phase, turn, pin,
                 cursor, built, base, cand, held>>

\* BackBegin starts a switch back, on a failed comparison or the runtime
\* switch: it takes the turn, applies the last diff since the last round to
\* the old root, and records that it is switching back before it proposes.
BackBegin ==
  /\ phase = "switched"
  /\ file.stage = "switched"
  /\ ~turn
  /\ head.fmt = "new"
  /\ Clean(swap.old.items, swap.new.items, head.items)
  /\ cand' = [base |-> head.seq,
              root |-> [fmt |-> "old",
                        items |-> Apply(swap.old.items, swap.new.items,
                                        head.items)]]
  /\ file' = [file EXCEPT !.stage = "switchingBack"]
  /\ turn' = TRUE
  /\ phase' = "switchingBack"
  /\ UNCHANGED <<head, history, commits, crashes, faulty, pin, cursor, built,
                 base, swap, held>>

\* Diverge stops on a diff that does not apply strictly, forward or back. Both
\* roots stay held and the stop is reported; nothing is installed.
Diverge ==
  /\ ~turn
  /\ \/ phase = "catchingUp" /\ head.fmt = "old"
        /\ ~Clean(built, base, head.items)
     \/ phase = "switched" /\ file.stage = "switched" /\ head.fmt = "new"
        /\ ~Clean(swap.old.items, swap.new.items, head.items)
  /\ phase' = "stopped"
  /\ file' = [file EXCEPT !.stage = "stopped"]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, turn, pin, cursor,
                 built, base, cand, swap, held>>

\* BackDecide is the Authority's decision on the switch back. A refused one
\* returns to switched, where the next switch back reads the head again.
BackDecide ==
  /\ phase = "switchingBack"
  /\ IF head.seq = cand.base
       THEN /\ head' = [fmt |-> "old", items |-> cand.root.items,
                        seq |-> head.seq + 1]
            /\ phase' = "switchedBack"
            /\ file' = [file EXCEPT !.stage = "switchedBack"]
       ELSE /\ phase' = "switched"
            /\ file' = [file EXCEPT !.stage = "switched"]
            /\ UNCHANGED head
  /\ cand' = NoCand
  /\ turn' = FALSE
  /\ UNCHANGED <<history, commits, crashes, faulty, pin, cursor, built, base,
                 swap, held>>

\* Crash loses the regenerator's memory and the turn.
Crash ==
  /\ crashes < MaxCrashes
  /\ phase \notin {"down"} \cup Terminal
  /\ crashes' = crashes + 1
  /\ phase' = "down"
  /\ turn' = FALSE
  /\ pin' = {}
  /\ cursor' = 0
  /\ built' = {}
  /\ base' = {}
  /\ cand' = NoCand
  /\ swap' = NoSwap
  /\ UNCHANGED <<head, history, commits, faulty, held, file>>

\* Recover resumes from the state file, taking whether a swap or a switch back
\* took from the head's format: a crash can fall between the Authority's
\* decision and the file's record of it.
Recover ==
  /\ phase = "down"
  /\ pin' = file.pin
  /\ cursor' = file.cursor
  /\ built' = file.built
  /\ base' = file.base
  /\ CASE file.stage = "none" ->
            /\ phase' = "idle" /\ swap' = NoSwap /\ UNCHANGED file
       [] file.stage = "building" ->
            /\ phase' = "building" /\ swap' = NoSwap /\ UNCHANGED file
       [] file.stage \in {"catchingUp", "switched"} /\ head.fmt = "new" ->
            /\ phase' = "switched" /\ swap' = file.swap
            /\ file' = [file EXCEPT !.stage = "switched"]
       [] file.stage = "catchingUp" ->
            /\ phase' = "catchingUp" /\ swap' = NoSwap
            /\ file' = [file EXCEPT !.swap = NoSwap]
       [] file.stage = "switchingBack" /\ head.fmt = "old" ->
            /\ phase' = "switchedBack" /\ swap' = file.swap
            /\ file' = [file EXCEPT !.stage = "switchedBack"]
       [] file.stage = "switchingBack" ->
            /\ phase' = "switched" /\ swap' = file.swap
            /\ file' = [file EXCEPT !.stage = "switched"]
  /\ UNCHANGED <<head, history, commits, crashes, faulty, turn, cand, held>>

\* Done ends a finished regeneration once the writers are done.
Done ==
  /\ phase \in Terminal
  /\ commits = MaxCommits
  /\ UNCHANGED vars

Regen ==
  Pin \/ Build \/ SavePosition \/ BuildDone \/ CatchUp \/ SwapBegin
  \/ SwapDecide \/ RecordSwitch \/ Verify \/ BackCatchUp \/ BackBegin
  \/ BackDecide \/ Diverge \/ Recover

Next ==
  \/ \E w \in Writers, i \in Items : Commit(w, i)
  \/ Regen \/ BuildSkip \/ Crash \/ Done

Spec == Init /\ [][Next]_vars /\ WF_vars(Regen)

\* The head holds exactly what the acknowledged commits define, through every
\* build, catch-up, swap, crash and switch back, unless the build was faulty.
HeadMatchesHistory == ~faulty => head.items = history

\* The new World reflects exactly the old content it records, whatever crashes
\* fell during the build: a function of the pinned root and the diffs applied,
\* so every replica building from the same point reaches the same content.
BuiltMatchesBase == phase \in {"catchingUp", "swapping"} /\ ~faulty =>
  built = base

\* The old roots stay held from the pin until a comparison proves the swap.
HeldUntilVerified == file.stage \notin {"none", "verified"} => held

\* The hold goes only when the swap's roots were proven equal.
ReleasedOnlyEqual == ~held => file.swap.old.items = file.swap.new.items

\* Once switched, the regenerator knows the old-format root to return to.
SwitchedKnowsOld == phase \in {"switched", "switchingBack"} =>
  swap.old.fmt = "old"

\* Only a faulty build makes a diff fail to apply strictly.
StopsOnlyOnFault == phase = "stopped" => faulty

\* Regeneration ends, verified, switched back or stopped.
Finishes == <>(phase \in Terminal)

====
