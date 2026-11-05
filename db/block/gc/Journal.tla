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
\* The journal. Each SetRoots transaction appends at most one record, and the
\* record carries only what changes a holder count:
\*   up    pairs <<holder, block>> that add a holder:
\*         - <<parent, child>> for each child a block the batch stored for the
\*           first time names and that the store already held. A child the
\*           batch stored too has the parent as its one implied holder, so
\*           nothing is recorded for it. In Go this is the birth record's
\*           found flag per child, from the put's existence check.
\*         - <<name, block>> for each root that now names a block the store
\*           already held, before the batch: a reader pinning an old root, a
\*           writer adopting a stage, SetRoots onto a stored reference. A root
\*           that names a block the batch stored is its implied holder.
\*   down  pairs <<name, block>> for each root that moved off a block, read
\*         from the root table inside the transaction.
\* A block's own birth adds no holder and is otherwise not recorded.
\*
\* The collector. Net consumes a prefix of the journal as one interval whose
\* records have all aged Grace, adds the interval's deltas to the netted counts
\* in one transaction, writes only the blocks whose holders changed, and puts a
\* block whose holders reach zero on the free queue. Delete frees one queued
\* block and decrements its children, which the cascade then finds at zero in
\* turn. A child in a dead page and a born page of one interval nets to no
\* change and keeps its implied holder unwritten.
\*
\* Mapping to Go, one action to one transaction:
\*   Land            Volume.SetRoots(entries, roots) in db/volume/common/kvtx:
\*                   checks each root's Previous (ErrRootMoved), refuses a
\*                   born block whose child the store lacks, stores the
\*                   entries with their child lists, moves the roots and
\*                   appends the journal record, all in one write transaction.
\*     Commit        the World engine's head swap: main moves by compare and
\*                   swap from the head the writer observed.
\*     StageWrite    a writer outside the World writer holding stage:<ulid>,
\*                   a tree built over the head it observed.
\*     AdoptStage    the World writer moving main onto a stage and releasing
\*                   the stage in one SetRoots.
\*     ReaderPin     a reader holding the root it saw as a leased root.
\*     Drop          a lease holder releasing its root.
\*     Sweep         the collector releasing a root whose lease lapsed; it
\*                   reads the root inside its own transaction.
\*   Net(k)          Collector.Collect: read the journal from the cursor to the
\*                   grace horizon, net per block, write the count records and
\*                   free queue entries that changed, advance the cursor.
\*   Delete(b)       one free queue entry in a collector slice: the tail check,
\*                   then delete the block with its child list and count
\*                   record and decrement its children. A slice yields to a
\*                   waiting writer between these transactions, so writers
\*                   interleave anywhere between two Deletes.
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
\* here. Flipping one frees a block a held root reaches (NoFreedRead), in the
\* config named and in the run TLC reports, which shows the rule is load
\* bearing; CAS is the exception.
\*   TailCheck  FALSE trusts the netted count. Journal.cfg: main moves R1 to
\*              R2 and back to R1 inside Grace, a pass nets only the first
\*              record, and R1 is freed while main names it again.
\*   Held       FALSE drops the up pair of a root moving onto a stored block.
\*              Journal-lease.cfg: a stage R2 is adopted onto main, the
\*              stage's release nets R2 to zero, and it is freed under main.
\*   Refuse     FALSE lets a born block name a child the store lacks.
\*              Journal.cfg: main moves from R1 to N, the collector frees R1's
\*              tree, and a writer that still has R1 as its observed head
\*              stages R2 over it, naming the freed F.
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
\*   CAS        FALSE still passes every config: liveness rests on the
\*              release read inside the transaction and on Refuse, and
\*              compare and swap is the World's guard against two writers
\*              both moving one root, not the collector's.
\*
\* Left out: the format 7 upgrade and Adopt (a Volume starts here with every
\* block listed, and an unadopted Volume frees nothing), the audit, lease
\* renewal (a holder that outlives its lease reads at its own risk), slices
\* smaller than one block, and crashes: every action is one transaction and
\* the free queue is durable, so a crash between two actions loses nothing
\* the model does not already interleave.

EXTENDS Integers, FiniteSets, Sequences, TLC

CONSTANTS Writers, MaxTxns, MaxNow, Grace, Window, Lease, MaxHoles,
          MainRoots, StageRoots, Reader,
          TailCheck, Held, Refuse, HoleStop, Reput, CAS

