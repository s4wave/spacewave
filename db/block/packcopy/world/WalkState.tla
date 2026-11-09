---------------------------- MODULE WalkState ----------------------------
(* A resumable walk reads every block reachable from a pinned World root. It
   keeps a frontier of blocks to read: queued blocks, and blocks whose read
   runs. A read stays running until it has queued each child the walk has not
   reached in this run, then the block is done.

   A save copies the frontier and the done set under the walk's lock, writes
   the copy beside the state file, syncs it, and renames it into place. A
   crash may come at any step: the copy and a half-written file are lost, and
   the state file keeps the last renamed position. A resume loads the state
   file into memory with an empty reached set, so it may read some blocks
   again, and goes on.

   An operator's cancel drains the reads, queues the running ones again and
   saves; with the discard flag it removes the state file instead. A restart
   never removes it. A walk that finishes replaces the frontier with a done
   record.

   Writes after the pin add blocks reachable from a later root, some shared
   with the pinned graph. Graph edges never change: a block's children are in
   its bytes.

   Abstraction: blocks are names and the graph is a constant; the stacks are a
   set, since order does not matter for coverage; types, retries, holes and
   keeps are left out; the save's copy is one step and its write with the sync
   another, which the rename follows. Bounds: a seven-block graph and
   MaxStops crashes or cancels; a discard is not bounded, as it ends the
   walk. *)
EXTENDS FiniteSets, Naturals

CONSTANTS MaxStops

\* The graph: the pinned root r reaches a, b, c and d; the later root l,
\* written after the pin, shares c and reaches e. a and b share c, so a run
\* reaches c twice.
Blocks == {"r", "a", "b", "c", "d", "l", "e"}
Root == "r"
Later == "l"
Kids == [x \in Blocks |->
            CASE x = "r" -> {"a", "b"}
              [] x = "a" -> {"c"}
              [] x = "b" -> {"c", "d"}
              [] x = "l" -> {"c", "e"}
              [] OTHER -> {}]

\* Reached returns the blocks reachable from s through Kids, s included.
RECURSIVE ReachFrom(_, _)
ReachFrom(s, n) ==
    IF n = 0 THEN s
    ELSE ReachFrom(s \cup UNION {Kids[b] : b \in s}, n - 1)
Reached(s) == ReachFrom(s, Cardinality(Blocks))

\* Pinned is every block reachable from the pinned root.
Pinned == Reached({Root})

VARIABLES
    status,     \* "idle", "running", "stopped", "done" or "discarded"
    front,      \* queued blocks
    flying,     \* blocks whose read runs
    seen,       \* blocks this run reached
    done,       \* blocks read with their children queued, as this run knows
    copy,       \* the save's copy: {} or one position
    tmp,        \* the file beside the state file: {} or one position
    file,       \* the state file: "none", "pos" or "done"
    pos,        \* the position the state file holds when file = "pos"
    reads,      \* every block any run read
    stops,      \* crashes and cancels so far
    discarded   \* the operator asked to discard

vars == <<status, front, flying, seen, done, copy, tmp, file, pos, reads, stops, discarded>>

Pos == [front : SUBSET Blocks, done : SUBSET Blocks]

TypeOK ==
    /\ status \in {"idle", "running", "stopped", "done", "discarded"}
    /\ front \subseteq Blocks /\ flying \subseteq Blocks
    /\ seen \subseteq Blocks /\ done \subseteq Blocks
    /\ copy \subseteq Pos /\ Cardinality(copy) <= 1
    /\ tmp \subseteq Pos /\ Cardinality(tmp) <= 1
    /\ file \in {"none", "pos", "done"}
    /\ pos \in Pos
    /\ reads \subseteq Blocks
    /\ stops \in 0..MaxStops
    /\ discarded \in BOOLEAN

Init ==
    /\ status = "idle"
    /\ front = {} /\ flying = {} /\ seen = {} /\ done = {}
    /\ copy = {} /\ tmp = {} /\ file = "none"
    /\ pos = [front |-> {}, done |-> {}]
    /\ reads = {} /\ stops = 0 /\ discarded = FALSE

\* A new walk queues the pinned root and saves before it reads.
Start ==
    /\ status = "idle"
    /\ status' = "running"
    /\ front' = {Root} /\ seen' = {Root}
    /\ file' = "pos" /\ pos' = [front |-> {Root}, done |-> {}]
    /\ UNCHANGED <<flying, done, copy, tmp, reads, stops, discarded>>

\* A worker pops a queued block and reads it.
Pop(b) ==
    /\ status = "running"
    /\ b \in front
    /\ front' = front \ {b}
    /\ flying' = flying \cup {b}
    /\ reads' = reads \cup {b}
    /\ UNCHANGED <<status, seen, done, copy, tmp, file, pos, stops, discarded>>

