---------------------------- MODULE NodePush ----------------------------
(* A commit's blocks go to the local store and, from memory, to a storage
   node. The node flushes them, uploads them to the remote store, and reports
   what the remote holds. The local queue entry of a block goes away only on
   that report, so the remote copy has one owner at a time and the local copy
   outlives every copy that might be lost.

   The point of the flush is that the local disk can die. A block acknowledged
   through the node survives that; a block acknowledged while the node was away
   or slow has only the local store's durability until it is uploaded, as
   before the node existed. The model allows one disk to die, never both.

   A receipt's wait for the node can time out at any moment, and the local
   upload takes any queue entry past a deadline, so a lost frame, a flapping
   node or a node back with an empty disk leaves nothing stuck.

   The local process never lists the remote: it knows only packs in its own
   index. So the remote holding a block is not enough, and a report carries the
   pack's index entry, which the local process stores before it drops the queue
   entry.

   The node stops reporting a pack once it hears it applied, so the answer
   follows the clear of its queue entries: a clear dropped after the answer
   would leave them queued, the local copy unevictable, until the upload's
   turn.

   Durable mode. With the switch at "durable", a publication's blocks skip the
   local disk: the local store holds them only in memory, and the node's log is
   the store's write-ahead log. Its push goes first; the local commit, which
   names the blocks without their bytes, waits for the push's flush, since the
   local write-ahead log survives a process crash and memory does not. The
   commit then commits without a local flush, and sends its record, the
   commit's writes, to the node's journal. Its receipt waits until every
   commit up to it is durable: flushed locally, or its record flushed on the
   node. The local store still flushes in the background, and tells the node
   which records it may drop.

   A host crash loses the commits the local store had not flushed. The restart
   replays the node's records past the local head, in order, stopping at the
   first it lacks. The journal is relied on only while a durable local marker
   says it is live, so a restart that finds the marker unset skips the node
   entirely, and switching back to "on" clears the marker only once every
   commit is flushed locally. A restart that cannot reach the node starts on
   the local head and sets the node's records aside: it spends the one failure
   the model allows, since the records it skips may hold acknowledged commits.

   A block held in memory leaves it when the node reports the remote holds it,
   or spills: written locally with a queue entry and flushed, when its report
   is late, the node's disk is gone, or the switch went back to "on". The
   memory set is capped; at the cap, or with the node away, a publication
   takes the local path. The accepted risk is a local crash, which loses the
   memory copies, followed by the node's disk dying before the remote holds
   them; NodeDiskLost excludes it.

   A drain sends the local backlog to the node from the local disk: the blocks
   committed while the node was away or slow. It never drops the queue entry
   of a block it sent; the report's apply does, as for any pushed block, so a
   push the node loses leaves the entry for the local upload. It drops an entry
   itself only for a block the local index names on the remote, as the upload's
   skip does.

   NodePush.cfg checks one block with a cap of one, which aptre-tla walks in
   seconds and which kills every mutant. Two blocks, for the order of
   receipts, the cap with a second publication and the journal's contiguity,
   is about 1.4 million distinct states before the drain's actions, beyond
   aptre-tla's memory on a workstation; check it with Blocks = {b1, b2} on a
   larger host.

   NodePush.mutants.json lists the guards each invariant depends on. *)
EXTENDS FiniteSets, Naturals, Sequences

CONSTANTS Blocks, Cap

VARIABLES
    state,    \* block -> "new" | "sent" | "acked" | "gone"
    path,     \* block -> "none" | "local" | "node": which path its commit took
    local,    \* blocks durable on the local store
    queued,   \* blocks with a local upload queue entry
    nodeBuf,  \* blocks the node received and has not flushed
    nodeDisk, \* blocks flushed to the node's disk
    remote,   \* blocks the remote store holds
    reported, \* blocks the node reported the remote holds, not yet applied
    indexed,  \* blocks the local pack index can find on the remote
    answered, \* blocks the node heard applied, and so reports no more
    nodeUp,   \* the node answers
    degraded, \* blocks acknowledged without the node's flush
    failed,   \* the failure spent: "none", "node" or "local"
    mode,     \* the switch: "on" or "durable"
    mem,      \* blocks the local store holds only in memory
    waiting,  \* durable publications pushed, waiting for the flush to commit
    wal,      \* the local commits in order, each naming its block
    durN,     \* the length of wal's locally flushed prefix
    lost,     \* the commits a host crash dropped, in order, until restart
    jbuf,     \* commits whose record the node received and has not flushed
    journal,  \* commits whose record the node flushed
    live,     \* the durable local marker that the journal is relied on
    up,       \* the local process serves
    bounced,  \* ghost: blocks whose only other copy a local crash dropped
    late      \* ghost: blocks queued by a push that timed out after their report

