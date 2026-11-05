---- MODULE Journal ----
\* The Volume's liveness: a journal the store appends to in each write
\* transaction, a collector that nets it after the fact, and the cascade
\* through child lists. This model leads the Go collector; each action below
\* names the code path it stands for.
\*
\* The store. A block is live while a held named root reaches it through
\* child lists. The Volume keeps, per block, the child list the put wrote
\* (Kids, fixed because blocks are content addressed) and a count record only
\* for a block with a second holder: cnt[b] is holders minus one, and a block
\* with no record has exactly one holder. The holders of a block are the named
\* roots that name it and the stored blocks whose child lists name it,
\* including dead blocks the collector has not freed yet. Nothing above the
\* Volume writes a count.
\*
\* The journal. Each SetRoots transaction of an adopting or adopted Volume
\* appends at most one record, and the record carries only what changes a
\* holder count:
\*   up    pairs <<holder, block>> that add a holder:
\*         - <<parent, child>> for each child a block the batch stored for the
\*           first time names, unless the batch stores that child for the
\*           first time too: a child the store already held, one put again in
\*           the batch, or one the store lacks, whose put may come in a later
\*           batch. A child born in the batch has the parent as its one
\*           implied holder, so nothing is recorded for it.
\*         - <<name, block>> for each root that now names a block the store
\*           already held, before the batch: a reader pinning an old root, a
\*           writer adopting a stage, SetRoots onto a stored reference. A root
\*           that names a block the batch stored is its implied holder.
\*   down  pairs <<name, block>> for each root that moved off a block, read
\*         from the root table inside the transaction.
\* A block the batch puts again, which already has a child list, gets an up
\* and a down pair: they net to nothing, but the tail check keeps the block
\* until they age Grace, so the put restarts its grace. A block born with a
\* holder in its batch, a parent born with it, a root or a stage root, takes
\* that holder as its implied one. A block born with none gets a down pair
\* <<"born", block>> for its implied holder (BornDown): it is garbage once that
\* record ages Grace, unless a parent's up pair or a root lands first, so a
\* put before its parent's is counted exactly and an abandoned put is freed.
\*
\* The collector. Net consumes a prefix of the journal as one interval whose
\* records have all aged Grace, adds the interval's deltas to the netted counts
\* in one transaction, writes only the blocks whose holders changed, stored or
\* not, and puts a stored block whose holders reach zero on the free queue.
\* Delete frees one queued block and decrements its children, stored or not
\* yet, which the cascade then finds at zero in turn. A child in a dead page
\* and a born page of one interval nets to no change and keeps its implied
\* holder unwritten.
\* Neither runs until the Volume is adopted.
\*
\* Adoption. A Volume older than the journal starts unadopted: its child lists
\* and roots are stored, its count records and free queue are whatever an
\* older format or an abandoned run left, and Land journals nothing. An Adopt
\* run claims the Volume under its run id, clears the old counts, queue and
\* marks, then marks the Volume adopting at the journal's end, the cursor.
\* From then on Land journals, marks each block it gives birth to (born), and
\* records the block each root it moves first held (moved). The run counts
\* the Volume as it stood at the cursor, one record per transaction, while
\* writers land between any two: each root at its moved mark or else its table
\* entry, then the children of each child list the born marks do not name. It
\* settles each such list's block, queueing one with no holder and dropping
\* the implied holder of any other, and marks the Volume adopted. Every step
\* after the claim first checks that the Volume still names its run, so a run
\* another claim or a Volume opening superseded writes nothing more. A crash
\* stops a run anywhere; the next opening returns an adopting Volume to
\* unadopted, and a later run starts over.
\*
\* Stage leases. A stage owner, the process behind stage:<proc>/<ulid>, builds
\* a tree bottom up: it puts T under the leased root sT, then puts S over T
\* under sS, releasing sT in the same transaction, and the World writer moves
\* main onto S and releases sS. The owner keeps its own record of the roots it
\* holds. Its renewal moves each one's expiry forward, and holds a root that
\* lapsed and was swept again while its block is stored, as reader pins are;
\* when the block is gone, the stage fails and its writes are refused. A put
\* that builds on a root the stage holds releases it with the owner's record
\* as its Previous, so a put over a root a sweep took is refused. Renewal may
\* stall for any time: the clock runs on while the owner does nothing.
\* Sweep releases any lapsed lease, as another process's collector does. A
\* process's own collector renews its own leases first, so it releases only
\* those of owners that are gone, which is a subset of these behaviors.
\*
\* Mapping to Go, one action to one transaction:
\*   Land            Volume.SetRoots(entries, roots) in db/volume/common/kvtx,
\*                   which is block_gc.Land: checks each root's Previous
\*                   (ErrRootMoved), stores the entries with their child
\*                   lists, moves the roots and appends the journal record,
\*                   all in one write transaction, refusing no child the
\*                   store lacks; while adopting, also
\*                   Records.MarkAdoptBorn and Records.NoteAdoptRoot.
\*     Commit        the World engine's head swap: main moves by compare and
\*                   swap from the head the writer observed.
\*     StageWrite    a writer outside the World writer holding stage:<ulid>,
\*                   a tree built over the head it observed and still holds.
\*     SessionPut    one PutBlockBatch of a writer that puts a tree across
\*                   several batches, BufferedStore's drain or the object
\*                   copy; SessionFinish is its closing SetRoots. Staged, a
\*                   session is OpenStage, PrepareStagedBlockBatch and a
\*                   publish that releases the stage.
\*     AdoptStage    the World writer moving main onto a stage and releasing
\*                   the stage in one SetRoots.
\*     ReaderPin     a reader holding the root it saw as a leased root.
\*     Drop          a lease holder releasing its root.
\*     Sweep         Collector.sweepLeases releasing a root whose lease lapsed;
\*                   it reads the root inside its own transaction.
\*     StagePut1     Volume.PrepareStagedBlockBatch: prepareStaged with
\*     StagePut2     stageMoves, holding each put block the batch does not
\*                   name and releasing the stage's root on each it names.
\*     StagePublish  the World writer's SetRoots onto the stage's top with
\*                   Volume.ReleaseStageRoots, in one transaction.
\*     StageRenew    Volume.renewRoots from runRootRenewal, and at the start
\*                   of Volume.Collect.
\*   Net(k)          Collector.netSlice with applyDelta: read the journal from
\*                   the cursor to the grace horizon, net per block, write the
\*                   count records and free queue entries that changed,
\*                   advance the cursor.
\*   Delete(b)       one free queue entry in a collector slice, freeBlock with
\*                   release: the tail check, then delete the block with its
\*                   child list and count record and decrement its children.
\*                   A slice yields to a waiting writer between these
\*                   transactions, so writers interleave anywhere between two
\*                   Deletes.
\*   AdoptStart(r)   block_gc.Adopt's resetAdoption: report an adopted Volume,
\*                   otherwise return it to unadopted and claim it for r.
\*   AdoptClear(r)   Adopt's clearKinds of the count, queue and mark records.
\*   AdoptMark(r)    Adopt's transaction that marks the Volume adopting in run
\*                   r at the cursor.
\*   AdoptRoot(r)    one name of adoption.countRoots.
\*   AdoptList(r)    one child list of the first forSlices scan in
\*                   adoption.count, with adoption.listed and adoption.hold.
\*   AdoptSettle(r)  one child list of the settle scan in adoption.count.
\*   AdoptEnd(r)     deleteJournalBefore and the transaction that marks the
\*                   Volume adopted, then the clearKinds of the marks.
\*   AdoptCrash(r)   the process running r dies, or r fails: it stops.
\*   Open            block_gc.ResetAdoption from NewVolume, in a restarted
\*                   process or another one on the same store.
\*   Punch(b)        a hole: a block the store lost with its child list.
\*   Tick            the clock Grace, Window and leases are read on.
\*
\* The tail check (TailCheck). The collector decides a block is dead from a
\* journal prefix, and a later transaction may add a holder to it: a put that
\* found it still stored, or a root that now names it. That record is in the
\* tail past the horizon, not in the interval the pass netted. Delete refuses a
\* queued block that an up pair in the tail names; the block stays queued until
\* the interval holding that record nets it back to one holder. In Go the
\* collector reads the tail past its cursor once per pass, keeps the up pairs
\* that name a queued block of the slice, and in each delete transaction reads
\* the records appended since its last look, so nothing lands unseen between
\* the check and the free. Applying the increments ahead of their interval
\* instead would also be safe, but it writes the count of every found child
\* twice, once up at the put's interval and once down at the release's, which
\* is the write amplification lazy netting exists to remove.
\*
\* Switches, each set to the rule the Go collector must keep in every config
\* here. Flipping one breaks a rule in the config named, in the run TLC
\* reports, which shows the rule is load bearing; CAS is the exception. The
\* configs under testdata/ flip the adoption, stage and writer switches and
\* fail.
\*   TailCheck  FALSE trusts the netted count. Journal.cfg: main moves R1 to
\*              R2 and back to R1 inside Grace, a pass nets only the first
\*              record, and R1 is freed while main names it again.
\*   Held       FALSE drops the up pair of a root moving onto a stored block.
\*              Journal-lease.cfg: a stage R2 is adopted onto main, the
\*              stage's release nets R2 to zero, and it is freed under main.
\*   PutPair    FALSE journals nothing for a put of a stored block.
\*              testdata/Journal-writers-nopair.cfg: main moves from R1 to
\*              R2, a session writing R3 over R2 puts C again, R1's release
\*              nets, R1 and then C are freed, and the session's last
\*              SetRoots lands R3 over the freed C.
\*   SessionGrace FALSE lets a session land after Grace from its start.
\*              testdata/Journal-writers-long.cfg: a session writing R2
\*              over R1 puts C2 alone, C2's born down ages Grace and nets, C2
\*              is freed, and the last SetRoots lands R2 over it. A session
\*              that puts C again fails the same way once its pair ages.
\*   BornDown   FALSE gives a block born with no holder in its batch no down
\*              pair, as Land's appendJournal does today, so its implied
\*              holder stays for good. testdata/Journal-writers-noborndown.cfg:
\*              a session puts N alone, and N stays stored with nothing
\*              holding it once the journal is netted.
\*   ReleaseUnlisted FALSE has the cascade step over a child with no child
\*              list, as release does today. testdata/
\*              Journal-writers-nounlisted.cfg: a session puts R2 before C2,
\*              which journals an up pair for C2, R2 is freed, the cascade
\*              skips C2, and C2 keeps that holder after a commit stores it.
\*   Staged     TRUE writes each session under a stage.
\*              Journal-writers-staged.cfg sets SessionGrace FALSE with it
\*              and passes; testdata/Journal-writers-long.cfg is the same
\*              configuration unstaged and fails as SessionGrace describes.
\*   Skips      TRUE adds a writer that names a block it found stored and did
\*              not put, and holds through no root.
\*              testdata/Journal-writers-skip.cfg: the session writing R3
\*              over R2 finds C stored and skips its put, R1 and then C are
\*              freed, and R3 lands over the freed C, all within Grace.
\*   HoleStop   FALSE decrements a lost block's children from a remembered
\*              child list. Journal-hole.cfg: C, named by R1 and R3, is lost;
\*              R1 dies and its cascade frees F and E under R3.
\*   Reput      TRUE lets a later put store a lost block again.
\*              Journal-hole.cfg: R2 is lost under main, a stage built over R1
\*              puts R2 again with the stage as its one implied holder, and
\*              dropping the stage frees R2 under main. The store cannot tell a
\*              refilled hole from a new block at the put, so this rule is an
\*              assumption the Go store must still be made to keep.
\*   Grace      at most Window fails ReaderWindow. Journal-lease.cfg with
\*              Window 2: a reader that saw a head and has not pinned it yet
\*              finds it freed.
\*   CAS        FALSE lets a commit build over a head main no longer names.
\*              Journal.cfg: a writer commits R2 over R1, R1 and then C are
\*              freed, and the writer, its observed head still R1, commits R3
\*              over the freed C. Land refuses no missing child, so compare
\*              and swap is what keeps a writer's base held.
\*   RunCheck   FALSE lets a run's count, settle and end steps write after
\*              another run superseded it. testdata/Journal-adopt-norun.cfg:
\*              run A counts main at R1, run B's start returns the Volume to
\*              unadopted, the World writer moves main onto the stage R3 and
\*              releases it with nothing journaled, A counts no holder of R3,
\*              settles it onto the free queue, marks the Volume adopted, and
\*              the collector frees R3 under main.
\*   Claim      FALSE clears before the run claims the Volume, unchecked, and
\*              marks it adopting over any state.
\*              testdata/Journal-adopt-noclaim.cfg: run A clears, run B
\*              starts while the Volume is still unadopted, so it supersedes
\*              nothing, A marks adopting and counts, B's clear deletes A's
\*              counts, A settles R1 onto the free queue, marks the Volume
\*              adopted, and the collector frees R1 under main.
\*   BornMark   FALSE drops the born marks. testdata/Journal-adopt-noborn.cfg:
\*              main moves onto N, born while the run scans, the settle scan
\*              reads N's child list as one that predates the run, finds no
\*              holder and queues it, and N is freed under main.
\*   RootMark   FALSE drops the moved-root marks.
\*              testdata/Journal-adopt-noroot.cfg: the root scan reads main
\*              at R1, the World writer then moves main onto R3 and releases
\*              the stage that held R3, the scan finds no stage root to count,
\*              and R3 settles onto the free queue and is freed under main.
\*   Rehold     FALSE renews only the stage roots still in the table.
\*              testdata/Journal-stage-norehold.cfg: sT lapses and is swept,
\*              the renewal finds nothing to renew and reports success, and T
\*              is freed while the owner builds on it.
\*   StageCAS   FALSE releases a stage root only when the table holds it.
\*              testdata/Journal-stage-nocas.cfg: the owner stalls past
\*              Lease and Grace, T is freed, and its next put lands S over
\*              the freed T.
\*
\* Left out: the format 7 upgrade (an unadopted Volume here holds every
\* child list it names), the audit, the refusal of blocks stored without a
\* child list, slices larger than one record (they are runs of one-record
\* slices with no writer between), an abandoned run's cleanup (an unadopted
\* Volume's counts are cleared by the next run before they are read), a
\* stage released without publishing, and crashes other than an adoption's:
\* every action is one transaction and the free queue is durable, so a crash
\* between two actions loses nothing the model does not already interleave.

