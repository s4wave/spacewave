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
\*   DurableSkip: a second put of a block the pack table holds stores nothing
\*                and receipts it only once the batch holding it is synced.
\*                PackedStore skipped on any table hit, synced or not.
\*   PoisonCheck: a pack whose sync failed is poisoned: it is never synced or
\*                finalized in place again, and Salvage copies its synced
\*                batches into a fresh file, synced through a descriptor that
\*                saw no failure. A failed sync may leave its dropped pages
\*                clean, so a later sync reports success for batches the disk
\*                lost.
\*   SalvageOnOpen: recovery copies the batches it keeps into a fresh file
\*                and syncs that, never finalizing the old file in place. A
\*                restart in the same boot reads the page cache, which shows
\*                every written batch intact, the lost ones too, and a sync of
\*                the old file does not bring back a page a failed sync lost.
\*
\* Not modeled: frees from the local pack table (memory only, and a lost free
\* keeps a block readable; a put after a free stores the block again), the
\* pack index's own transaction (one step here), more than one block per
\* batch, since a batch is accepted or dropped whole, and Close, which waits
\* for the running syncs and is a crash with nothing lost to this model.

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
  NBlocks,      \* the blocks the writer appends, 1..NBlocks, one batch each
  NPacks,       \* the pack ids, 1..NPacks, used in order
  Cap,          \* batches in a pack before it rotates
  MaxCrashes,   \* crashes the model allows
  DataCheck,
  RowAfterSync,
  EvictCheck,
  DurableSkip,
  PoisonCheck,
  SalvageOnOpen,
  MaxRestarts   \* process restarts within one boot the model allows

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
  crashes,  \* crashes so far
  poisoned, \* the packs a failed sync poisoned since the last recovery
  lost,     \* the batches <<p, i>> whose pages a failed sync dropped clean
  rebooted, \* the last stop was a crash of the machine, not of the process
  restarts  \* process restarts so far
vars == <<up, next, open, log, synced, disk, file, fin, valid, rows, remote,
          entries, reg, crashes, poisoned, lost, rebooted, restarts>>

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
  /\ poisoned = {}
  /\ lost = {}
  /\ rebooted = FALSE
  /\ restarts = 0

\* Write appends the next block's values and record to the open pack.
Write ==
  /\ up /\ next <= NBlocks /\ Len(log[open]) < Cap
  /\ log' = [log EXCEPT ![open] = Append(@, next)]
  /\ disk' = [disk EXCEPT ![open] = Append(@, [d |-> FALSE, r |-> FALSE])]
  /\ valid' = [valid EXCEPT ![open] = Append(@, TRUE)]
  /\ next' = next + 1
  /\ UNCHANGED <<up, open, synced, file, fin, rows, remote, entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Sync flushes every batch of the open pack. On a poisoned pack it reports
\* success and flushes nothing: the failed sync left the lost pages clean.
Sync ==
  /\ up /\ synced[open] < Len(log[open])
  /\ PoisonCheck => open \notin poisoned
  /\ synced' = [synced EXCEPT ![open] = Len(log[open])]
  /\ disk' = IF open \in poisoned THEN disk
             ELSE [disk EXCEPT ![open] = [i \in DOMAIN @ |-> [d |-> TRUE, r |-> TRUE]]]
  /\ UNCHANGED <<up, next, open, log, file, fin, valid, rows, remote, entries,
                 reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* SyncFail is a sync of the open pack that fails: the disk keeps any subset
\* of each unsynced batch, as a crash would, the page cache keeps showing them
\* intact, and the pack is poisoned.
SyncFail ==
  /\ up /\ synced[open] < Len(log[open]) /\ open \notin poisoned
  /\ poisoned' = poisoned \cup {open}
  /\ lost' = lost \cup {<<open, i>> : i \in synced[open] + 1 .. Len(log[open])}
  /\ \E keep \in [1..Cap -> [d : BOOLEAN, r : BOOLEAN]] :
       disk' = [disk EXCEPT ![open] =
                  [i \in DOMAIN @ |-> IF i <= synced[open] THEN @[i] ELSE keep[i]]]
  /\ UNCHANGED <<up, next, open, log, synced, file, fin, valid, rows, remote,
                 entries, reg, crashes, rebooted, restarts>>