vars == <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
          nodeUp, degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

Range(s) == {s[i] : i \in DOMAIN s}

InWal(b) == b \in Range(wal)

Pos(b) == CHOOSE i \in DOMAIN wal : wal[i] = b

\* Every commit up to b's is durable: flushed locally, or its record flushed
\* on the node.
HeadDurable(b) ==
    /\ InWal(b)
    /\ \A i \in (durN + 1)..Pos(b) : wal[i] \in journal

\* Receipts resolve in commit order, so the writer hears its commits in order.
EarlierDone(b) == \A i \in 1..(Pos(b) - 1) : state[wal[i]] # "sent"

\* The longest prefix of s whose records the node holds.
RECURSIVE Held(_)
Held(s) == IF s # <<>> /\ Head(s) \in journal THEN <<Head(s)>> \o Held(Tail(s)) ELSE <<>>

Init ==
    /\ state = [b \in Blocks |-> "new"]
    /\ path = [b \in Blocks |-> "none"]
    /\ local = {}
    /\ queued = {}
    /\ nodeBuf = {}
    /\ nodeDisk = {}
    /\ remote = {}
    /\ reported = {}
    /\ indexed = {}
    /\ answered = {}
    /\ nodeUp = TRUE
    /\ degraded = {}
    /\ failed = "none"
    /\ mode \in {"on", "durable"}
    /\ mem = {}
    /\ waiting = <<>>
    /\ wal = <<>>
    /\ durN = 0
    /\ lost = <<>>
    /\ jbuf = {}
    /\ journal = {}
    /\ live = FALSE
    /\ up = TRUE
    /\ bounced = {}
    /\ late = {}

\* The local path: the commit writes the block and its queue entry in one
\* local transaction, flushed before its receipt, and sends the block to the
\* node from memory beside it. Every mode takes it; durable mode takes it at
\* the cap or with the node away. It commits behind any waiting publication.
Submit(b) ==
    /\ up
    /\ state[b] = "new"
    /\ waiting = <<>>
    /\ state' = [state EXCEPT ![b] = "sent"]
    /\ path' = [path EXCEPT ![b] = "local"]
    /\ local' = local \cup {b}
    /\ queued' = queued \cup {b}
    /\ nodeBuf' = IF nodeUp THEN nodeBuf \cup {b} ELSE nodeBuf
    /\ wal' = Append(wal, b)
    /\ durN' = Len(wal) + 1
    /\ UNCHANGED <<indexed, answered, nodeDisk, remote, reported, nodeUp, degraded, failed,
                   mode, mem, waiting, lost, jbuf, journal, live, up, bounced, late>>

\* The durable path: the block goes to memory and to the node, and its commit
\* waits for the push.
SubmitDurable(b) ==
    /\ up
    /\ mode = "durable"
    /\ live
    /\ nodeUp
    /\ state[b] = "new"
    /\ Cardinality(mem) < Cap
    /\ state' = [state EXCEPT ![b] = "sent"]
    /\ path' = [path EXCEPT ![b] = "node"]
    /\ mem' = mem \cup {b}
    /\ nodeBuf' = nodeBuf \cup {b}
    /\ waiting' = Append(waiting, b)
    /\ UNCHANGED <<local, queued, indexed, answered, nodeDisk, remote, reported, nodeUp, degraded,
                   failed, mode, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* Once the node flushed the push, the local store commits the publication,
\* naming the block without its bytes and without a local flush, and sends
\* the commit's record.
CommitDurable ==
    /\ up
    /\ waiting # <<>>
    /\ Head(waiting) \in nodeDisk
    /\ waiting' = Tail(waiting)
    /\ wal' = Append(wal, Head(waiting))
    /\ jbuf' = IF nodeUp THEN jbuf \cup {Head(waiting)} ELSE jbuf
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, durN, lost, journal, live, up, bounced, late>>

