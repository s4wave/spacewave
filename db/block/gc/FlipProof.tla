---- MODULE FlipProof ----
\* The flip's proof, read in a snapshot and confirmed by point reads.
\*
\* The flip ends the Open generation: it may commit only while the front
\* holds no entry, no hole is recorded and the free queue holds no ripe
\* block. A seek of a kind's start steps over every key deleted there since
\* the store's last merge, so the flip's transaction, which holds the
\* writer, seeks none of the front, the holes or the queue. The collector
\* instead reads them in a read snapshot (settle.go proveFlip) and the
\* flip's transaction confirms the snapshot still holds with point reads
\* (settle.go flip):
\*
\*  - the add revision, which every writer of a front or hole key raises in
\*    the same transaction (Records.addFront, Records.addHole), is the one
\*    the snapshot read;
\*  - the holes-clear mark, which a hole's writer deletes and only the
\*    collector writes (clearHoles), once a snapshot found no hole and the
\*    revision had not moved since, is still present;
\*  - the adoption's mark time is the one the snapshot read, since an
\*    adoption is the one writer besides the collector that adds queue
\*    entries, and a Land only deletes them, which never makes the queue
\*    riper; so the snapshot's finding of no ripe queued block holds.
\*
\* Writers run between any two collector steps. The collector runs one step
\* at a time: nothing of its own runs between its snapshot and its flip.
\* Every queue entry is modeled ripe, the worst case for the pop mark.
\*
\* Skip names one writer that writes its key without the revision, and,
\* for a hole, without deleting the mark; Stamp is whether an adoption
\* stamps a new mark time. Each mutant in testdata breaks NoUnsafeFlip.

EXTENDS Naturals, FiniteSets

CONSTANTS
  Keys,       \* block keys
  Positions,  \* queue positions, in key order
  MaxRev,     \* bound on the add revision
  MaxAdopt,   \* bound on the adoptions
  Skip,       \* "none", "land", "copied", "adopt" or "hole"
  Stamp       \* whether an adoption stamps a new mark time

VARIABLES
  front,      \* keys on the front
  holes,      \* recorded holes
  clear,      \* whether the holes-clear mark is present
  rev,        \* the add revision
  queue,      \* positions of queued blocks
  adoptAt,    \* the adoption's mark time
  pc,         \* "idle", "clearing" or "proved"
  seen,       \* the revision the collector's last snapshot read
  ripe,       \* whether the proof's snapshot found a ripe queued block
  seenAdopt,  \* the mark time the proof read
  unsafe      \* a flip committed while the front, holes or queue held one

vars == <<front, holes, clear, rev, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

TypeOK ==
  /\ front \subseteq Keys
  /\ holes \subseteq Keys
  /\ clear \in BOOLEAN
  /\ rev \in 0..MaxRev
  /\ queue \subseteq Positions
  /\ adoptAt \in 0..MaxAdopt
  /\ pc \in {"idle", "clearing", "proved"}
  /\ seen \in 0..MaxRev
  /\ ripe \in BOOLEAN
  /\ seenAdopt \in 0..MaxAdopt
  /\ unsafe \in BOOLEAN

Init ==
  /\ front = {} /\ holes = {} /\ clear = TRUE /\ rev = 0
  /\ queue = {} /\ adoptAt = 0
  /\ pc = "idle" /\ seen = 0 /\ ripe = FALSE /\ seenAdopt = 0
  /\ unsafe = FALSE

\* Raise the revision unless the writer w skips it.
Bump(w) == rev' = IF Skip = w THEN rev ELSE rev + 1

CanBump == rev < MaxRev

\* Writers.

\* A Land pushes a block it holds onto the front.
LandPush(k) ==
  /\ CanBump
  /\ front' = front \cup {k}
  /\ Bump("land")
  /\ UNCHANGED <<holes, clear, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* A copied put stores an unverified list and pushes its block.
CopiedPut(k) ==
  /\ CanBump
  /\ front' = front \cup {k}
  /\ Bump("copied")
  /\ UNCHANGED <<holes, clear, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* An adoption's recount returns a block to the front.