None == "none"

\* main is the World root, moved by the writers by compare and swap. rdr is a
\* reader's pin of an old root and stg a stage, stage:<ulid>, held by a writer
\* outside the World writer; both are leased roots.
Names == {"main", "rdr", "stg"}
Leases == {"rdr", "stg"}

\* Two versions of a tree R -> C -> {E, F} -> G: C2 keeps F, and with it G,
\* and drops E. R3 is a second page over the same C, and N an empty tree.
\* Every tree gives each page one parent. F has two holders while both
\* versions are stored, and G is its only child.
Blocks == {"R1", "R2", "R3", "C", "C2", "E", "F", "G", "N"}
Kids(b) ==
  CASE b = "R1" -> {"C"}
    [] b = "R2" -> {"C2"}
    [] b = "R3" -> {"C"}
    [] b = "C" -> {"E", "F"}
    [] b = "C2" -> {"F"}
    [] b = "F" -> {"G"}
    [] OTHER -> {}

RECURSIVE Reach(_)
Reach(b) == {b} \cup UNION {Reach(k) : k \in Kids(b)}
ReachOf(x) == IF x = None THEN {} ELSE Reach(x)

Perms == Permutations(Writers)

VARIABLES
  root,     \* each named root's block, or None
  expires,  \* each leased root's expiry on the clock
  stored,   \* blocks with a child list in the store
  lost,     \* blocks the store lost that no put stored again
  cnt,      \* each block's count record: holders minus one, 0 for none
  zero,     \* the free queue: stored blocks whose netted holders are zero
  jr,       \* the journal past the collector's cursor, oldest first
  under,    \* some transaction took a holder from a block that had none
  now,      \* the clock
  obs,      \* each writer's observed head of main
  seen,     \* the head of main the reader saw and has not pinned, or None
  seenAt,   \* when the reader saw it
  txns,     \* write transactions spent, which keeps every run finite
  holes     \* blocks punched
gcVars == <<cnt, zero, under>>
vars == <<root, expires, stored, lost, cnt, zero, jr, under, now, obs, seen,
          seenAt, txns, holes>>

Init ==
  /\ root = [n \in Names |-> IF n = "main" THEN "R1" ELSE None]
  /\ expires = [n \in Leases |-> 0]
  /\ stored = Reach("R1")
  /\ lost = {}
  /\ cnt = [b \in Blocks |-> 0]
  /\ zero = {}
  /\ jr = <<>>
  /\ under = FALSE
  /\ now = 0
  /\ obs = [w \in Writers |-> None]
  /\ seen = None
  /\ seenAt = 0
  /\ txns = 0
  /\ holes = 0

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
      zero |-> {b \in st : IF d[b] # 0 THEN nh(b) <= 0 ELSE b \in zero},
      under |-> \E b \in st : nh(b) < 0]

\* ---------------------------------------------------------------------------
\* The writer side: SetRoots, one write transaction.

\* Every record must be able to age Grace before the clock stops, so the
\* collector can consume it.
Live == now + Grace <= MaxNow /\ txns < MaxTxns

