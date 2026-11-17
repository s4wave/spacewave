----------------------------- MODULE EvictBudget ----------------------------
\* The drop decision of a bounded eviction pass.
\*
\* A batch works on one remote pack. It reads the operator's budget, then in
\* one metadata transaction reserves up to the pack's bytes from the bytes left
\* under the budget's id. It then fetches the pack from the remote, verifies
\* each value it would drop against its hash from the fetched bytes, tombstones
\* held local values that verified while the reservation lasts, and finally,
\* in one metadata transaction, deletes the batch's queue entries and returns
\* the unspent reservation. The local values and the metadata are separate
\* stores with no shared transaction, so the reservation commits before the
\* tombstones: a crash between them loses the reservation, never charges a
\* drop late, and never returns bytes.
\*
\* The budget is a file the operator writes at any time: absent, unreadable, or
\* an id with a byte count. A valid file with an id the store has not seen
\* grants its bytes; a seen id keeps the bytes left, lowered to the file's
\* count, and nothing ever adds to it. Without a valid file a batch starts
\* nothing.
\*
\* The remote copy of a block may be good, missing, truncated, or of the same
\* size with other bytes, and may change at any time. A fetch takes the
\* remote's state of the pack's blocks at that moment; verify passes only a
\* good copy in the fetch of its own batch. The window between that fetch and
\* the tombstone remains, and is a batch long.
\*
\* A batch drops a block only while it is held locally, no upload marker was
\* seen for it when the batch read its candidates, and the whole pack's fetch
\* verified: one value that fails, or a fetch that is short or does not parse,
\* drops nothing from the pack in that batch. The recently read blocks a pass
\* keeps are a choice among eligible blocks and are not modelled.
\*
\* Writers run concurrently with a batch. A put of the block's hash writes the
\* value and its upload marker, even between the batch's verify and its
\* tombstone, and whichever of the put and the tombstone commits last wins. A
\* read fill of a dropped block reads the remote copy, checks its hash, and
\* writes the value back without a marker. An upload clears the marker; it
\* skips a block whose local value is gone, whose copy the bucket holds.
\*
\* Every block has size one. The ghost variables granted, dropped, truthAt,
\* fresh, proofTruth, proofFresh, proofPack, and dropRead record what the
\* invariants check; the implementation keeps none of them.
EXTENDS Integers, FiniteSets

CONSTANTS NB, NP, Ids, MaxGrant, MaxOps

Blocks == 1..NB
Packs == 1..NP

\* PackOf(b) is the remote pack that holds block b.
PackOf(b) == ((b - 1) % NP) + 1
BlocksOf(p) == {b \in Blocks : PackOf(b) = p}

RemoteStates == {"good", "missing", "short", "wrong"}
Budgets == [kind : {"ok"}, id : Ids, bytes : 0..MaxGrant]
NoBudget == [kind |-> "none"]
Files == {[kind |-> "absent"], [kind |-> "bad"]} \cup Budgets
Unseen == -1

Min(a, b) == IF a < b THEN a ELSE b

VARIABLES
    local,       \* block -> "value" or "tomb", durable
    marker,      \* block -> an upload marker awaits, durable
    remote,      \* block -> the remote copy's state
    file,        \* the budget file
    left,        \* id -> bytes left, Unseen before the id is seen, durable
    pc,          \* "idle", "reserved", or "down"
    mem,         \* the last valid budget the process read, or NoBudget
    bid,         \* the id the batch reserved under, or 0
    pack,        \* the batch's pack, or 0
    resv,        \* the bytes the batch reserved
    spent,       \* the bytes the batch dropped
    fetched,     \* block -> the remote state the batch's fetch saw, or "none"
    seen,        \* block -> the verify's answer this batch, or "none"
    elig,        \* block -> held and unmarked when the batch read it
    fresh,       \* block -> the batch's own fetch read it, ghost
    truthAt,     \* block -> remote state at that fetch, ghost
    readValid,   \* the batch read a valid file, ghost
    granted,     \* id -> bytes granted when first seen, ghost
    dropped,     \* id -> values dropped under the id, ghost
    proofTruth,  \* block -> remote state at the fetch its drop rests on
    proofFresh,  \* block -> that fetch was in the drop's batch
    proofPack,   \* block -> that fetch saw every block of the pack good
    dropRead,    \* "none", or whether the last drop's batch read a valid file
    ops          \* environment steps so far, bounding the state space

