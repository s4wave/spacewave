---- MODULE Durability ----
\* Commits, the shared durable flush, a flush of the memtable into a run with
\* its superblock, and the release of log chunks, against every state a crash
\* can leave on the drive, as the design's commit, flush and open paths run
\* them.
\*
\* One key is overwritten by each commit, which appends its value to a value
\* slot (an entry in the open value chunk) and then a record carrying the
\* writer's identity, the identity of the record before it, and the durable
\* mark (the last commit the writer knew flushed). A durable commit then
\* flushes the file and is acknowledged; an ordered commit is acknowledged at
\* once and flushed later. A flush of the memtable writes a run that holds the
\* commits through some seq, flushes, appends a flush record, then writes the
\* superblock pointing after that record; the log before the superblock's
\* replay position, the position after the seq the run holds and not the
\* flush record's own, is released only once the superblock is durable.
\*
\* The only ordering primitive is the durable flush: a crash keeps every write
\* before the last flush and any subset of the writes after it, each torn or
\* whole on its own, which also covers a write that failed with ENOSPC after
\* landing part of its bytes. Recovery reads the newer intact superblock, the
\* run it points at, and the records after its position: each must be whole,
\* carry the next seq and name the previous record's writer as its
\* predecessor; past the highest durable mark among them its value must match.
\* The first record that fails ends the log.
\*
\* After a crash, a new handle with a fresh identity continues the log at the
\* cut, while the stale records behind the cut stay on the drive; the
\* predecessor check is what keeps them out.
\*
\* Callers found from source: Tx.Commit and Tx.CommitOrdered (the two commit
\* kinds), the flusher's syncThrough and deadline flush (Flush), the flush job
\* (RunWrite, RunFlush, FlushRecord, SuperWrite), and release (ReleaseLog).
\* Not modeled: other processes and pins (Pins.tla), merges and the cleaner,
\* which append records the same way and free chunks under the same rule
\* (Free.tla), the link record, and the manifest's own chunks, which follow
\* the superblock like the run.

EXTENDS Naturals, Sequences, FiniteSets

MaxSeq == 3
Slots == {1, 2, 3}     \* value slots in the open value chunk; a fresh one is always free
MaxWriters == 2        \* handle identities, a fresh one per open; one reopen reaches every case
Empty == 9
\* Gone is a log position whose record is absent, released or lost.
Gone == [seq |-> 0, mark |-> 0, slot |-> 1, writer |-> 1, prev |-> 1]

VARIABLES
  seq,       \* the last commit appended
  live,      \* the key's value: its slot and the commit that wrote it
  log,       \* the record at each log position, or Gone
  written,   \* positions written since the last flush (any subset may be lost)
  acked,     \* commits acknowledged durable to a caller
  ordered,   \* commits acknowledged ordered (may be lost with every later one)
  durable,   \* the writer's durable mark
  vmem,      \* each slot's value as written
  vdisk,     \* each slot's values a crash may leave
  writer,    \* the identity of the open handle
  prev,      \* the writer identity of the last record
  run,       \* the run written by the flush job: the seq it holds, or 0
  runLive,   \* the key's value as the run holds it
  runDisk,   \* the runs (by the seq they hold) a crash keeps
  fpc,       \* the flush job: "idle", "written", "flushed", "recorded"
  sbMem,     \* the newest superblock written: seq and log position
  sbDisk,    \* the superblocks a crash may leave
  released,  \* the log positions released (their chunk freed)
  logged,    \* the highest durable mark a durable record carries
  freedAt,   \* for each value slot, the commit that freed it; 0 for never used
  dead,      \* records a recovery cut, which later recoveries must never revive
  down,      \* whether the process crashed and recovery has not run
  drive      \* the drive after a crash: what recovery would read
vars == <<seq, live, log, written, acked, ordered, durable, vmem, vdisk, writer,
          prev, run, runLive, runDisk, fpc, sbMem, sbDisk, released, logged,
          freedAt, dead, down, drive>>

Max(S) == IF S = {} THEN 0 ELSE CHOOSE x \in S : \A y \in S : y <= x

Init ==
  /\ seq = 0
  /\ live = [slot |-> 1, tag |-> 0]
  /\ log = [p \in 1..MaxSeq |-> Gone]
  /\ written = {}
  /\ acked = {} /\ ordered = {}
  /\ durable = 0
  /\ vmem = [s \in Slots |-> IF s = 1 THEN 0 ELSE Empty]
  /\ vdisk = [s \in Slots |-> {vmem[s]}]
  /\ writer = 1 /\ prev = 1
  /\ run = 0 /\ runDisk = {0} /\ runLive = [slot |-> 1, tag |-> 0]
  /\ fpc = "idle"
  /\ sbMem = [seq |-> 0, pos |-> 1, writer |-> 1, live |-> [slot |-> 1, tag |-> 0]]
  /\ sbDisk = {sbMem}
  /\ released = {}
  /\ logged = 0
  /\ freedAt = [s \in Slots |-> 0]
  /\ dead = {}
  /\ down = FALSE
  /\ drive = [log |-> log, values |-> vmem, sb |-> sbMem, runOk |-> TRUE]

