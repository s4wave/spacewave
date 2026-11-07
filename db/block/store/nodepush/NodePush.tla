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

   A drain sends the local backlog to the node from the local disk: the blocks
   committed while the node was away or slow. It never drops the queue entry
   of a block it sent; the report's apply does, as for any pushed block, so a
   push the node loses leaves the entry for the local upload. It drops an entry
   itself only for a block the local index names on the remote, as the upload's
   skip does.

   NodePush.mutants.json lists the guards each invariant depends on. *)
EXTENDS FiniteSets, Naturals

CONSTANTS Blocks

VARIABLES
    state,    \* block -> "new" | "sent" | "acked"
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
    failed    \* the disk that died: "none", "node" or "local"

vars == <<state, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, answered, nodeUp, degraded, failed>>

Init ==
    /\ state = [b \in Blocks |-> "new"]
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

\* The commit writes the block and its queue entry in one local transaction,
\* and sends the block to the node from memory beside it.
Submit(b) ==
    /\ state[b] = "new"
    /\ state' = [state EXCEPT ![b] = "sent"]
    /\ local' = local \cup {b}
    /\ queued' = queued \cup {b}
    /\ nodeBuf' = IF nodeUp THEN nodeBuf \cup {b} ELSE nodeBuf
    /\ UNCHANGED <<indexed, answered, nodeDisk, remote, reported, nodeUp, degraded, failed>>

NodeFlush(b) ==
    /\ nodeUp
    /\ b \in nodeBuf
    /\ nodeBuf' = nodeBuf \ {b}
    /\ nodeDisk' = nodeDisk \cup {b}
    /\ UNCHANGED <<indexed, answered, state, local, queued, remote, reported, nodeUp, degraded, failed>>

\* The receipt waits for the node's flush as well as the local commit.
Ack(b) ==
    /\ state[b] = "sent"
    /\ b \in nodeDisk
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ UNCHANGED <<indexed, answered, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, failed>>

\* The receipt's wait for the node times out, which can happen at any moment:
\* the node is away, slow, or lost the frame.
AckDegraded(b) ==
    /\ state[b] = "sent"
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ degraded' = degraded \cup {b}
    /\ UNCHANGED <<indexed, answered, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, failed>>

NodeUpload(b) ==
    /\ nodeUp
    /\ b \in nodeDisk
    /\ b \notin remote
    /\ remote' = remote \cup {b}
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeBuf, nodeDisk, reported, nodeUp, degraded, failed>>

\* The node reports only what the remote holds, until it hears it applied.
NodeReport(b) ==
    /\ nodeUp
    /\ b \in remote
    /\ b \in queued
    /\ b \notin answered
    /\ reported' = reported \cup {b}
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed>>

\* A report first stores the pack's index entry, durably.
IndexReport(b) ==
    /\ b \in reported
    /\ indexed' = indexed \cup {b}
    /\ UNCHANGED <<answered, state, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, failed>>

\* Then it drops the queue entry, and only then answers the node. The local
\* value may then be evicted, which is the case the invariants have to
\* survive, so it is modeled at once.
ApplyReport(b) ==
    /\ b \in reported
    /\ b \in indexed
    /\ queued' = queued \ {b}
    /\ local' = local \ {b}
    /\ reported' = reported \ {b}
    /\ answered' = answered \cup {b}
    /\ UNCHANGED <<indexed, state, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed>>

\* Or the apply fails, such as on a failed index lookup, and clears nothing.
\* The report goes unanswered, so the node repeats it.
ApplyFails(b) ==
    /\ b \in reported
    /\ reported' = reported \ {b}
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed>>

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
    /\ UNCHANGED <<answered, state, local, nodeBuf, nodeDisk, reported, nodeUp, degraded, failed>>

\* The drain reads a queued block from the local disk and sends it to the node,
\* unless the local index names it on the remote already. The entry stays.
DrainPush(b) ==
    /\ nodeUp
    /\ b \in queued
    /\ b \in local
    /\ b \notin indexed
    /\ nodeBuf' = nodeBuf \cup {b}
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeDisk, remote, reported, nodeUp, degraded, failed>>

\* The drain drops the queue entry of a block the local index names on the
\* remote. The local value may then be evicted.
DrainClearLocated(b) ==
    /\ b \in queued
    /\ b \in indexed
    /\ queued' = queued \ {b}
    /\ local' = local \ {b}
    /\ UNCHANGED <<indexed, answered, state, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, failed>>