\* The push failed or timed out, which can happen at any moment: the
\* publication takes the local path after all, writing its block and queue
\* entry with the local flush.
PushFails ==
    /\ up
    /\ waiting # <<>>
    /\ LET b == Head(waiting) IN
        /\ waiting' = Tail(waiting)
        /\ path' = [path EXCEPT ![b] = "local"]
        /\ mem' = mem \ {b}
        /\ local' = local \cup {b}
        /\ queued' = queued \cup {b}
        /\ late' = IF b \in answered THEN late \cup {b} ELSE late
        /\ wal' = Append(wal, b)
        /\ durN' = Len(wal) + 1
    /\ UNCHANGED <<state, nodeBuf, nodeDisk, remote, reported, indexed, answered, nodeUp,
                   degraded, failed, mode, lost, jbuf, journal, live, up, bounced>>

NodeFlush(b) ==
    /\ nodeUp
    /\ b \in nodeBuf
    /\ nodeBuf' = nodeBuf \ {b}
    /\ nodeDisk' = nodeDisk \cup {b}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, remote, reported, nodeUp, degraded,
                   failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

JournalFlush(b) ==
    /\ nodeUp
    /\ b \in jbuf
    /\ jbuf' = jbuf \ {b}
    /\ journal' = journal \cup {b}
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, waiting, wal, durN, lost, live, up, bounced, late>>

\* The receipt of a local commit waits for the node's flush as well.
Ack(b) ==
    /\ up
    /\ state[b] = "sent"
    /\ path[b] = "local"
    /\ b \in nodeDisk
    /\ HeadDurable(b)
    /\ EarlierDone(b)
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ UNCHANGED <<indexed, answered, path, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The receipt's wait for the node times out, which can happen at any moment:
\* the node is away, slow, or lost the frame.
AckDegraded(b) ==
    /\ up
    /\ state[b] = "sent"
    /\ path[b] = "local"
    /\ HeadDurable(b)
    /\ EarlierDone(b)
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ degraded' = degraded \cup {b}
    /\ UNCHANGED <<indexed, answered, path, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp,
                   failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The receipt of a durable commit: its block and every commit up to it are
\* durable.
AckDurable(b) ==
    /\ up
    /\ state[b] = "sent"
    /\ path[b] = "node"
    /\ HeadDurable(b)
    /\ EarlierDone(b)
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ UNCHANGED <<indexed, answered, path, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The local store's background flush, which tells the node to drop the
\* records it covers.
LocalFlush ==
    /\ up
    /\ durN < Len(wal)
    /\ durN' = Len(wal)
    /\ journal' = journal \ Range(wal)
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, waiting, wal, lost, jbuf, live, up, bounced, late>>

\* A block held in memory spills: written locally with a queue entry and
\* flushed, which also flushes every earlier commit.
Spill(b) ==
    /\ up
    /\ b \in mem
    /\ b \notin Range(waiting)
    /\ mem' = mem \ {b}
    /\ local' = local \cup {b}
    /\ queued' = queued \cup {b}
    /\ durN' = Len(wal)
    /\ UNCHANGED <<state, path, nodeBuf, nodeDisk, remote, reported, indexed, answered, nodeUp,
                   degraded, failed, mode, waiting, wal, lost, jbuf, journal, live, up, bounced, late>>

\* The switch, flipped at any moment.
SwitchOn ==
    /\ mode = "durable"
    /\ mode' = "on"
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

SwitchDurable ==
    /\ mode = "on"
    /\ mode' = "durable"
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* Durable mode sets the marker, flushed, before any receipt relies on the
\* journal.
JournalLive ==
    /\ up
    /\ mode = "durable"
    /\ ~live
    /\ live' = TRUE
    /\ durN' = Len(wal)
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, waiting, wal, lost, jbuf, journal, up, bounced, late>>

\* Back at "on", the marker clears, flushed, once every commit is flushed
\* locally and no durable publication waits.
JournalOff ==
    /\ up
    /\ mode = "on"
    /\ live
    /\ waiting = <<>>
    /\ durN = Len(wal)
    /\ live' = FALSE
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, up, bounced, late>>