vars == <<local, marker, remote, file, left, pc, mem, bid, pack, resv, spent,
          fetched, seen, elig, fresh, truthAt, readValid, granted,
          dropped, proofTruth, proofFresh, proofPack, dropRead, ops>>

ghost == <<granted, dropped, proofTruth, proofFresh, proofPack, dropRead>>

\* batch is the batch's state the writers leave alone.
batch == <<pc, mem, bid, pack, resv, spent, fetched, seen, elig, fresh,
           truthAt, readValid>>

\* Total(i) is what id i may still drop or has dropped: the bytes left, the
\* unspent reservation of a batch under it, and the drops. Nothing raises it.
Total(i) == left[i] + (IF bid = i THEN resv - spent ELSE 0) + dropped[i]

Init ==
    /\ local = [b \in Blocks |-> "value"]
    /\ marker = [b \in Blocks |-> FALSE]
    /\ remote = [b \in Blocks |-> "good"]
    /\ file = [kind |-> "absent"]
    /\ left = [i \in Ids |-> Unseen]
    /\ pc = "idle"
    /\ mem = NoBudget
    /\ bid = 0
    /\ pack = 0
    /\ resv = 0
    /\ spent = 0
    /\ fetched = [b \in Blocks |-> "none"]
    /\ seen = [b \in Blocks |-> "none"]
    /\ elig = [b \in Blocks |-> FALSE]
    /\ fresh = [b \in Blocks |-> FALSE]
    /\ truthAt = [b \in Blocks |-> "none"]
    /\ readValid = FALSE
    /\ granted = [i \in Ids |-> Unseen]
    /\ dropped = [i \in Ids |-> 0]
    /\ proofTruth = [b \in Blocks |-> "none"]
    /\ proofFresh = [b \in Blocks |-> FALSE]
    /\ proofPack = [b \in Blocks |-> FALSE]
    /\ dropRead = "none"
    /\ ops = 0

\* The operator writes, breaks, or removes the budget file.
WriteFile(f) ==
    /\ ops < MaxOps
    /\ file' = f
    /\ ops' = ops + 1
    /\ UNCHANGED <<local, marker, remote, left, batch, ghost>>

\* The remote copy of a block changes: lost, cut short, or rewritten.
MutateRemote(b, s) ==
    /\ ops < MaxOps
    /\ remote' = [remote EXCEPT ![b] = s]
    /\ ops' = ops + 1
    /\ UNCHANGED <<local, marker, file, left, batch, ghost>>

\* A writer puts the block's hash: the value and its upload marker, at any
\* moment of a batch.
Put(b) ==
    /\ ops < MaxOps
    /\ local' = [local EXCEPT ![b] = "value"]
    /\ marker' = [marker EXCEPT ![b] = TRUE]
    /\ ops' = ops + 1
    /\ UNCHANGED <<remote, file, left, batch, ghost>>

\* A read fills a dropped block from the remote copy, which it checks against
\* the block's hash, without a marker.
Fill(b) ==
    /\ local[b] = "tomb"
    /\ remote[b] = "good"
    /\ local' = [local EXCEPT ![b] = "value"]
    /\ UNCHANGED <<marker, remote, file, left, batch, ghost, ops>>

\* An upload clears the marker; a block whose value is gone it skips.
Upload(b) ==
    /\ marker[b]
    /\ marker' = [marker EXCEPT ![b] = FALSE]
    /\ UNCHANGED <<local, remote, file, left, batch, ghost, ops>>

