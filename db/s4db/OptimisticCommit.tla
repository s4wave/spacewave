---- MODULE OptimisticCommit ----
\* Optimistic write transactions. A write transaction opened optimistically
\* reads a snapshot without the writer lock and buffers its writes. Commit
\* takes the lock and validates: no commit after the snapshot changed a key
\* the transaction read. The handle keeps the changed keys of recent commits
\* in a log that covers every commit after from; a snapshot older than from
\* cannot be validated and conflicts. A conflict discards the attempt, and
\* the caller runs the body again; after Retries conflicts it runs holding
\* the lock from the start, as every write transaction did before, so each
\* transaction commits.
\*
\* A key set stands for both point reads and scanned prefixes: a scan reads
\* every key in its range, present or not, so a commit that inserts into the
\* range changes a key the scan read.
\*
\* The log loses entries two ways. Trim drops the oldest to bound its memory.
\* Jump models a tail that adopts another process's checkpoint past
\* commits this handle never applied: the log restarts empty after them.
\*
\* Each body writes, to every key of its write set, one more than the sum of
\* the values it read. Serial applies the same bodies in commit order to the
\* values each would read running alone; the store must equal it.
\*
\* Not modeled: the lock queue (WriterLock.tla), how long holds last
\* (WriterStall.tla), and crash and recovery (Durability.tla).
\*
\* The transactions: scan reads the prefix of keys 1 and 2 and writes what
\* it found to key 3; insert writes key 2 into the scanned range without
\* reading; bump reads key 3 and writes it back, racing scan's write.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Retries, Jumps, LogMax
Txs == {"scan", "insert", "bump"}
Keys == {1, 2, 3}
Reads == [t \in Txs |-> CASE t = "scan" -> {1, 2} [] t = "insert" -> {} [] t = "bump" -> {3}]
Writes == [t \in Txs |-> CASE t = "scan" -> {3} [] t = "insert" -> {2} [] t = "bump" -> {3}]

VARIABLES val, serial, seq, log, from, lock, pc, snapSeq, snapVal, tries, jumps
vars == <<val, serial, seq, log, from, lock, pc, snapSeq, snapVal, tries, jumps>>

\* Sum of the values of the keys in S.
RECURSIVE SumOf(_, _)
SumOf(v, S) ==
  IF S = {} THEN 0
  ELSE LET k == CHOOSE x \in S : TRUE IN v[k] + SumOf(v, S \ {k})

\* The store after t commits having read the values r.
Apply(v, t, r) ==
  [k \in Keys |-> IF k \in Writes[t] THEN SumOf(r, Reads[t]) + 1 ELSE v[k]]

\* Whether a logged commit after s changed a key in R, or the log cannot
\* tell because it starts after s.
Conflicts(s, R) ==
  \/ s < from
  \/ \E i \in 1..Len(log) : log[i].seq > s /\ log[i].keys \cap R # {}

Init ==
  /\ val = [k \in Keys |-> 0]
  /\ serial = val
  /\ seq = 0 /\ log = <<>> /\ from = 0
  /\ lock = "none"
  /\ pc = [t \in Txs |-> "idle"]
  /\ snapSeq = [t \in Txs |-> 0]
  /\ snapVal = [t \in Txs |-> [k \in Keys |-> 0]]
  /\ tries = [t \in Txs |-> 0]
  /\ jumps = 0

\* Record a commit of keys W as the next commit, in the store, the log and
\* the serial order.
Commit(W, newVal, newSerial) ==
  /\ val' = newVal /\ serial' = newSerial
  /\ seq' = seq + 1
  /\ log' = Append(log, [seq |-> seq + 1, keys |-> W])

\* Open an optimistic attempt: a snapshot, no lock.
Begin(t) ==
  /\ pc[t] = "idle" /\ tries[t] < Retries
  /\ pc' = [pc EXCEPT ![t] = "body"]
  /\ snapSeq' = [snapSeq EXCEPT ![t] = seq]
  /\ snapVal' = [snapVal EXCEPT ![t] = val]
  /\ UNCHANGED <<val, serial, seq, log, from, lock, tries, jumps>>

