---- MODULE SharedCounts ----
\* Shared counts under version reclamation: which writes add a holder to a
\* stored page, and what applying a deadlist record does to a page another
\* holder still names.
\*
\* Pages are content addressed, so a page's children never change: Kids is
\* fixed and a writer that builds a page identical to a stored one finds it.
\* Each lineage holds one root. Commit moves a lineage to a new root: it writes
\* the pages of the new tree that are not stored, finds the stored ones, and
\* queues a record listing the pages of the old tree the new tree does not
\* reach. extra[p] is the page's count record, its holders minus one; zero means
\* no record. Apply takes one listed page of a record, parent before child, as
\* one store transaction.
\*
\* Holder-creating callers, found from source on Spacewave master aa2254798:
\* - Volume puts that find their block stored: modeled, Land's present pages.
\* - Object copy with skipSubtreeExists (db/bucket/lookup/object-copy.go):
\*   modeled, the writer prunes below a found page and puts nothing there.
\* - SetRootRef from a supplied reference (db/world/block/object-state.go and
\*   sdk/world): modeled, a Commit whose new root is stored.
\* - The deadlist writer (the World engine's nameWorld): modeled. Each commit
\*   joins its lineage's interval, and Name composes the interval into the
\*   records it queues, as Compose sets.
\* - unixfs Copy and the World change log's link: left out. Both give a page
\*   two parents inside one tree, which ReplacementStep.Net refuses to list;
\*   across two trees they are the SetRootRef case.
\* - Owner edges outside a put's flush, such as a reader pin of a World root:
\*   modeled as Pin, which adds a count, and Unpin, which queues a release.
\* The watermark is left out: a record applies only once due.
\*
\* In the code, Apply is RefGraph.ResolveDeaths with ChangeHolds
\* (refgraph-deaths.go), run one record at a time by the Volume's collector.
\* The record's split between pages the commit wrote and pages that existed is
\* Deadlist.Existed, which ReplacementStep.Net derives from the put results.
\* The code nets a named interval of commits once, where the model queues one
\* record per commit. Whole-interval netting is not covered by these configs.
\*
\* The put is the one place existence is exact, so the Volume counts there a
\* page the put finds stored whose parent in the new tree it did not also find
\* stored. Under PutRule "reported" the put reports which pages it found, and
\* the record treats only the others as written; under the other rules the
\* record carries the log's view, in which every page the commit wrote is
\* created. The pages the new tree shares with the old
\* one are not written: in the code the log marks them counted, and they are
\* stored at the put because the old root pins them until the root moves,
\* which KeptStored checks.
\*
\* With Race, a commit is two steps, as in the code: Take nets the log, then
\* Put writes the batch, and another lineage may store an identical page in
\* between. The race model gives each lineage its own root over one shared
\* fresh page L, so only that leaf races. RaceApply lets records apply while a
\* commit waits for its put, to show the leaks that allows never free a live
\* page.
\*
\* With Pins, a lineage may pin its current root as a reader does. The pin is an
\* owner edge, so it counts at once, and dropping it queues a release record.
\* Resolve takes one release as the dedicated resolver does: it takes one count
\* from the page and stops if the page lives, and otherwise frees it and pushes
\* one release to each child. It runs at any time after the release's commit,
\* but not while a deadlist record is part way through: the Volume's collector
\* runs one at a time, and a record's push-down counts for its unlisted
\* children land only as it applies their parents.
\*
\* The lineage's hold on its old root stays in extra until the record applies:
\* in the code the record's name under deadlist/ holds its From, and the move
\* that releases the old name nets against it because both land in one physical
\* transaction. HoldAtMove FALSE splits them: the move queues a release of the
\* old root and Name adds the record's count later, which frees a page a new
\* tree still names when the release resolves first.
\*
\* With Fails, a commit is Take then Put over the ordinary roots, and its
\* publication may fail instead: the engine reinstalls the World from before
\* it, so the commit leaves no put, no record and no root, and the next commit
\* starts from the reinstalled root. A put drained before the failure is left
\* out; it stores pages no root names, which leaks.
\*
\* Switches, each TRUE or set to its correct value in every config here;
\* flipping one makes the model fail, which shows the rule it guards is load
\* bearing. SharedCounts.cfg checks two lineages; the variants check one
\* lineage exactly with liveness, failed publications, and the put race:
\* - Serial: one record in flight at a time.
\* - PushDown: an alive listed page adds a holder to its unlisted children.
\* - Refined: an unlisted child's change depends on whether its parent in the
\*   new tree was written by this record's commit (an edge the old page hands
\*   over) or already existed (an edge already counted).
\* - PutRule: "reported" adds a count for each page the put finds stored whose
\*   parent the put did not find stored, and the record learns which pages the
\*   put found; "batch-parent" counts the same way but the record treats every
\*   written page as created; "every" adds one for every page the put finds
\*   stored, and "none" adds no count at the put.
\* - Compose: how Name turns an interval of commits into records. "sequence"
\*   queues each commit's record in order, as if each kept its own found set.
\*   "first" and "last" compose one record the way Replacements.Then does,
\*   dropping a page that died and came back from the dead list, and key the
\*   written set by ref: a page is written if its first, or its last, put in
\*   the interval did not find it stored. "revived" keys by last put and adds
\*   to the record's loss each put that counted a page an earlier commit of
\*   the interval killed.
\* - HoldAtMove: a move and the naming of the record that holds its old root
\*   land in one transaction.