\* A commit appends its value to a free slot and its record after it. The
\* value write lands or tears on its own, as does the record. A slot freed at
\* commit f is reused only once a durable record carries a mark of at least
\* f: recovery verifies the values of records past the highest mark it finds,
\* and the record that wrote the freed value would fail that check against
\* the new bytes and cut the log before it.
Commit(kind) ==
  /\ ~down /\ seq < MaxSeq
  /\ \E s \in Slots \ {live.slot} :
       /\ freedAt[s] <= logged
       /\ freedAt' = [freedAt EXCEPT ![live.slot] = seq + 1]
       /\ seq' = seq + 1
       /\ vmem' = [vmem EXCEPT ![s] = seq + 1]
       /\ vdisk' = [vdisk EXCEPT ![s] = @ \cup {seq + 1}]
       /\ live' = [slot |-> s, tag |-> seq + 1]
       /\ log' = [log EXCEPT ![seq + 1] = [seq |-> seq + 1, mark |-> durable, slot |-> s, writer |-> writer, prev |-> prev]]
       /\ written' = written \cup {seq + 1}
       /\ prev' = writer
       /\ IF kind = "ordered" THEN ordered' = ordered \cup {seq + 1} /\ UNCHANGED acked
                             ELSE UNCHANGED <<ordered, acked>>
  /\ UNCHANGED <<durable, writer, run, runLive, runDisk, fpc, sbMem, sbDisk, released, logged, dead, down, drive>>

\* The shared durable flush: everything written reaches the drive. A durable
\* commit is acknowledged by it; ordered commits before it become durable too.
FlushEffects ==
  /\ ~down
  /\ written' = {}
  /\ vdisk' = [s \in Slots |-> {vmem[s]}]
  /\ runDisk' = runDisk \cup {run}
  /\ sbDisk' = {sbMem}
  /\ durable' = seq
  /\ logged' = Max({log[p].mark : p \in 1..MaxSeq} \cup {sbMem.seq, logged})
  /\ acked' = acked \cup (1..seq)
  /\ UNCHANGED <<seq, live, log, ordered, vmem, writer, prev, run, runLive, sbMem, released, freedAt, dead, down, drive>>

Flush == FlushEffects /\ UNCHANGED fpc

\* The flush job writes a run holding the commits through seq. A crash may
\* lose the run until it is flushed.
RunWrite ==
  /\ ~down /\ fpc = "idle" /\ seq > sbMem.seq
  /\ run' = seq /\ runLive' = live
  /\ fpc' = "written"
  /\ UNCHANGED <<seq, live, log, written, acked, ordered, durable, vmem, vdisk, writer, prev, runDisk, sbMem, sbDisk, released, logged, freedAt, dead>>
  /\ UNCHANGED <<down, drive>>

\* The job flushes the run durably (a Flush), then records it.
RunFlush ==
  /\ fpc = "written"
  /\ FlushEffects
  /\ fpc' = "flushed"

\* The flush record is modeled as the superblock write that follows it: the
\* superblock names the run's seq and the log position after it.
SuperWrite ==
  /\ ~down /\ fpc = "flushed"
  /\ sbMem' = [seq |-> run, pos |-> run + 1, writer |-> log[run].writer, live |-> runLive]
  /\ sbDisk' = sbDisk \cup {sbMem'}
  /\ fpc' = "idle"
  /\ UNCHANGED <<seq, live, log, written, acked, ordered, durable, vmem, vdisk, writer, prev, run, runLive, runDisk, released, logged, freedAt, dead>>
  /\ UNCHANGED <<down, drive>>

\* The log before the durable superblock's position is released: its chunk is
\* freed and may be overwritten, so its records are gone.
ReleaseLog ==
  /\ ~down
  /\ \E p \in 1..MaxSeq :
       /\ p \notin released /\ log[p].seq # 0
       /\ p < sbMem.pos
       /\ sbDisk = {sbMem}          \* the superblock is durable
       /\ released' = released \cup {p}
       /\ log' = [log EXCEPT ![p] = Gone]
  /\ UNCHANGED <<seq, live, written, acked, ordered, durable, vmem, vdisk, writer, prev, run, runLive, runDisk, fpc, sbMem, sbDisk, logged, freedAt, dead>>
  /\ UNCHANGED <<down, drive>>