\* Take the lock and validate. On success append the commit; on a conflict
\* discard the attempt for the caller to run again.
Validate(t) ==
  /\ pc[t] = "body" /\ lock = "none"
  /\ IF Conflicts(snapSeq[t], Reads[t])
     THEN /\ pc' = [pc EXCEPT ![t] = "idle"]
          /\ tries' = [tries EXCEPT ![t] = @ + 1]
          /\ UNCHANGED <<val, serial, seq, log>>
     ELSE /\ Commit(Writes[t], Apply(val, t, snapVal[t]), Apply(serial, t, serial))
          /\ pc' = [pc EXCEPT ![t] = "done"]
          /\ UNCHANGED tries
  /\ UNCHANGED <<from, lock, snapSeq, snapVal, jumps>>

\* After Retries conflicts, take the lock before reading.
Lock(t) ==
  /\ pc[t] = "idle" /\ tries[t] = Retries /\ lock = "none"
  /\ lock' = t
  /\ pc' = [pc EXCEPT ![t] = "locked"]
  /\ snapSeq' = [snapSeq EXCEPT ![t] = seq]
  /\ snapVal' = [snapVal EXCEPT ![t] = val]
  /\ UNCHANGED <<val, serial, seq, log, from, tries, jumps>>

\* Commit under the lock held since the reads, then release it.
LockedCommit(t) ==
  /\ pc[t] = "locked"
  /\ Commit(Writes[t], Apply(val, t, snapVal[t]), Apply(serial, t, serial))
  /\ lock' = "none"
  /\ pc' = [pc EXCEPT ![t] = "done"]
  /\ UNCHANGED <<from, snapSeq, snapVal, tries, jumps>>

\* Drop the oldest log entry; the log now covers only the commits after it.
Trim ==
  /\ Len(log) > 0
  /\ from' = Head(log).seq /\ log' = Tail(log)
  /\ UNCHANGED <<val, serial, seq, lock, pc, snapSeq, snapVal, tries, jumps>>

\* Another process commits to key k and checkpoints past it, and this
\* handle adopts the checkpoint without the commit's record, between two
\* holds of the lock.
Jump(k) ==
  /\ jumps < Jumps /\ lock = "none"
  /\ val' = [val EXCEPT ![k] = @ + 1] /\ serial' = [serial EXCEPT ![k] = @ + 1]
  /\ seq' = seq + 1 /\ log' = <<>> /\ from' = seq + 1
  /\ jumps' = jumps + 1
  /\ UNCHANGED <<lock, pc, snapSeq, snapVal, tries>>

Done ==
  /\ \A t \in Txs : pc[t] = "done"
  /\ UNCHANGED vars

Next ==
  \/ \E t \in Txs : Begin(t) \/ Validate(t) \/ Lock(t) \/ LockedCommit(t)
  \/ Len(log) > LogMax /\ Trim
  \/ \E k \in Keys : Jump(k)
  \/ Done

\* Every commit equals running the bodies alone in commit order.
Serializable == val = serial

\* The log covers exactly the commits after from.
LogCovers ==
  /\ from <= seq
  /\ Len(log) = seq - from
  /\ \A i \in 1..Len(log) : log[i].seq = from + i

\* Only the pessimistic attempt holds the lock across its reads.
LockHeld == \A t \in Txs : (pc[t] = "locked") <=> lock = t

Safety == Serializable /\ LogCovers /\ LockHeld

\* Every transaction commits.
Finishes == <>(\A t \in Txs : pc[t] = "done")

Spec == Init /\ [][Next]_vars
  /\ \A t \in Txs : WF_vars(Begin(t)) /\ WF_vars(Validate(t)) /\ WF_vars(Lock(t)) /\ WF_vars(LockedCommit(t))
====
