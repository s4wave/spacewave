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
\* and a down pair, which a Go record's again list carries as one entry: they
\* net to nothing, but a block at zero they name takes
\* the journal's end as its ripe sequence again when they net (Reripen), so
\* the put restarts its grace. A block born with a
\* holder in its batch, a parent born with it, a root or a stage root, takes
\* that holder as its implied one. A block born with none gets a down pair
\* <<"born", block>> for its implied holder (BornDown): it is garbage once that
\* record ages Grace, unless a parent's up pair or a root lands first, so a
\* put before its parent's is counted exactly and an abandoned put is freed.
\*
\* The collector. A block with no holders holds nothing: the holders of a
\* block are the roots that name it and the child lists of parents that have
\* holders. Two marks give each stored block one of four states:
\*   Live     holders, and its children count it; neither mark.
\*   Dying    no holders, and its children still count it: on the free
\*            queue (zero), its release due.
\*   Dead     no holders, its children released (zero and rel): its delete
\*            due.
\*   Holding  holders again, its children released (rel): its re-hold due.
\* Net consumes a prefix of the journal as one interval whose records have all
\* aged Grace, adds the interval's deltas to the netted counts in one
\* transaction, writes only the blocks whose holders changed, stored or not,
\* and marks a stored block whose holders reach zero Dying, with the journal's
\* end as its ripe sequence; a Dead block that gains a holder turns Holding.
\* Cascade starts a due release or re-hold, and Release moves each child's
\* holders by one, which may make the child Dying in turn, or Live again.
\* Delete frees a Dead block and changes no other count. A child in a dead
\* page and a born page of one interval nets to no change and keeps its
\* implied holder unwritten. None of these runs until the Volume is adopted.
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
\* lapsed and was swept again while its block is stored with holders, as
\* reader pins are; when the block is gone or at zero, the stage fails and its
\* writes are refused. A put
\* that builds on a root the stage holds releases it with the owner's record
\* as its Previous, so a put over a root a sweep took is refused. Renewal may
\* stall for any time: the clock runs on while the owner does nothing.
\* Sweep releases any lapsed lease, as another process's collector does. A
\* process's own collector renews its own leases first, so it releases only
\* those of owners that are gone, which is a subset of these behaviors.
\*
\* Cache fills. A fill copies a block a peer holds into the store when a read
\* misses, and is an ordinary unheld put: it stores the block with its child
\* list, journals an up pair for each child the list names and a down pair
\* for the block's own implied holder, as a block born with no holder does.
\* Its children are held from the moment it lands, and the block itself is
\* garbage after Grace unless something holds it. A fill lands at any time,
\* also after a root came to hold its parent without putting the block. Its
\* record goes to a keyspace of its own, keyed by the block, as a blind write
\* with no shared sequence number, so an optimistic fill never conflicts with
\* a commit over the journal's bookkeeping. The collector nets each aged fill
\* record whole and alone, before or after the main records that name the
\* same blocks: the sums do not depend on the order. Its ups do not wait:
\* the collector applies them when it first finds the record (FillEager), and
\* Delete waits while any fill's ups are unapplied, so a fill that lands
\* after its child turned Dying holds the child before it can be freed; one
\* that lands after the free names a block no stored block reached when it
\* was freed. Netting a record then takes only its down. A fill
\* lands whatever the adoption state. While the Volume is unadopted the
\* collector deletes fill records, and an adoption's clear deletes each with
\* its index entries; one that lands after the clear makes its block born to
\* the adoption, as a Land while adopting does, so the adoption neither
\* counts the block's children nor settles it, and the record supplies both.
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
\*                   several batches, BufferedStore's drain, the object
\*                   copy, or the Space World replay, whose pages past
\*                   memory land before the cursor save that holds their
\*                   World; SessionFinish is its closing SetRoots. Staged, a
\*                   session is OpenStage, PrepareStagedBlockBatch and a
\*                   publish that releases the stage. The replay can wait
\*                   on a save past Grace, so it writes staged: one save
\*                   epoch of its replayStage is one session, whose
\*                   puts, each transaction's early drains among them, go
\*                   to the epoch's one stage, and whose publish is the
\*                   cursor save that lands. Its target is the World the
\*                   save names; replay computes the Worlds before that
\*                   one again, once an epoch that stored a page ends.
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
\*   Net(k)          Collector.netSlice with the netting's resolve and write:
\*                   read the journal from the cursor to the grace horizon,
\*                   net per block, write the count records and free queue
\*                   entries that changed with the ripe sequence of each new
\*                   one, advance the cursor.
\*   Cascade(b)      one due block in a collector slice: mark it Dead or
\*                   Live, and start its children's release or re-hold.
\*   Release(c)      one child of the cascade, in its transaction or a
\*                   later one, as continueRelease does. A slice yields to a
\*                   waiting writer between these transactions, so writers
\*                   interleave anywhere between two of them.
\*   Delete(b)       one Dead block in a collector slice, freeBlock: once no
\*                   cascade is due or running, the cursor has passed the
\*                   block's ripe sequence and the hold mark, and no fill's
\*                   ups wait, delete the block with its child list and count
\*                   record.
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
\*   Fill(b)         block_gc.LandFill, from a cache fill on the Volume's
\*                   read path, with its fill record.
\*   ApplyFill(r)    the collector applying a fill record's ups when it
\*                   first finds the record.
\*   NetFill(r)      the collector netting one aged fill record's down.
\*   HoldStored(w,r) a root moving onto a block the store holds without a put
\*                   of its tree, such as a copy that finds its head stored.
\*   Punch(b)        a hole: a block the store lost with its child list.
\*   Tick            the clock Grace, Window and leases are read on.
\*
\* Ripeness (Ripe). The collector decides a block has no holders from a
\* journal prefix, and a record past the cursor may add a holder to it: a put
\* that found it still stored, or a root that now names it. When Net marks a
\* block Dying it records the journal's end as the block's ripe sequence, and
\* Delete waits until the cursor passes it, so every record that landed
\* before the block reached zero has netted, and a holder one added has made
\* the block Holding. Delete reads one sequence per block and indexes no
\* record.
\*
\* The hold mark (HoldMark). A release at zero takes the holder from the
\* children at once, so a later hold of a Dead block gives its children a
\* holder back only through the re-hold after its record nets, which may come
\* after a child's own ripe sequence. A Land that moves a root or stage root
\* onto a stored block, or puts one again, reads that block's count; when it
\* is at zero or its children are released, Dying, Dead or Holding, or any
\* cascade is due or running, so that a block below a Live one may have no
\* holder, Land sets the Volume's hold mark to its own record's sequence,
\* and Delete waits until the cursor passes the hold mark. By then the record
\* has netted and the block's re-hold is due, which Delete waits for too. A
\* put again of a block that stays at zero once its record nets sets the
\* block's ripe sequence to the journal's end again (Reripen), covering the
\* rest of the session that put it.
\*
\* Completion proofs. A child released by its parent's cascade may be freed
\* before the parent, since both ripen at the journal's end and the free walks
\* the queue by sequence, then by block. A stored block therefore no longer
\* implies its tree, so the collector drops a block's completion proof when
\* its release starts, and a stage pin, HoldStageCompleteRoots, refuses a
\* head whose children do not hold it (SessionBegin).
\*
\* Switches, each set to the rule the Go collector must keep in every config
\* here. Flipping one breaks a rule in the config named, in the run TLC
\* reports, which shows the rule is load bearing; CAS is the exception. The
\* configs under testdata/ flip the adoption, stage and writer switches and
\* fail.
\*   Ripe       FALSE trusts the netted count. testdata/Journal-noripe.cfg:
\*              main moves from R1 to R3 and back, the first record nets R1
\*              to zero, and R1 is freed under main before the move back
\*              nets (NoFreedRead).
\*   Revive     FALSE has a Dead block that gains a holder turn Live without
\*              re-holding its children. testdata/Journal-norevive.cfg:
\*              main moves from R1 to R2 and back, the first record nets R1
\*              to zero and starts its release of C, and the move back nets
\*              R1 Live with C still released (CountsExact; without that
\*              rule, CountsCover once the cascade settles).
\*   HoldMark   FALSE lets Land hold a block at zero without the hold mark.
\*              testdata/Journal-collect-noholdmark.cfg: main moves from R1
\*              to N, R1's release cascades down to F, main moves to R2,
\*              whose batch puts F again under C2, and F is freed before
\*              that record nets (NoFreedRead).
\*   Reripen    FALSE leaves a put again's block at its first ripe sequence.
\*              testdata/Journal-writers-noreripen.cfg: a session puts C2
\*              and is abandoned, the next puts C2 again, the first one's
\*              born down nets C2 to zero behind that put, the put again
\*              nets alone, and C2 is freed under R2 before R2's record
\*              nets (NoFreedRead).
\*   FillEager  FALSE applies a fill's ups when its record nets, with Delete
\*              refusing a block an unnetted fill names, as the fill hold
\*              index does. testdata/Journal-fill-nofilleager.cfg: a fill
\*              stores C2 over F, main moves from R1 to R2, R1's release
\*              cascades through F, and G is freed under C2 while the fill's
\*              up for F waits: the refusal covers F but not its subtree
\*              (NoFreedStoredRead).
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
\*   FillUps    FALSE journals no up pair for a fill's children, as a fill
\*              mark did. testdata/Journal-fill-noups.cfg: a read fills C2,
\*              which names F, and gives F no holder; C2 is unheld and is
\*              freed after Grace, its release takes C's holder from F, and F
\*              is freed while main reaches it through R1 and C.
\*   FillBorn   FALSE has the adoption count and settle a filled block as
\*              one stored before it began. testdata/
\*              Journal-fill-adopt-noborn.cfg: main holds R2 naming C2, which
\*              the store lacks; while an adoption runs a read fills C2, the
\*              adoption counts R2's holder of C2 and settle drops C2's
\*              implied holder, the fill record's down drops it again, and C2
\*              is freed under main.
\*   StageCAS   FALSE releases a stage root only when the table holds it.
\*              testdata/Journal-stage-nocas.cfg: the owner stalls past
\*              Lease and Grace, T is freed, and its next put lands S over
\*              the freed T.
\*
\* Left out: a fill of a recorded hole, which LandFill stores as a refill,
\* journaling nothing; the format 7 upgrade (an unadopted Volume here holds
\* every child list it names), the audit, the refusal of blocks stored without a
\* child list, slices larger than one record (they are runs of one-record
\* slices with no writer between), an abandoned run's cleanup (an unadopted
\* Volume's counts are cleared by the next run before they are read), a
\* stage released without publishing, and crashes other than an adoption's:
\* every action is one transaction and the free queue is durable, so a crash
\* between two actions loses nothing the model does not already interleave.