\* Commit writes a block's liveness row in the Volume, its receipt.
Commit ==
  /\ up
  /\ \E p \in Packs, i \in 1..Cap :
       /\ i <= Len(log[p])
       /\ log[p][i] \notin rows
       /\ RowAfterSync => i <= synced[p]
       /\ rows' = rows \cup {log[p][i]}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* SkipPut is a second put of a block the pack table holds: it appends
\* nothing and commits the block's liveness row. The table holds every
\* written batch of a local pack, synced or not.
SkipPut ==
  /\ up
  /\ \E p \in Packs, i \in 1..Cap :
       /\ file[p] \in {"open", "final"}
       /\ i <= Len(log[p])
       /\ log[p][i] \notin rows
       /\ DurableSkip => i <= synced[p]
       /\ PoisonCheck => p \notin poisoned
       /\ rows' = rows \cup {log[p][i]}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Rotate moves the writer to the next pack once the open one is full and
\* synced.
Rotate ==
  /\ up /\ Len(log[open]) = Cap /\ synced[open] = Cap
  /\ \E q \in Packs :
       /\ q = open + 1
       /\ open' = q
       /\ file' = [file EXCEPT ![q] = "open"]
  /\ UNCHANGED <<up, next, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* RotatePoisoned moves the writer off a poisoned pack, which takes no more
\* batches.
RotatePoisoned ==
  /\ up /\ open \in poisoned
  /\ \E q \in Packs :
       /\ q = open + 1
       /\ open' = q
       /\ file' = [file EXCEPT ![q] = "open"]
  /\ UNCHANGED <<up, next, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Salvage copies the synced batches of a rotated poisoned pack from memory
\* into a fresh file, syncs it and finalizes it in the pack's place.
Salvage ==
  /\ up /\ PoisonCheck
  /\ \E p \in poisoned :
       /\ p # open /\ file[p] = "open"
       /\ log' = [log EXCEPT ![p] = SubSeq(@, 1, synced[p])]
       /\ disk' = [disk EXCEPT ![p] = [i \in 1..synced[p] |-> [d |-> TRUE, r |-> TRUE]]]
       /\ valid' = [valid EXCEPT ![p] = SubSeq(@, 1, synced[p])]
       /\ file' = [file EXCEPT ![p] = "final"]
       /\ fin' = [fin EXCEPT ![p] = "index"]
       /\ poisoned' = poisoned \ {p}
       /\ lost' = {x \in lost : x[1] # p}
  /\ UNCHANGED <<up, next, open, synced, rows, remote, entries, reg, crashes,
                 rebooted, restarts>>

\* FinIndex writes and syncs a rotated pack's index after its last record.
FinIndex ==
  /\ up
  /\ \E p \in Packs :
       /\ p # open /\ file[p] = "open" /\ fin[p] = "none"
       /\ synced[p] = Len(log[p])
       /\ PoisonCheck => p \notin poisoned
       /\ fin' = [fin EXCEPT ![p] = "index"]
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, valid, rows, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* FinRename renames the pack to its finalized name and syncs the directory.
FinRename ==
  /\ up
  /\ \E p \in Packs :
       /\ file[p] = "open" /\ fin[p] = "index"
       /\ file' = [file EXCEPT ![p] = "final"]
  /\ UNCHANGED <<up, next, open, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Upload writes a finalized pack's object to the remote.
Upload ==
  /\ up
  /\ \E p \in Packs :
       /\ file[p] = "final" /\ p \notin remote
       /\ remote' = remote \cup {p}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, rows,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Entry writes the pack's entry once its object is there.
Entry ==
  /\ up
  /\ \E p \in remote :
       /\ p \notin entries
       /\ entries' = entries \cup {p}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, rows,
                 remote, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Register commits the pack's keys and its indexed row in the pack index.
Register ==
  /\ up
  /\ \E p \in entries :
       /\ p \notin reg
       /\ reg' = reg \cup {p}
  /\ UNCHANGED <<up, next, open, log, synced, disk, file, fin, valid, rows,
                 remote, entries, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Evict unlinks a finalized local pack.
Evict ==
  /\ up
  /\ \E p \in Packs :
       /\ file[p] = "final"
       /\ EvictCheck => (p \in reg /\ p \in entries /\ p \in remote)
       /\ file' = [file EXCEPT ![p] = "gone"]
  /\ UNCHANGED <<up, next, open, log, synced, disk, fin, valid, rows, remote,
                 entries, reg, crashes, poisoned,
                 rebooted, lost, restarts>>