NodeUpload(b) ==
    /\ nodeUp
    /\ b \in nodeDisk
    /\ b \notin remote
    /\ remote' = remote \cup {b}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, nodeBuf, nodeDisk, reported, nodeUp,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The node reports only what the remote holds, until it hears it applied.
NodeReport(b) ==
    /\ nodeUp
    /\ up
    /\ b \in remote
    /\ b \in queued \cup mem
    /\ b \notin answered
    /\ reported' = reported \cup {b}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, nodeBuf, nodeDisk, remote, nodeUp,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* A report first stores the pack's index entry, durably.
IndexReport(b) ==
    /\ b \in reported
    /\ indexed' = indexed \cup {b}
    /\ UNCHANGED <<answered, state, path, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* Then it drops the queue entry and the memory copy, and only then answers
\* the node. The local value may then be evicted, which is the case the
\* invariants have to survive, so it is modeled at once.
ApplyReport(b) ==
    /\ b \in reported
    /\ b \in indexed
    /\ queued' = queued \ {b}
    /\ local' = local \ {b}
    /\ mem' = mem \ {b}
    /\ reported' = reported \ {b}
    /\ answered' = answered \cup {b}
    /\ UNCHANGED <<indexed, state, path, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed,
                   mode, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* Or the apply fails, such as on a failed index lookup, and clears nothing.
\* The report goes unanswered, so the node repeats it.
ApplyFails(b) ==
    /\ b \in reported
    /\ reported' = reported \ {b}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, nodeBuf, nodeDisk, remote, nodeUp,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The local upload takes a queue entry past its deadline, whatever the node
\* says: a flapping node then strands nothing, and this carries every degraded
\* block and every block a node lost with its disk. Blocks are addressed by
\* their content, so a block both sides upload is only stored twice. The model
\* has no clock, so the deadline can pass at any moment.
LocalUpload(b) ==
    /\ b \in queued
    /\ b \in local
    /\ remote' = remote \cup {b}
    /\ indexed' = indexed \cup {b}
    /\ queued' = queued \ {b}
    /\ UNCHANGED <<answered, state, path, local, nodeBuf, nodeDisk, reported, nodeUp, degraded, failed,
                   mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The drain reads a queued block from the local disk and sends it to the node,
\* unless the local index names it on the remote already. The entry stays.
DrainPush(b) ==
    /\ nodeUp
    /\ b \in queued
    /\ b \in local
    /\ b \notin indexed
    /\ nodeBuf' = nodeBuf \cup {b}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, nodeDisk, remote, reported, nodeUp, degraded,
                   failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The drain drops the queue entry of a block the local index names on the
\* remote. The local value may then be evicted.
DrainClearLocated(b) ==
    /\ b \in queued
    /\ b \in indexed
    /\ queued' = queued \ {b}
    /\ local' = local \ {b}
    /\ UNCHANGED <<indexed, answered, state, path, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded,
                   failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The node goes away, losing what it had not flushed.
NodeDown ==
    /\ nodeUp
    /\ nodeUp' = FALSE
    /\ nodeBuf' = {}
    /\ jbuf' = {}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, nodeDisk, remote, reported, degraded,
                   failed, mode, mem, waiting, wal, durN, lost, journal, live, up, bounced, late>>

NodeBack ==
    /\ ~nodeUp
    /\ nodeUp' = TRUE
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, nodeBuf, nodeDisk, remote, reported,
                   degraded, failed, mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The node's disk dies: everything it flushed and had not uploaded is gone,
\* its journal too. A local crash that dropped the memory copies of blocks
\* the local index cannot yet find on the remote, followed by this, is the
\* accepted risk, which this excludes: the node's report is then the only
\* way to find them.
NodeDiskLost ==
    /\ failed = "none"
    /\ bounced \subseteq indexed
    /\ failed' = "node"
    /\ nodeBuf' = {}
    /\ nodeDisk' = {}
    /\ jbuf' = {}
    /\ journal' = {}
    /\ UNCHANGED <<indexed, answered, state, path, local, queued, remote, reported, nodeUp, degraded,
                   mode, mem, waiting, wal, durN, lost, live, up, bounced, late>>