EXTENDS Integers, FiniteSets, Sequences, TLC

CONSTANTS Writers, MaxTxns, MaxNow, Grace, Window, Lease, MaxHoles,
          MainRoots, StageRoots, Reader,
          Ripe, Revive, HoldMark, Reripen, Held, HoleStop, Reput, CAS,
          PutPair,
          Runs, MaxOpens, RunCheck, Claim, BornMark, RootMark,
          Stage, Rehold, StageCAS,
          Sessions, SessionGrace, Skips, BornDown, Staged, ReleaseUnlisted,
          FillBlocks, FillUps, Holds, FillBorn, FillEager

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
  zero,      \* stored blocks whose netted holders are zero
  rel,       \* stored blocks whose children do not hold them
  ripe,      \* each zero block's ripe sequence: the journal's end when it
             \* reached zero, or when netting last met a put of it again
  jr,        \* the journal past the collector's cursor, oldest first
  appended,  \* records ever appended: the journal's end as a sequence
  hseq,      \* the hold mark: the end of the last record that held a stored
             \* block that was released or at zero, or while a cascade was due
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
  freed,     \* blocks the collector freed that no put stored again
  fj,        \* the fill records not netted yet
  freedLive, \* the collector freed a block a held root reached through
             \* stored blocks
  pend,      \* the children a cascade in progress has not reached yet
  pdir       \* that cascade's direction: -1 releases, 1 re-holds, 0 none