\* Reserve reads the budget and, in one metadata transaction, reserves up to
\* the bytes of pack p from the bytes left under its id. Without a valid
\* budget no batch starts. A batch starts with nothing fetched or verified.
Reserve(p) ==
    /\ pc = "idle"
    /\ LET valid == file.kind = "ok"
           budget == IF valid THEN file ELSE NoBudget
       IN /\ budget.kind = "ok"
          /\ LET i == budget.id
                 cur == IF left[i] = Unseen THEN budget.bytes
                        ELSE Min(left[i], budget.bytes)
                 c == Min(cur, Cardinality(BlocksOf(p)))
             IN /\ left' = [left EXCEPT ![i] = cur - c]
                /\ resv' = c
                /\ granted' = IF granted[i] = Unseen
                                 THEN [granted EXCEPT ![i] = budget.bytes]
                                 ELSE granted
                /\ bid' = i
          /\ mem' = budget
          /\ readValid' = valid
    /\ pc' = "reserved"
    /\ pack' = p
    /\ spent' = 0
    /\ fetched' = [b \in Blocks |-> "none"]
    /\ seen' = [b \in Blocks |-> "none"]
    /\ elig' = [b \in Blocks |->
                   PackOf(b) = p /\ local[b] = "value" /\ ~marker[b]]
    /\ fresh' = [b \in Blocks |-> FALSE]
    /\ UNCHANGED <<local, marker, remote, file, truthAt, dropped,
                   proofTruth, proofFresh, proofPack, dropRead, ops>>

\* Fetch reads the batch's pack from the remote: the bytes of every block in
\* it as the remote holds them now, once a batch.
Fetch ==
    /\ pc = "reserved"
    /\ \A b \in BlocksOf(pack) : fetched[b] = "none"
    /\ fetched' = [b \in Blocks |->
                      IF PackOf(b) = pack THEN remote[b] ELSE fetched[b]]
    /\ fresh' = [b \in Blocks |-> PackOf(b) = pack \/ fresh[b]]
    /\ truthAt' = [b \in Blocks |->
                      IF PackOf(b) = pack THEN remote[b] ELSE truthAt[b]]
    /\ UNCHANGED <<local, marker, remote, file, left, pc, mem, bid, pack, resv,
                   spent, seen, elig, readValid, ghost, ops>>

\* PackGood is whether the batch's fetch holds every block of its pack good.
PackGood == \A b \in BlocksOf(pack) : fetched[b] = "good"

\* Verify hashes every fetched value of the pack against its key, after the
\* reservation: each block passes only when the whole pack does, and a pack
\* not fetched fails.
Verify ==
    /\ pc = "reserved"
    /\ \A b \in BlocksOf(pack) : fetched[b] # "none"
    /\ seen' = [b \in Blocks |->
                   IF PackOf(b) # pack THEN seen[b]
                   ELSE IF PackGood THEN "good" ELSE "bad"]
    /\ UNCHANGED <<local, marker, remote, file, left, pc, mem, bid, pack, resv,
                   spent, fetched, elig, fresh, truthAt, readValid, ghost, ops>>

\* Tombstone drops a held local value whose verify passed this batch, while
\* the reservation lasts.
Tombstone(b) ==
    /\ pc = "reserved"
    /\ PackOf(b) = pack
    /\ elig[b]
    /\ local[b] = "value"
    /\ seen[b] = "good"
    /\ spent < resv
    /\ local' = [local EXCEPT ![b] = "tomb"]
    /\ spent' = spent + 1
    /\ dropped' = [dropped EXCEPT ![bid] = @ + 1]
    /\ proofTruth' = [proofTruth EXCEPT ![b] = truthAt[b]]
    /\ proofFresh' = [proofFresh EXCEPT ![b] = fresh[b]]
    /\ proofPack' = [proofPack EXCEPT ![b] =
                        \A c \in BlocksOf(pack) : truthAt[c] = "good" /\ fresh[c]]
    /\ dropRead' = IF readValid THEN "valid" ELSE "invalid"
    /\ UNCHANGED <<marker, remote, file, left, pc, mem, bid, pack, resv, fetched,
                   seen, elig, fresh, truthAt, readValid, granted, ops>>

