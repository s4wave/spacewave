---- MODULE PackAhead ----
\* A directory of append-only packs that new block values go into, against
\* every state a crash can leave on the drive.
\*
\* A writer appends one batch at a time to the open pack: the batch's values,
\* then its record. Sync flushes every batch appended to the pack, and only a
\* synced batch's block may gain its liveness row in the Volume, which is the
\* receipt. A full pack rotates: the writer moves to the next pack and the old
\* one is finalized, its index written and synced, then renamed. A finalized
\* pack uploads as one object, then its entry, then its registration in the
\* pack index, and a registered pack may be evicted, unlinking the file.
\*
\* One fsync does not order the sectors it writes, so a crash keeps, for every
\* batch not yet synced, its values, its record, both or neither. Recovery reads
\* each pack the rename did not finalize, keeps the batches in order up to the
\* first whose record is missing or whose values are missing (the record's
\* checksum of its values), truncates there and finalizes the pack.
\*
\* Guards, which a mutant turns off to show the invariant needs it:
\*   DataCheck:   recovery rejects a record whose values did not survive.
\*   RowAfterSync: a liveness row is committed only for a synced batch.
\*   EvictCheck:  eviction requires the object, the entry and the registration.
\*
\* Not modeled: frees from the local pack table (memory only, and a lost free
\* keeps a block readable), the pack index's own transaction (one step here),
\* and more than one block per batch, since a batch is accepted or dropped
\* whole.

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
  NBlocks,      \* the blocks the writer appends, 1..NBlocks, one batch each
  NPacks,       \* the pack ids, 1..NPacks, used in order
  Cap,          \* batches in a pack before it rotates
  MaxCrashes,   \* crashes the model allows
  DataCheck,
  RowAfterSync,
  EvictCheck

VARIABLES
  up,       \* the process runs; FALSE between a crash and its recovery
  next,     \* the next block to append
  open,     \* the pack the writer appends to
  log,      \* each pack's batches as written: a sequence of blocks
  synced,   \* each pack's count of batches known durable
  disk,     \* each pack's batches as a crash left them: [d, r] for values, record
  file,     \* each pack's file: "none", "open", "final" or "gone"
  fin,      \* each pack's finalize step: "none", "index" (written and synced)
  valid,    \* each pack's batch values a reader would find intact
  rows,     \* the blocks with a liveness row: the receipted blocks
  remote,   \* the packs whose object the remote holds
  entries,  \* the packs whose entry the remote holds
  reg,      \* the packs registered in the pack index
  crashes   \* crashes so far
vars == <<up, next, open, log, synced, disk, file, fin, valid, rows, remote,
          entries, reg, crashes>>

Packs == 1..NPacks
FirstPack == 1

Init ==
  /\ up = TRUE
  /\ next = 1
  /\ open = FirstPack
  /\ log = [p \in Packs |-> <<>>]
  /\ synced = [p \in Packs |-> 0]
  /\ disk = [p \in Packs |-> <<>>]
  /\ file = [p \in Packs |-> IF p = FirstPack THEN "open" ELSE "none"]
  /\ fin = [p \in Packs |-> "none"]
  /\ valid = [p \in Packs |-> <<>>]
  /\ rows = {}
  /\ remote = {}
  /\ entries = {}
  /\ reg = {}
  /\ crashes = 0

\* Write appends the next block's values and record to the open pack.
Write ==
  /\ up /\ next <= NBlocks /\ Len(log[open]) < Cap
  /\ log' = [log EXCEPT ![open] = Append(@, next)]
  /\ disk' = [disk EXCEPT ![open] = Append(@, [d |-> FALSE, r |-> FALSE])]
  /\ valid' = [valid EXCEPT ![open] = Append(@, TRUE)]
  /\ next' = next + 1
  /\ UNCHANGED <<up, open, synced, file, fin, rows, remote, entries, reg, crashes>>

\* Sync flushes every batch of the open pack.
Sync ==
  /\ up /\ synced[open] < Len(log[open])
  /\ synced' = [synced EXCEPT ![open] = Len(log[open])]
  /\ disk' = [disk EXCEPT ![open] = [i \in DOMAIN @ |-> [d |-> TRUE, r |-> TRUE]]]
  /\ UNCHANGED <<up, next, open, log, file, fin, valid, rows, remote, entries,
                 reg, crashes>>

\* Commit writes a block's liveness row in the Volume, its receipt.
Commit ==
  /\ up
  /\ \E p \in Packs, i \in 1..Cap :
       /\ i <= Len(log[p])
       /\ log[p][i] \notin rows
       /\ RowAfterSync => i <= synced[p]
       /\ rows' = rows \cup {log[p][i]}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, remote,
                 entries, reg, crashes>>

\* Rotate moves the writer to the next pack once the open one is full and
\* synced.
Rotate ==
  /\ up /\ Len(log[open]) = Cap /\ synced[open] = Cap
  /\ \E q \in Packs :
       /\ q = open + 1
       /\ open' = q
       /\ file' = [file EXCEPT ![q] = "open"]
  /\ UNCHANGED <<up, next, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes>>

\* FinIndex writes and syncs a rotated pack's index after its last record.
FinIndex ==
  /\ up
  /\ \E p \in Packs :
       /\ p # open /\ file[p] = "open" /\ fin[p] = "none"
       /\ synced[p] = Len(log[p])
       /\ fin' = [fin EXCEPT ![p] = "index"]
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, valid, rows, remote,
                 entries, reg, crashes>>