EXTENDS Integers, FiniteSets, Sequences, TLC

CONSTANTS Writers, MaxTxns, MaxNow, Grace, Window, Lease, MaxHoles,
          MainRoots, StageRoots, Reader,
          TailCheck, Held, HoleStop, Reput, CAS, PutPair,
          Runs, MaxOpens, RunCheck, Claim, BornMark, RootMark,
          Stage, Rehold, StageCAS,
          Sessions, SessionGrace, Skips, BornDown, Staged, ReleaseUnlisted

None == "none"
Unmoved == "unmoved"

\* main is the World root, moved by the writers by compare and swap. rdr is a
\* reader's pin of an old root and stg a stage, stage:<ulid>, held by a writer
\* outside the World writer; sT and sS are the roots of the stage owner's
\* build. All but main are leased roots, and Drop reaches only rdr and stg.
Names == {"main", "rdr", "stg", "sT", "sS"}
Leases == {"rdr", "stg", "sT", "sS"}
Dropped == {"rdr", "stg"}

\* Two versions of a tree R -> C -> {E, F} -> G: C2 keeps F, and with it G,
\* and drops E. R3 is a second page over the same C, and N an empty tree.
\* Every tree gives each page one parent. F has two holders while both
\* versions are stored, and G is its only child. S -> T is the stage owner's
\* build.
Blocks == {"R1", "R2", "R3", "C", "C2", "E", "F", "G", "N", "S", "T"}
Kids(b) ==
  CASE b = "R1" -> {"C"}
    [] b = "R2" -> {"C2"}
    [] b = "R3" -> {"C"}
    [] b = "C" -> {"E", "F"}
    [] b = "C2" -> {"F"}
    [] b = "F" -> {"G"}
    [] b = "S" -> {"T"}
    [] OTHER -> {}