EXTENDS Integers, FiniteSets, Sequences

CONSTANTS Lineages, MaxCommits, Serial, PushDown, Refined,
          Race, PutRule, RaceApply, Compose,
          Fails, Pins, HoldAtMove

None == "none"

\* Two versions of a path R -> X -> C -> E, with F kept beside E, and an
\* empty root N; for the race, two roots P1 and P2 over one leaf L. Every tree
\* gives each page one parent.
Pages == {"R1", "R2", "X", "X2", "C", "C2", "E", "E2", "F", "N", "P1", "P2", "L"}
Kids(p) ==
  CASE p = "R1" -> {"X"}
    [] p = "R2" -> {"X2"}
    [] p = "X" -> {"C"}
    [] p = "X2" -> {"C2"}
    [] p = "C" -> {"E", "F"}
    [] p = "C2" -> {"E2", "F"}
    [] p \in {"P1", "P2"} -> {"L"}
    [] OTHER -> {}

RECURSIVE Reach(_)
Reach(p) == {p} \cup UNION {Reach(k) : k \in Kids(p)}

\* Parent(r, c) is c's parent in the tree under r, or None at its root.
Parent(r, c) == IF c = r THEN None ELSE CHOOSE q \in Reach(r) : c \in Kids(q)

VARIABLES
  root,     \* each lineage's root
  stored,   \* pages in the store
  extra,    \* each page's count record: holders minus one
  queue,    \* each lineage's records, oldest first
  pending,  \* each lineage's taken root awaiting its put, or None
  ivl,      \* each lineage's commit records since its last Name, oldest first
  pin,      \* each lineage's pinned root, or None
  rel,      \* each page's release records not yet resolved
  commits
vars == <<root, stored, extra, queue, pending, ivl, pin, rel, commits>>

