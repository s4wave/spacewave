---- MODULE FlipProof ----
\* The collector proves the flip in a read snapshot (settle.go proveFlip)
\* and confirms it with point reads (flip). The front and holes must be
\* empty. A queue entry holds the flip only when a pop mark exists and the
\* entry is at or below the lesser of the pop mark and up cursor.
\*
\* The collector serializes its steps. A Land, the audit, or an adoption
\* can write between the snapshot and its confirmation. A fresh adoption
\* stamps a fresh mark time, and neither proof nor flip can run while the
\* Volume is unadopted or adopting. Grace, recount debt and run completion
\* are assumed satisfied whenever adopted. Each modeled transaction adds
\* at most one front or hole key, so adds counts the transactions that
\* Records.addFront and Records.addHole must stamp with the add revision.

EXTENDS Integers, FiniteSets

CONSTANTS Keys, MaxUp, MaxRev, MaxAdopt, Skip
VARIABLE s

NoPop == -1
MaxPop == MaxUp + 1
Positions == 0..MaxUp
MinBound(mark, cursor) == IF mark < cursor THEN mark ELSE cursor
PopBound(mark, cursor) == IF mark = NoPop THEN NoPop ELSE MinBound(mark, cursor)
Ripe(queue, bound) == (queue \cap (0..bound)) # {}
GroundProof == [rev |-> 0, adoptAt |-> 0, upTo |-> NoPop, ripe |-> FALSE]

TypeOK ==
  /\ s.front \subseteq Keys
  /\ s.holes \subseteq Keys
  /\ s.clear \in BOOLEAN
  /\ s.rev \in 0..MaxRev
  /\ s.adds \in 0..MaxRev
  /\ s.queue \subseteq Positions
  /\ s.parked \subseteq Positions
  /\ s.mark \in NoPop..MaxPop
  /\ s.upSeq \in Positions
  /\ s.upCursor \in 0..s.upSeq
  /\ s.adoptAt \in 0..MaxAdopt
  /\ s.adoption \in {"adopted", "claimed", "cleared", "adopting"}
  /\ s.pc \in {"idle", "clearing", "proved", "flipped"}
  /\ s.seen \in 0..MaxRev
  /\ s.proof \in [rev : 0..MaxRev, adoptAt : 0..MaxAdopt,
                   upTo : NoPop..MaxUp, ripe : BOOLEAN]
  /\ s.unsafe \in BOOLEAN

Init ==
  s = [front |-> {}, holes |-> {}, clear |-> FALSE, rev |-> 0, adds |-> 0,
       queue |-> {}, parked |-> {}, mark |-> NoPop, upSeq |-> 0, upCursor |-> 0,
       adoptAt |-> 0, adoption |-> "adopted", pc |-> "idle", seen |-> 0,
       proof |-> GroundProof, unsafe |-> FALSE]

\* A Land or copied put pushes a block, or an adoption's count holds an
\* unverified list and pushes it (adopt.go hold).
Push(k, writer) ==
  /\ s.adds < MaxRev
  /\ writer = "adopt-rev" => s.adoption = "adopting"
  /\ s' = [s EXCEPT !.front = @ \cup {k}, !.adds = @ + 1,
                   !.rev = IF Skip = writer THEN @ ELSE @ + 1]

\* The audit records a hole, raising the revision and deleting the mark.
RecordHole(k) ==
  /\ s.adds < MaxRev
  /\ s' = [s EXCEPT !.holes = @ \cup {k}, !.adds = @ + 1,
                   !.rev = IF Skip = "hole-rev" THEN @ ELSE @ + 1,
                   !.clear = Skip = "hole-clear" /\ @]

\* A put fills a hole. A Land can delete a queue entry or append an up.
FillHole(k) ==
  /\ k \in s.holes
  /\ s' = [s EXCEPT !.holes = @ \ {k}]

LandDequeue(p) ==
  /\ p \in s.queue
  /\ s' = [s EXCEPT !.queue = @ \ {p}]

LandAppend ==
  /\ s.upSeq < MaxUp
  /\ s' = [s EXCEPT !.upSeq = @ + 1]

\* An external writer returns the Volume to unadopted and claims a recount.
AdoptClaim ==
  /\ s.adoption = "adopted" /\ s.adoptAt < MaxAdopt
  /\ s' = [s EXCEPT !.adoption = "claimed"]

\* clearRun raises an existing pop mark before clearing old queue entries.
AdoptClear ==
  /\ s.adoption = "claimed"
  /\ s' = [s EXCEPT !.adoption = "cleared", !.queue = {}, !.parked = {},
                   !.mark = IF @ = NoPop THEN @ ELSE MaxPop]

\* markAdopting starts fresh cursors at the logs' ends and stamps the mark.
AdoptMark ==
  /\ s.adoption = "cleared"
  /\ s' = [s EXCEPT !.adoption = "adopting", !.upCursor = s.upSeq,
                   !.adoptAt = IF Skip = "stamp" THEN @ ELSE @ + 1]

\* The recount settles a block at zero, ripe at the up log's end.
AdoptQueue ==
  /\ s.adoption = "adopting"
  /\ s' = [s EXCEPT !.queue = @ \cup {s.upSeq}]