\* The key order of the child list and root records an adoption scans.
BlockKeys == <<"C", "C2", "E", "F", "G", "N", "R1", "R2", "R3", "S", "T">>
NameKeys == <<"main", "rdr", "sS", "sT", "stg">>

RECURSIVE Reach(_)
Reach(b) == {b} \cup UNION {Reach(k) : k \in Kids(b)}
ReachOf(x) == IF x = None THEN {} ELSE Reach(x)

Perms == Permutations(Writers)
RunPerms == Permutations(Runs)

\* A Volume adopted from the start is the model of every config without an
\* adoption run. With runs, it starts unadopted, with a stage R3 that predates
\* the journal and the stale records an older format left: a count on E and
\* G on the free queue.
Unadopted == Runs # {}

VARIABLES
  root,      \* each named root's block, or None
  expires,   \* each leased root's expiry on the clock
  stored,    \* blocks with a child list in the store
  lost,      \* blocks the store lost that no put stored again
  cnt,       \* each block's count record: holders minus one, 0 for none;
             \* during an adoption, the holders the run has counted
  zero,      \* the free queue: stored blocks whose netted holders are zero
  jr,        \* the journal past the collector's cursor, oldest first
  under,     \* some transaction took a holder from a block that had none
  now,       \* the clock
  obs,       \* each writer's observed head of main
  seen,      \* the head of main the reader saw and has not pinned, or None
  seenAt,    \* when the reader saw it
  txns,      \* write transactions spent, which keeps every run finite
  holes,     \* blocks punched
  adoption,  \* "unadopted", "adopting" or "adopted"
  owner,     \* the run the state record names, or None
  pc,        \* each run's step
  pos,       \* each run's scan position, an index into the scanned keys
  born,      \* blocks Land gave birth to while adopting
  moved,     \* each root's block when it first moved while adopting
  opens,     \* Volume openings
  stage,     \* the stage owner's build: "none", "T", "S", "done", "failed"
  renewed,   \* when the stage owner last renewed or wrote
  tgt,       \* each writer's session: the tree it is writing, or None
  began,     \* when each writer's session began
  wrote,     \* the blocks each writer's session put
  skipped,   \* the blocks each writer's session found stored and did not put
  sheld,     \* the blocks each writer's stage holds under its leased roots
  freed      \* blocks the collector freed that no put stored again
gcVars == <<cnt, zero, under>>
landVars == <<born, moved, sheld, freed>>
adoptVars == <<adoption, owner, pc, pos, opens>>
stageVars == <<stage, renewed>>
writerVars == <<tgt, began, wrote, skipped>>
vars == <<root, expires, stored, lost, cnt, zero, jr, under, now, obs, seen,
          seenAt, txns, holes, adoption, owner, pc, pos, born, moved, opens,
          stage, renewed, tgt, began, wrote, skipped, sheld, freed>>

