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

   The receipt's success is the local commit alone; the node only adds a wait.
   A push is also skipped, and its receipt is then degraded, when the node was
   quiet after a timeout, when the entry's references are not yet known, or when
   pushing is off, so a block may never reach the node at all.

   A read of a pack's byte range goes to the node first. The node answers from
   its disk only after the range matches the pack's chunk sums, so a flipped
   byte on the node's disk is a miss, never an answer: the damaged copy is
   dropped and the node fills the pack again from the remote, while the local
   store reads the remote meanwhile. A report the local store cannot apply
   yet, a PackApplied that says failed, is forgotten like one lost to a restart
   and the node repeats it later.

   AckBeforeFlush, DropOnFlush, DropBeforeIndex and ServeUnchecked are mutants:
   each must fail an invariant. *)
EXTENDS FiniteSets, Naturals

CONSTANTS Blocks, AckBeforeFlush, DropOnFlush, DropBeforeIndex, ServeUnchecked

VARIABLES
    state,    \* block -> "new" | "sent" | "acked"
    local,    \* blocks durable on the local store
    queued,   \* blocks with a local upload queue entry
    nodeBuf,  \* blocks the node received and has not flushed
    nodeDisk, \* blocks flushed to the node's disk
    remote,   \* blocks the remote store holds
    reported, \* blocks the node reported the remote holds, not yet applied
    indexed,  \* blocks the local pack index can find on the remote
    nodeUp,   \* the node answers
    degraded, \* blocks acknowledged without the node's flush
    damaged,  \* blocks whose copy on the node's disk has a flipped byte
    served,   \* blocks a read took from a damaged copy
    failed    \* the disk that died: "none", "node" or "local"

vars == <<state, local, queued, nodeBuf, nodeDisk, remote, reported, indexed, nodeUp, degraded, damaged, served, failed>>

Init ==
    /\ state = [b \in Blocks |-> "new"]
    /\ local = {}
    /\ queued = {}
    /\ nodeBuf = {}
    /\ nodeDisk = {}
    /\ remote = {}
    /\ reported = {}
    /\ indexed = {}
    /\ nodeUp = TRUE
    /\ degraded = {}
    /\ damaged = {}
    /\ served = {}
    /\ failed = "none"

\* The commit writes the block and its queue entry in one local transaction,
\* and sends the block to the node from memory beside it.
Submit(b) ==
    /\ state[b] = "new"
    /\ state' = [state EXCEPT ![b] = "sent"]
    /\ local' = local \cup {b}
    /\ queued' = queued \cup {b}
    /\ nodeBuf' = IF nodeUp THEN nodeBuf \cup {b} ELSE nodeBuf
    /\ UNCHANGED <<indexed, nodeDisk, remote, reported, nodeUp, degraded, failed, damaged, served>>

\* The commit skips the push: the node was quiet after a timeout, the entry's
\* references are not known yet, or pushing is off. The local transaction is
\* the same.
SubmitQuiet(b) ==
    /\ state[b] = "new"
    /\ state' = [state EXCEPT ![b] = "sent"]
    /\ local' = local \cup {b}
    /\ queued' = queued \cup {b}
    /\ UNCHANGED <<indexed, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, failed, damaged, served>>

\* A flush writes the whole file again, so it leaves no damaged copy.
NodeFlush(b) ==
    /\ nodeUp
    /\ b \in nodeBuf
    /\ nodeBuf' = nodeBuf \ {b}
    /\ nodeDisk' = nodeDisk \cup {b}
    /\ damaged' = damaged \ {b}
    /\ UNCHANGED <<indexed, state, local, queued, remote, reported, nodeUp, degraded, failed, served>>

\* The receipt waits for the node's flush as well as the local commit.
Ack(b) ==
    /\ state[b] = "sent"
    /\ b \in nodeDisk \/ AckBeforeFlush
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ UNCHANGED <<indexed, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, failed, damaged, served>>

\* The receipt's wait for the node times out, which can happen at any moment:
\* the node is away, slow, or lost the frame.
AckDegraded(b) ==
    /\ state[b] = "sent"
    /\ state' = [state EXCEPT ![b] = "acked"]
    /\ degraded' = degraded \cup {b}
    /\ UNCHANGED <<indexed, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, failed, damaged, served>>

NodeUpload(b) ==
    /\ nodeUp
    /\ b \in nodeDisk
    /\ b \notin remote
    /\ remote' = remote \cup {b}
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, nodeDisk, reported, nodeUp, degraded, failed, damaged, served>>

