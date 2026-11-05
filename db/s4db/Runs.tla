---- MODULE Runs ----
\* Sorted runs and the stepped merge, as the writer flushes and merges them,
\* against every state a crash can leave on the drive.
\*
\* A flush writes the overlay's entries since the last flush, tombstones
\* included, as a run: a tree read alone, holding only the keys written since
\* that flush. The superblock names the base tree and the runs, newest first,
\* and moves the checkpoint seq to the flush, so the log before it frees. A
\* lookup reads the overlay, then each run newest first, then the base.
\*
\* A merge takes the runs present when it starts as its round and folds them
\* into the base one root subtree at a time. Each step writes the subtree's
\* new pages and saves a superblock naming them, freeing the old subtree's
\* pages; the round's runs stay named until every subtree holds them, so a
\* lookup that reaches a run still finds its newer entry first. The last step
\* drops the round's runs and frees their pages. Flushes go on between steps,
\* and their runs stay above the round's.
\*
\* Each key stands for one root subtree. Every superblock write follows a
\* durable flush (Persist), which also makes the superblock before it
\* durable, so a crash recovers from the durable superblock or the newest
\* one. A page freed by a superblock is reused only once that superblock is
\* durable. Not modeled: commit durability, barriers on darwin and log reuse
\* (Durability.tla), and snapshots and other processes (Pins.tla).

EXTENDS Naturals, Sequences, FiniteSets

Keys == {"a", "b"}
MaxSeq == 3
Pages == {"p1", "p2", "p3", "p4", "p5"}
Absent == 9
None == [k \in Keys |-> Absent]

VARIABLES
  seq,     \* the last commit
  wrote,   \* the key each commit wrote, by seq
  pmem,    \* each page's contents as written: a value tag, or Absent, per key
  pdisk,   \* each page's contents a crash may leave
  pfree,   \* pages free to write
  ppend,   \* pages a superblock freed, and its gen
  sbNew,   \* the newest superblock written: gen, seq, base and runs
  sbDur,   \* the durable superblock
  fpc,     \* the flush: "idle" or "super" once its run is written
  fnext,   \* the run the flush wrote: its page and seq
  mpc,     \* the merge: "idle", "step", "super" or "end"
  round,   \* the run pages the merge folds in
  mdone,   \* the subtrees the round has merged
  mnext    \* the step's subtree and its new page
vars == <<seq, wrote, pmem, pdisk, pfree, ppend, sbNew, sbDur, fpc, fnext, mpc,
          round, mdone, mnext>>

Max(S) == CHOOSE x \in S : \A y \in S : y <= x

\* The value of k as the commits through hi left it, after lo: the last
\* commit in (lo, hi] that wrote k, or Absent.
Since(k, lo, hi) ==
  LET w == {s \in (lo + 1)..hi : wrote[s] = k}
  IN IF w = {} THEN Absent ELSE Max(w)

\* The value of k the commits through seq left: its last write, or the
\* initial 0.
Live(k) == IF Since(k, 0, seq) = Absent THEN 0 ELSE Since(k, 0, seq)

\* The value of k in the first of runs, read through c, that holds it, or d.
RECURSIVE First(_, _, _, _)
First(k, runs, c, d) ==
  IF runs = <<>> THEN d
  ELSE IF c[Head(runs)][k] # Absent THEN c[Head(runs)][k]
  ELSE First(k, Tail(runs), c, d)

\* A lookup of k from superblock sb with pages read through c: the overlay
\* replayed from the log, then the runs, then the base.
Lookup(k, sb, c) ==
  IF Since(k, sb.seq, seq) # Absent THEN Since(k, sb.seq, seq)
  ELSE First(k, sb.runs, c, c[sb.base[k]][k])

\* The runs of s that are not in the set r, in order.
Without(s, r) == SelectSeq(s, LAMBDA p : p \notin r)

Init ==
  /\ seq = 0
  /\ wrote = [s \in 1..MaxSeq |-> "none"]
  /\ pmem = [p \in Pages |->
               IF p = "p1" THEN [k \in Keys |-> IF k = "a" THEN 0 ELSE Absent]
               ELSE IF p = "p2" THEN [k \in Keys |-> IF k = "b" THEN 0 ELSE Absent]
               ELSE None]
  /\ pdisk = [p \in Pages |-> {pmem[p]}]
  /\ pfree = Pages \ {"p1", "p2"}
  /\ ppend = {}
  /\ sbNew = [gen |-> 1, seq |-> 0, base |-> [k \in Keys |-> IF k = "a" THEN "p1" ELSE "p2"],
              runs |-> <<>>]
  /\ sbDur = sbNew
  /\ fpc = "idle" /\ fnext = [page |-> "p1", seq |-> 0]
  /\ mpc = "idle" /\ round = {} /\ mdone = {} /\ mnext = [key |-> "a", page |-> "p1"]

\* A commit writes one key. Its record is in the log, which Durability.tla
\* keeps through the durable superblock's seq.
Commit ==
  /\ seq < MaxSeq
  /\ \E k \in Keys :
       /\ seq' = seq + 1
       /\ wrote' = [wrote EXCEPT ![seq + 1] = k]
  /\ UNCHANGED <<pmem, pdisk, pfree, ppend, sbNew, sbDur, fpc, fnext, mpc, round,
                 mdone, mnext>>

\* Write page q without a barrier: a crash may keep either contents.
WritePage(q, c) ==
  /\ pmem' = [pmem EXCEPT ![q] = c]
  /\ pdisk' = [pdisk EXCEPT ![q] = @ \cup {c}]
  /\ pfree' = pfree \ {q}