Pinned == {pin[l] : l \in {m \in Lineages : pin[m] # None}}
Named == UNION {Reach(p) : p \in {root[l] : l \in Lineages} \cup Pinned}
Holders(p) == Cardinality({l \in Lineages : root[l] = p})
                + Cardinality({l \in Lineages : pin[l] = p})
                + Cardinality({q \in Named : p \in Kids(q)})

First == CHOOSE l \in Lineages : TRUE

\* The roots lineage l may move to.
Roots(l) ==
  IF ~Race THEN {"R1", "R2", "X", "X2", "N"}
  ELSE IF l = First THEN {"P1", "N"} ELSE {"P2", "N"}

Init ==
  /\ root = [l \in Lineages |-> IF l = First /\ ~Race THEN "X" ELSE "N"]
  /\ pin = [l \in Lineages |-> None]
  /\ stored = Named
  /\ extra = [p \in Pages |-> IF p \in Named THEN Holders(p) - 1 ELSE 0]
  /\ queue = [l \in Lineages |-> <<>>]
  /\ pending = [l \in Lineages |-> None]
  /\ ivl = [l \in Lineages |-> <<>>]
  /\ rel = [p \in Pages |-> 0]
  /\ commits = 0

\* Land moves lineage l to root r in one store transaction: it writes the
\* new pages the store lacks, counts the pages the put rule names, and adds the
\* record of the old pages r does not reach to the lineage's interval.
Land(l, r) ==
  LET old == root[l]
      batch == Reach(r) \ Reach(old)
      present == batch \cap stored
      written == batch \ stored
      view == IF PutRule = "reported" THEN written ELSE batch
      newHolder(p) == p = r \/ Parent(r, p) \in view
      atPut == CASE PutRule \in {"batch-parent", "reported"} ->
                      {p \in present : Parent(r, p) \notin present}
                 [] PutRule = "every" -> present
                 [] PutRule = "none" -> {}
      dead == Reach(old) \ Reach(r)
      \* The lineage's edge to an old root the new tree still reaches dies;
      \* its new parent hands it over only if this commit wrote it. The
      \* record carries the loss, so it applies after the records before it.
      loss == IF Refined /\ old \in Reach(r) /\ ~newHolder(old)
                THEN {<<old, 0>>} ELSE {}
  IN /\ stored' = stored \cup written
     /\ extra' = [p \in Pages |->
                    (IF p \in written THEN 0 ELSE extra[p])
                    + (IF p \in atPut THEN 1 ELSE 0)]
     /\ ivl' = [ivl EXCEPT ![l] = Append(@,
                [old |-> old, new |-> r, dead |-> dead, loss |-> loss,
                 written |-> view, put |-> batch, fresh |-> written,
                 counted |-> atPut,
                 done |-> {}, alive |-> {}])]
     /\ root' = [root EXCEPT ![l] = r]
     /\ rel' = IF HoldAtMove THEN rel ELSE [rel EXCEPT ![old] = @ + 1]
     /\ commits' = commits + 1

\* Commit nets and puts in one step, so nothing races it.
Commit(l, r) ==
  /\ ~Race /\ ~Fails
  /\ commits < MaxCommits
  /\ r # root[l]
  /\ Land(l, r)
  /\ UNCHANGED <<queue, pending, pin>>

\* Take nets lineage l's commit to r; its put comes later.
Take(l, r) ==
  /\ Race \/ Fails
  /\ commits < MaxCommits
  /\ pending[l] = None
  /\ r # root[l]
  /\ pending' = [pending EXCEPT ![l] = r]
  /\ UNCHANGED <<root, stored, extra, queue, ivl, pin, rel, commits>>

\* Put finishes lineage l's taken commit.
Put(l) ==
  /\ pending[l] # None
  /\ Land(l, pending[l])
  /\ pending' = [pending EXCEPT ![l] = None]
  /\ UNCHANGED <<queue, pin>>

Empty(rec) == rec.dead = {} /\ rec.loss = {}

\* Index of the first or last commit in s whose put carried page p, or 0.
FirstPut(s, p) == LET is == {i \in 1..Len(s) : p \in s[i].put}
                  IN IF is = {} THEN 0 ELSE CHOOSE i \in is : \A j \in is : i <= j
LastPut(s, p) == LET is == {i \in 1..Len(s) : p \in s[i].put}
                 IN IF is = {} THEN 0 ELSE CHOOSE i \in is : \A j \in is : i >= j