gcVars == <<cnt, zero, rel, ripe, under, pend, pdir>>
jrVars == <<jr, appended, hseq>>
fillVars == <<fj, freedLive>>
landVars == <<born, moved, sheld, freed, fillVars>>
adoptVars == <<adoption, owner, pc, pos, opens>>
stageVars == <<stage, renewed>>
writerVars == <<tgt, began, wrote, skipped>>
vars == <<root, expires, stored, lost, cnt, zero, rel, ripe, jr, appended,
          hseq, under, now, obs, seen,
          seenAt, txns, holes, adoption, owner, pc, pos, born, moved, opens,
          stage, renewed, tgt, began, wrote, skipped, sheld, freed, fj,
          freedLive, pend, pdir>>

Init ==
  /\ root = [n \in Names |->
               IF n = "main" THEN "R1"
               ELSE IF n = "stg" /\ Unadopted THEN "R3" ELSE None]
  /\ expires = [n \in Leases |-> 0]
  /\ stored = Reach("R1") \cup (IF Unadopted THEN {"R3"} ELSE {})
  /\ lost = {}
  /\ cnt = [b \in Blocks |-> IF b = "E" /\ Unadopted THEN 1 ELSE 0]
  /\ zero = IF Unadopted THEN {"G"} ELSE {}
  /\ rel = {}
  /\ ripe = [b \in Blocks |-> 0]
  /\ jr = <<>>
  /\ appended = 0
  /\ hseq = 0
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
  /\ fj = {}
  /\ freedLive = FALSE
  /\ pend = {}
  /\ pdir = 0

\* The holders the netted counts give b.
Holders(b) == IF b \in zero THEN 0 ELSE cnt[b] + 1

