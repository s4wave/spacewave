---- MODULE Durability ----
\* Commits, flushes, checkpoints and the release of freed space, as the writer
\* and flusher run them, against every state a crash can leave on the drive.
\*
\* Each commit sets or deletes one of two keys. A set writes its value into a
\* free run; either way the commit appends a record carrying the durable mark
\* it was written under. The value the commit replaced is freed at the commit
\* and released, for reuse, once the flusher's logged mark reaches it
\* (releaseSpace). A flush (syncThrough) makes everything written durable and
\* raises the durable, logged and checkpoint marks from the state it claimed.
\*
\* A checkpoint takes the published state as its snapshot and picks a cut, a
\* commit after the published checkpoint and at or before the snapshot. Its
\* tree (build) is the old tree with the snapshot's overlay entries at or
\* before the cut applied; a key whose latest change in the snapshot follows
\* the cut keeps its old tree entry, which may name a value already released,
\* and stays in the overlay. saveTree writes the tree into a free page, flushes
\* with a barrier, writes a superblock naming the tree and the log after the
\* cut, and publishes the overlay carried over the tree: every entry but those
\* the tree saved that are still the newest, and a filter rebuilt over the
\* kept keys. The page the old tree used is freed at the cut and released once
\* the cut's checkpoint is durable. Commits, and the releases of values and
\* pages they run, continue while the checkpoint builds and saves, so the
\* build, the barrier and the publish each see later commits. A cut at the
\* snapshot is the whole-overlay checkpoint; an earlier one is a slice, and the
\* next slice starts from the published checkpoint as any checkpoint does.
\*
\* A crash keeps the durable superblock or the newer written one, the flushed
\* records and any prefix of the rest, and for each run and page either what
\* was durable or what was written since. Recovery (load, tail with verify)
\* reads the tree the superblock names, takes the highest durable mark among
\* the records after it, ends the log at the first record past that mark
\* whose value no longer matches, and replays the rest over the tree. The
\* recovered keys must equal the committed keys as of the last record kept,
\* which must reach every commit known durable. The published state, its tree
\* under its overlay read through the filter, must hold every key as the last
\* commit left it, with a value that matches.
\*
\* On Linux and Windows the barrier is a durable flush and raises the marks.
\* On darwin it is F_BARRIERFSYNC, which only orders the writes before it
\* ahead of those after, and raises none. So the drive is tracked twice: as a
\* crash that keeps no write after the last barrier may leave it (phys,
\* vdisk, pdisk, from sbDur), and as one that keeps a later write, which
\* then keeps every write before the barrier (physB, vdiskB, pdiskB, from the
\* superblock written before the barrier or the one after it).
\*
\* ReplayFromCut selects where the superblock resumes the log: after the cut,
\* as saveTree does, or after the snapshot, which loses every change a slice
\* left in the overlay (testdata/Durability-SnapshotPosition.cfg).
\*
\* Compaction needs no action of its own: relocate rewrites values through a
\* commit, which frees their old runs like any commit, and moveLog continues
\* the log in a free chunk, leaving the old chunk to be freed by the next
\* checkpoint like a tree page. Not modeled: pins and other processes
\* (Pins.tla), the log reader's checksum chain, which ends the log at a stale
\* record in a reused chunk, and the space record.

EXTENDS Naturals, Sequences, FiniteSets

CONSTANT ReplayFromCut

MaxSeq == 3
Keys == {"a", "b"}
Slots == {1, 2, 3, 4}
Pages == {"p1", "p2", "p3"}
Empty == 9
\* A key's entry: the run holding its value, 0 once deleted, and the commit
\* that wrote it.
Initial == [k \in Keys |-> [slot |-> IF k = "a" THEN 1 ELSE 2, tag |-> 0]]
NoTree == [k \in Keys |-> [slot |-> 0, tag |-> Empty]]
\* The overlay's mark for a key it holds no entry for.
NoEntry == [slot |-> 0, tag |-> 0]