\* One record for interval s, composed as Replacements.Then does: a page that
\* died and is named again at the end is not dead, and the written set is
\* keyed by ref.
Composed(s) ==
  LET old == s[1].old
      new == s[Len(s)].new
      dead == UNION {s[i].dead : i \in 1..Len(s)} \ Reach(new)
      keyed(p) == IF Compose = "first" THEN FirstPut(s, p) ELSE LastPut(s, p)
      written == {p \in Pages : keyed(p) # 0 /\ p \in s[keyed(p)].fresh}
      newHolder(p) == p = new \/ Parent(new, p) \in written
      \* Under "revived" each commit keeps the root loss it computed with its
      \* own found set; the other rules recompute it over the interval.
      rootLoss == IF Compose = "revived"
                  THEN UNION {{<<x[1], i>> : x \in s[i].loss} : i \in 1..Len(s)}
                  ELSE IF Refined /\ old # new /\ old \in Reach(new)
                          /\ ~newHolder(old)
                       THEN {<<old, 0>>} ELSE {}
      \* Under "revived", each put that counted a page an earlier commit of the
      \* interval killed gives back that death's count, which Then dropped.
      revived == IF Compose # "revived" THEN {}
                 ELSE {<<p, i>> \in Pages \X (1..Len(s)) :
                         p \in s[i].counted
                         /\ \E j \in 1..(i - 1) : p \in s[j].dead}
      loss == rootLoss \cup revived
  IN [old |-> old, new |-> new, dead |-> dead, loss |-> loss,
      written |-> written, done |-> {}, alive |-> {}]

\* Name ends lineage l's interval and queues what died in it.
Name(l) ==
  /\ ivl[l] # <<>>
  /\ LET s == ivl[l]
         recs == IF Compose = "sequence"
                 THEN SelectSeq(s, LAMBDA rec : ~Empty(rec))
                 ELSE IF Empty(Composed(s)) THEN <<>> ELSE <<Composed(s)>>
     IN /\ queue' = [queue EXCEPT ![l] = @ \o recs]
        \* Each record named adds its hold on its old root, unless the move
        \* already left that hold in place.
        /\ extra' = IF HoldAtMove THEN extra
                    ELSE [p \in Pages |-> extra[p]
                            + Cardinality({i \in 1..Len(recs) : recs[i].old = p})]
  /\ ivl' = [ivl EXCEPT ![l] = <<>>]
  /\ UNCHANGED <<root, stored, pending, pin, rel, commits>>

InFlight(m) == queue[m] # <<>> /\ Head(queue[m]).done # {}

\* Apply takes listed page p of lineage l's oldest record, after its parent.
Apply(l, p) ==
  /\ queue[l] # <<>>
  /\ Serial => \A m \in Lineages \ {l} : ~InFlight(m)
  /\ ~RaceApply => \A m \in Lineages : pending[m] = None
  /\ LET rec == Head(queue[l])
         \* The listed pages that name p. A composed record may list pages of
         \* several trees, so p may have more than one.
         pars == {q \in rec.dead : p \in Kids(q)}
     IN
     /\ p \in rec.dead \ rec.done
     /\ pars \subseteq rec.done
     /\ LET protected == pars \cap rec.alive # {}
            counted == ~protected /\ extra[p] > 0
            frees == ~protected /\ ~counted
            alive == protected \/ counted
            \* A surviving page stays stored and still names its children.
            names == ~frees
            unlisted == Kids(p) \ rec.dead
            handedOver(c) == c = rec.new \/ Parent(rec.new, c) \in rec.written
            delta(c) ==
              IF ~Refined THEN (IF names /\ PushDown THEN 1 ELSE 0)
              ELSE IF names THEN (IF handedOver(c) /\ PushDown THEN 1 ELSE 0)
              ELSE (IF handedOver(c) THEN 0 ELSE -1)
            rec2 == [rec EXCEPT !.done = @ \cup {p},
                                !.alive = IF alive THEN @ \cup {p} ELSE @]
        IN /\ stored' = IF frees THEN stored \ {p} ELSE stored
           /\ extra' = [q \in Pages |->
                          extra[q] - (IF q = p /\ counted THEN 1 ELSE 0)
                                   + (IF q \in unlisted THEN delta(q) ELSE 0)]
           /\ queue' = IF rec2.done = rec.dead /\ rec.loss = {}
                       THEN [queue EXCEPT ![l] = Tail(@)]
                       ELSE [queue EXCEPT ![l] = <<rec2>> \o Tail(@)]
           /\ UNCHANGED <<root, pending, ivl, pin, rel, commits>>

\* ApplyLoss ends a record whose lineage lost its edge to the old root: the
\* root's new parent already existed, so its holder count falls by one.
ApplyLoss(l) ==
  /\ queue[l] # <<>>
  /\ Serial => \A m \in Lineages \ {l} : ~InFlight(m)
  /\ ~RaceApply => \A m \in Lineages : pending[m] = None
  /\ LET rec == Head(queue[l]) IN
     /\ rec.dead \subseteq rec.done
     /\ rec.loss # {}
     /\ extra' = [q \in Pages |->
                    extra[q] - Cardinality({x \in rec.loss : x[1] = q})]
     /\ queue' = [queue EXCEPT ![l] = Tail(@)]
     /\ UNCHANGED <<root, stored, pending, ivl, pin, rel, commits>>

ApplyAny ==
  \/ \E l \in Lineages, p \in Pages : Apply(l, p)
  \/ \E l \in Lineages : ApplyLoss(l)