Init ==
  /\ root = [n \in Names |->
               IF n = "main" THEN "R1"
               ELSE IF n = "stg" /\ Unadopted THEN "R3" ELSE None]
  /\ expires = [n \in Leases |-> 0]
  /\ stored = Reach("R1") \cup (IF Unadopted THEN {"R3"} ELSE {})
  /\ lost = {}
  /\ cnt = [b \in Blocks |-> IF b = "E" /\ Unadopted THEN 1 ELSE 0]
  /\ zero = IF Unadopted THEN {"G"} ELSE {}
  /\ jr = <<>>
  /\ under = FALSE
  /\ now = 0
  /\ obs = [w \in Writers |-> None]
  /\ seen = None
  /\ seenAt = 0
  /\ txns = 0
  /\ holes = 0
  /\ adoption = IF Unadopted THEN "unadopted" ELSE "adopted"
  /\ owner = None
  /\ pc = [r \in Runs |-> "idle"]
  /\ pos = [r \in Runs |-> 0]
  /\ born = {}
  /\ moved = [n \in Names |-> Unmoved]
  /\ opens = 0
  /\ stage = "none"
  /\ renewed = 0
  /\ tgt = [w \in Writers |-> None]
  /\ began = [w \in Writers |-> 0]
  /\ wrote = [w \in Writers |-> {}]
  /\ skipped = [w \in Writers |-> {}]
  /\ sheld = [w \in Writers |-> {}]
  /\ freed = {}

\* The holders the netted counts give b.
Holders(b) == IF b \in zero THEN 0 ELSE cnt[b] + 1

\* The up and down pairs of the records at idx that name b. A pair is tagged
\* with its record, so one holder in two records counts twice.
Ups(idx, b) == Cardinality(UNION {{<<i, x>> : x \in {y \in jr[i].up : y[2] = b}} : i \in idx})
Downs(idx, b) == Cardinality(UNION {{<<i, x>> : x \in {y \in jr[i].down : y[2] = b}} : i \in idx})
Pending == 1..Len(jr)