VARIABLES
  platform,    \* "linux": the barrier is durable; "darwin": it orders only
  seq,         \* the last commit
  live,        \* each key's entry as of the last commit
  log,         \* the records written: the durable mark, the key, the run
  phys,        \* how many records are certainly on the drive
  vmem,        \* each run's value as written
  vdisk,       \* each run's values a crash may leave
  free,        \* runs free for a commit's value
  pend,        \* freed runs and the commit that freed them
  durable,     \* flusher.durable: the last commit known durable
  logged,      \* flusher.logged
  curLogged,   \* the published state's logged mark
  page,        \* the page holding the published tree
  pmem,        \* each page's tree as written
  pdisk,       \* each page's trees a crash may leave
  pfree,       \* pages free for a checkpoint
  ppend,       \* freed pages and the checkpoint that freed them
  ckpt,        \* the published checkpoint
  ckptWritten, \* flusher.ckptWritten
  ckptDurable, \* flusher.ckptDurable
  sbDur,       \* the superblock a crash keeps
  sbDurB,      \* the superblock a crash keeps once it keeps a later write
  physB,       \* phys, once a crash keeps a write after the last barrier
  vdiskB,      \* vdisk, likewise
  pdiskB,      \* pdisk, likewise
  sbNew,       \* the newest superblock written
  cpc,         \* the checkpoint's step: "idle", "build", "barrier", "super",
               \* "publish"
  cnext,       \* the checkpoint: its cut, snapshot, seq, page, tree and the
               \* keys the tree saved
  ov,          \* the published overlay: each key's entry, or NoEntry
  filter       \* the keys the published overlay's filter holds
vars == <<platform, seq, live, log, phys, vmem, vdisk, free, pend, durable,
          logged, curLogged, page, pmem, pdisk, pfree, ppend, ckpt,
          ckptWritten, ckptDurable, sbDur, sbNew, cpc, cnext, sbDurB, physB,
          vdiskB, pdiskB, ov, filter>>

Max(S) == CHOOSE x \in S : \A y \in S : y <= x

\* The last record through hi that changes k, 0 when none does. Record i is
\* commit i.
Last(k, hi) == Max({i \in 1..hi : log[i].key = k} \cup {0})

\* The entry of k as of commit m.
Committed(k, m) ==
  LET i == Last(k, m) IN
  IF i = 0 THEN Initial[k] ELSE [slot |-> log[i].slot, tag |-> i]

Init ==
  /\ platform \in {"linux", "darwin"}
  /\ seq = 0
  /\ live = Initial
  /\ log = <<>>
  /\ phys = 0
  /\ vmem = [s \in Slots |-> IF s \in {1, 2} THEN 0 ELSE Empty]
  /\ vdisk = [s \in Slots |-> {vmem[s]}]
  /\ free = Slots \ {1, 2}
  /\ pend = {}
  /\ durable = 0 /\ logged = 0 /\ curLogged = 0
  /\ page = "p1"
  /\ pmem = [p \in Pages |-> IF p = "p1" THEN Initial ELSE NoTree]
  /\ pdisk = [p \in Pages |-> {pmem[p]}]
  /\ pfree = Pages \ {"p1"}
  /\ ppend = {}
  /\ ckpt = 0 /\ ckptWritten = 0 /\ ckptDurable = 0
  /\ sbDur = [seq |-> 0, page |-> "p1", tree |-> Initial]
  /\ sbNew = sbDur
  /\ sbDurB = sbDur /\ physB = 0 /\ vdiskB = vdisk /\ pdiskB = pdisk
  /\ cpc = "idle"
  /\ cnext = [cut |-> 0, snap |-> 0, seq |-> 0, page |-> "p1", tree |-> Initial,
             saved |-> {}]
  /\ ov = [k \in Keys |-> NoEntry]
  /\ filter = {}