\* flushRun writes the commits since the last flush as a run.
FlushWrite ==
  /\ fpc = "idle" /\ sbNew.seq < seq
  /\ \E q \in pfree :
       /\ WritePage(q, [k \in Keys |-> Since(k, sbNew.seq, seq)])
       /\ fnext' = [page |-> q, seq |-> seq]
  /\ fpc' = "super"
  /\ UNCHANGED <<seq, wrote, ppend, sbNew, sbDur, mpc, round, mdone, mnext>>

\* Every write so far reaches the drive, the newest superblock included.
Persist ==
  /\ pdisk' = [p \in Pages |-> {pmem[p]}]
  /\ sbDur' = sbNew

\* saveRuns flushes, then names the run first and moves the seq to it. The
\* commits after it stay in the overlay and the log.
FlushSuper ==
  /\ fpc = "super"
  /\ Persist
  /\ sbNew' = [sbNew EXCEPT !.gen = @ + 1, !.seq = fnext.seq,
                            !.runs = <<fnext.page>> \o @]
  /\ fpc' = "idle"
  /\ UNCHANGED <<seq, wrote, pmem, pfree, ppend, fnext, mpc, round, mdone, mnext>>

\* A merge takes the runs named now as its round.
MergeStart ==
  /\ mpc = "idle" /\ sbNew.runs # <<>>
  /\ round' = {sbNew.runs[i] : i \in 1..Len(sbNew.runs)}
  /\ mdone' = {}
  /\ mpc' = "step"
  /\ UNCHANGED <<seq, wrote, pmem, pdisk, pfree, ppend, sbNew, sbDur, fpc, fnext,
                 mnext>>

\* mergeStep rewrites subtree k with the round's runs over it, the newest
\* first, and writes the new subtree.
MergeWrite ==
  /\ mpc = "step"
  /\ \E k \in Keys \ mdone, q \in pfree :
       LET runs == SelectSeq(sbNew.runs, LAMBDA p : p \in round)
           v == First(k, runs, pmem, pmem[sbNew.base[k]][k])
       IN /\ WritePage(q, [j \in Keys |-> IF j = k THEN v ELSE Absent])
          /\ mnext' = [key |-> k, page |-> q]
  /\ mpc' = "super"
  /\ UNCHANGED <<seq, wrote, ppend, sbNew, sbDur, fpc, fnext, round, mdone>>

\* saveStep flushes, then names the new subtree and frees the old one.
MergeSuper ==
  /\ mpc = "super"
  /\ Persist
  /\ sbNew' = [sbNew EXCEPT !.gen = @ + 1, !.base[mnext.key] = mnext.page]
  /\ ppend' = ppend \cup {[page |-> sbNew.base[mnext.key], at |-> sbNew.gen + 1]}
  /\ mdone' = mdone \cup {mnext.key}
  /\ mpc' = IF mdone' = Keys THEN "end" ELSE "step"
  /\ UNCHANGED <<seq, wrote, pmem, pfree, fpc, fnext, round, mnext>>

\* saveRound drops the round's runs and frees their pages.
MergeEnd ==
  /\ mpc = "end"
  /\ Persist
  /\ sbNew' = [sbNew EXCEPT !.gen = @ + 1, !.runs = Without(@, round)]
  /\ ppend' = ppend \cup {[page |-> p, at |-> sbNew.gen + 1] : p \in round}
  /\ round' = {}
  /\ mpc' = "idle"
  /\ UNCHANGED <<seq, wrote, pmem, pfree, fpc, fnext, mdone, mnext>>

\* A flush with no superblock to write.
Sync ==
  /\ Persist
  /\ UNCHANGED <<seq, wrote, pmem, pfree, ppend, sbNew, fpc, fnext, mpc, round,
                 mdone, mnext>>

\* releaseSpace returns the pages freed by durable superblocks.
Release ==
  /\ \E e \in ppend :
       /\ e.at <= sbDur.gen
       /\ ppend' = ppend \ {e}
       /\ pfree' = pfree \cup {e.page}
  /\ UNCHANGED <<seq, wrote, pmem, pdisk, sbNew, sbDur, fpc, fnext, mpc, round,
                 mdone, mnext>>

Idle == UNCHANGED vars

Next ==
  \/ Commit \/ FlushWrite \/ FlushSuper \/ MergeStart \/ MergeWrite
  \/ MergeSuper \/ MergeEnd \/ Sync \/ Release \/ Idle
Spec == Init /\ [][Next]_vars

\* The choices a crash makes for each page among what it may hold.
Choices(d) ==
  {f \in [DOMAIN d -> UNION {d[x] : x \in DOMAIN d}] : \A x \in DOMAIN d : f[x] \in d[x]}

\* A lookup of the published state reads every key's last commit.
ReadsLive == \A k \in Keys : Lookup(k, sbNew, pmem) = Live(k)

\* A crash recovers every key's last commit from the durable superblock or
\* the newest one, whatever each page kept.
RecoversCommitted ==
  \A sb \in {sbDur, sbNew}, c \in Choices(pdisk), k \in Keys :
    Lookup(k, sb, c) = Live(k)

\* A free page is named by neither superblock a crash may recover.
FreeUnnamed ==
  \A sb \in {sbDur, sbNew} :
    \A p \in pfree :
      /\ \A k \in Keys : sb.base[k] # p
      /\ \A i \in 1..Len(sb.runs) : sb.runs[i] # p
====