\* The local disk dies, and its queue with it.
LocalDiskLost ==
    /\ failed = "none"
    /\ failed' = "local"
    /\ local' = {}
    /\ queued' = {}
    /\ reported' = {}
    /\ UNCHANGED <<indexed, answered, state, path, nodeBuf, nodeDisk, remote, nodeUp, degraded,
                   mode, mem, waiting, wal, durN, lost, jbuf, journal, live, up, bounced, late>>

\* The local process restarts on the same host: what it committed survives,
\* its memory does not. Reports it had not applied are forgotten; the node
\* repeats them. Receipts in flight are gone with their writers. After the
\* node's disk died, the memory copies are the only ones, which is the
\* accepted risk, so the model stops crashing there.
LocalRestart ==
    /\ up
    /\ failed # "node"
    /\ state' = [b \in Blocks |-> IF state[b] = "sent" THEN "gone" ELSE state[b]]
    /\ bounced' = bounced \cup (mem \ indexed)
    /\ mem' = {}
    /\ waiting' = <<>>
    /\ reported' = {}
    /\ UNCHANGED <<indexed, answered, path, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded,
                   failed, mode, wal, durN, lost, jbuf, journal, live, up, late>>

\* The host crashes: the commits not flushed locally are lost with the
\* memory, until the process starts again.
LocalCrash ==
    /\ up
    /\ failed # "node"
    /\ up' = FALSE
    /\ state' = [b \in Blocks |-> IF state[b] = "sent" THEN "gone" ELSE state[b]]
    /\ wal' = SubSeq(wal, 1, durN)
    /\ lost' = SubSeq(wal, durN + 1, Len(wal))
    /\ bounced' = bounced \cup (mem \ indexed)
    /\ mem' = {}
    /\ waiting' = <<>>
    /\ reported' = {}
    /\ UNCHANGED <<indexed, answered, path, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded,
                   failed, mode, durN, jbuf, journal, live, late>>

\* The restart replays the node's records past the local head, stopping at
\* the first it lacks, flushes them locally, and has the node drop the rest.
Replay ==
    /\ ~up
    /\ live
    /\ nodeUp
    /\ wal' = wal \o Held(lost)
    /\ durN' = Len(wal) + Len(Held(lost))
    /\ lost' = <<>>
    /\ journal' = {}
    /\ jbuf' = {}
    /\ up' = TRUE
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, waiting, live, bounced, late>>

\* With the marker unset no receipt relied on the journal: start at once.
StartLocal ==
    /\ ~up
    /\ ~live
    /\ lost' = <<>>
    /\ up' = TRUE
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, failed, mode, mem, waiting, wal, durN, jbuf, journal, live, bounced, late>>

\* The node stays away past the restart's bound: start on the local head,
\* journal off, and set the node's records aside. It spends the failure.
StartWithoutNode ==
    /\ ~up
    /\ live
    /\ ~nodeUp
    /\ failed = "none"
    /\ failed' = "node"
    /\ lost' = <<>>
    /\ journal' = {}
    /\ jbuf' = {}
    /\ live' = FALSE
    /\ up' = TRUE
    /\ UNCHANGED <<state, path, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered,
                   nodeUp, degraded, mode, mem, waiting, wal, durN, bounced, late>>

Next ==
    \/ \E b \in Blocks :
        \/ Submit(b) \/ SubmitDurable(b) \/ NodeFlush(b) \/ JournalFlush(b)
        \/ Ack(b) \/ AckDegraded(b) \/ AckDurable(b) \/ Spill(b)
        \/ NodeUpload(b) \/ NodeReport(b) \/ IndexReport(b) \/ ApplyReport(b) \/ ApplyFails(b)
        \/ LocalUpload(b) \/ DrainPush(b) \/ DrainClearLocated(b)
    \/ CommitDurable \/ PushFails \/ LocalFlush
    \/ SwitchOn \/ SwitchDurable \/ JournalLive \/ JournalOff
    \/ NodeDown \/ NodeBack \/ NodeDiskLost \/ LocalDiskLost
    \/ LocalRestart \/ LocalCrash \/ Replay \/ StartLocal \/ StartWithoutNode

Spec == Init /\ [][Next]_vars