\* Finish deletes the batch's queue entries and returns the unspent
\* reservation in one metadata transaction.
Finish ==
    /\ pc = "reserved"
    /\ left' = [left EXCEPT ![bid] = @ + (resv - spent)]
    /\ pc' = "idle"
    /\ bid' = 0
    /\ pack' = 0
    /\ resv' = 0
    /\ spent' = 0
    /\ UNCHANGED <<local, marker, remote, file, mem, fetched, seen, elig,
                   fresh, truthAt, readValid, ghost, ops>>

\* A crash at any step loses the process's memory, the reservation with it;
\* the local values and the bytes left are durable.
Crash ==
    /\ pc # "down"
    /\ ops < MaxOps
    /\ pc' = "down"
    /\ mem' = NoBudget
    /\ bid' = 0
    /\ pack' = 0
    /\ resv' = 0
    /\ spent' = 0
    /\ fetched' = [b \in Blocks |-> "none"]
    /\ seen' = [b \in Blocks |-> "none"]
    /\ elig' = [b \in Blocks |-> FALSE]
    /\ ops' = ops + 1
    /\ fresh' = [b \in Blocks |-> FALSE]
    /\ UNCHANGED <<local, marker, remote, file, left, truthAt, readValid,
                   ghost>>

Restart ==
    /\ pc = "down"
    /\ pc' = "idle"
    /\ UNCHANGED <<local, marker, remote, file, left, mem, bid, pack, resv,
                   spent, fetched, seen, elig, fresh, truthAt, readValid,
                   ghost, ops>>

Next ==
    \/ \E f \in Files : WriteFile(f)
    \/ \E b \in Blocks, s \in RemoteStates : MutateRemote(b, s)
    \/ \E p \in Packs : Reserve(p)
    \/ Fetch
    \/ \E b \in Blocks : Put(b) \/ Fill(b) \/ Upload(b) \/ Tombstone(b)
    \/ Verify
    \/ Finish
    \/ Crash
    \/ Restart

Spec == Init /\ [][Next]_vars

TypeOK ==
    /\ local \in [Blocks -> {"value", "tomb"}]
    /\ marker \in [Blocks -> BOOLEAN]
    /\ remote \in [Blocks -> RemoteStates]
    /\ file \in Files
    /\ left \in [Ids -> {Unseen} \cup 0..MaxGrant]
    /\ pc \in {"idle", "reserved", "down"}
    /\ bid \in {0} \cup Ids
    /\ spent \in Nat
    /\ dropped \in [Ids -> Nat]

\* DropVerified: every dropped value rests on a fetch made in its own batch
\* that found the remote copy good, and every block of its pack good.
DropVerified ==
    \A b \in Blocks :
        local[b] = "tomb" =>
            proofTruth[b] = "good" /\ proofFresh[b] /\ proofPack[b]

\* NoLoss: every block has a local value or a remote copy found good by the
\* fetch of the batch that dropped it, through concurrent puts and fills.
NoLoss ==
    \A b \in Blocks : local[b] = "value" \/ proofTruth[b] = "good"

\* MarkedHeldOrVerified: a block with an upload marker whose value a racing
\* drop removed has a verified remote copy, which the upload's skip relies on.
MarkedHeldOrVerified ==
    \A b \in Blocks :
        marker[b] /\ local[b] = "tomb" => proofTruth[b] = "good" 

\* DroppedWithinBudget: the values dropped under an id never pass the bytes
\* it granted, and an id never seen drops nothing.
DroppedWithinBudget ==
    \A i \in Ids :
        IF granted[i] = Unseen THEN dropped[i] = 0 ELSE dropped[i] <= granted[i]

\* BudgetDurable: the bytes left and the bytes dropped never sum past the
\* grant, in every state, including after a crash between the reservation and
\* the tombstones.
BudgetDurable ==
    \A i \in Ids : left[i] # Unseen => left[i] + dropped[i] <= granted[i]

\* NoBudgetNoDrop: every drop's batch read a valid budget file.
NoBudgetNoDrop == dropRead \in {"none", "valid"}

\* SameIdNoRefill: what a seen id may still drop or has dropped never rises,
\* through any number of restarts, refunds, and rewrites of the file.
SameIdNoRefill ==
    [][\A i \in Ids : left[i] # Unseen => Total(i)' <= Total(i)]_vars
=============================================================================