AdoptPush(k) ==
  /\ CanBump
  /\ front' = front \cup {k}
  /\ Bump("adopt")
  /\ UNCHANGED <<holes, clear, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* A Land or an audit records a hole, deleting the holes-clear mark.
RecordHole(k) ==
  /\ CanBump
  /\ holes' = holes \cup {k}
  /\ clear' = (Skip = "hole" /\ clear)
  /\ Bump("hole")
  /\ UNCHANGED <<front, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* A put fills a hole.
FillHole(k) ==
  /\ k \in holes
  /\ holes' = holes \ {k}
  /\ UNCHANGED <<front, clear, rev, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* A Land stores a queued block again, deleting its queue entry.
LandDequeue(p) ==
  /\ p \in queue
  /\ queue' = queue \ {p}
  /\ UNCHANGED <<front, holes, clear, rev, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* An adoption settles a block at zero onto the queue, at any position, and
\* stamps its mark time.
AdoptQueue(p) ==
  /\ adoptAt < MaxAdopt
  /\ queue' = queue \cup {p}
  /\ adoptAt' = IF Stamp THEN adoptAt + 1 ELSE adoptAt
  /\ UNCHANGED <<front, holes, clear, rev, pc, seen, ripe, seenAdopt, unsafe>>

\* The collector, one step at a time.

\* The walk pops a front entry, and the free frees a queued block.
Walk(k) ==
  /\ pc = "idle" /\ k \in front
  /\ front' = front \ {k}
  /\ UNCHANGED <<holes, clear, rev, queue, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

Free(p) ==
  /\ pc = "idle" /\ p \in queue
  /\ queue' = queue \ {p}
  /\ UNCHANGED <<front, holes, clear, rev, adoptAt, pc, seen, ripe, seenAdopt, unsafe>>

\* clearHoles: a snapshot finds no hole and no mark and reads the
\* revision; the transaction writes the mark when the revision stands.
ClearRead ==
  /\ pc = "idle" /\ ~clear /\ holes = {}
  /\ pc' = "clearing" /\ seen' = rev
  /\ UNCHANGED <<front, holes, clear, rev, queue, adoptAt, ripe, seenAdopt, unsafe>>

ClearWrite ==
  /\ pc = "clearing"
  /\ clear' = (clear \/ rev = seen)
  /\ pc' = "idle"
  /\ UNCHANGED <<front, holes, rev, queue, adoptAt, seen, ripe, seenAdopt, unsafe>>

\* proveFlip: the snapshot finds the front empty and the mark present, and
\* reads the revision, the mark time and whether a ripe block is queued.
Prove ==
  /\ pc = "idle" /\ front = {} /\ clear
  /\ pc' = "proved" /\ seen' = rev /\ seenAdopt' = adoptAt
  /\ ripe' = (queue # {})
  /\ UNCHANGED <<front, holes, clear, rev, queue, adoptAt, unsafe>>

\* flip: point reads confirm the proof, which found no ripe queued block.
\* It records whether the flip it commits was unsafe.
Flip ==
  /\ pc = "proved"
  /\ pc' = "idle"
  /\ IF clear /\ rev = seen /\ adoptAt = seenAdopt /\ ~ripe
     THEN unsafe' = (unsafe \/ front # {} \/ holes # {} \/ queue # {})
     ELSE UNCHANGED unsafe
  /\ UNCHANGED <<front, holes, clear, rev, queue, adoptAt, seen, ripe, seenAdopt>>

Next ==
  \/ \E k \in Keys :
       LandPush(k) \/ CopiedPut(k) \/ AdoptPush(k) \/ RecordHole(k)
       \/ FillHole(k) \/ Walk(k)
  \/ \E p \in Positions : LandDequeue(p) \/ AdoptQueue(p) \/ Free(p)
  \/ ClearRead \/ ClearWrite \/ Prove \/ Flip

Spec == Init /\ [][Next]_vars

\* The mark is present only while no hole is recorded.
ClearSound == clear => holes = {}

\* No flip commits while the front holds an entry, a hole is recorded, or
\* a ripe block is queued.
NoUnsafeFlip == ~unsafe

====