\* Crash loses memory and keeps, for each unsynced batch of a pack not yet
\* finalized, any subset of its values and record. A finalized pack was synced
\* whole before its rename.
Crash ==
  /\ up /\ crashes < MaxCrashes
  /\ up' = FALSE
  /\ crashes' = crashes + 1
  /\ rebooted' = TRUE
  /\ \E keep \in [Packs -> [1..Cap -> [d : BOOLEAN, r : BOOLEAN]]] :
       disk' = [p \in Packs |->
                  [i \in DOMAIN disk[p] |->
                     IF i <= synced[p] THEN disk[p][i] ELSE keep[p][i]]]
  /\ UNCHANGED <<next, open, log, synced, file, fin, valid, rows, remote,
                 entries, reg, poisoned, lost, restarts>>

\* Restart stops the process in the same boot: memory is lost, and the disk
\* and the page cache stay as they are.
Restart ==
  /\ up /\ restarts < MaxRestarts
  /\ up' = FALSE
  /\ restarts' = restarts + 1
  /\ rebooted' = FALSE
  /\ UNCHANGED <<next, open, log, synced, disk, file, fin, valid, rows, remote,
                 entries, reg, crashes, poisoned, lost>>

\* Accepted holds for a batch recovery keeps: its record survived, and with
\* DataCheck its values did too.
Accepted(b) == b.r /\ (DataCheck => b.d)

\* Kept is how many leading batches of pack p recovery accepts.
Kept(p) ==
  LET n == Len(disk[p])
      bad == {i \in 1..n : ~Accepted(disk[p][i])}
  IN IF bad = {} THEN n ELSE (CHOOSE i \in bad : \A j \in bad : i <= j) - 1

\* Recover reads each unfinalized pack and keeps its leading accepted
\* batches: after a reboot from the disk, and in the same boot from the page
\* cache, which shows every written batch intact. With SalvageOnOpen it copies
\* them into a fresh file and syncs it, so the disk then holds them; in place,
\* the sync of the old file brings back no page a failed sync lost. After a
\* reboot a finalized pack also holds only what its disk holds. Recovery then
\* opens the next unused pack.
Recover ==
  /\ ~up
  /\ LET live == {p \in Packs : file[p] = "open"}
         reread == {p \in Packs : rebooted /\ file[p] = "final"}
         n == [p \in Packs |->
                 IF p \in live THEN (IF rebooted THEN Kept(p) ELSE Len(log[p]))
                 ELSE IF p \in reread THEN Kept(p) ELSE Len(log[p])]
         copied(p, i) == p \in live /\ ~rebooted /\ (SalvageOnOpen \/ <<p, i>> \notin lost)
         unused == {p \in Packs : file[p] = "none"}
     IN
     /\ unused # {}
     /\ LET q == CHOOSE u \in unused : \A v \in unused : u <= v IN
        /\ log' = [p \in Packs |-> SubSeq(log[p], 1, n[p])]
        /\ disk' = [p \in Packs |->
                      [i \in 1..n[p] |->
                         IF copied(p, i) THEN [d |-> TRUE, r |-> TRUE] ELSE disk[p][i]]]
        /\ valid' = [p \in Packs |->
                       IF p \in live /\ ~rebooted THEN [i \in 1..n[p] |-> TRUE]
                       ELSE IF p \in live \cup reread THEN [i \in 1..n[p] |-> disk[p][i].d]
                       ELSE valid[p]]
        /\ synced' = n
        /\ file' = [p \in Packs |->
                      IF p \in live THEN "final"
                      ELSE IF p = q THEN "open" ELSE file[p]]
        /\ fin' = [p \in Packs |-> IF p \in live THEN "index" ELSE fin[p]]
        /\ open' = q
  /\ up' = TRUE
  /\ poisoned' = {}
  /\ lost' = {}
  /\ UNCHANGED <<next, rows, remote, entries, reg, crashes, rebooted, restarts>>

Next ==
  \/ Write \/ Sync \/ SyncFail \/ Commit \/ SkipPut \/ Rotate
  \/ RotatePoisoned \/ Salvage \/ FinIndex \/ FinRename
  \/ Upload \/ Entry \/ Register \/ Evict \/ Crash \/ Restart \/ Recover

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
  /\ poisoned \subseteq Packs
  /\ \A p \in Packs : synced[p] <= Len(log[p]) /\ Len(log[p]) <= Cap

====