\* Fail drops lineage l's taken commit: its publication failed, so nothing it
\* wrote landed and the lineage stays at its root. The engine reinstalls that
\* root, so the next commit's base is the root before the failed one. A failure
\* spends a commit of the budget, which keeps every behavior finite.
Fail(l) ==
  /\ Fails
  /\ pending[l] # None
  /\ pending' = [pending EXCEPT ![l] = None]
  /\ commits' = commits + 1
  /\ UNCHANGED <<root, stored, extra, queue, ivl, pin, rel>>

\* Pin names lineage l's current root as a reader does. The owner edge counts
\* at once. A pin spends a commit of the budget, which keeps behaviors finite.
Pin(l) ==
  /\ Pins
  /\ commits < MaxCommits
  /\ pin[l] = None
  /\ pending[l] = None
  /\ pin' = [pin EXCEPT ![l] = root[l]]
  /\ extra' = [extra EXCEPT ![root[l]] = @ + 1]
  /\ commits' = commits + 1
  /\ UNCHANGED <<root, stored, queue, pending, ivl, rel>>

\* Unpin drops lineage l's pin and queues its release.
Unpin(l) ==
  /\ pin[l] # None
  /\ pin' = [pin EXCEPT ![l] = None]
  /\ rel' = [rel EXCEPT ![pin[l]] = @ + 1]
  /\ UNCHANGED <<root, stored, extra, queue, pending, ivl, commits>>

\* Resolve takes one release of page p: a counted page loses one count and
\* lives; an uncounted one frees and releases each child.
Resolve(p) ==
  /\ rel[p] > 0
  /\ \A m \in Lineages : ~InFlight(m)
  /\ ~RaceApply => \A m \in Lineages : pending[m] = None
  /\ IF extra[p] > 0
     THEN /\ extra' = [extra EXCEPT ![p] = @ - 1]
          /\ rel' = [rel EXCEPT ![p] = @ - 1]
          /\ UNCHANGED stored
     ELSE /\ stored' = stored \ {p}
          /\ rel' = [q \in Pages |-> rel[q] - (IF q = p THEN 1 ELSE 0)
                                    + (IF q \in Kids(p) THEN 1 ELSE 0)]
          /\ UNCHANGED extra
  /\ UNCHANGED <<root, queue, pending, ivl, pin, commits>>

ResolveAny == \E p \in Pages : Resolve(p)

PutAny == \E l \in Lineages : Put(l)

NameAny == \E l \in Lineages : Name(l)

Next ==
  \/ \E l \in Lineages : \E r \in Roots(l) : Commit(l, r) \/ Take(l, r)
  \/ PutAny
  \/ \E l \in Lineages : Fail(l)
  \/ NameAny
  \/ ApplyAny
  \/ \E l \in Lineages : Pin(l) \/ Unpin(l)
  \/ ResolveAny
  \/ UNCHANGED vars

Spec == Init /\ [][Next]_vars /\ WF_vars(ApplyAny) /\ WF_vars(PutAny)
          /\ WF_vars(NameAny) /\ WF_vars(ResolveAny)

Quiet == /\ \A l \in Lineages :
              queue[l] = <<>> /\ pending[l] = None /\ ivl[l] = <<>>
         /\ \A p \in Pages : rel[p] = 0

\* No live root names a freed page.
NoFreedRead == Named \subseteq stored

\* A page the old and new trees share is stored while the commit waits for
\* its put: the log marks it counted, and a marked page the put stored afresh
\* would leave its children's counts short.
KeptStored == \A l \in Lineages : pending[l] # None =>
                (Reach(root[l]) \cap Reach(pending[l])) \subseteq stored

\* A release names a stored page, so no page frees twice.
ReleasesStored == \A p \in Pages : rel[p] > 0 => p \in stored

\* No count record goes below zero.
CountsNonNegative == \A p \in Pages : extra[p] >= 0

\* Once every record has applied, no count record is below the page's holders
\* minus one. A count above it leaks; one below it frees a live page.
CountsCover == Quiet => \A p \in Named : extra[p] >= Holders(p) - 1

\* Once every record has applied, each count record equals the page's holders
\* minus one, so the next death of each page frees or keeps it correctly.
CountsExact == Quiet => \A p \in Named : extra[p] = Holders(p) - 1

\* Once every record has applied, the store holds only named pages.
NoLeak == Quiet => stored = Named

\* Every page no root names is eventually freed.
Collected == <>[](Quiet /\ stored = Named)
====