\* The node goes away, losing what it had not flushed.
NodeDown ==
    /\ nodeUp
    /\ nodeUp' = FALSE
    /\ nodeBuf' = {}
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeDisk, remote, reported, degraded, failed>>

NodeBack ==
    /\ ~nodeUp
    /\ nodeUp' = TRUE
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeBuf, nodeDisk, remote, reported, degraded, failed>>

\* The node's disk dies: everything it flushed and had not uploaded is gone.
NodeDiskLost ==
    /\ failed = "none"
    /\ failed' = "node"
    /\ nodeBuf' = {}
    /\ nodeDisk' = {}
    /\ UNCHANGED <<indexed, answered, state, local, queued, remote, reported, nodeUp, degraded>>

\* The local disk dies, and its queue with it.
LocalDiskLost ==
    /\ failed = "none"
    /\ failed' = "local"
    /\ local' = {}
    /\ queued' = {}
    /\ reported' = {}
    /\ UNCHANGED <<indexed, answered, state, nodeBuf, nodeDisk, remote, nodeUp, degraded>>

\* A restart of the local process keeps what is durable and forgets reports
\* it had not applied; the node repeats them. Blocks in flight stay "sent",
\* and the local transaction already holds them.
LocalRestart ==
    /\ reported' = {}
    /\ UNCHANGED <<indexed, answered, state, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed>>

Next ==
    \/ \E b \in Blocks :
        \/ Submit(b) \/ NodeFlush(b) \/ Ack(b) \/ AckDegraded(b)
        \/ NodeUpload(b) \/ NodeReport(b) \/ IndexReport(b) \/ ApplyReport(b) \/ ApplyFails(b)
        \/ LocalUpload(b) \/ DrainPush(b) \/ DrainClearLocated(b)
    \/ NodeDown \/ NodeBack \/ NodeDiskLost \/ LocalDiskLost \/ LocalRestart

Spec == Init /\ [][Next]_vars

\* Fair runs finish what they start; a node that goes away comes back.
FairSpec ==
    /\ Spec
    /\ WF_vars(NodeBack)
    /\ \A b \in Blocks :
        /\ WF_vars(NodeFlush(b)) /\ WF_vars(AckDegraded(b))
        /\ WF_vars(NodeUpload(b)) /\ WF_vars(NodeReport(b))
        /\ WF_vars(IndexReport(b)) /\ WF_vars(ApplyReport(b))
        /\ WF_vars(LocalUpload(b))

\* While the local disk lives, every acknowledged block has a copy there or on
\* the remote. The node's disk never counts here: it can die.
NoAckedBlockLost ==
    failed # "local" =>
        \A b \in Blocks : state[b] = "acked" => b \in local \cup remote

\* The local process can read every acknowledged block: from the local store,
\* or from the remote through its index. A block the remote holds and the index
\* does not name is a block not found.
AckedStaysReadable ==
    failed # "local" =>
        \A b \in Blocks : state[b] = "acked" => b \in local \cup indexed

\* The durable commit's condition: a block acknowledged through the node
\* survives the local disk, on the node's disk or the remote.
AckedSurvivesLocalDisk ==
    failed = "local" =>
        \A b \in Blocks : (state[b] = "acked" /\ b \notin degraded)
            => b \in nodeDisk \cup remote

\* A queue entry goes away only when the remote holds the block.
QueueHoldsUntilRemote ==
    failed # "local" =>
        \A b \in Blocks : state[b] # "new" => b \in queued \cup remote

\* The node stops reporting a block only once its queue entry is gone, so no
\* answered report leaves its block queued behind the upload's backlog.
AnsweredCleared ==
    \A b \in Blocks : b \in answered => b \notin queued

\* No receipt waits forever.
ReceiptsResolve == \A b \in Blocks : state[b] = "sent" ~> state[b] = "acked"

\* No queue entry is stuck: it reaches the remote, or the local disk died.
QueueDrains ==
    \A b \in Blocks : b \in queued ~> (b \in remote \/ failed = "local")

TypeOK ==
    /\ state \in [Blocks -> {"new", "sent", "acked"}]
    /\ local \subseteq Blocks /\ queued \subseteq Blocks
    /\ nodeBuf \subseteq Blocks /\ nodeDisk \subseteq Blocks
    /\ remote \subseteq Blocks /\ reported \subseteq Blocks
    /\ indexed \subseteq Blocks /\ answered \subseteq Blocks
    /\ degraded \subseteq Blocks
    /\ failed \in {"none", "node", "local"}
=============================================================================