\* Land is SetRoots(batch, moves). moves is a set of [n, from, to] root moves,
\* from the root's Previous; batch is the blocks the caller puts. born, up and
\* down are computed against the store and the root table as they stand
\* inside the transaction.
Land(batch, moves) ==
  LET born == batch \ stored
      found == UNION {{<<p, k>> : k \in Kids(p) \cap stored} : p \in born}
      held == {<<m.n, m.to>> : m \in {x \in moves : x.to \in stored}}
      up == found \cup (IF Held THEN held ELSE {})
      down == {<<m.n, root[m.n]>> : m \in {x \in moves : root[x.n] # None}}
  IN /\ Live
     /\ \A m \in moves : m.to # root[m.n]
     /\ CAS => \A m \in moves : root[m.n] = m.from
     \* A root names a block the store holds or the batch stores.
     /\ \A m \in moves : m.to = None \/ m.to \in stored \cup batch
     \* A born block names only children the store holds or the batch stores.
     /\ Refuse => \A p \in born : Kids(p) \subseteq stored \cup batch
     /\ ~Reput => batch \cap lost = {}
     /\ stored' = stored \cup batch
     /\ lost' = lost \ batch
     /\ root' = [n \in Names |->
                   IF \E m \in moves : m.n = n
                   THEN (CHOOSE m \in moves : m.n = n).to ELSE root[n]]
     /\ jr' = IF up \cup down = {} THEN jr
              ELSE Append(jr, [at |-> now, up |-> up, down |-> down])
     /\ txns' = txns + 1

\* Observe reads the head of main, as a writer does before it builds a tree.
Observe(w) ==
  /\ obs' = [obs EXCEPT ![w] = root["main"]]
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, seen, seenAt,
                 txns, holes>>

\* Commit moves main from the head the writer observed to r. The writer puts
\* the pages of r its observed head does not reach; the pages they share it
\* names without putting them again.
Commit(w, r) ==
  /\ obs[w] # None /\ r # obs[w]
  /\ Land(Reach(r) \ Reach(obs[w]), {[n |-> "main", from |-> obs[w], to |-> r]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes>>

\* StageWrite holds a stage tree r built over the head of main the writer
\* observed, under a free stage root, and starts its lease. The pages r shares
\* with that head are named, not put, and nothing but main holds them.
StageWrite(w, r) ==
  /\ obs[w] # None
  /\ Land(Reach(r) \ Reach(obs[w]), {[n |-> "stg", from |-> None, to |-> r]})
  /\ expires' = [expires EXCEPT !["stg"] = now + Lease]
  /\ UNCHANGED <<gcVars, now, obs, seen, seenAt, holes>>

\* AdoptStage moves main from the observed head to stage s and releases the
\* stage in the same transaction, so s never has zero holders between.
AdoptStage(w, s) ==
  /\ obs[w] # None
  /\ Land({}, {[n |-> "main", from |-> obs[w], to |-> s],
               [n |-> "stg", from |-> s, to |-> None]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes>>

\* ReaderSee: a reader learns the head of main and reads its tree without a
\* pin until it pins, which it must do within Window.
ReaderSee ==
  /\ Reader /\ seen = None /\ root["rdr"] = None
  /\ seen' = root["main"]
  /\ seenAt' = now
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, txns, holes>>

\* ReaderPin holds the block the reader saw as a leased root. SetRoots refuses
\* it when the block is gone, which Grace keeps from happening inside Window.
ReaderPin ==
  /\ seen # None /\ now <= seenAt + Window
  /\ Land({}, {[n |-> "rdr", from |-> None, to |-> seen]})
  /\ expires' = [expires EXCEPT !["rdr"] = now + Lease]
  /\ seen' = None
  /\ UNCHANGED <<gcVars, now, obs, seenAt, holes>>

\* ReaderLate: a reader that missed its window gives up its unpinned read.
ReaderLate ==
  /\ seen # None /\ now > seenAt + Window
  /\ seen' = None
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, now, obs, seenAt,
                 txns, holes>>

\* Drop releases a leased root, as its holder does when it is done.
Drop(n) ==
  /\ root[n] # None
  /\ Land({}, {[n |-> n, from |-> root[n], to |-> None]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes>>

\* Sweep is the collector releasing a root whose lease lapsed.
Sweep(n) ==
  /\ root[n] # None /\ now > expires[n]
  /\ Land({}, {[n |-> n, from |-> root[n], to |-> None]})
  /\ UNCHANGED <<expires, gcVars, now, obs, seen, seenAt, holes>>

\* ---------------------------------------------------------------------------
\* The collector.

\* Net consumes the first k records as one interval; every one has aged Grace.
Net(k) ==
  /\ \A i \in 1..k : jr[i].at + Grace <= now
  /\ LET s == Step(stored, [b \in Blocks |-> Ups(1..k, b) - Downs(1..k, b)])
     IN /\ cnt' = s.cnt
        /\ zero' = s.zero
        /\ under' = (under \/ s.under)
  /\ jr' = SubSeq(jr, k + 1, Len(jr))
  /\ UNCHANGED <<root, expires, stored, lost, now, obs, seen, seenAt, txns,
                 holes>>

\* What Delete decrements: the children of the freed block. A lost child has
\* no child list, so HoleStop keeps its children; the cascade stops there.
Decrement(b) ==
  Kids(b) \cup IF HoleStop THEN {}
               ELSE UNION {Kids(c) : c \in Kids(b) \cap lost}

\* Delete frees queued block b and decrements its children, which may queue
\* them. With TailCheck it refuses while an up pair in the tail names b.
Delete(b) ==
  /\ b \in zero
  /\ TailCheck => Ups(Pending, b) = 0
  /\ LET d == [c \in Blocks |-> IF c \in Decrement(b) THEN -1 ELSE 0]
         s == Step(stored \ {b}, d)
     IN /\ stored' = stored \ {b}
        /\ cnt' = [s.cnt EXCEPT ![b] = 0]
        /\ zero' = s.zero
        /\ under' = (under \/ s.under)
  /\ UNCHANGED <<root, expires, lost, jr, now, obs, seen, seenAt, txns, holes>>

\* ---------------------------------------------------------------------------
\* The environment.

\* Punch loses block b with its child list and count record. Blocks it named
\* stay stored, held by a page nobody can read.
Punch(b) ==
  /\ holes < MaxHoles /\ b \in stored
  /\ stored' = stored \ {b} /\ lost' = lost \cup {b}
  /\ cnt' = [cnt EXCEPT ![b] = 0] /\ zero' = zero \ {b}
  /\ holes' = holes + 1
  /\ UNCHANGED <<root, expires, jr, under, now, obs, seen, seenAt, txns>>

Tick ==
  /\ now < MaxNow /\ now' = now + 1
  /\ UNCHANGED <<root, expires, stored, lost, gcVars, jr, obs, seen, seenAt,
                 txns, holes>>

NetAny == \E k \in 1..Len(jr) : Net(k)
DeleteAny == \E b \in Blocks : Delete(b)

Next ==
  \/ \E w \in Writers :
       \/ Observe(w)
       \/ \E r \in MainRoots : Commit(w, r)
       \/ \E r \in StageRoots : StageWrite(w, r) \/ AdoptStage(w, r)
  \/ ReaderSee \/ ReaderPin \/ ReaderLate
  \/ \E n \in Leases : Drop(n) \/ Sweep(n)
  \/ \E b \in Blocks : Punch(b)
  \/ Tick
  \/ NetAny
  \/ DeleteAny

Spec == Init /\ [][Next]_vars /\ WF_vars(Tick) /\ WF_vars(NetAny)
          /\ WF_vars(DeleteAny)

\* ---------------------------------------------------------------------------
\* Properties.

HeldBlocks == {root[n] : n \in Names} \ {None}
LiveSet == UNION {Reach(b) : b \in HeldBlocks}
BelowLost == UNION {Reach(h) \ {h} : h \in lost}

\* No block a held root reaches is freed: it is stored, or the store lost it.
\* This covers main moved by compare and swap while a pass ran, a reader's
\* pin and a stage.
NoFreedRead == LiveSet \subseteq stored \cup lost

\* A reader that saw a head and still has time to pin it finds all of it.
ReaderWindow ==
  seen # None /\ now <= seenAt + Window => ReachOf(seen) \subseteq stored \cup lost

\* No transaction took a holder from a block that had none.
NoUnderflow == ~under

\* The free queue holds only stored blocks.
ZeroStored == zero \subseteq stored

\* The holders of a block, from the roots and the stored child lists.
TrueHolders(b) ==
  Cardinality({n \in Names : root[n] = b}) + Cardinality({p \in stored : b \in Kids(p)})

\* The netted holders plus the journal past the cursor.
JournalHolders(b) == Holders(b) + Ups(Pending, b) - Downs(Pending, b)

\* The journal never counts fewer holders than a block has: fewer frees a
\* live block. More leaks, which only a hole may cause.
CountsCover == \A b \in stored : JournalHolders(b) >= TrueHolders(b)

\* With no hole, the journal counts each block's holders exactly.
CountsExact == lost = {} => \A b \in stored : JournalHolders(b) = TrueHolders(b)

\* A lost block's children stay stored: the cascade stopped at the hole.
HoleKeeps == \A h \in lost : Kids(h) \subseteq stored \cup lost

Quiet == jr = <<>> /\ zero = {}

\* Once the journal is consumed and the free queue empty, the store holds what
\* the roots reach and what lies below a hole: the audit's Leaked.
NoLeak == Quiet => stored \subseteq LiveSet \cup BelowLost

\* Every block no root reaches is freed in the end, but those below a hole.
Collected == <>[](Quiet /\ stored \subseteq LiveSet \cup BelowLost)
====