\* Fair runs finish what they start; a node that goes away comes back, and a
\* stopped process starts again.
FairSpec ==
    /\ Spec
    /\ WF_vars(NodeBack) /\ WF_vars(CommitDurable \/ PushFails) /\ WF_vars(LocalFlush)
    /\ WF_vars(Replay \/ StartLocal \/ StartWithoutNode)
    /\ \A b \in Blocks :
        /\ WF_vars(NodeFlush(b)) /\ WF_vars(JournalFlush(b))
        /\ WF_vars(AckDegraded(b)) /\ WF_vars(AckDurable(b)) /\ WF_vars(Spill(b))
        /\ WF_vars(NodeUpload(b)) /\ WF_vars(NodeReport(b))
        /\ WF_vars(IndexReport(b)) /\ WF_vars(ApplyReport(b))
        /\ WF_vars(LocalUpload(b))

\* While the local disk lives, every acknowledged block has a copy there, in
\* the local memory, on the node's disk or on the remote. A durable block's
\* two copies are the memory and the node's disk until the remote holds it.
NoAckedBlockLost ==
    failed # "local" =>
        \A b \in Blocks : state[b] = "acked" => b \in local \cup mem \cup nodeDisk \cup remote

\* The local process can read every acknowledged block: from the local store,
\* its memory, the node by key, or the remote through its index. A block the
\* remote holds and the index does not name is a block not found.
AckedStaysReadable ==
    (failed # "local" /\ up) =>
        \A b \in Blocks : state[b] = "acked" => b \in local \cup mem \cup nodeDisk \cup indexed

\* The durable commit's condition: a block acknowledged through the node
\* survives the local disk, on the node's disk or the remote.
AckedSurvivesLocalDisk ==
    failed = "local" =>
        \A b \in Blocks : (state[b] = "acked" /\ b \notin degraded)
            => b \in nodeDisk \cup remote

\* No local commit names a block nobody holds, whatever crashed.
CommittedBlocksHeld ==
    failed # "local" =>
        \A b \in Range(wal) : b \in local \cup mem \cup nodeDisk \cup remote

\* Every acknowledged commit is in the local store once it serves, across
\* crashes and replays, unless the one failure was spent.
AckedCommitSurvives ==
    (failed = "none" /\ up) =>
        \A b \in Blocks : state[b] = "acked" => InWal(b)

\* A local block's queue entry goes away only when the remote holds it.
QueueHoldsUntilRemote ==
    failed # "local" =>
        \A b \in local : b \in queued \cup remote

\* The node stops reporting a block only once its queue entry and memory copy
\* are gone, so no answered report leaves its block held behind the upload.
\* A push that timed out after the node flushed, uploaded and reported its
\* block queues it again; the upload drops that entry in its turn.
AnsweredCleared ==
    \A b \in Blocks : b \in answered => (b \notin mem /\ (b \in queued => b \in late))

\* No receipt waits forever.
ReceiptsResolve == \A b \in Blocks : state[b] = "sent" ~> state[b] \in {"acked", "gone"}

\* No queue entry is stuck: it reaches the remote, or the local disk died.
QueueDrains ==
    \A b \in Blocks : b \in queued ~> (b \in remote \/ failed = "local")

\* No block stays in memory forever: the node reports it or it spills.
MemoryDrains ==
    \A b \in Blocks : b \in mem ~> b \notin mem

TypeOK ==
    /\ state \in [Blocks -> {"new", "sent", "acked", "gone"}]
    /\ path \in [Blocks -> {"none", "local", "node"}]
    /\ local \subseteq Blocks /\ queued \subseteq Blocks
    /\ nodeBuf \subseteq Blocks /\ nodeDisk \subseteq Blocks
    /\ remote \subseteq Blocks /\ reported \subseteq Blocks
    /\ indexed \subseteq Blocks /\ answered \subseteq Blocks
    /\ degraded \subseteq Blocks /\ mem \subseteq Blocks
    /\ jbuf \subseteq Blocks /\ journal \subseteq Blocks
    /\ failed \in {"none", "node", "local"}
    /\ mode \in {"on", "durable"}
    /\ durN \in 0..Len(wal)
    /\ live \in BOOLEAN /\ up \in BOOLEAN
    /\ bounced \subseteq Blocks /\ late \subseteq Blocks
=============================================================================
