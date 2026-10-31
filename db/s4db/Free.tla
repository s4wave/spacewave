---- MODULE Free ----
\* The chunk table's partition and the cleaner's move, as the design's
\* chunk-table.go and cleaner.go run them, across commits, reservations,
\* sealing, release and readers holding snapshots.
\*
\* Two keys are written into value chunks; each write appends an entry (the
\* key and a tag that stands for the value's content) to the hot chunk and
\* points the key's reference at it, freeing the entry it replaced. The
\* cleaner picks a victim chunk, open or not, reads its entries on a snapshot,
\* closes the victim when it is open, and moves each entry that is live in the
\* snapshot: under the writer lock it copies the entry into the cold chunk and
\* repoints the key only while the key still holds the reference it read. A
\* chunk whose live count reaches zero is sealed and pending, closing it when
\* it is still open; it is free once every reader's pin passes the sealing
\* record.
\* A job reserves a free chunk by slot; the job finishes (the chunk becomes a
\* run) or its process dies, which frees the chunk by the slot rule.
\* Allocating a free chunk clears what it held. Readers take a snapshot of the
\* references, read through them, and release.
\*
\* Callers found from source (the design, and the chunk layout branch at
\* 92804dfd4 whose cleaner this follows): Tx.Commit (Write), the cleaner's
\* choose, scan, commit of moves and seal (Scan, Close, Move, Seal), the
\* flusher's and merger's reserve and completion (Reserve, Finish, Die),
\* release (Release), NewTransaction(read) and Discard (Snap, Read, End). Not
\* modeled: durability (Durability.tla), several processes' pins beyond the
\* readers here (Pins.tla), and the log and run kinds, which the reserved and
\* finished chunks stand for.

EXTENDS Naturals, FiniteSets

Chunks == 1..3
Keys == {"k1", "k2"}
Readers == {"r1"}
MaxTag == 3
MaxVersion == 9  \* bounds the records, so the model is finite
Room == 2        \* entries a chunk holds before it is full
None == [chunk |-> 0, tag |-> 0]

VARIABLES
  kind,      \* each chunk: "free", "value", "pending", "reserved", "run"
  entries,   \* each chunk's entries: a set of [key, tag]
  refs,      \* each key's reference: [chunk, tag] or None
  latest,    \* each key's latest written tag
  live,      \* the table's live count per chunk
  hot, cold, \* the open value chunks, 0 when none
  tag,       \* the last tag issued
  version,   \* the record count, which dates pending chunks and pins
  pendAt,    \* the version that sealed each pending chunk
  owner,     \* the slot that reserved each reserved chunk, and whether it lives
  cpc,       \* the cleaner: "idle", "scanned"
  victim,    \* the cleaner's victim
  moves,     \* the entries the cleaner found live in its snapshot
  snap,      \* each reader's snapshot of refs, or None for idle
  pin        \* each reader's pin: the version it took its snapshot at
vars == <<kind, entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, cpc, victim, moves, snap, pin>>

Min(S) == CHOOSE x \in S : \A y \in S : x <= y
More == version < MaxVersion

Init ==
  /\ kind = [c \in Chunks |-> IF c <= 2 THEN "value" ELSE "free"]
  /\ entries = [c \in Chunks |-> {}]
  /\ refs = [k \in Keys |-> None]
  /\ latest = [k \in Keys |-> 0]
  /\ live = [c \in Chunks |-> 0]
  /\ hot = 1 /\ cold = 2
  /\ tag = 0 /\ version = 0
  /\ pendAt = [c \in Chunks |-> 0]
  /\ owner = [c \in Chunks |-> [slot |-> 0, alive |-> FALSE]]
  /\ cpc = "idle" /\ victim = 0 /\ moves = {}
  /\ snap = [r \in Readers |-> [k \in Keys |-> None]]
  /\ pin = [r \in Readers |-> 0]

\* Allocation: the lowest free chunk, cleared.
Lowest == Min({c \in Chunks : kind[c] = "free"})
HasFree == \E c \in Chunks : kind[c] = "free"

\* An open chunk c needs replacing: none is open, or it is full.
Spent(c) == IF c = 0 THEN TRUE ELSE Cardinality(entries[c]) >= Room

\* Append an entry for k with tag t to chunk c, freeing the entry k held.
Append(k, t, c) ==
  /\ entries' = [entries EXCEPT ![c] = @ \cup {[key |-> k, tag |-> t]}]
  /\ live' = [live EXCEPT ![c] = @ + 1, ![refs[k].chunk] = IF refs[k] = None THEN @ ELSE @ - 1]
  /\ refs' = [refs EXCEPT ![k] = [chunk |-> c, tag |-> t]]

\* A commit writes k into the hot chunk, opening one when none is open or it
\* is full.
Write(k) ==
  /\ More /\ tag < MaxTag /\ hot # 0
  /\ Cardinality(entries[hot]) < Room
  /\ tag' = tag + 1 /\ version' = version + 1
  /\ latest' = [latest EXCEPT ![k] = tag + 1]
  /\ Append(k, tag + 1, hot)
  /\ UNCHANGED <<kind, hot, cold, pendAt, owner, cpc, victim, moves, snap, pin>>

OpenHot ==
  /\ More /\ Spent(hot) /\ HasFree
  /\ hot' = Lowest
  /\ kind' = [kind EXCEPT ![Lowest] = "value"]
  /\ entries' = [entries EXCEPT ![Lowest] = {}]
  /\ version' = version + 1
  /\ UNCHANGED <<refs, latest, live, cold, tag, pendAt, owner, cpc, victim, moves, snap, pin>>

OpenCold ==
  /\ More /\ Spent(cold) /\ HasFree
  /\ cold' = Lowest
  /\ kind' = [kind EXCEPT ![Lowest] = "value"]
  /\ entries' = [entries EXCEPT ![Lowest] = {}]
  /\ version' = version + 1
  /\ UNCHANGED <<refs, latest, live, hot, tag, pendAt, owner, cpc, victim, moves, snap, pin>>

\* The value chunk c holds the file's last chunk and free chunks sit below
\* it: the cleaner's top victim, which may be open.
Top(c) ==
  /\ \A d \in Chunks : d > c => kind[d] = "free"
  /\ \E d \in Chunks : d < c /\ kind[d] = "free"

\* The cleaner picks a closed value chunk, or the top one open or not, and
\* reads its entries on a snapshot: an entry is live when its key's reference
\* is this entry.
Scan ==
  /\ cpc = "idle"
  /\ \E c \in Chunks :
       /\ kind[c] = "value"
       /\ c \notin {hot, cold} \/ Top(c)
       /\ victim' = c
       /\ moves' = {e \in entries[c] : refs[e.key] = [chunk |-> c, tag |-> e.tag]}
  /\ cpc' = "scanned"
  /\ UNCHANGED <<kind, entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, snap, pin>>

\* The first move out of an open victim closes it in the move's record, so no
\* later write lands in it; a victim that was the cold chunk leaves the moves
\* to the next cold chunk.
Close ==
  /\ More /\ cpc = "scanned" /\ victim \in {hot, cold}
  /\ hot' = IF hot = victim THEN 0 ELSE hot
  /\ cold' = IF cold = victim THEN 0 ELSE cold
  /\ version' = version + 1
  /\ UNCHANGED <<kind, entries, refs, latest, live, tag, pendAt, owner, cpc, victim, moves, snap, pin>>

\* The cleaner moves one entry out of the closed victim under the lock: the
\* key must still hold the reference read, or the move is dropped.
Move ==
  /\ More /\ cpc = "scanned" /\ moves # {} /\ victim \notin {hot, cold}
  /\ cold # 0 /\ Cardinality(entries[cold]) < Room
  /\ \E e \in moves :
       /\ moves' = moves \ {e}
       /\ IF refs[e.key] = [chunk |-> victim, tag |-> e.tag]
          THEN /\ Append(e.key, e.tag, cold)
               /\ version' = version + 1
          ELSE UNCHANGED <<entries, live, refs, version>>
  /\ UNCHANGED <<kind, latest, hot, cold, tag, pendAt, owner, cpc, victim, snap, pin>>

\* An emptied victim is sealed: pending from this record, and closed when it
\* is still open.
Seal ==
  /\ More /\ cpc = "scanned" /\ moves = {} /\ live[victim] = 0
  /\ kind' = [kind EXCEPT ![victim] = "pending"]
  /\ hot' = IF hot = victim THEN 0 ELSE hot
  /\ cold' = IF cold = victim THEN 0 ELSE cold
  /\ version' = version + 1
  /\ pendAt' = [pendAt EXCEPT ![victim] = version + 1]
  /\ cpc' = "idle"
  /\ UNCHANGED <<entries, refs, latest, live, tag, owner, victim, moves, snap, pin>>

\* A victim that still holds live entries after the moves goes back to the
\* table: a key was rewritten meanwhile and the cleaner tries again later.
Abandon ==
  /\ cpc = "scanned" /\ moves = {} /\ live[victim] # 0
  /\ cpc' = "idle"
  /\ UNCHANGED <<kind, entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, victim, moves, snap, pin>>

\* A pending chunk is free once no reader pins a version before its sealing.
Release ==
  /\ \E c \in Chunks :
       /\ kind[c] = "pending"
       /\ \A r \in Readers : pin[r] = 0 \/ pin[r] >= pendAt[c]
       /\ kind' = [kind EXCEPT ![c] = "free"]
  /\ UNCHANGED <<entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, cpc, victim, moves, snap, pin>>

\* A job reserves the lowest free chunk for slot 1, which lives.
Reserve ==
  /\ More /\ HasFree
  /\ kind' = [kind EXCEPT ![Lowest] = "reserved"]
  /\ entries' = [entries EXCEPT ![Lowest] = {}]
  /\ owner' = [owner EXCEPT ![Lowest] = [slot |-> 1, alive |-> TRUE]]
  /\ version' = version + 1
  /\ UNCHANGED <<refs, latest, live, hot, cold, tag, pendAt, cpc, victim, moves, snap, pin>>

\* The job finishes: the chunk holds a run.
Finish ==
  /\ More /\ \E c \in Chunks :
       /\ kind[c] = "reserved" /\ owner[c].alive
       /\ kind' = [kind EXCEPT ![c] = "run"]
  /\ version' = version + 1
  /\ UNCHANGED <<entries, refs, latest, live, hot, cold, tag, pendAt, owner, cpc, victim, moves, snap, pin>>

\* The job's process dies: its slot lock drops, and the next table check
\* frees the chunk.
Die ==
  /\ \E c \in Chunks :
       /\ kind[c] = "reserved" /\ owner[c].alive
       /\ owner' = [owner EXCEPT ![c] = [slot |-> 1, alive |-> FALSE]]
  /\ UNCHANGED <<kind, entries, refs, latest, live, hot, cold, tag, version, pendAt, cpc, victim, moves, snap, pin>>

Reclaim ==
  /\ \E c \in Chunks :
       /\ kind[c] = "reserved" /\ ~owner[c].alive
       /\ kind' = [kind EXCEPT ![c] = "free"]
  /\ UNCHANGED <<entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, cpc, victim, moves, snap, pin>>

\* A reader takes a snapshot and pins its version, reads, and ends.
Snap(r) ==
  /\ pin[r] = 0
  /\ snap' = [snap EXCEPT ![r] = refs] /\ pin' = [pin EXCEPT ![r] = version + 1]
  /\ UNCHANGED <<kind, entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, cpc, victim, moves>>

End(r) ==
  /\ pin[r] # 0
  /\ pin' = [pin EXCEPT ![r] = 0] /\ snap' = [snap EXCEPT ![r] = [k \in Keys |-> None]]
  /\ UNCHANGED <<kind, entries, refs, latest, live, hot, cold, tag, version, pendAt, owner, cpc, victim, moves>>

Next ==
  \/ \E k \in Keys : Write(k)
  \/ OpenHot \/ OpenCold \/ Scan \/ Close \/ Move \/ Seal \/ Abandon \/ Release
  \/ Reserve \/ Finish \/ Die \/ Reclaim
  \/ \E r \in Readers : Snap(r) \/ End(r)
  \/ UNCHANGED vars

Fairness ==
  /\ WF_vars(Release) /\ WF_vars(Reclaim) /\ WF_vars(Seal) /\ WF_vars(Move) /\ WF_vars(Scan)
  /\ WF_vars(Close)
  /\ \A r \in Readers : WF_vars(End(r))
Spec == Init /\ [][Next]_vars /\ Fairness

\* Every key's reference points at an entry that exists in a chunk in use and
\* carries the key's latest value.
RefsLatest ==
  \A k \in Keys : refs[k] # None =>
    /\ kind[refs[k].chunk] = "value"
    /\ [key |-> k, tag |-> refs[k].tag] \in entries[refs[k].chunk]
    /\ refs[k].tag = latest[k]

\* The table's live count is the number of entries the references point at.
CountsAgree ==
  \A c \in Chunks : live[c] = Cardinality({e \in entries[c] : refs[e.key] = [chunk |-> c, tag |-> e.tag]})

\* A free chunk is reachable from nothing: no reference, no snapshot, no
\* cursor.
FreeUnreachable ==
  \A c \in Chunks : kind[c] = "free" =>
    /\ c # hot /\ c # cold
    /\ \A k \in Keys : refs[k].chunk # c
    /\ \A r \in Readers : \A k \in Keys : snap[r][k].chunk # c

\* An open chunk is a value chunk: no write lands in a pending or free chunk.
OpenIsValue ==
  /\ hot # 0 => kind[hot] = "value"
  /\ cold # 0 => kind[cold] = "value"

\* A reader's snapshot still reads every entry it took.
SnapshotsRead ==
  \A r \in Readers : pin[r] # 0 => \A k \in Keys : snap[r][k] # None =>
    /\ kind[snap[r][k].chunk] # "free"
    /\ [key |-> k, tag |-> snap[r][k].tag] \in entries[snap[r][k].chunk]

\* Every pending chunk is eventually free, and a dead job's chunk too.
PendingDrains == \A c \in Chunks : kind[c] = "pending" ~> kind[c] = "free"
====