\* A crash: the drive keeps the flushed writes and any subset of the rest.
Crash ==
  /\ ~down
  /\ \E keep \in SUBSET written, vc \in {f \in [Slots -> 0..MaxSeq \cup {Empty}] : \A s \in Slots : f[s] \in vdisk[s]},
       sb \in sbDisk :
       /\ down' = TRUE
       /\ drive' = [log |-> [p \in 1..MaxSeq |-> IF p \in written /\ p \notin keep THEN Gone ELSE log[p]],
                    values |-> vc, sb |-> sb, runOk |-> sb.seq = 0 \/ sb.seq \in runDisk]
  /\ UNCHANGED <<seq, live, log, written, acked, ordered, durable, vmem, vdisk, writer, prev, run, runLive, runDisk, fpc, sbMem, sbDisk, released, logged, freedAt, dead>>

\* Recovery from a crashed drive: the records after the superblock that are
\* whole, in sequence, chained by writer, and past the highest mark have their
\* value. Returns the seq recovery ends at.
RecoverFrom(d) ==
  LET sb == d.sb
      Ok(p, pw) == d.log[p].seq = p /\ d.log[p].prev = pw
      RECURSIVE Walk(_, _)
      Walk(p, pw) == IF p > MaxSeq \/ ~Ok(p, pw) THEN p - 1 ELSE Walk(p + 1, d.log[p].writer)
      lastChained == Walk(sb.pos, sb.writer)
      mark == Max({d.log[p].mark : p \in sb.pos..lastChained} \cup {sb.seq})
      bad == {p \in sb.pos..lastChained : p > mark /\ d.values[d.log[p].slot] # p}
  IN IF bad = {} THEN lastChained ELSE Max({sb.pos - 1} \cup {p \in sb.pos..lastChained : \A b \in bad : p < b})

\* Recovery reads a whole run, every acknowledged commit, a prefix with no
\* record an earlier recovery cut, and the key's recovered value is the one
\* its last recovered commit wrote. Earlier
\* records' values may have been reused once freed under the mark rule; replay
\* overwrites them in the memtable and never reads them.
Recovers(d) ==
  LET r == RecoverFrom(d)
  IN /\ d.runOk
     /\ \A a \in acked : a <= r
     /\ r >= d.sb.seq
     /\ \A p \in d.sb.pos..r : d.log[p] \notin dead
     /\ r > d.sb.seq => d.values[d.log[r].slot] = r

\* After recovery, a new handle continues at the cut with a fresh identity.
\* The stale records behind the cut stay; recovery from the same drive must
\* still stop at the cut.
Reopen ==
  /\ down /\ writer < MaxWriters
  /\ LET crashed == drive
         r == RecoverFrom(crashed)
     IN /\ seq' = r
        /\ live' = IF r = crashed.sb.seq THEN crashed.sb.live ELSE [slot |-> crashed.log[r].slot, tag |-> r]
        /\ log' = crashed.log
        /\ vmem' = crashed.values
        /\ vdisk' = [s \in Slots |-> {crashed.values[s]}]
        /\ prev' = IF r = crashed.sb.seq THEN crashed.sb.writer ELSE crashed.log[r].writer
        /\ sbMem' = crashed.sb /\ sbDisk' = {crashed.sb}
        /\ run' = crashed.sb.seq /\ runLive' = crashed.sb.live /\ UNCHANGED runDisk
        /\ acked' = {a \in acked : a <= r} /\ ordered' = {}
        \* The records behind the cut are dead: a later recovery that read
        \* one would revive a commit from a history the cut ended.
        /\ dead' = dead \cup {crashed.log[p] : p \in {q \in (r + 1)..MaxSeq : crashed.log[q].seq # 0}}
        \* The new handle knows nothing durable yet, as master's flusher
        \* starts: its first record carries mark 0, and slots freed before
        \* the crash wait for a flush, which raises logged past r.
        /\ durable' = 0
        /\ logged' = IF r = 0 THEN 0 ELSE Max({crashed.log[p].mark : p \in crashed.sb.pos..r} \cup {crashed.sb.seq})
        /\ freedAt' = [s \in Slots |-> IF s = live'.slot THEN 0 ELSE r]
  /\ written' = {}
  /\ writer' = writer + 1
  /\ fpc' = "idle"
  /\ released' = {}
  /\ down' = FALSE /\ UNCHANGED drive

Next ==
  \/ Commit("durable") \/ Commit("ordered") \/ Flush
  \/ RunWrite \/ RunFlush \/ SuperWrite \/ ReleaseLog
  \/ Crash \/ Reopen
  \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars

\* Every crash view recovers every acknowledged commit as a prefix with
\* matching values; the marks never pass what was flushed.
RecoversCommitted == down => Recovers(drive)
MarksFlushed == durable <= seq
====