\* FinRename renames the pack to its finalized name and syncs the directory.
FinRename ==
  /\ up
  /\ \E p \in Packs :
       /\ file[p] = "open" /\ fin[p] = "index"
       /\ file' = [file EXCEPT ![p] = "final"]
  /\ UNCHANGED <<up, next, open, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes>>

\* Upload writes a finalized pack's object to the remote.
Upload ==
  /\ up
  /\ \E p \in Packs :
       /\ file[p] = "final" /\ p \notin remote
       /\ remote' = remote \cup {p}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, rows,
                 entries, reg, crashes>>

\* Entry writes the pack's entry once its object is there.
Entry ==
  /\ up
  /\ \E p \in remote :
       /\ p \notin entries
       /\ entries' = entries \cup {p}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, rows,
                 remote, reg, crashes>>

\* Register commits the pack's keys and its indexed row in the pack index.
Register ==
  /\ up
  /\ \E p \in entries :
       /\ p \notin reg
       /\ reg' = reg \cup {p}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, rows,
                 remote, entries, crashes>>

\* Evict unlinks a finalized local pack.
Evict ==
  /\ up
  /\ \E p \in Packs :
       /\ file[p] = "final"
       /\ EvictCheck => (p \in reg /\ p \in entries /\ p \in remote)
       /\ file' = [file EXCEPT ![p] = "gone"]
  /\ UNCHANGED <<up, next, open, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes>>

\* Crash loses memory and keeps, for each unsynced batch of a pack not yet
\* finalized, any subset of its values and record. A finalized pack was synced
\* whole before its rename.
Crash ==
  /\ up /\ crashes < MaxCrashes
  /\ up' = FALSE
  /\ crashes' = crashes + 1
  /\ \E keep \in [Packs -> [1..Cap -> [d : BOOLEAN, r : BOOLEAN]]] :
       disk' = [p \in Packs |->
                  [i \in DOMAIN disk[p] |->
                     IF i <= synced[p] THEN disk[p][i] ELSE keep[p][i]]]
  /\ UNCHANGED <<next, open, log, synced, file, fin, valid, rows, remote,
                 entries, reg>>

\* Accepted holds for a batch recovery keeps: its record survived, and with
\* DataCheck its values did too.
Accepted(b) == b.r /\ (DataCheck => b.d)

\* Kept is how many leading batches of pack p recovery accepts.
Kept(p) ==
  LET n == Len(disk[p])
      bad == {i \in 1..n : ~Accepted(disk[p][i])}
  IN IF bad = {} THEN n ELSE (CHOOSE i \in bad : \A j \in bad : i <= j) - 1

\* Recover truncates each unfinalized pack to its accepted batches, finalizes
\* it, and opens the next unused pack.
Recover ==
  /\ ~up
  /\ LET live == {p \in Packs : file[p] = "open"}
         n == [p \in Packs |-> IF p \in live THEN Kept(p) ELSE Len(log[p])]
         unused == {p \in Packs : file[p] = "none"}
     IN
     /\ unused # {}
     /\ LET q == CHOOSE u \in unused : \A v \in unused : u <= v IN
        /\ log' = [p \in Packs |-> SubSeq(log[p], 1, n[p])]
        /\ disk' = [p \in Packs |-> SubSeq(disk[p], 1, n[p])]
        /\ valid' = [p \in Packs |->
                       IF p \in live THEN [i \in 1..n[p] |-> disk[p][i].d]
                       ELSE valid[p]]
        /\ synced' = n
        /\ file' = [p \in Packs |->
                      IF p \in live THEN "final"
                      ELSE IF p = q THEN "open" ELSE file[p]]
        /\ fin' = [p \in Packs |-> IF p \in live THEN "index" ELSE fin[p]]
        /\ open' = q
  /\ up' = TRUE
  /\ UNCHANGED <<next, rows, remote, entries, reg, crashes>>

Next ==
  \/ Write \/ Sync \/ Commit \/ Rotate \/ FinIndex \/ FinRename
  \/ Upload \/ Entry \/ Register \/ Evict \/ Crash \/ Recover

Spec == Init /\ [][Next]_vars

\* Holds is the set of blocks some durable copy holds: a synced batch of a
\* pack whose file exists, or a batch of a pack the remote lists.
Holds ==
  LET local == {q \in Packs : file[q] \in {"open", "final"}}
      listed == remote \cap entries \cap reg
  IN UNION {{log[p][i] : i \in 1..synced[p]} : p \in local}
     \cup UNION {{log[p][i] : i \in DOMAIN log[p]} : p \in listed}

\* ReceiptedHeld: every receipted block is held by a durable copy.
ReceiptedHeld == up => rows \subseteq Holds

\* RecoveredValid: every batch a pack keeps has its values intact.
RecoveredValid ==
  \A p \in Packs : \A i \in DOMAIN valid[p] : i <= synced[p] => valid[p][i]

\* EvictedRegistered: a pack whose file is gone is registered remotely.
EvictedRegistered ==
  \A p \in Packs : file[p] = "gone" => p \in remote /\ p \in entries /\ p \in reg

TypeOK ==
  /\ open \in Packs
  /\ \A p \in Packs : synced[p] <= Len(log[p]) /\ Len(log[p]) <= Cap

====