\* A cascade is due for a stored block whose children's holders disagree with
\* its own: a Dying block, at zero with children that hold it, and a Holding
\* one, with holders and children that do not.
Due == {b \in stored : (b \in zero) # (b \in rel)}

\* Settled: no cascade is due or running. Then every block below a block
\* with holders has holders too, since a block's children count it.
Settled == Due = {} /\ pend = {}


\* The up and down pairs of the records at idx that name b. A pair is tagged
\* with its record, so one holder in two records counts twice.
Ups(idx, b) == Cardinality(UNION {{<<i, x>> : x \in {y \in jr[i].up : y[2] = b}} : i \in idx})
Downs(idx, b) == Cardinality(UNION {{<<i, x>> : x \in {y \in jr[i].down : y[2] = b}} : i \in idx})
Pending == 1..Len(jr)

\* Step adds the deltas d to the netted holders of the blocks in st and writes
\* only the blocks whose holders changed. A stored block at zero holders is
\* zero, and one below zero is an underflow. A block that reaches zero takes
\* the journal's end as its ripe sequence, and so does one a put again in the
\* interval names that stays at zero (again, with Reripen). Without Revive, a
\* Dead block that gains a holder counts its children as holding it again
\* without the cascade that gives them their holder back.
Step(st, d, again) ==
  LET nh(b) == Holders(b) + d[b]
      z == {b \in st \cap stored : IF d[b] # 0 THEN nh(b) <= 0 ELSE b \in zero}
  IN [cnt |-> [b \in Blocks |->
                IF b \in st /\ d[b] # 0 THEN (IF nh(b) > 1 THEN nh(b) - 1 ELSE 0)
                ELSE cnt[b]],
      zero |-> z,
      ripe |-> [b \in Blocks |->
                 IF b \notin z THEN 0
                 ELSE IF b \notin zero \/ (Reripen /\ b \in again) THEN appended
                 ELSE ripe[b]],
      rel |-> IF Revive THEN rel ELSE rel \ (zero \ z),
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
      append == adoption # "unadopted" /\ up \cup down # {}
      \* The blocks the batch found stored and holds: a root or a stage root
      \* moving onto one, or a put of one again. Land reads each one's count.
      onStored == {x[2] : x \in (IF Held THEN held ELSE {}) \cup again}
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
     /\ jr' = IF append THEN Append(jr, [at |-> now, up |-> up, down |-> down])
              ELSE jr
     /\ appended' = IF append THEN appended + 1 ELSE appended
     /\ hseq' = IF append /\ HoldMark /\ onStored # {} /\
                   (onStored \cap (zero \cup rel) # {} \/ ~Settled)
                THEN appended + 1 ELSE hseq
     /\ born' = IF adopting /\ BornMark THEN born \cup new ELSE born
     /\ moved' = IF adopting /\ RootMark
                 THEN [n \in Names |->
                         IF moved[n] = Unmoved /\ \E m \in moves : m.n = n
                         THEN root[n] ELSE moved[n]]
                 ELSE moved
     /\ freed' = freed \ batch
     /\ txns' = txns + 1
     /\ UNCHANGED fillVars

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
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, seen, seenAt,
                 txns, holes, landVars, adoptVars, stageVars, writerVars>>

\* Commit moves main from the head the writer observed to r. The writer puts
\* the pages of r its observed head does not reach; the pages they share it
\* names without putting them again.
Commit(w, r) ==
  /\ obs[w] # None /\ r # obs[w]
  /\ Land(Reach(r) \ Reach(obs[w]), {[n |-> "main", from |-> obs[w], to |-> r]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes, adoptVars,
                 stageVars, writerVars>>

\* HoldStored moves main from the observed head onto r, a block the store
\* holds, without putting its tree: r may be a filled block whose children
\* the store lacks, which a read fills later.
HoldStored(w, r) ==
  /\ Holds /\ obs[w] # None /\ r # obs[w] /\ r \in stored
  /\ Land({}, {[n |-> "main", from |-> obs[w], to |-> r]})
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
\* several SetRoots, as BufferedStore's capacity drain, the object copy and
\* the Space World replay do: batches of puts with no root, in any order, a
\* parent before its child or after it, then the root move with what is
\* left. Need is what the session must store: the pages its observed head
\* does not reach, which main holds while it names that head, as Commit's
\* compare and swap checks.
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
\* reader pin held for the write. HoldStageCompleteRoots needs the head's
\* completion proof, which the collector drops when it starts releasing the
\* head's children, since it may free them before the head; a head whose
\* children do not hold it has none. A proof written again after a re-hold
\* is a writer's claim about its own tree, which the spec leaves out.
SessionBegin(w, r) ==
  /\ Sessions /\ tgt[w] = None /\ sheld[w] = {} /\ obs[w] # None /\ r # obs[w]
  /\ Staged => obs[w] \notin rel
  /\ tgt' = [tgt EXCEPT ![w] = r]
  /\ began' = [began EXCEPT ![w] = now]
  /\ IF Staged
     THEN /\ LandAs(w, {}, {}, {obs[w]}, {})
          /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes,
                         adoptVars, stageVars, wrote, skipped>>
     ELSE UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
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
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
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
     THEN UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
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
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, txns, holes,
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
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seenAt,
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
\* held again while its block is stored and has holders, through Land, which
\* journals the up pair and checks the hold mark; when the block is gone or at
\* zero the stage fails, since a Dead block's children may already be freed.
\* Without Rehold the renewal renews only what the table still holds and
\* reports success.
StageRenew ==
  /\ Building
  /\ \/ /\ root[StageName] = StageTop
        /\ expires' = [expires EXCEPT ![StageName] = now + Lease]
        /\ renewed' = now
        /\ UNCHANGED <<root, stored, lost, gcVars, jrVars, txns, landVars, stage>>
     \/ /\ root[StageName] = None /\ Rehold /\ StageTop \in stored \ zero
        /\ Land({}, {[n |-> StageName, from |-> None, to |-> StageTop]})
        /\ expires' = [expires EXCEPT ![StageName] = now + Lease]
        /\ renewed' = now
        /\ UNCHANGED <<gcVars, stage>>
     \/ /\ root[StageName] = None /\ Rehold /\ StageTop \notin stored \ zero
        /\ stage' = "failed"
        /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, txns, landVars,
                       renewed>>
     \/ /\ root[StageName] = None /\ ~Rehold
        /\ renewed' = now
        /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, txns, landVars,
                       stage>>
  /\ UNCHANGED <<now, obs, seen, seenAt, holes, adoptVars, writerVars, pend, pdir>>

\* ---------------------------------------------------------------------------
\* The collector.

\* The fill records whose ups the collector has not applied yet.
Unapplied == {r \in fj : ~r.applied}

\* The up pairs of the fill records not applied yet and the down pairs of
\* those not netted yet that name b.
FUps(b) == Cardinality({r \in Unapplied : \E x \in r.up : x[2] = b})
FDowns(b) == Cardinality({r \in fj : \E x \in r.down : x[2] = b})

\* What a held root reaches through stored blocks: a block a stored block
\* names is reached, and so are its children only while it is stored.
RECURSIVE StoredReach(_)
StoredReach(b) ==
  {b} \cup IF b \in stored THEN UNION {StoredReach(k) : k \in Kids(b)} ELSE {}
\* The blocks the writers' sessions have still to put.
ToPutNow == UNION {IF tgt[w] = None THEN {} ELSE Unsent(w) : w \in Writers}
StoredLive ==
  UNION {StoredReach(b) :
           b \in ({root[n] : n \in Names} \ {None}) \cup UNION {sheld[w] : w \in Writers}}

\* The cursor as a sequence: the records before it are netted.
Cursor == appended - Len(jr)

\* The blocks a put again in the first k records names, as a journal
\* record's again list does.
AgainIn(k) == {b \in Blocks : \E i \in 1..k : <<"put", b>> \in jr[i].up}

\* Net consumes the first k records as one interval; every one has aged Grace.
Net(k) ==
  /\ adoption = "adopted"
  /\ \A i \in 1..k : jr[i].at + Grace <= now
  /\ LET s == Step(Blocks, [b \in Blocks |-> Ups(1..k, b) - Downs(1..k, b)],
                  AgainIn(k))
     IN /\ cnt' = s.cnt
        /\ zero' = s.zero
        /\ ripe' = s.ripe
        /\ rel' = s.rel
        /\ under' = (under \/ s.under)
  /\ jr' = SubSeq(jr, k + 1, Len(jr))
  /\ UNCHANGED <<root, expires, stored, lost, appended, hseq, now, obs, seen,
                 seenAt, txns, holes, landVars, adoptVars, stageVars, writerVars,
                 pend, pdir>>

\* What a cascade reaches: the children of its block. A lost child has no
\* child list, so HoleStop keeps its children; the cascade stops there.
Decrement(b) ==
  Kids(b) \cup IF HoleStop THEN {}
               ELSE UNION {Kids(c) : c \in Kids(b) \cap lost}

\* Cascade starts block b's release, for a Dying block, or its re-hold, for a
\* Holding one, and leaves its children for Release to reach one at a time,
\* as netting's resolve does within a slice and continueRelease across
\* slices. The block is Dead or Live from here; its children catch up.
Cascade(b) ==
  /\ adoption = "adopted"
  /\ b \in Due /\ pend = {}
  /\ rel' = IF b \in zero THEN rel \cup {b} ELSE rel \ {b}
  /\ pend' = Decrement(b)
  /\ pdir' = IF Decrement(b) = {} THEN 0 ELSE IF b \in zero THEN -1 ELSE 1
  /\ UNCHANGED <<root, expires, stored, lost, cnt, zero, ripe, under, jrVars,
                 now, obs, seen, seenAt, txns, holes, landVars, adoptVars,
                 stageVars, writerVars>>

\* Release moves child c's holders by the cascade's direction, which may take
\* it to zero or back. Writers interleave between two releases: a root or a
\* second parent may hold c meanwhile. A child not stored yet, which a parent
\* put before it named, holds the up pair that parent journaled; with
\* ReleaseUnlisted the release reaches its count too. A lost child keeps its
\* count, which went with it.
Release(c) ==
  /\ c \in pend
  /\ LET d == [x \in Blocks |-> IF x = c THEN pdir ELSE 0]
         s == Step(IF ReleaseUnlisted THEN Blocks \ lost ELSE stored, d, {})
     IN /\ cnt' = s.cnt
        /\ zero' = s.zero
        /\ ripe' = s.ripe
        /\ rel' = s.rel
        /\ under' = (under \/ s.under)
  /\ pend' = pend \ {c}
  /\ pdir' = IF pend' = {} THEN 0 ELSE pdir
  /\ UNCHANGED <<root, expires, stored, lost, jrVars, now, obs, seen, seenAt,
                 txns, holes, landVars, adoptVars, stageVars, writerVars>>

\* Delete frees Dead block b: it deletes the block with its child list and
\* count record and changes no other count, since the cascade already took
\* its holder from its children. It waits while any cascade is due or
\* running, until the cursor passes the block's ripe sequence (Ripe) and the
\* hold mark (HoldMark), and while a fill's ups are unapplied (FillEager), or
\* without FillEager while a fill record not netted yet names b, as the fill
\* hold index does.
Delete(b) ==
  /\ adoption = "adopted"
  /\ b \in zero \cap rel /\ Settled
  /\ Ripe => Cursor >= ripe[b]
  /\ HoldMark => Cursor >= hseq
  /\ IF FillEager THEN Unapplied = {} ELSE FUps(b) = 0
  /\ stored' = stored \ {b}
  /\ cnt' = [cnt EXCEPT ![b] = 0]
  /\ zero' = zero \ {b}
  /\ rel' = rel \ {b}
  /\ ripe' = [ripe EXCEPT ![b] = 0]
  /\ freed' = freed \cup {b}
  /\ freedLive' = (freedLive \/ b \in StoredLive \ ToPutNow)
  /\ UNCHANGED <<root, expires, lost, jrVars, under, now, obs, seen, seenAt,
                 txns, holes, born, moved, sheld, fj, adoptVars, stageVars,
                 writerVars, pend, pdir>>

\* Fill stores b, a block a peer holds, with its child list, and writes its
\* record: an up pair for each child, with FillUps, and a down pair for its
\* own implied holder. It reads no liveness state, so it lands whatever the
\* adoption state, and its record names b, as LandFill's fill-born index
\* entry does.
FillRecorded(b) == \E r \in fj : r.b = b
Fill(b) ==
  /\ b \in FillBlocks /\ b \notin stored \cup lost /\ Live
  /\ stored' = stored \cup {b}
  /\ freed' = freed \ {b}
  /\ fj' = fj \cup {[b |-> b, at |-> now, applied |-> FALSE,
                     up |-> IF FillUps THEN {<<b, k>> : k \in Kids(b)} ELSE {},
                     down |-> {<<"fill", b>>}]}
  /\ txns' = txns + 1
  /\ UNCHANGED <<root, expires, lost, gcVars, jrVars, now, obs, seen, seenAt,
                 holes, born, moved, sheld, freedLive, adoptVars, stageVars,
                 writerVars>>

\* DropFill is the collector deleting a fill record of an unadopted Volume,
\* whose adoption counts every child list instead.
DropFill(r) ==
  /\ adoption = "unadopted" /\ r \in fj
  /\ fj' = fj \ {r}
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
                 seenAt, txns, holes, born, moved, sheld, freed, freedLive,
                 adoptVars, stageVars, writerVars>>

\* The deltas of a fill record's up and down pairs.
FillUpDelta(r) == [b \in Blocks |-> Cardinality({x \in r.up : x[2] = b})]
FillDownDelta(r) == [b \in Blocks |-> Cardinality({x \in r.down : x[2] = b})]

\* ApplyFill applies a new fill record's ups with FillEager, in the first
\* collector slice that finds it, so a child the fill names is held before
\* any free runs. Only adding holders early is safe, and each is written once.
ApplyFill(r) ==
  /\ FillEager /\ adoption = "adopted" /\ r \in Unapplied
  /\ LET s == Step(Blocks, FillUpDelta(r), {})
     IN /\ cnt' = s.cnt
        /\ zero' = s.zero
        /\ ripe' = s.ripe
        /\ rel' = s.rel
        /\ under' = (under \/ s.under)
  /\ fj' = (fj \ {r}) \cup {[r EXCEPT !.applied = TRUE]}
  /\ UNCHANGED <<root, expires, stored, lost, jrVars, now, obs, seen, seenAt,
                 txns, holes, born, moved, sheld, freed, freedLive, adoptVars,
                 stageVars, writerVars, pend, pdir>>

\* NetFill nets one aged fill record, in one transaction, whenever the
\* collector reaches it: before or after the main records naming its blocks.
\* With FillEager its ups are already applied and it nets only its down.
NetFill(r) ==
  /\ adoption = "adopted"
  /\ r \in fj /\ r.at + Grace <= now /\ (FillEager => r.applied)
  /\ LET up == IF FillEager THEN [b \in Blocks |-> 0] ELSE FillUpDelta(r)
         s == Step(Blocks, [b \in Blocks |-> up[b] - FillDownDelta(r)[b]], {})
     IN /\ cnt' = s.cnt
        /\ zero' = s.zero
        /\ ripe' = s.ripe
        /\ rel' = s.rel
        /\ under' = (under \/ s.under)
  /\ fj' = fj \ {r}
  /\ UNCHANGED <<root, expires, stored, lost, jrVars, now, obs, seen, seenAt,
                 txns, holes, born, moved, sheld, freed, freedLive, adoptVars,
                 stageVars, writerVars, pend, pdir>>

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
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
                 seenAt, txns, holes, landVars, adoption, owner, pos, opens,
                 stageVars, writerVars>>

\* Go moves r to the next step, leaving the records alone.
Go(r, step) ==
  /\ pc' = [pc EXCEPT ![r] = step]
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
                 seenAt, txns, holes, landVars, adoption, owner, opens,
                 stageVars, writerVars>>

AdoptUnchanged ==
  UNCHANGED <<root, expires, stored, lost, rel, under, jrVars, now, obs, seen,
              seenAt, txns, holes, opens, sheld, freed, fillVars, stageVars,
              writerVars, pend, pdir>>

\* AdoptStart reports an adopted Volume, and otherwise returns it to
\* unadopted, superseding any run, and with Claim names r in it.
AdoptStart(r) ==
  /\ pc[r] = "idle"
  /\ IF adoption = "adopted"
     THEN Go(r, "done") /\ UNCHANGED pos
     ELSE /\ adoption' = "unadopted"
          /\ owner' = IF Claim THEN r ELSE None
          /\ pc' = [pc EXCEPT ![r] = "clear"]
          /\ UNCHANGED <<cnt, zero, ripe, landVars, pos>>
          /\ AdoptUnchanged

\* AdoptClear deletes every count, queue entry, mark and fill record while r
\* holds its claim; without Claim it deletes them whatever the state. Each
\* fill record goes with its index entries in one transaction.
AdoptClear(r) ==
  /\ pc[r] = "clear"
  /\ IF Claim /\ ~Claimed(r)
     THEN Superseded(r)
     ELSE /\ cnt' = [b \in Blocks |-> 0]
          /\ zero' = {}
          /\ rel' = {}
          /\ ripe' = [b \in Blocks |-> 0]
          /\ born' = {}
          /\ moved' = [n \in Names |-> Unmoved]
          /\ fj' = {}
          /\ pc' = [pc EXCEPT ![r] = "mark"]
          /\ UNCHANGED <<adoption, owner, pos, pend, pdir>>
          /\ UNCHANGED <<root, expires, stored, lost, under, jrVars, now, obs, seen,
                         seenAt, txns, holes, opens, sheld, freed, freedLive,
                         stageVars, writerVars>>

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
          /\ UNCHANGED <<gcVars, appended, hseq, landVars, root, expires, stored,
                         lost, now,
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
                  /\ UNCHANGED <<zero, ripe, landVars, adoption, owner, pc>>
                  /\ AdoptUnchanged

\* Skipped is a block whose holders the journal supplies, so the adoption
\* neither counts its children nor settles it: one Land gave birth to while
\* the Volume adopts, and with FillBorn one a fill record not netted yet
\* names, which holds its children and takes its implied holder back.
Skipped(b) == b \in born \/ (FillBorn /\ FillRecorded(b))

\* AdoptList counts a holder of each child of the next child list, in key
\* order, that no born mark names.
AdoptList(r) ==
  /\ pc[r] = "lists"
  /\ IF ~Owned(r) THEN Superseded(r)
     ELSE LET i == NextKey(BlockKeys, pos[r], stored)
              b == BlockKeys[i]
          IN IF i = 0 THEN Go(r, "settle") /\ pos' = [pos EXCEPT ![r] = 0]
             ELSE /\ cnt' = IF Skipped(b) THEN cnt
                            ELSE [c \in Blocks |->
                                    cnt[c] + IF c \in Kids(b) THEN 1 ELSE 0]
                  /\ pos' = [pos EXCEPT ![r] = i]
                  /\ UNCHANGED <<zero, ripe, landVars, adoption, owner, pc>>
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
             ELSE /\ IF Skipped(b) THEN UNCHANGED <<cnt, zero, ripe>>
                     ELSE IF cnt[b] = 0
                     THEN /\ zero' = zero \cup {b}
                          /\ ripe' = [ripe EXCEPT ![b] = appended]
                          /\ UNCHANGED cnt
                     ELSE cnt' = [cnt EXCEPT ![b] = @ - 1] /\ UNCHANGED <<zero, ripe>>
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
          /\ UNCHANGED <<cnt, zero, ripe, owner, pos>>
          /\ AdoptUnchanged

\* AdoptCrash stops r at any step.
AdoptCrash(r) ==
  /\ pc[r] \in {"clear", "mark", "roots", "lists", "settle", "end"}
  /\ Go(r, "dead") /\ UNCHANGED pos

\* Open returns an adopting Volume to unadopted, superseding its run.
Open ==
  /\ opens < MaxOpens /\ adoption = "adopting"
  /\ adoption' = "unadopted" /\ owner' = None /\ opens' = opens + 1
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, now, obs, seen,
                 seenAt, txns, holes, landVars, pc, pos, stageVars, writerVars>>

\* ---------------------------------------------------------------------------
\* The environment.

\* Punch loses block b with its child list and count record. Blocks it named
\* stay stored, held by a page nobody can read. A released block is left
\* out: losing a Dead one is an early Delete, and losing a Holding one, in the
\* window between the Net that revived it and its re-hold, loses the
\* children only that re-hold would hold again.
Punch(b) ==
  /\ holes < MaxHoles /\ b \in stored \ rel
  /\ stored' = stored \ {b} /\ lost' = lost \cup {b}
  /\ cnt' = [cnt EXCEPT ![b] = 0] /\ zero' = zero \ {b}
  /\ ripe' = [ripe EXCEPT ![b] = 0]
  /\ holes' = holes + 1
  /\ UNCHANGED <<root, expires, jrVars, under, now, obs, seen, seenAt, txns,
                 landVars, adoptVars, stageVars, writerVars, rel, pend, pdir>>

Tick ==
  /\ now < MaxNow /\ now' = now + 1
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jrVars, obs, seen, seenAt,
                 txns, holes, landVars, adoptVars, stageVars, writerVars>>

NetAny == \E k \in 1..Len(jr) : Net(k)
NetFillAny == \E r \in fj : NetFill(r) \/ DropFill(r) \/ ApplyFill(r)
DeleteAny == \E b \in Blocks : Delete(b) \/ Cascade(b) \/ Release(b)

Next ==
  \/ \E w \in Writers :
       \/ Observe(w)
       \/ \E r \in MainRoots : Commit(w, r) \/ HoldStored(w, r)
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
  \/ \E b \in FillBlocks : Fill(b)
  \/ NetFillAny
  \/ Tick
  \/ NetAny
  \/ DeleteAny

Spec == Init /\ [][Next]_vars /\ WF_vars(Tick) /\ WF_vars(NetAny)
          /\ WF_vars(NetFillAny) /\ WF_vars(DeleteAny)

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
ToPut == ToPutNow
NoFreedRead == (LiveSet \cap freed) \subseteq ToPut

\* No block a held root reached through stored blocks when it was freed was
\* freed, but one a session has still to put. A stored block may name a
\* child the store lacks, which a read fills, so this walk stops at a block
\* the store does not hold, where NoFreedRead walks every child.
NoFreedStoredRead == ~freedLive

\* At most one fill record names a block, so keying the records by block
\* loses none.
OneFillRecord == \A r1, r2 \in fj : r1.b = r2.b => r1 = r2

\* A reader that saw a head and still has time to pin it finds all of it.
ReaderWindow ==
  seen # None /\ now <= seenAt + Window => ReachOf(seen) \subseteq stored \cup lost

\* No transaction took a holder from a block that had none.
NoUnderflow == ~under

\* Only stored blocks are zero or released.
ZeroStored == zero \cup rel \subseteq stored

\* The roots and stage roots that hold b.
RootHolders(b) ==
  Cardinality({n \in Names : root[n] = b}) +
  Cardinality({w \in Writers : b \in sheld[w]})

\* The holders of a block: the roots, and the stored child lists of parents
\* that have holders. A block with no holders holds nothing.
TrueHolders(b) ==
  RootHolders(b) + Cardinality({p \in stored \ zero : b \in Kids(p)})

\* The netted holders plus the journal past the cursor.
JournalHolders(b) ==
  Holders(b) + Ups(Pending, b) + FUps(b) - Downs(Pending, b) - FDowns(b)

\* Once adopted and settled, the journal never counts fewer holders than a
\* block has: fewer frees a live block. More leaks, which only a hole may
\* cause. While a cascade is due or running the counts lag by its block, and
\* Delete waits.
CountsCover ==
  Adopted /\ Settled => \A b \in stored : JournalHolders(b) >= TrueHolders(b)

\* Once adopted with no hole, the journal counts each block's holders
\* exactly: the roots and the parents whose children hold them, Live and
\* Dying ones, with the holder a cascade in progress has not moved yet.
Owed(b) == IF b \in pend THEN -pdir ELSE 0
CountsExact ==
  Adopted /\ lost = {} =>
    \A b \in stored :
      JournalHolders(b) =
        RootHolders(b) + Cardinality({p \in stored \ rel : b \in Kids(p)}) + Owed(b)

\* A lost block's children stay stored: the cascade stopped at the hole.
HoleKeeps == \A h \in lost : Kids(h) \subseteq stored \cup lost

Quiet == Adopted /\ jr = <<>> /\ fj = {} /\ zero = {} /\ rel = {} /\ pend = {}

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