\* Append the record of a commit that gives k the entry e, freeing the run
\* of the value it replaces, and publish e in the overlay.
Append1(k, e) ==
  /\ seq' = seq + 1
  /\ pend' = IF live[k].slot # 0 THEN pend \cup {[slot |-> live[k].slot, at |-> seq + 1]}
             ELSE pend
  /\ live' = [live EXCEPT ![k] = e]
  /\ log' = Append(log, [dur |-> durable, key |-> k, slot |-> e.slot])
  /\ curLogged' = Max({curLogged, durable})
  /\ ov' = [ov EXCEPT ![k] = e]
  /\ filter' = filter \cup {k}

\* A commit, under the writer lock, while a checkpoint may build or save: write
\* the value of k into a free run, or delete k.
Commit ==
  /\ seq < MaxSeq
  /\ \E k \in Keys :
       \/ \E s \in free :
            /\ Append1(k, [slot |-> s, tag |-> seq + 1])
            /\ vmem' = [vmem EXCEPT ![s] = seq + 1]
            /\ vdisk' = [vdisk EXCEPT ![s] = @ \cup {seq + 1}]
            /\ vdiskB' = [vdiskB EXCEPT ![s] = @ \cup {seq + 1}]
            /\ free' = free \ {s}
       \/ /\ live[k].slot # 0
          /\ Append1(k, [slot |-> 0, tag |-> seq + 1])
          /\ UNCHANGED <<vmem, vdisk, vdiskB, free>>
  /\ UNCHANGED <<platform, phys, durable, logged, page, pmem, pdisk, pfree,
                 ppend, ckpt, ckptWritten, ckptDurable, sbDur, sbNew, cpc, cnext,
                 sbDurB, physB, pdiskB>>

\* releaseSpace, under the writer lock, in any checkpoint step: return the
\* values freed by commits through the logged mark.
ReleaseValues ==
  /\ \E e \in pend :
       /\ e.at <= logged
       /\ pend' = pend \ {e}
       /\ free' = free \cup {e.slot}
  /\ UNCHANGED <<platform, seq, live, log, phys, vmem, vdisk, durable, logged,
                 curLogged, page, pmem, pdisk, pfree, ppend, ckpt, ckptWritten,
                 ckptDurable, sbDur, sbNew, cpc, cnext, sbDurB, physB, vdiskB,
                 pdiskB, ov, filter>>

\* Every write so far is kept by a crash that keeps any later write.
Order ==
  /\ physB' = Len(log)
  /\ vdiskB' = [s \in Slots |-> {vmem[s]}]
  /\ pdiskB' = [p \in Pages |-> {pmem[p]}]
  /\ sbDurB' = sbNew

\* Every write so far reaches the drive.
Persist ==
  /\ Order
  /\ phys' = Len(log)
  /\ vdisk' = vdiskB'
  /\ pdisk' = pdiskB'
  /\ sbDur' = sbNew

\* syncThrough: flush, then raise the marks from the published state and the
\* newest superblock published.
Flush ==
  /\ Persist
  /\ durable' = seq
  /\ logged' = Max({logged, curLogged})
  /\ ckptDurable' = Max({ckptDurable, ckptWritten})
  /\ UNCHANGED <<platform, seq, live, log, vmem, free, pend, curLogged, page,
                 sbNew, pmem, pfree, ppend, ckpt, ckptWritten, cpc, cnext, ov,
                 filter>>

\* The keys whose latest change as of snapshot s follows the checkpoint and
\* lies at or before cut x: those a checkpoint at x of s saves.
Saved(x, s) == {k \in Keys : Last(k, s) > ckpt /\ Last(k, s) <= x}

\* The tree of a checkpoint at cut x of snapshot s: each saved key takes its
\* latest change as of s, and every other key keeps the published tree's
\* entry.
CutTree(x, s) ==
  [k \in Keys |->
     IF k \in Saved(x, s) THEN [slot |-> log[Last(k, s)].slot, tag |-> Last(k, s)]
     ELSE pmem[page][k]]

