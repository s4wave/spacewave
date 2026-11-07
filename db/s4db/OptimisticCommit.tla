---- MODULE OptimisticCommit ----
\* Optimistic write transactions. A write transaction opened optimistically
\* reads a snapshot without the writer lock and buffers its writes. Commit
\* takes the lock and validates against the state there: no commit after the
\* snapshot changed a key the transaction read. The memtable holds each key's
\* newest change since the last flush with the commit that made it, so it
\* answers the question for every key a flush has not dropped; a flush past
\* the snapshot may have dropped one, and conflicts. A conflict discards the
\* attempt, and the caller runs the body again; after Retries conflicts it
\* runs holding the lock from the start, so each transaction commits.
\*
\* A key set stands for both point reads and scanned prefixes: a scan reads
\* every key in its range, present or not, so a commit that inserts into the
\* range changes a key the scan read.
\*
\* Commits of other processes reach the memtable through the tail like this
\* handle's own, and Other models one: it changes a key between two holds of
\* the lock. Flush, by this handle or another, drops the entries at or below
\* the commit it covers.
\*
\* Each body writes, to every key of its write set, one more than the sum of
\* the values it read. Serial applies the same bodies in commit order to the
\* values each would read running alone; the store must equal it.
\*
\* Not modeled: the lock queue (WriterLock.tla), how long holds last
\* (WriterStall.tla), crash and recovery (Durability.tla), and the memtable's
\* own reads and flushes (Memtable.tla).
\*
\* A deferred write runs at the commit, after validation and holding the
\* lock, and reads the state there; its reads join no read set.
\*
\* The transactions: scan reads the prefix of keys 1 and 2 and writes what
\* it found to key 3; insert writes key 2 into the scanned range without
\* reading; bump reads key 3 and writes it back, racing scan's write; add
\* defers adding one to key 3, so it never conflicts with them.
EXTENDS Naturals, FiniteSets
CONSTANTS Retries, Others, Flushes
Txs == {"scan", "insert", "bump", "add"}
Keys == {1, 2, 3}
Reads == [t \in Txs |-> CASE t = "scan" -> {1, 2} [] t = "bump" -> {3} [] OTHER -> {}]
Writes == [t \in Txs |-> CASE t = "scan" -> {3} [] t = "insert" -> {2} [] t = "bump" -> {3} [] OTHER -> {}]
Defers == [t \in Txs |-> IF t = "add" THEN {3} ELSE {}]

\* changed is the commit that last changed each key, ghost state the store
\* does not keep; mem is the memtable's entry seq for each key, 0 for none.
VARIABLES val, serial, seq, changed, mem, flushed, flushes, lock, pc, snapSeq, snapVal, tries, others
vars == <<val, serial, seq, changed, mem, flushed, flushes, lock, pc, snapSeq, snapVal, tries, others>>

\* Sum of the values of the keys in S.
RECURSIVE SumOf(_, _)
SumOf(v, S) ==
  IF S = {} THEN 0
  ELSE LET k == CHOOSE x \in S : TRUE IN v[k] + SumOf(v, S \ {k})

\* The store after t commits having read the values r, then runs its
\* deferred writes on the result.
Apply(v, t, r) ==
  LET w == [k \in Keys |-> IF k \in Writes[t] THEN SumOf(r, Reads[t]) + 1 ELSE v[k]]
  IN [k \in Keys |-> IF k \in Defers[t] THEN w[k] + 1 ELSE w[k]]

\* changedSince: a flush past s, or a memtable entry after s for a key in R.
Conflicts(s, R) ==
  \/ flushed > s
  \/ \E k \in R : mem[k] > s

Init ==
  /\ val = [k \in Keys |-> 0]
  /\ serial = val
  /\ seq = 0
  /\ changed = [k \in Keys |-> 0]
  /\ mem = [k \in Keys |-> 0]
  /\ flushed = 0 /\ flushes = 0
  /\ lock = "none"
  /\ pc = [t \in Txs |-> "idle"]
  /\ snapSeq = [t \in Txs |-> 0]
  /\ snapVal = [t \in Txs |-> [k \in Keys |-> 0]]
  /\ tries = [t \in Txs |-> 0]
  /\ others = 0

\* Record a commit of keys W as the next commit, in the store, the memtable
\* and the serial order.
Commit(W, newVal, newSerial) ==
  /\ val' = newVal /\ serial' = newSerial
  /\ seq' = seq + 1
  /\ changed' = [k \in Keys |-> IF k \in W THEN seq + 1 ELSE changed[k]]
  /\ mem' = [k \in Keys |-> IF k \in W THEN seq + 1 ELSE mem[k]]