\* Step adds the deltas d to the netted holders of the blocks in st and writes
\* only the blocks whose holders changed. A block at zero holders joins the
\* free queue and one below zero is an underflow.
Step(st, d) ==
  LET nh(b) == Holders(b) + d[b]
  IN [cnt |-> [b \in Blocks |->
                IF b \in st /\ d[b] # 0 THEN (IF nh(b) > 1 THEN nh(b) - 1 ELSE 0)
                ELSE cnt[b]],
      zero |-> {b \in st \cap stored :
                  IF d[b] # 0 THEN nh(b) <= 0 ELSE b \in zero},
      under |-> \E b \in st : nh(b) < 0]

\* ---------------------------------------------------------------------------
\* The writer side: SetRoots, one write transaction.

\* Every record must be able to age Grace before the clock stops, so the
\* collector can consume it.
Live == now + Grace <= MaxNow /\ txns < MaxTxns

\* Land is SetRoots(batch, moves). moves is a set of [n, from, to] root moves,
\* from the root's Previous; batch is the blocks the caller puts. born, up and
\* down are computed against the store and the root table as they stand
\* inside the transaction. An unadopted Volume journals nothing; an adopting
\* one also marks the blocks it gives birth to and each root's first move.
\* holds and rels are the blocks a writer's stage takes and releases a leased
\* root on, one root per block, as stageMoves names them. A born block that
\* nothing in the transaction holds, no parent born with it, no root and no
\* stage root, gets a down pair for its implied holder with BornDown.
LandCore(batch, moves, holds, rels) ==
  LET new == batch \ stored
      found == UNION {{<<p, k>> : k \in Kids(p) \ new} : p \in new}
      held == {<<m.n, m.to>> : m \in {x \in moves : x.to \in stored}} \cup
              {<<"stage", b>> : b \in holds \cap stored}
      again == IF PutPair THEN {<<"put", b>> : b \in batch \cap stored} ELSE {}
      holder(b) == \/ \E p \in new : b \in Kids(p)
                   \/ \E m \in moves : m.to = b
                   \/ b \in holds
      orphan == IF BornDown
                THEN {<<"born", b>> : b \in {x \in new : ~holder(x)}} ELSE {}
      up == found \cup again \cup (IF Held THEN held ELSE {})
      down == again \cup orphan \cup {<<"stage", b>> : b \in rels} \cup
              {<<m.n, root[m.n]>> : m \in {x \in moves : root[x.n] # None}}
      adopting == adoption = "adopting"
  IN /\ Live
     /\ \A m \in moves : m.to # root[m.n]
     /\ CAS => \A m \in moves : root[m.n] = m.from
     \* A root names a block the store holds or the batch stores.
     /\ \A m \in moves : m.to = None \/ m.to \in stored \cup batch
     /\ holds \subseteq stored \cup batch
     /\ ~Reput => batch \cap lost = {}
     /\ stored' = stored \cup batch
     /\ lost' = lost \ batch
     /\ root' = [n \in Names |->
                   IF \E m \in moves : m.n = n
                   THEN (CHOOSE m \in moves : m.n = n).to ELSE root[n]]
     /\ jr' = IF adoption = "unadopted" \/ up \cup down = {} THEN jr
              ELSE Append(jr, [at |-> now, up |-> up, down |-> down])
     /\ born' = IF adopting /\ BornMark THEN born \cup new ELSE born
     /\ moved' = IF adopting /\ RootMark
                 THEN [n \in Names |->
                         IF moved[n] = Unmoved /\ \E m \in moves : m.n = n
                         THEN root[n] ELSE moved[n]]
                 ELSE moved
     /\ freed' = freed \ batch
     /\ txns' = txns + 1

Land(batch, moves) == LandCore(batch, moves, {}, {}) /\ sheld' = sheld

\* LandAs is Land for writer w's stage, which takes holds and releases rels.
LandAs(w, batch, moves, holds, rels) ==
  /\ holds \cap sheld[w] = {} /\ rels \subseteq sheld[w]
  /\ LandCore(batch, moves, holds, rels)
  /\ sheld' = [sheld EXCEPT ![w] = (@ \ rels) \cup holds]

\* Observe reads the head of main, as a writer does before it builds a tree.
Observe(w) ==
  /\ tgt[w] = None
  /\ obs' = [obs EXCEPT ![w] = root["main"]]
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, seen, seenAt,
                 txns, holes, landVars, adoptVars, stageVars, writerVars>>

\* Commit moves main from the head the writer observed to r. The writer puts
\* the pages of r its observed head does not reach; the pages they share it
\* names without putting them again.
Commit(w, r) ==
  /\ obs[w] # None /\ r # obs[w]
  /\ Land(Reach(r) \ Reach(obs[w]), {[n |-> "main", from |-> obs[w], to |-> r]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, writerVars>>

\* StageWrite holds a stage tree r built over the head of main the writer
\* observed, under a free stage root, and starts its lease. The pages r shares
\* with that head are named, not put, so the writer must still hold them: the
\* head is main's when the stage lands, which stands for the leased pin a
\* writer holds on its base for the life of its write.
StageWrite(w, r) ==
  /\ obs[w] # None /\ obs[w] = root["main"]
  /\ Land(Reach(r) \ Reach(obs[w]), {[n |-> "stg", from |-> None, to |-> r]})
  /\ expires' = [expires EXCEPT !["stg"] = now + Lease]
  /\ UNCHANGED <<gcVars, now, obs, seen, seenAt, holes, adoptVars, stageVars,
                 writerVars>>

\* AdoptStage moves main from the observed head to stage s and releases the
\* stage in the same transaction, so s never has zero holders between.
AdoptStage(w, s) ==
  /\ obs[w] # None
  /\ Land({}, {[n |-> "main", from |-> obs[w], to |-> s],
               [n |-> "stg", from |-> s, to |-> None]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, writerVars>>

\* A writer session writes the tree tgt[w] over its observed head across
\* several SetRoots, as BufferedStore's capacity drain and the object copy
\* do: batches of puts with no root, in any order, a parent before its child
\* or after it, then the root move with what is left. Need is what the
\* session must store: the pages its observed head does not reach, which
\* main holds while it names that head, as Commit's compare and swap checks.
\*
\* With Staged the session writes under a stage instead, the way a write that
\* can outlast Grace must: it pins its observed head under the stage when it
\* begins, each put holds the blocks the batch does not name under a leased
\* stage root and releases the stage's roots on the blocks it names, and the
\* last SetRoots moves main onto the tree and releases the stage. The leases
\* are renewed for the session's life; Journal-stage.cfg checks a renewal
\* that stalls.
Need(w) == Reach(tgt[w]) \ Reach(obs[w])
Unsent(w) == Need(w) \ (wrote[w] \cup skipped[w])
Named(batch) == UNION {Kids(p) : p \in batch}

\* SessionBegin starts writing tree r over the observed head. Staged, it is
\* OpenStage and a stage root on the head: HoldStageCompleteRoots, or a
\* reader pin held for the write.
SessionBegin(w, r) ==
  /\ Sessions /\ tgt[w] = None /\ sheld[w] = {} /\ obs[w] # None /\ r # obs[w]
  /\ tgt' = [tgt EXCEPT ![w] = r]
  /\ began' = [began EXCEPT ![w] = now]
  /\ IF Staged
     THEN /\ LandAs(w, {}, {}, {obs[w]}, {})
          /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes,
                         adoptVars, stageVars, wrote, skipped>>
     ELSE UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seen,
                      seenAt, txns, holes, landVars, adoptVars, stageVars,
                      wrote, skipped>>

\* SessionPut is one PutBlockBatch of the session: any blocks it has not put,
\* through Land. A block it names that the batch does not store gains an up
\* pair, stored or not, and a block already stored is put again, which
\* journals an up and a down pair. Staged, it is PrepareStagedBlockBatch.
SessionPut(w, batch) ==
  /\ tgt[w] # None /\ batch # {} /\ batch \subseteq Unsent(w)
  /\ IF Staged
     THEN LandAs(w, batch, {}, (batch \ Named(batch)) \ sheld[w],
                 (sheld[w] \ {obs[w]}) \cap Named(batch))
     ELSE Land(batch, {})
  /\ wrote' = [wrote EXCEPT ![w] = @ \cup batch]
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, tgt, began, skipped>>

\* SessionSkip is a writer that reads GetBlockExists as true and does not put
\* the block: it neither put it in this session nor reaches it from a root it
\* holds, and later names it. No transaction records the read.
SessionSkip(w, b) ==
  /\ Skips /\ tgt[w] # None /\ b \in Unsent(w) /\ b \in stored
  /\ skipped' = [skipped EXCEPT ![w] = @ \cup {b}]
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seen,
                 seenAt, txns, holes, landVars, adoptVars, stageVars, tgt,
                 began, wrote>>

\* SessionFinish is the session's last SetRoots: the blocks it has not put or
\* skipped, and main moved by compare and swap from the observed head; staged,
\* it releases every stage root in the same transaction. With SessionGrace it
\* lands within Grace of the session's start, the contract an unstaged
\* session must keep.
SessionFinish(w) ==
  /\ tgt[w] # None
  /\ SessionGrace => now < began[w] + Grace
  /\ LandAs(w, Unsent(w), {[n |-> "main", from |-> obs[w], to |-> tgt[w]]},
            {}, sheld[w])
  /\ tgt' = [tgt EXCEPT ![w] = None]
  /\ wrote' = [wrote EXCEPT ![w] = {}]
  /\ skipped' = [skipped EXCEPT ![w] = {}]
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, began>>

\* SessionAbandon is a session whose write failed. What it put stays stored
\* until the collector frees it: unstaged, once the down pair of each block
\* born with no holder ages Grace; staged, once the stage's release, which
\* the next transaction makes after releaseStage, ages Grace.
SessionAbandon(w) ==
  /\ tgt[w] # None
  /\ tgt' = [tgt EXCEPT ![w] = None]
  /\ wrote' = [wrote EXCEPT ![w] = {}]
  /\ skipped' = [skipped EXCEPT ![w] = {}]
  /\ IF sheld[w] = {}
     THEN UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seen,
                      seenAt, txns, holes, landVars, adoptVars, stageVars,
                      began>>
     ELSE /\ LandAs(w, {}, {}, {}, sheld[w])
          /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes,
                         adoptVars, stageVars, began>>

\* ReaderSee: a reader learns the head of main and reads its tree without a
\* pin until it pins, which it must do within Window.
ReaderSee ==
  /\ Reader /\ seen = None /\ root["rdr"] = None
  /\ seen' = root["main"]
  /\ seenAt' = now
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, txns, holes,
                 landVars, adoptVars, stageVars, writerVars>>

\* ReaderPin holds the block the reader saw as a leased root. SetRoots refuses
\* it when the block is gone, which Grace keeps from happening inside Window.
ReaderPin ==
  /\ seen # None /\ now <= seenAt + Window
  /\ Land({}, {[n |-> "rdr", from |-> None, to |-> seen]})
  /\ expires' = [expires EXCEPT !["rdr"] = now + Lease]
  /\ seen' = None
  /\ UNCHANGED <<gcVars, now, obs, seenAt, holes, adoptVars, stageVars,
                 writerVars>>

\* ReaderLate: a reader that missed its window gives up its unpinned read.
ReaderLate ==
  /\ seen # None /\ now > seenAt + Window
  /\ seen' = None
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seenAt,
                 txns, holes, landVars, adoptVars, stageVars, writerVars>>

\* Drop releases a leased root, as its holder does when it is done.
Drop(n) ==
  /\ root[n] # None
  /\ Land({}, {[n |-> n, from |-> root[n], to |-> None]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, writerVars>>

\* Sweep is the collector releasing a root whose lease lapsed, whoever holds
\* it and whether or not its owner still runs.
Sweep(n) ==
  /\ root[n] # None /\ now > expires[n]
  /\ Land({}, {[n |-> n, from |-> root[n], to |-> None]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, writerVars>>

\* ---------------------------------------------------------------------------
\* The stage owner: a tree S -> T built in two puts under leased roots.

\* The root the owner's record says it holds, and its block.
StageName == IF stage = "T" THEN "sT" ELSE "sS"
StageTop == IF stage = "T" THEN "T" ELSE "S"
Building == stage \in {"T", "S"}

\* StagePut1 is prepareStaged storing T under the new root sT.
StagePut1 ==
  /\ Stage /\ stage = "none"
  /\ Land({"T"}, {[n |-> "sT", from |-> None, to |-> "T"]})
  /\ expires' = [expires EXCEPT !["sT"] = now + Lease]
  /\ stage' = "T" /\ renewed' = now
  /\ UNCHANGED <<gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 writerVars>>

\* StagePut2 is prepareStaged storing S, which names T, under the new root sS
\* and releasing sT, whose holder S now is. With StageCAS the release carries
\* the owner's own record of sT as its Previous, so the put is refused once a
\* sweep took sT; without it the put releases sT only if the table still
\* holds it.
StagePut2 ==
  /\ stage = "T"
  /\ Land({"S"}, {[n |-> "sS", from |-> None, to |-> "S"]} \cup
                 (IF StageCAS \/ root["sT"] = "T"
                  THEN {[n |-> "sT", from |-> "T", to |-> None]} ELSE {}))
  /\ expires' = [expires EXCEPT !["sS"] = now + Lease]
  /\ stage' = "S" /\ renewed' = now
  /\ UNCHANGED <<gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 writerVars>>

\* StagePublish moves main from the head the World writer observed onto S
\* and releases sS in one transaction.
StagePublish(w) ==
  /\ stage = "S" /\ obs[w] # None
  /\ Land({}, {[n |-> "main", from |-> obs[w], to |-> "S"],
               [n |-> "sS", from |-> "S", to |-> None]})
  /\ stage' = "done"
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 renewed, writerVars>>

\* StageRenew moves the owner's root's expiry forward. A root a sweep took is
\* held again while its block is stored, through Land, which journals the up
\* pair the tail check reads; when the block is gone the stage fails. Without
\* Rehold the renewal renews only what the table still holds and reports
\* success.
StageRenew ==
  /\ Building
  /\ \/ /\ root[StageName] = StageTop
        /\ expires' = [expires EXCEPT ![StageName] = now + Lease]
        /\ renewed' = now
        /\ UNCHANGED <<root, stored, lost, gcVars, jr, txns, landVars, stage>>
     \/ /\ root[StageName] = None /\ Rehold /\ StageTop \in stored
        /\ Land({}, {[n |-> StageName, from |-> None, to |-> StageTop]})
        /\ expires' = [expires EXCEPT ![StageName] = now + Lease]
        /\ renewed' = now
        /\ UNCHANGED <<gcVars, stage>>
     \/ /\ root[StageName] = None /\ Rehold /\ StageTop \notin stored
        /\ stage' = "failed"
        /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, txns, landVars,
                       renewed>>
     \/ /\ root[StageName] = None /\ ~Rehold
        /\ renewed' = now
        /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, txns, landVars,
                       stage>>
  /\ UNCHANGED <<now, obs, seen, seenAt, holes, adoptVars, writerVars>>

\* ---------------------------------------------------------------------------
\* The collector.

\* Net consumes the first k records as one interval; every one has aged Grace.
Net(k) ==
  /\ adoption = "adopted"
  /\ \A i \in 1..k : jr[i].at + Grace <= now
  /\ LET s == Step(Blocks, [b \in Blocks |-> Ups(1..k, b) - Downs(1..k, b)])
     IN /\ cnt' = s.cnt
        /\ zero' = s.zero
        /\ under' = (under \/ s.under)
  /\ jr' = SubSeq(jr, k + 1, Len(jr))
  /\ UNCHANGED <<root, expires, stored, lost, now, obs, seen, seenAt, txns,
                 holes, landVars, adoptVars, stageVars, writerVars>>

\* What Delete decrements: the children of the freed block. A lost child has
\* no child list, so HoleStop keeps its children; the cascade stops there.
Decrement(b) ==
  Kids(b) \cup IF HoleStop THEN {}
               ELSE UNION {Kids(c) : c \in Kids(b) \cap lost}

\* Delete frees queued block b and decrements its children, which may queue
\* them. With TailCheck it refuses while an up pair in the tail names b. A
\* child not stored yet, which a parent put before it named, holds the up
\* pair that parent journaled; with ReleaseUnlisted the decrement reaches its
\* count too, and only a stored child is queued. A lost child keeps its count,
\* which went with it.
Delete(b) ==
  /\ adoption = "adopted"
  /\ b \in zero
  /\ TailCheck => Ups(Pending, b) = 0
  /\ LET d == [c \in Blocks |-> IF c \in Decrement(b) THEN -1 ELSE 0]
         s == Step(IF ReleaseUnlisted THEN (Blocks \ {b}) \ lost
                   ELSE stored \ {b}, d)
     IN /\ stored' = stored \ {b}
        /\ cnt' = [s.cnt EXCEPT ![b] = 0]
        /\ zero' = s.zero
        /\ under' = (under \/ s.under)
  /\ freed' = freed \cup {b}
  /\ UNCHANGED <<root, expires, lost, jr, now, obs, seen, seenAt, txns, holes,
                 born, moved, sheld, adoptVars, stageVars, writerVars>>

\* ---------------------------------------------------------------------------
\* Adoption: runs r of block_gc.Adopt, one transaction per action.

\* The least index past p whose key is in set, or 0 when none is.
NextKey(keys, p, set) ==
  LET later == {i \in (p + 1)..Len(keys) : keys[i] \in set}
  IN IF later = {} THEN 0 ELSE CHOOSE i \in later : \A j \in later : i <= j

\* r holds its claim: the Volume is unadopted and names r.
Claimed(r) == adoption = "unadopted" /\ owner = r
\* r still owns the adoption: the Volume is adopting in r.
InRun(r) == adoption = "adopting" /\ owner = r

\* A superseded run returns ErrAdoptSuperseded and writes nothing.
Superseded(r) ==
  /\ pc' = [pc EXCEPT ![r] = "dead"]
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seen,
                 seenAt, txns, holes, landVars, adoption, owner, pos, opens,
                 stageVars, writerVars>>

\* Go moves r to the next step, leaving the records alone.
Go(r, step) ==
  /\ pc' = [pc EXCEPT ![r] = step]
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seen,
                 seenAt, txns, holes, landVars, adoption, owner, opens,
                 stageVars, writerVars>>

AdoptUnchanged ==
  UNCHANGED <<root, expires, stored, lost, under, jr, now, obs, seen, seenAt,
              txns, holes, opens, sheld, freed, stageVars, writerVars>>

\* AdoptStart reports an adopted Volume, and otherwise returns it to
\* unadopted, superseding any run, and with Claim names r in it.
AdoptStart(r) ==
  /\ pc[r] = "idle"
  /\ IF adoption = "adopted"
     THEN Go(r, "done") /\ UNCHANGED pos
     ELSE /\ adoption' = "unadopted"
          /\ owner' = IF Claim THEN r ELSE None
          /\ pc' = [pc EXCEPT ![r] = "clear"]
          /\ UNCHANGED <<cnt, zero, landVars, pos>>
          /\ AdoptUnchanged

\* AdoptClear deletes every count, queue entry and mark while r holds its
\* claim; without Claim it deletes them whatever the state.
AdoptClear(r) ==
  /\ pc[r] = "clear"
  /\ IF Claim /\ ~Claimed(r)
     THEN Superseded(r)
     ELSE /\ cnt' = [b \in Blocks |-> 0]
          /\ zero' = {}
          /\ born' = {}
          /\ moved' = [n \in Names |-> Unmoved]
          /\ pc' = [pc EXCEPT ![r] = "mark"]
          /\ UNCHANGED <<adoption, owner, pos>>
          /\ AdoptUnchanged

\* AdoptMark marks the Volume adopting in r at the journal's end. The
\* collector's cursor moves there, and the records before it, which the
\* counts will reflect, are never read again.
AdoptMark(r) ==
  /\ pc[r] = "mark"
  /\ IF Claim /\ ~Claimed(r)
     THEN Superseded(r)
     ELSE /\ adoption' = "adopting"
          /\ owner' = r
          /\ jr' = <<>>
          /\ pc' = [pc EXCEPT ![r] = "roots"]
          /\ pos' = [pos EXCEPT ![r] = 0]
          /\ UNCHANGED <<gcVars, landVars, root, expires, stored, lost, now,
                         obs, seen, seenAt, txns, holes, opens, stageVars,
                         writerVars>>

\* A count, settle or end step of r: with RunCheck it first checks that the
\* Volume is still adopting in r.
Owned(r) == RunCheck => InRun(r)

\* AdoptRoot counts the next name, in key order, of the root table and the
\* moved marks, at the block it held when the adoption began: its mark, or
\* else its table entry.
AdoptRoot(r) ==
  /\ pc[r] = "roots"
  /\ IF ~Owned(r) THEN Superseded(r)
     ELSE LET i == NextKey(NameKeys, pos[r],
                           {n \in Names : root[n] # None \/ moved[n] # Unmoved})
              n == NameKeys[i]
              start == IF moved[n] # Unmoved THEN moved[n] ELSE root[n]
          IN IF i = 0 THEN Go(r, "lists") /\ pos' = [pos EXCEPT ![r] = 0]
             ELSE /\ cnt' = IF start = None THEN cnt
                            ELSE [cnt EXCEPT ![start] = @ + 1]
                  /\ pos' = [pos EXCEPT ![r] = i]
                  /\ UNCHANGED <<zero, landVars, adoption, owner, pc>>
                  /\ AdoptUnchanged

\* AdoptList counts a holder of each child of the next child list, in key
\* order, that no born mark names.
AdoptList(r) ==
  /\ pc[r] = "lists"
  /\ IF ~Owned(r) THEN Superseded(r)
     ELSE LET i == NextKey(BlockKeys, pos[r], stored)
              b == BlockKeys[i]
          IN IF i = 0 THEN Go(r, "settle") /\ pos' = [pos EXCEPT ![r] = 0]
             ELSE /\ cnt' = IF b \in born THEN cnt
                            ELSE [c \in Blocks |->
                                    cnt[c] + IF c \in Kids(b) THEN 1 ELSE 0]
                  /\ pos' = [pos EXCEPT ![r] = i]
                  /\ UNCHANGED <<zero, landVars, adoption, owner, pc>>
                  /\ AdoptUnchanged

\* AdoptSettle settles the block of the next child list no born mark names:
\* one with no holder goes on the free queue, and any other drops the holder
\* its child list implies.
AdoptSettle(r) ==
  /\ pc[r] = "settle"
  /\ IF ~Owned(r) THEN Superseded(r)
     ELSE LET i == NextKey(BlockKeys, pos[r], stored)
              b == BlockKeys[i]
          IN IF i = 0 THEN Go(r, "end") /\ UNCHANGED pos
             ELSE /\ IF b \in born THEN UNCHANGED <<cnt, zero>>
                     ELSE IF cnt[b] = 0
                     THEN zero' = zero \cup {b} /\ UNCHANGED cnt
                     ELSE cnt' = [cnt EXCEPT ![b] = @ - 1] /\ UNCHANGED zero
                  /\ pos' = [pos EXCEPT ![r] = i]
                  /\ UNCHANGED <<landVars, adoption, owner, pc>>
                  /\ AdoptUnchanged

\* AdoptEnd marks the Volume adopted and deletes the marks, which nothing
\* reads once it is.
AdoptEnd(r) ==
  /\ pc[r] = "end"
  /\ IF ~Owned(r) THEN Superseded(r)
     ELSE /\ adoption' = "adopted"
          /\ born' = {}
          /\ moved' = [n \in Names |-> Unmoved]
          /\ pc' = [pc EXCEPT ![r] = "done"]
          /\ UNCHANGED <<gcVars, owner, pos>>
          /\ AdoptUnchanged

\* AdoptCrash stops r at any step.
AdoptCrash(r) ==
  /\ pc[r] \in {"clear", "mark", "roots", "lists", "settle", "end"}
  /\ Go(r, "dead") /\ UNCHANGED pos

\* Open returns an adopting Volume to unadopted, superseding its run.
Open ==
  /\ opens < MaxOpens /\ adoption = "adopting"
  /\ adoption' = "unadopted" /\ owner' = None /\ opens' = opens + 1
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seen,
                 seenAt, txns, holes, landVars, pc, pos, stageVars, writerVars>>

\* ---------------------------------------------------------------------------
\* The environment.

\* Punch loses block b with its child list and count record. Blocks it named
\* stay stored, held by a page nobody can read.
Punch(b) ==
  /\ holes < MaxHoles /\ b \in stored
  /\ stored' = stored \ {b} /\ lost' = lost \cup {b}
  /\ cnt' = [cnt EXCEPT ![b] = 0] /\ zero' = zero \ {b}
  /\ holes' = holes + 1
  /\ UNCHANGED <<root, expires, jr, under, now, obs, seen, seenAt, txns,
                 landVars, adoptVars, stageVars, writerVars>>

Tick ==
  /\ now < MaxNow /\ now' = now + 1
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, obs, seen, seenAt,
                 txns, holes, landVars, adoptVars, stageVars, writerVars>>

NetAny == \E k \in 1..Len(jr) : Net(k)
DeleteAny == \E b \in Blocks : Delete(b)

Next ==
  \/ \E w \in Writers :
       \/ Observe(w)
       \/ \E r \in MainRoots : Commit(w, r)
       \/ \E r \in StageRoots : StageWrite(w, r) \/ AdoptStage(w, r)
       \/ StagePublish(w)
       \/ \E r \in MainRoots : SessionBegin(w, r)
       \/ tgt[w] # None /\ \E batch \in SUBSET Unsent(w) : SessionPut(w, batch)
       \/ tgt[w] # None /\ \E b \in Unsent(w) : SessionSkip(w, b)
       \/ SessionFinish(w) \/ SessionAbandon(w)
  \/ ReaderSee \/ ReaderPin \/ ReaderLate
  \/ \E n \in Dropped : Drop(n)
  \/ \E n \in Leases : Sweep(n)
  \/ StagePut1 \/ StagePut2 \/ StageRenew
  \/ \E r \in Runs :
       \/ AdoptStart(r) \/ AdoptClear(r) \/ AdoptMark(r) \/ AdoptRoot(r)
       \/ AdoptList(r) \/ AdoptSettle(r) \/ AdoptEnd(r) \/ AdoptCrash(r)
  \/ Open
  \/ \E b \in Blocks : Punch(b)
  \/ Tick
  \/ NetAny
  \/ DeleteAny

Spec == Init /\ [][Next]_vars /\ WF_vars(Tick) /\ WF_vars(NetAny)
          /\ WF_vars(DeleteAny)

\* ---------------------------------------------------------------------------
\* Properties.

HeldBlocks ==
  ({root[n] : n \in Names} \ {None}) \cup UNION {sheld[w] : w \in Writers}
LiveSet == UNION {Reach(b) : b \in HeldBlocks}
BelowLost == UNION {Reach(h) \ {h} : h \in lost}
Adopted == adoption = "adopted"

\* No block a held root reaches is freed, but one a session has still to put:
\* a staged session may hold a parent before it puts the child again. A block
\* a writer skipped is not one it has to put. This covers main moved by
\* compare and swap while a pass ran, a reader's pin, a stage, every adoption
\* and every writer session.
ToPut == UNION {IF tgt[w] = None THEN {} ELSE Unsent(w) : w \in Writers}
NoFreedRead == (LiveSet \cap freed) \subseteq ToPut

\* A reader that saw a head and still has time to pin it finds all of it.
ReaderWindow ==
  seen # None /\ now <= seenAt + Window => ReachOf(seen) \subseteq stored \cup lost

\* No transaction took a holder from a block that had none.
NoUnderflow == ~under

\* The free queue holds only stored blocks.
ZeroStored == zero \subseteq stored

\* The holders of a block, from the roots and the stored child lists.
TrueHolders(b) ==
  Cardinality({n \in Names : root[n] = b}) +
  Cardinality({w \in Writers : b \in sheld[w]}) +
  Cardinality({p \in stored : b \in Kids(p)})

\* The netted holders plus the journal past the cursor.
JournalHolders(b) == Holders(b) + Ups(Pending, b) - Downs(Pending, b)

\* Once adopted, the journal never counts fewer holders than a block has:
\* fewer frees a live block. More leaks, which only a hole may cause.
CountsCover == Adopted => \A b \in stored : JournalHolders(b) >= TrueHolders(b)

\* Once adopted with no hole, the journal counts each block's holders
\* exactly.
CountsExact ==
  Adopted /\ lost = {} => \A b \in stored : JournalHolders(b) = TrueHolders(b)

\* A lost block's children stay stored: the cascade stopped at the hole.
HoleKeeps == \A h \in lost : Kids(h) \subseteq stored \cup lost

Quiet == Adopted /\ jr = <<>> /\ zero = {}

\* Once the journal is consumed and the free queue empty, the store holds what
\* the roots reach and what lies below a hole: the audit's Leaked.
NoLeak == Quiet => stored \subseteq LiveSet \cup BelowLost

\* An owner that renewed or wrote within Lease and Grace finds every block
\* of the tree its record holds: a stall shorter than that loses nothing,
\* and after a longer one the renewal fails the stage or holds it again.
StageKept ==
  Building /\ now <= renewed + Lease + Grace => Reach(StageTop) \subseteq stored

\* Every block no root reaches is freed in the end, but those below a hole.
Collected == <>[](Quiet /\ stored \subseteq LiveSet \cup BelowLost)
====