\* chooseCut takes the published state as the snapshot and picks the cut.
CkptCut ==
  /\ cpc = "idle" /\ ckpt < seq
  /\ \E x \in (ckpt + 1)..seq : cnext' = [cnext EXCEPT !.cut = x, !.snap = seq]
  /\ cpc' = "build"
  /\ UNCHANGED <<platform, seq, live, log, phys, vmem, vdisk, free, pend,
                 durable, logged, curLogged, page, pmem, pdisk, pfree, ppend,
                 ckpt, ckptWritten, ckptDurable, sbDur, sbNew, sbDurB, physB,
                 vdiskB, pdiskB, ov, filter>>

\* build and saveTree write the tree at the cut into a free page, and free
\* the old tree's page at the cut. Commits may have followed the snapshot.
CkptWrite ==
  /\ cpc = "build"
  /\ LET x == cnext.cut
         s == cnext.snap
         t == CutTree(x, s)
     IN \E q \in pfree :
          /\ pmem' = [pmem EXCEPT ![q] = t]
          /\ pdisk' = [pdisk EXCEPT ![q] = @ \cup {t}]
          /\ pdiskB' = [pdiskB EXCEPT ![q] = @ \cup {t}]
          /\ pfree' = pfree \ {q}
          /\ ppend' = ppend \cup {[page |-> page, at |-> x]}
          /\ cnext' = [cut |-> x, snap |-> s,
                       seq |-> IF ReplayFromCut THEN x ELSE s, page |-> q,
                       tree |-> t, saved |-> Saved(x, s)]
  /\ cpc' = "barrier"
  /\ UNCHANGED <<platform, seq, live, log, phys, vmem, vdisk, free, pend,
                 durable, logged, curLogged, page, ckpt, ckptWritten,
                 ckptDurable, sbDur, sbNew, sbDurB, physB, vdiskB, ov, filter>>

\* flushBarrier orders the tree and every record before the superblock. A
\* durable barrier also raises the marks: setDurable(cur, prev).
Barrier ==
  /\ cpc = "barrier"
  /\ IF platform = "linux"
     THEN /\ Persist
          /\ durable' = seq
          /\ logged' = Max({logged, curLogged})
          /\ ckptDurable' = Max({ckptDurable, ckptWritten})
     ELSE /\ Order
          /\ UNCHANGED <<durable, logged, ckptDurable, phys, vdisk, pdisk, sbDur>>
  /\ cpc' = "super"
  /\ UNCHANGED <<platform, seq, live, log, vmem, free, pend, curLogged, page,
                 sbNew, pmem, pfree, ppend, ckpt, ckptWritten, cnext, ov,
                 filter>>

SuperWrite ==
  /\ cpc = "super"
  /\ sbNew' = [seq |-> cnext.seq, page |-> cnext.page, tree |-> cnext.tree]
  /\ cpc' = "publish"
  /\ UNCHANGED <<platform, seq, live, log, phys, vmem, vdisk, free, pend,
                 durable, logged, curLogged, page, pmem, pdisk, pfree, ppend,
                 ckpt, ckptWritten, ckptDurable, sbDur, cnext, sbDurB, physB,
                 vdiskB, pdiskB, ov, filter>>

\* The overlay carried over a checkpoint: carry drops each key the tree saved
\* whose entry is still at or before the cut, and keeps every other entry.
Carried == [k \in Keys |->
              IF k \in cnext.saved /\ ov[k].tag <= cnext.cut THEN NoEntry
              ELSE ov[k]]

\* publish the state onto the new checkpoint with the carried overlay and a
\* filter over its keys, then flush.written.
Publish ==
  /\ cpc = "publish"
  /\ page' = cnext.page
  /\ ckpt' = cnext.seq
  /\ curLogged' = Max({curLogged, cnext.seq})
  /\ ckptWritten' = Max({ckptWritten, cnext.seq})
  /\ ov' = Carried
  /\ filter' = {k \in Keys : Carried[k] # NoEntry}
  /\ cpc' = "idle"
  /\ UNCHANGED <<platform, seq, live, log, phys, vmem, vdisk, free, pend,
                 durable, logged, pmem, pdisk, pfree, ppend, ckptDurable, sbDur,
                 sbNew, cnext, sbDurB, physB, vdiskB, pdiskB>>