\* The node reports only what the remote holds, or under the mutant, what it
\* flushed.
NodeReport(b) ==
    /\ nodeUp
    /\ b \in remote \/ (DropOnFlush /\ b \in nodeDisk)
    /\ b \in queued
    /\ reported' = reported \cup {b}
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed, damaged, served>>

\* A report first stores the pack's index entry, durably.
IndexReport(b) ==
    /\ b \in reported
    /\ indexed' = indexed \cup {b}
    /\ UNCHANGED <<state, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, failed, damaged, served>>

\* Then it drops the queue entry. The local value may then be evicted, which
\* is the case the invariants have to survive, so it is modeled at once.
ApplyReport(b) ==
    /\ b \in reported
    /\ b \in indexed \/ DropBeforeIndex
    /\ queued' = queued \ {b}
    /\ local' = local \ {b}
    /\ reported' = reported \ {b}
    /\ UNCHANGED <<indexed, state, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed, damaged, served>>

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
    /\ UNCHANGED <<state, local, nodeBuf, nodeDisk, reported, nodeUp, degraded, failed, damaged, served>>

\* The node goes away, losing what it had not flushed.
NodeDown ==
    /\ nodeUp
    /\ nodeUp' = FALSE
    /\ nodeBuf' = {}
    /\ UNCHANGED <<indexed, state, local, queued, nodeDisk, remote, reported, degraded, failed, damaged, served>>

NodeBack ==
    /\ ~nodeUp
    /\ nodeUp' = TRUE
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, nodeDisk, remote, reported, degraded, failed, damaged, served>>

\* The node's disk dies: everything it flushed and had not uploaded is gone.
NodeDiskLost ==
    /\ failed = "none"
    /\ failed' = "node"
    /\ nodeBuf' = {}
    /\ nodeDisk' = {}
    /\ damaged' = {}
    /\ UNCHANGED <<indexed, state, local, queued, remote, reported, nodeUp, degraded, served>>

\* A byte on the node's disk flips. The copy is no longer a copy, so this is the
\* node's one disk failure, like a lost disk.
NodeCorrupt(b) ==
    /\ failed = "none"
    /\ b \in nodeDisk
    /\ failed' = "node"
    /\ damaged' = damaged \cup {b}
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, nodeDisk, remote, reported, nodeUp, degraded, served>>

\* A read of a damaged copy. The range fails its chunk sums, so the node drops
\* the copy and answers a miss; under the mutant the bytes go out as an answer.
NodeRead(b) ==
    /\ nodeUp
    /\ b \in damaged
    /\ IF ServeUnchecked
          THEN /\ served' = served \cup {b}
               /\ UNCHANGED <<nodeDisk, damaged>>
          ELSE /\ nodeDisk' = nodeDisk \ {b}
               /\ damaged' = damaged \ {b}
               /\ UNCHANGED served
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, remote, reported, nodeUp, degraded, failed>>

\* After a miss the node fills the pack again from the remote.
NodeFill(b) ==
    /\ nodeUp
    /\ b \in remote
    /\ b \notin nodeDisk
    /\ nodeDisk' = nodeDisk \cup {b}
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, remote, reported, nodeUp, degraded, damaged, served, failed>>

\* The local disk dies, and its queue with it.
LocalDiskLost ==
    /\ failed = "none"
    /\ failed' = "local"
    /\ local' = {}
    /\ queued' = {}
    /\ reported' = {}
    /\ UNCHANGED <<indexed, state, nodeBuf, nodeDisk, remote, nodeUp, degraded, damaged, served>>

\* A restart of the local process keeps what is durable and forgets reports
\* it had not applied; the node repeats them. A PackApplied that says failed
\* has the same effect on one report. Blocks in flight stay "sent", and the
\* local transaction already holds them.
LocalRestart ==
    /\ reported' = {}
    /\ UNCHANGED <<indexed, state, local, queued, nodeBuf, nodeDisk, remote, nodeUp, degraded, failed, damaged, served>>

Next ==
    \/ \E b \in Blocks :
        \/ Submit(b) \/ SubmitQuiet(b) \/ NodeFlush(b) \/ Ack(b) \/ AckDegraded(b)
        \/ NodeUpload(b) \/ NodeReport(b) \/ IndexReport(b) \/ ApplyReport(b)
        \/ LocalUpload(b) \/ NodeCorrupt(b) \/ NodeRead(b) \/ NodeFill(b)
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

\* A read never takes bytes from a damaged copy.
ReadsVerified == served = {}

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
    /\ indexed \subseteq Blocks
    /\ degraded \subseteq Blocks
    /\ damaged \subseteq nodeDisk /\ served \subseteq Blocks
    /\ failed \in {"none", "node", "local"}
=============================================================================