\* The recount caps the pop mark at the current up log's end as it finishes.
AdoptFinish ==
  /\ s.adoption = "adopting"
  /\ s' = [s EXCEPT !.adoption = "adopted", !.mark = MinBound(@, s.upSeq)]

\* Netting advances the up cursor and can enqueue. Freeing or parking a
\* parent can enqueue its children through the same netting writer.
Net(to, enqueue) ==
  /\ s.pc = "idle" /\ s.adoption = "adopted"
  /\ to \in s.upCursor..s.upSeq
  /\ s' = [s EXCEPT !.upCursor = to,
                   !.queue = IF enqueue THEN @ \cup {s.upSeq} ELSE @]

Park(p, enqueue) ==
  /\ s.pc = "idle" /\ s.adoption = "adopted"
  /\ p \in s.queue /\ p <= s.upCursor
  /\ s' = [s EXCEPT !.parked = @ \cup {p},
                   !.queue = (@ \ {p}) \cup (IF enqueue THEN {s.upSeq} ELSE {})]

\* The collector's third enqueue path moves a parked block after the flip.
Requeue(p) ==
  /\ s.pc = "flipped" /\ p \in s.parked
  /\ s' = [s EXCEPT !.queue = @ \cup {p}, !.parked = @ \ {p}]

\* The collector pops a proved front entry and raises the pop mark.
Pop(k, p) ==
  /\ s.pc = "idle" /\ s.adoption = "adopted"
  /\ k \in s.front /\ p <= s.upCursor
  /\ s' = [s EXCEPT !.front = @ \ {k}, !.mark = IF p > @ THEN p ELSE @]

\* clearHoles finds the absent mark and an empty hole range, then writes
\* the mark if the revision is still the one read in that snapshot.
ClearRead ==
  /\ s.pc = "idle" /\ ~s.clear /\ s.holes = {}
  /\ s' = [s EXCEPT !.pc = "clearing", !.seen = s.rev]

ClearWrite ==
  /\ s.pc = "clearing"
  /\ s' = [s EXCEPT !.pc = "idle", !.seen = 0, !.clear = @ \/ s.rev = s.seen]

\* proveFlip reads the bound, with NoPop representing an absent pop mark.
Prove ==
  /\ s.pc = "idle" /\ s.adoption = "adopted" /\ s.front = {} /\ s.clear
  /\ LET upTo == PopBound(s.mark, s.upCursor)
     IN s' = [s EXCEPT !.pc = "proved",
                       !.proof = [rev |-> s.rev, adoptAt |-> s.adoptAt,
                                  upTo |-> upTo, ripe |-> Ripe(s.queue, upTo)]]

\* flip rechecks the records and deletes the pop mark on success. Record
\* whether the generation changed while a condition for the flip was false.
\* The flip-clear mutant survives: deleting the mark also raises rev.
\* The flip-pop mutant survives: only the idle collector can change the
\* bound while adopted. A recount can change the bound while the Volume is
\* ineligible for the flip, and stamps before it becomes adopted again.
\* The Go flip still confirms both records.
Flip ==
  /\ s.pc = "proved"
  /\ LET upTo == PopBound(s.mark, s.upCursor)
         confirmed == s.adoption = "adopted" /\ ~s.proof.ripe
           /\ (Skip = "flip-clear" \/ s.clear)
           /\ (Skip = "flip-rev" \/ s.rev = s.proof.rev)
           /\ (Skip = "flip-adopt" \/ s.adoptAt = s.proof.adoptAt)
           /\ (Skip = "flip-pop" \/ upTo = s.proof.upTo)
     IN IF confirmed
        THEN s' = [s EXCEPT !.pc = "flipped", !.proof = GroundProof, !.mark = NoPop,
                            !.unsafe = s.front # {} \/ s.holes # {} \/ Ripe(s.queue, upTo)]
        ELSE s' = [s EXCEPT !.pc = "idle", !.proof = GroundProof]

Next ==
  \/ \E p \in Positions : Requeue(p)
  \/ /\ s.pc # "flipped"
     /\ \/ \E k \in Keys :
             \/ \E writer \in {"land-rev", "copied-rev", "adopt-rev"} : Push(k, writer)
             \/ RecordHole(k) \/ FillHole(k)
             \/ \E p \in Positions : Pop(k, p)
        \/ \E p \in Positions :
             \/ LandDequeue(p)
             \/ \E enqueue \in BOOLEAN : Net(p, enqueue) \/ Park(p, enqueue)
        \/ LandAppend \/ AdoptClaim \/ AdoptClear \/ AdoptMark \/ AdoptQueue \/ AdoptFinish
        \/ ClearRead \/ ClearWrite \/ Prove \/ Flip

Spec == Init /\ [][Next]_s

\* Each transaction adding a front or hole record advances the revision.
AddRevision == s.rev = s.adds

\* The mark is present only while no hole is recorded.
ClearSound == s.clear => s.holes = {}

\* No flip commits with a front entry, a hole or a ripe queue entry.
NoUnsafeFlip == ~s.unsafe

====