\* releaseSpace and punch, in any checkpoint step: return the pages freed by
\* checkpoints through the durable checkpoint.
ReleasePages ==
  /\ \E e \in ppend :
       /\ e.at <= ckptDurable
       /\ ppend' = ppend \ {e}
       /\ pfree' = pfree \cup {e.page}
  /\ UNCHANGED <<platform, seq, live, log, phys, vmem, vdisk, free, pend,
                 durable, logged, curLogged, page, pmem, pdisk, ckpt,
                 ckptWritten, ckptDurable, sbDur, sbNew, cpc, cnext, sbDurB,
                 physB, vdiskB, pdiskB, ov, filter>>

Idle == UNCHANGED vars

Next ==
  \/ Commit \/ ReleaseValues \/ Flush \/ CkptCut \/ CkptWrite \/ Barrier
  \/ SuperWrite
  \/ Publish \/ ReleasePages \/ Idle
Spec == Init /\ [][Next]_vars

\* The choices a crash makes for each run or page among what it may hold.
Choices(d) ==
  {f \in [DOMAIN d -> UNION {d[x] : x \in DOMAIN d}] : \A x \in DOMAIN d : f[x] \in d[x]}

\* Recovery from superblock sb, the first n records, and the runs and pages
\* as vc and pc hold them: the tree is intact, the log reaches every commit
\* known durable, and the tree under the replayed records holds each key as
\* that commit left it, with a value that matches.
Recovered(sb, n, vc, pc) ==
  LET idx == {i \in 1..n : i > sb.seq}
      d == Max({log[i].dur : i \in idx} \cup {0})
      bad == {i \in idx : i > d /\ log[i].slot # 0 /\ vc[log[i].slot] # i}
      kept == {i \in idx : \A j \in bad : i < j}
      last == Max(kept \cup {sb.seq})
      got(k) == LET ks == {i \in kept : log[i].key = k} IN
                IF ks = {} THEN sb.tree[k]
                ELSE [slot |-> log[Max(ks)].slot, tag |-> Max(ks)]
  IN /\ pc[sb.page] = sb.tree
     /\ last >= durable
     /\ \A k \in Keys :
          /\ got(k) = Committed(k, last)
          /\ got(k).slot # 0 => vc[got(k).slot] = got(k).tag

\* A crash that keeps no write after the last barrier recovers from the
\* durable superblock; one that keeps a later write also keeps everything
\* before the barrier, and recovers from the superblock written before it or
\* the newest one.
RecoversCommitted ==
  /\ \A n \in phys..Len(log), vc \in Choices(vdisk), pc \in Choices(pdisk) :
       Recovered(sbDur, n, vc, pc)
  /\ \A sb \in {sbDurB, sbNew}, n \in physB..Len(log),
        vc \in Choices(vdiskB), pc \in Choices(pdiskB) :
       Recovered(sb, n, vc, pc)

\* The published state reads every key as the last commit left it: from the
\* overlay when the filter and the overlay hold it, else from the tree, with
\* a value no release has handed to another commit.
ReadsLive ==
  \A k \in Keys :
    LET e == IF k \in filter /\ ov[k] # NoEntry THEN ov[k] ELSE pmem[page][k] IN
    /\ e = live[k]
    /\ e.slot # 0 => vmem[e.slot] = e.tag

\* The marks never pass what was flushed.
MarksFlushed == durable <= seq /\ logged <= durable /\ ckptDurable <= ckpt

\* A slice leaves in the overlay a key it also changed before the cut. TLC
\* reports this invariant violated (testdata/Durability-Slice.cfg), which
\* shows the checks above reach that case.
NoRewriteAcrossCut ==
  ~(/\ cpc = "barrier"
    /\ \E k \in Keys :
         /\ Last(k, seq) > cnext.seq
         /\ \E i \in (ckpt + 1)..cnext.seq : log[i].key = k)
====