\* The read ends: its children not reached in this run are queued, then it
\* leaves the frontier.
Finish(b) ==
    /\ status = "running"
    /\ b \in flying
    /\ front' = front \cup (Kids[b] \ seen)
    /\ seen' = seen \cup Kids[b]
    /\ flying' = flying \ {b}
    /\ done' = done \cup {b}
    /\ UNCHANGED <<status, copy, tmp, file, pos, reads, stops, discarded>>

\* A save copies the frontier, running reads included, under the lock. One
\* save runs at a time.
Copy ==
    /\ status = "running"
    /\ copy = {} /\ tmp = {}
    /\ copy' = {[front |-> front \cup flying, done |-> done]}
    /\ UNCHANGED <<status, front, flying, seen, done, tmp, file, pos, reads, stops, discarded>>

\* It writes and syncs the copy beside the state file.
Write ==
    /\ status = "running"
    /\ copy # {}
    /\ tmp' = copy
    /\ copy' = {}
    /\ UNCHANGED <<status, front, flying, seen, done, file, pos, reads, stops, discarded>>

\* It renames the synced file into place.
Rename ==
    /\ tmp # {}
    /\ file' = "pos"
    /\ pos' = CHOOSE p \in tmp : TRUE
    /\ tmp' = {}
    /\ UNCHANGED <<status, front, flying, seen, done, copy, reads, stops, discarded>>

\* A crash loses memory, the copy and the unrenamed file; the state file
\* stays.
Crash ==
    /\ status = "running"
    /\ stops < MaxStops
    /\ status' = "stopped"
    /\ front' = {} /\ flying' = {} /\ seen' = {} /\ done' = {}
    /\ copy' = {} /\ tmp' = {}
    /\ stops' = stops + 1
    /\ UNCHANGED <<file, pos, reads, discarded>>

\* A cancel drains the reads, queues the running ones again and saves under
\* the save's lock, or removes the state file when the operator discards.
Cancel(discard) ==
    /\ status = "running"
    /\ stops < MaxStops \/ discard
    /\ copy = {} /\ tmp = {}
    /\ status' = IF discard THEN "discarded" ELSE "stopped"
    /\ file' = IF discard THEN "none" ELSE "pos"
    /\ pos' = IF discard THEN pos ELSE [front |-> front \cup flying, done |-> done]
    /\ front' = {} /\ flying' = {} /\ seen' = {} /\ done' = {}
    /\ stops' = IF discard THEN stops ELSE stops + 1
    /\ discarded' = discard
    /\ UNCHANGED <<copy, tmp, reads>>

\* A start after a restart, or an operator's resume, loads the state file.
Resume ==
    /\ status = "stopped"
    /\ file = "pos"
    /\ status' = "running"
    /\ front' = pos.front /\ seen' = pos.front /\ done' = pos.done
    /\ UNCHANGED <<flying, copy, tmp, file, pos, reads, stops, discarded>>

\* The walk ends when nothing is queued or running, and records the end
\* under the save's lock.
End ==
    /\ status = "running"
    /\ front = {} /\ flying = {}
    /\ copy = {} /\ tmp = {}
    /\ status' = "done"
    /\ file' = "done"
    /\ UNCHANGED <<front, flying, seen, done, copy, tmp, pos, reads, stops, discarded>>

\* A finished or discarded walk stays so.
Rest ==
    /\ status \in {"done", "discarded"}
    /\ UNCHANGED vars

Next ==
    \/ Start
    \/ \E b \in Blocks : Pop(b) \/ Finish(b)
    \/ Copy \/ Write \/ Rename
    \/ Crash
    \/ \E d \in BOOLEAN : Cancel(d)
    \/ Resume
    \/ End
    \/ Rest

Spec == Init /\ [][Next]_vars

\* The save loop stops when the walk drains, so the end is strongly fair: a
\* save between the drain and the end does not hold the end off forever.
FairSpec ==
    /\ Spec
    /\ WF_vars(Start)
    /\ \A b \in Blocks : WF_vars(Pop(b)) /\ WF_vars(Finish(b))
    /\ WF_vars(Write) /\ WF_vars(Rename)
    /\ WF_vars(Resume) /\ SF_vars(End)

-----------------------------------------------------------------------------
\* Every pinned block is done or reachable from the frontier in memory.
MemCovers ==
    status = "running" => Pinned \subseteq done \cup Reached(front \cup flying)

\* So it is for the state file, whatever crashed.
FileCovers ==
    file = "pos" => Pinned \subseteq pos.done \cup Reached(pos.front)

\* A done block was read.
DoneRead ==
    /\ done \subseteq reads
    /\ file = "pos" => pos.done \subseteq reads

\* No block written after the pin is read unless the pin reaches it.
ReadsPinned == reads \subseteq Pinned

\* A finished walk read every pinned block.
EndComplete == file = "done" => Pinned \subseteq reads

\* Only a discard removes a started walk's state file.
StateKept == (status # "idle" /\ file = "none") => discarded

\* The walk ends or is discarded.
Ends == <>(status \in {"done", "discarded"})
=============================================================================