\* Open an optimistic attempt: a snapshot, no lock.
Begin(t) ==
  /\ pc[t] = "idle" /\ tries[t] < Retries
  /\ pc' = [pc EXCEPT ![t] = "body"]
  /\ snapSeq' = [snapSeq EXCEPT ![t] = seq]
  /\ snapVal' = [snapVal EXCEPT ![t] = val]
  /\ UNCHANGED <<val, serial, seq, changed, mem, flushed, flushes, lock, tries, others>>

\* Take the lock and validate. On success append the commit; on a conflict
\* discard the attempt for the caller to run again.
Validate(t) ==
  /\ pc[t] = "body" /\ lock = "none"
  /\ IF Conflicts(snapSeq[t], Reads[t])
     THEN /\ pc' = [pc EXCEPT ![t] = "idle"]
          /\ tries' = [tries EXCEPT ![t] = @ + 1]
          /\ UNCHANGED <<val, serial, seq, changed, mem>>
     ELSE /\ Commit(Writes[t] \cup Defers[t], Apply(val, t, snapVal[t]), Apply(serial, t, serial))
          /\ pc' = [pc EXCEPT ![t] = "done"]
          /\ UNCHANGED tries
  /\ UNCHANGED <<flushed, flushes, lock, snapSeq, snapVal, others>>

\* After Retries conflicts, take the lock before reading.
Lock(t) ==
  /\ pc[t] = "idle" /\ tries[t] = Retries /\ lock = "none"
  /\ lock' = t
  /\ pc' = [pc EXCEPT ![t] = "locked"]
  /\ snapSeq' = [snapSeq EXCEPT ![t] = seq]
  /\ snapVal' = [snapVal EXCEPT ![t] = val]
  /\ UNCHANGED <<val, serial, seq, changed, mem, flushed, flushes, tries, others>>

\* Commit under the lock held since the reads, then release it.
LockedCommit(t) ==
  /\ pc[t] = "locked"
  /\ Commit(Writes[t] \cup Defers[t], Apply(val, t, snapVal[t]), Apply(serial, t, serial))
  /\ lock' = "none"
  /\ pc' = [pc EXCEPT ![t] = "done"]
  /\ UNCHANGED <<flushed, flushes, snapSeq, snapVal, tries, others>>

\* Another process commits to key k between two holds of the lock, and the
\* tail applies its record to the memtable.
Other(k) ==
  /\ others < Others /\ lock = "none"
  /\ Commit({k}, [val EXCEPT ![k] = @ + 1], [serial EXCEPT ![k] = @ + 1])
  /\ others' = others + 1
  /\ UNCHANGED <<flushed, flushes, lock, pc, snapSeq, snapVal, tries>>

\* A flush covers the commits through some commit after the last flush and
\* drops their memtable entries; Flushes bounds how many the model runs.
Flush(through) ==
  /\ flushes < Flushes /\ flushes' = flushes + 1
  /\ mem' = [k \in Keys |-> IF mem[k] <= through THEN 0 ELSE mem[k]]
  /\ flushed' = through
  /\ UNCHANGED <<val, serial, seq, changed, lock, pc, snapSeq, snapVal, tries, others>>

Done ==
  /\ \A t \in Txs : pc[t] = "done"
  /\ UNCHANGED vars

Next ==
  \/ \E t \in Txs : Begin(t) \/ Validate(t) \/ Lock(t) \/ LockedCommit(t)
  \/ \E k \in Keys : Other(k)
  \/ \E through \in (flushed + 1)..seq : Flush(through)
  \/ Done

\* Every commit equals running the bodies alone in commit order.
Serializable == val = serial

\* The memtable holds the last change of every key changed after the last
\* flush, and no entry at or below it.
MemCovers ==
  /\ flushed <= seq
  /\ \A k \in Keys : IF changed[k] > flushed THEN mem[k] = changed[k] ELSE mem[k] = 0

\* Only the pessimistic attempt holds the lock across its reads.
LockHeld == \A t \in Txs : (pc[t] = "locked") <=> lock = t

Safety == Serializable /\ MemCovers /\ LockHeld

\* Every transaction commits.
Finishes == <>(\A t \in Txs : pc[t] = "done")

Spec == Init /\ [][Next]_vars
  /\ \A t \in Txs : WF_vars(Begin(t)) /\ WF_vars(Validate(t)) /\ WF_vars(Lock(t)) /\ WF_vars(LockedCommit(t))
====
