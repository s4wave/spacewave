---- MODULE IntervalCounts ----
\* Shared counts across whole intervals: one World lineage commits into an
\* interval, names it, and the record its net produces applies by the push-down
\* rule, while puts, releases, pins and the replay hold change the same counts.
\* SharedCounts.tla checks the push-down rule on per-commit records over trees
\* that give each page one parent; this spec checks the code's netting instead:
\*
\* - ReplacementSteps.Join and ReplacementStep.Net (db/block/replacement-step.go)
\*   over every commit since the last name, with found pages from the put
\*   receipts as pendingNet.net folds them (core/sobject/world/engine);
\* - NetPuts, the record of an interval whose From the counts do not hold;
\* - the Volume's put counting, addExistedHolds (db/volume/common/kvtx), with
\*   MarkOriginal's counted entries in the naming batch;
\* - RefGraph.ResolveDeaths and ChangeHolds, in any walk order, and
\*   ResolveReleases (db/block/gc);
\* - the replay hold, a block marked HoldsRoots that names the last named root,
\*   as a holder that moves at each name and releases the root before;
\* - a reader pin, an owner edge that counts at once and releases later;
\* - a forfeit, an interval whose puts were dropped: it names no record, the
\*   save carries a recount, and the next interval is unfunded.
\*
\* Pages are content addressed, so Kids is fixed. L has two parents, A and B,
\* and R1 and R3 reach it through both: a write that copies a page under a
\* second parent, or moves it, is a commit between these roots. R4 names A
\* directly and through D, so a commit from R0 to R4 copies the stored A under
\* two new parents while the dead R0 named it.
\*
\* A commit replaces a parent-closed set of the current tree's pages, encodes
\* every page of the new tree the current one lacks, and writes back each
\* replaced page a written page still names; a replaced page only an untouched
\* page names may be written back or not. It puts some of its pages at once
\* when Overflow allows, as the page set's overflow does; the rest stay in
\* memory until the name puts the ones the new root reaches. Each put and each
\* name is one store transaction. Record resolution reads a snapshot outside
\* the writer; an O(1) physical revision check rejects any changed basis before
\* committing count changes and retirement together. Counted-free removes a
\* logical death's bytes. Actual shadow retains them and atomically invalidates
\* counts at the first nonleaf free, before another writer can adopt unfunded
\* children. Source shadow separates checkpoint sealing, ordinary leaf
\* preparation and record carrying. DeadlistLedger/DeadlistPreparation check
\* worker readiness, stage leases and terminal I/O failures. The
\* recount exposes clearing/sighting/completion here; SharingWalk.tla checks
\* the individual slices.
\* The generation is a finite current/stale equality certificate, invalidated
\* on every rebuild and actual count-metadata erasure. Byte-only eviction
\* preserves that certificate but a raw-new tracked refill is uncertified.
\* Encoding is pure over immutable steps and receipts, so
\* Name's certificate check covers recount before or after encoding.
\*
\* Left out: failed publications, since the engine reinstalls the World from
\* before the failed commit, which leaves no step on the chain; a chain gap,
\* which nets from the puts as a forfeit's successor does; several lineages,
\* which SharedCounts.tla covers. Source-shadow wrappers below use literal
\* ordered base/written/pending/waiting root identities.
\*
\* IntervalCounts.cfg checks three commits without overflow puts, and
\* IntervalCounts-overflow.cfg two commits with them and reader pins.
\* IntervalCounts-discard.cfg checks two commits with overflow puts and
\* discarded writes, including fresh pages no accepted step created. The
\* manual three-commit discard proof changes only MaxCommits to 3; it also
\* covers a Dropped page adopted before its earlier record applies. Under
\* CountRule "volume" an overflow put counts a found page once, however many
\* new parents name it in memory, failing CountsCover, and overflow puts of
\* an old page or of the named root leak, failing CountsExact. CountRule
\* "hold", the code's rule, passes both configs.
\*
\* Switches, each set to the code's rule in IntervalCounts.cfg; the mutant
\* configs flip one each:
\* - Overflow: "none", "whole" (a commit puts all its pages or none) or
\*   "pages" (any bottom-up subset).
\* - HeldRule: "parent" holds a page once per written parent naming it;
\*   "once" holds it once, skipping a page a dead original page named.
\* - FoundRule: "every" calls a page found when every put found it; "any"
\*   when any put did.
\* - Unfunded: "puts" nets an unfunded interval from its puts; "steps" nets
\*   it from its steps as a funded one.
\* - HoldRule: "always" counts the replay hold's edge to the named root;
\*   "found" counts it only when the root was already stored.
\* - Recount: a forfeit's save requests the global rebuild too. External
\*   rebuilds always remain enabled, including while a record's read is pending.
\* - OriginalRule: "stored" pushes a page's children in the live walk as new
\*   edges only when the interval stored the page afresh: an original or found
\*   page is the stored page, whose edges counted, and which is held for
\*   each new edge to it, and listed existed without one, as an untouched
\*   page is; "created" pushes them for every created or replaced page,
\*   holds no written page, and lists a written page existed only when found.
\* - ListRule: "reached" lists a replaced original page dead only when the old
\*   root reaches it through dead original pages, since a page dirtied through
\*   a parent the step dropped may still sit under a parent no step touched;
\*   "all" lists every dead replaced page. "reached" also removes each dead
\*   page a stored replaced page names that is not listed replaced, written or
\*   not: a stored page's children are stored; "all" removes only pages no
\*   step wrote.
\* - MarkedParent: a marked entry the put finds stored stands in as the found
\*   parent of a found child, whose edge from it the store counted.
\* - FreshHeld: a page stored afresh is held once for each fresh parent
\*   naming it past its first holder; FALSE holds it never.
\* - WalkRule: "parents" decides each page of the death walk once all its
\*   walk parents are decided, taking one count per parent that frees;
\*   "first" decides it by whichever parent reaches it first.
\* - DropRule: "own", the code's rule, lists a dead page stored afresh as
\*   Dropped: its death takes no count from its children, while survival funds
\*   each edge before deciding any walked child. "replaced" instead lists it
\*   with the replaced pages, whose edges the walk takes back. Only an overflow
\*   put stores such a page, and "replaced" takes counts that were never added.
\* - CountRule: "volume" has the Volume count a found unmarked entry's new
\*   edges from the batch and a found entry with no parent there. "net" has
\*   the Volume count nothing for a found entry, and Net hold each live page
\*   the interval found once per fresh parent naming it. "hold" nets as "net"
\*   does, but the Volume counts each put that finds an unmarked entry once,
\*   holding the page through its record's child funding, then returning it
\*   through the release queue. An old or incomplete basis instead funds
\*   NetPuts' fresh-parent edges and leaves uncertain Found holds high. No
\*   historical death or blind refund may consume a missing holder. Under
\*   "net" a record applying before the name frees a page the put found.
\* - Discards: a commit may put pages at once and then be discarded, as a
\*   write the replay reorders away or a replayed position a later replay
\*   drops, so no step on the chain created its pages.
\* - StrayRule: "created" has Net take each page a put stored afresh that no
\*   step created or replaced as a created page, as ReplacementStep.AddFresh
\*   does; "ignore" leaves it out, so its edges, which the put deferred to the
\*   record, are never counted.

EXTENDS Integers, FiniteSets, Sequences, TLC

CONSTANTS MaxCommits, Overflow, Pins, Forfeits, Discards,
          HeldRule, FoundRule, Unfunded, HoldRule, Recount, OriginalRule, ListRule,
          FreshHeld, WalkRule, DropRule, CountRule,
          MarkedParent, StrayRule

None == "none"
Idle == [on |-> FALSE]  \* no record applying

Pages == {"L", "M", "A", "B", "D", "R0", "R1", "R2", "R3", "R4"}
GraphKids(p) ==
  CASE p \in {"A", "B"} -> {"L"}
    [] p = "D" -> {"A"}
    [] p = "R0" -> {"A"}
    [] p = "R1" -> {"A", "B"}
    [] p = "R2" -> {"B", "M"}
    [] p = "R3" -> {"D", "B"}
    [] p = "R4" -> {"D", "A"}
    [] OTHER -> {}
KidsMap == TLCEval([p \in Pages |-> GraphKids(p)])
Kids(p) == KidsMap[p]
Roots == {"R0", "R1", "R2", "R3", "R4"}

RECURSIVE GraphReach(_)
GraphReach(p) == {p} \cup UNION {GraphReach(k) : k \in Kids(p)}
ReachMap == TLCEval([p \in Pages |-> GraphReach(p)])
Reach(p) == ReachMap[p]

Sum(f, S) == LET RECURSIVE F(_) F(T) == IF T = {} THEN 0
                                         ELSE LET x == CHOOSE x \in T : TRUE
                                              IN f[x] + F(T \ {x})
             IN F(S)

\* The sets of the current tree's pages a commit may replace: the root and
\* any pages each with a parent in the set.
ClosedMap == TLCEval([r \in Roots |-> {S \in SUBSET Reach(r) :
                r \in S /\ \A p \in S \ {r} : \E q \in S : p \in Kids(q)}])
Closed(r) == ClosedMap[r]

VARIABLES
  headerAliases, \* independent ordinary owners of actual record headers
  aliases,   \* other ordinary named owners of already-stored wrapper blocks
  save,      \* immutable root-hold input gathered under the engine writer
  receiptSeen, \* at least one real publication input belongs to this interval
  firstCurrent, \* first raw-new receipt with a current complete certificate
  wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch,
  sighted,   \* the active sharing walk has passed its clearing prefix
  tainted,   \* metadata erasure lost a count in an already-sighted prefix
  partStore, \* immutable physical record leaves, keyed by their complete list payload
  retired,   \* ordinary record headers whose root releases have not resolved
  doubt,     \* a reached child list is unknown; counts cannot decide deaths
  storageMode, \* actual retained shadow bytes or idealized counted deletion
  cur,       \* the root in memory
  named,     \* the root last named
  stored,    \* accounted incarnations, including metadata-preserving refills
  extra,     \* each page's shared count: holders minus one
  rel,       \* each page's releases not yet resolved
  kept,      \* pages encoded this interval and held in memory
  steps,     \* the commits since the last name, oldest first
  putSet,    \* pages a put of this interval carried
  freshSet,  \* pages some put of this interval stored afresh
  firstFresh,\* pages whose first put this interval stored afresh
  foundPuts, \* each page's puts this interval that found it, unmarked
  funded,    \* the engine counts on the lineage of named as a holder
  lineage,   \* ghost: the counts do hold the lineage of named
  queue,     \* named records not yet applied, oldest first
  walk,      \* the record applying, or Idle
  pin,       \* the reader pin's root, or None
  hold,      \* the replay hold's root
  staged,    \* roots the put bucket still owns
  counting,  \* the Volume has complete counts; FALSE is a running recount
  lineageCurrent, \* the first lineage root receipt used this generation
  basisCurrent, \* every open receipt used this complete generation
  commits,
  lossy,     \* a forfeit or unfunded record ran: leaks are allowed
  underflow  \* a count change went below zero
vars == <<headerAliases, aliases, save, receiptSeen, firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch,
          storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet,
          firstFresh, foundPuts, funded, lineage, queue, walk, pin, hold,
          commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent,
          partStore, retired, doubt, sighted, tainted>>

Zero == [p \in Pages |-> 0]
\* Found membership is the positive support of the same receipt counts. Init,
\* every receipt increment and all interval resets preserve this equality;
\* deriving it avoids an independently stored copy of receipt history.
foundSet == {p \in Pages : foundPuts[p] > 0}
Ready == counting /\ ~doubt
Shadow == storageMode = "shadow"
PinRoots == IF pin = None THEN {} ELSE {pin}
HoldKids(h) == {h[i] : i \in 1..Len(h)}
WrapperHolders(p) == Cardinality({h \in wrappers : p \in HoldKids(h)})
UserLive == UNION {Reach(r) : r \in {named} \cup PinRoots
                    \cup IF Shadow THEN HoldKids(wrapper) \cup UNION {HoldKids(h) : h \in aliases} ELSE {hold}}
PartKids(part) == {a[2] : a \in {atom \in part : atom[1] = 7}}
AliasParts == UNION {h.parts : h \in {rec \in retired : rec.id \in headerAliases}}
PendingParts == AliasParts \cup UNION {queue[i].parts : i \in 1..Len(queue)}
                  \cup IF walk.on THEN walk.rec.parts ELSE {}
                  \cup UNION {waiting[i].prepared : i \in 1..Len(waiting)}
PartRoots == UNION {PartKids(part) : part \in PendingParts}
PhysicalPartHolders(p) == Cardinality({part \in partStore : p \in PartKids(part)})
FoundRoots == IF Shadow
             THEN UNION {UNION {PartKids(part) : part \in queue[i].parts} :
                           i \in {j \in 1..Len(queue) : queue[j].current}}
                    \cup IF walk.on /\ walk.rec.current
                           THEN UNION {PartKids(part) : part \in walk.rec.parts} ELSE {}
                    \cup UNION {UNION {PartKids(part) : part \in waiting[i].prepared} :
                           i \in {j \in 1..Len(waiting) : waiting[j].record.current}}
             ELSE {p \in Pages : \E i \in 1..Len(queue) : queue[i].found[p] > 0}
                    \cup IF walk.on THEN {p \in Pages : walk.rec.found[p] > 0} ELSE {}
Live == UserLive \cup IF Ready THEN UNION {Reach(r) : r \in FoundRoots} ELSE {}
Holders(p, live) == Cardinality({q \in live : p \in Kids(q)})
                + (IF Shadow THEN WrapperHolders(p) ELSE IF p = hold THEN 1 ELSE 0)
                + (IF p = pin THEN 1 ELSE 0)
                + (IF p = named /\ lineage THEN 1 ELSE 0)
                + (IF Shadow THEN PhysicalPartHolders(p) ELSE 0)

\* Source shadow never reconstructs historical claims. Physical leaf identity
\* remains in parts even when duplicate historical fields become unobservable.
StaleRecord(rec) ==
  IF ~Shadow THEN [rec EXCEPT !.current = FALSE]
  ELSE [rec EXCEPT !.current = FALSE, !.transfer = FALSE, !.from = None,
                   !.replaced = {}, !.dropped = {}, !.removed = {},
                   !.kept = {}, !.existed = {}, !.held = Zero, !.found = Zero]
StaleQueue(q) == [i \in 1..Len(q) |-> StaleRecord(q[i])]

\* A stale pending record retains its actual list blocks and its ordinary
\* waiting-From role; only its now-unusable historical decision fields clear.
StaleWaiting(ws) == [i \in 1..Len(ws) |->
  [ws[i] EXCEPT !.record = StaleRecord(ws[i].record)]]

RemoveAt(s, i) == SubSeq(s, 1, i - 1) \o SubSeq(s, i + 1, Len(s))
RECURSIVE WithoutRoot(_, _)
WithoutRoot(s, root) == IF s = <<>> THEN <<>>
                       ELSE (IF Head(s) = root THEN <<>> ELSE <<Head(s)>>)
                              \o WithoutRoot(Tail(s), root)
RECURSIVE UniqueRoots(_)
UniqueRoots(s) == IF s = <<>> THEN <<>>
                  ELSE <<Head(s)>> \o WithoutRoot(UniqueRoots(Tail(s)), Head(s))
\* This order is the actual holdBase producer order, with first occurrence
\* deduplication. Tuple identity preserves different serialized permutations.
RootOrder(b, w, checkpoints, ws) == UniqueRoots(<<b, w>>
  \o [i \in 1..Len(checkpoints) |-> checkpoints[i].root]
  \o [i \in 1..Len(ws) |-> ws[i].from])
DesiredHold == RootOrder(base, written, pendingWorlds, waiting)

\* The unchanged configurations explore both contracts. Shadow keeps physical
\* incarnations and rebuilds before their unfunded children can be trusted.
\* Counted-free is the future primitive's separate exact-reclamation claim.
Init ==
  /\ headerAliases = {}
  /\ aliases = {}
  /\ save = Idle /\ receiptSeen = FALSE
  /\ storageMode \in {"shadow", "counted-free"}
  /\ wrapper = IF Shadow THEN <<"R0">> ELSE <<>>
  /\ wrappers = IF Shadow THEN {<<"R0">>} ELSE {}
  /\ wrapperReleases = {} /\ base = "R0" /\ written = "R0"
  /\ writtenEpoch = 0 /\ curEpoch = 0 /\ checkpointEpoch = 0
  /\ unwritten = <<>> /\ pendingWorlds = <<>> /\ waiting = <<>>
  /\ partStore = {} /\ retired = {} /\ doubt = FALSE
  /\ sighted = FALSE /\ tainted = FALSE
  /\ cur = "R0" /\ named = "R0" /\ hold = "R0" /\ pin = None
  /\ stored = Reach("R0")
  /\ extra = [p \in Pages |-> IF p = "R0" THEN 1 ELSE 0]
  /\ rel = [p \in Pages |-> 0]
  /\ kept = {} /\ steps = <<>>
  /\ putSet = {} /\ freshSet = {} /\ firstFresh = {}
  /\ firstCurrent = {}
  /\ foundPuts = [p \in Pages |-> 0]
  /\ funded = TRUE /\ lineage = TRUE
  /\ queue = <<>> /\ walk = Idle
  /\ lineageCurrent = TRUE
  /\ staged = {}
  /\ counting = TRUE /\ basisCurrent = TRUE
  /\ commits = 0 /\ lossy = FALSE /\ underflow = FALSE

\* ---------------------------------------------------------------------------
\* The Volume's put: addExistedHolds over a batch of entries E, of which the
\* writer marked K counted. existed is the store before the batch. H holds
\* the pages a hold block in the batch names, which never count as having no
\* parent there.

PutCounts(E, K, H) ==
  LET existed(p) == p \in stored
      keptK == {p \in E \cap K : existed(p)}
      revived == {p \in E \cap K : ~existed(p)}
      listed == (E \ K) \cup revived
      found(p) == p \in E \ K /\ existed(p)
      parents(k) == Cardinality({e \in listed \cup
                                   (IF MarkedParent THEN keptK ELSE {}) :
                                   k \in Kids(e)})
      fromNew(k) == IF CountRule = "volume" /\ k \in listed /\ found(k)
                    THEN Cardinality({e \in listed : ~found(e) /\ k \in Kids(e)})
                    ELSE 0
      noParent(k) == IF CountRule = "volume" /\ found(k) /\ parents(k) = 0
                        /\ k \notin H THEN 1 ELSE 0
      provisional(k) == IF CountRule = "hold" /\ found(k) THEN 1 ELSE 0
      fromRevived(k) == Cardinality({e \in revived : k \in Kids(e)
                          /\ (k \in keptK \/ (k \notin listed /\ k \in stored))})
  IN [p \in Pages |-> (IF p \in E /\ ~existed(p) THEN 0 ELSE extra[p])
                       + (IF Ready THEN fromNew(p) + noParent(p) + provisional(p)
                                           + fromRevived(p) ELSE 0)]

\* A gathered save can contain both Netted kept pages and ordinary stored
\* root copies. tx.publish marks only candidate overflow pages; checkpoint
\* and applyHold also select their kept save pages. Count the actual first
\* entry's kind, including mixed fresh ordinary/Netted parent-child pairs.
\* Ordinary out-of-batch children of fresh ordinary parents are funded at
\* publication too. Netted parents alone defer those edges to their record;
\* revived Counted and fresh HoldsRoots parents retain their existing probes.
ClassCounts(E, N, K, h) ==
  LET marked == E \cap K \cap stored
      revived == (E \cap K) \ stored
      listed == (E \ K) \cup revived
      fresh == E \ stored
      old == (E \ K) \cap stored
      netted == N \ K
      parents(k) == Cardinality({e \in listed \cup (IF MarkedParent THEN marked ELSE {}) :
                        k \in Kids(e) /\ ~(CountRule # "volume" /\ e \in netted /\ k \in netted)})
                      + (IF k \in HoldKids(h) THEN 1 ELSE 0)
      newParents(k) == Cardinality({e \in listed \ stored : k \in Kids(e) /\ k \in old
                        /\ ~(CountRule # "volume" /\ e \in netted /\ k \in netted)})
      direct(k) == IF k \notin old THEN 0
                    ELSE IF k \in netted /\ CountRule = "hold" THEN 1
                    ELSE IF k \in netted /\ CountRule = "net" THEN 0
                    ELSE IF parents(k) = 0 THEN 1 ELSE 0
      revivedKids(k) == Cardinality({e \in revived : k \in Kids(e)
                        /\ (k \in marked \/ (k \notin listed /\ k \in stored))})
      ordinaryKids(k) == Cardinality({e \in ((E \ K) \ stored) \ N :
                        k \in Kids(e) /\ (k \in marked \/ (k \notin listed /\ k \in stored))})
      wrapperKid(k) == IF h \notin wrappers /\ k \in HoldKids(h) /\ k \in stored \cup E THEN 1 ELSE 0
  IN [p \in Pages |-> (IF p \in fresh THEN 0 ELSE extra[p])
        + (IF Ready THEN newParents(p) + direct(p) + revivedKids(p) + ordinaryKids(p) + wrapperKid(p) ELSE 0)]

\* The receipts of a batch's unmarked entries, folded into the interval's.
\* Cache lookup is a raw receipt observation, not an ownership transition.
\* Refills are raw-new/accounted-existing; cached entries are raw-existing but
\* untracked. The latter graph publication requests recount in the same TX.
SelectedInputs(E, N, K, refills, cached, H, submitted) ==
  LET R == N \ K
      rawFound == ((R \cap stored) \ refills) \cup (R \cap cached)
      fresh == R \ rawFound
  IN /\ putSet' = putSet \cup R
     /\ freshSet' = freshSet \cup fresh
     /\ firstFresh' = firstFresh \cup (fresh \ putSet)
     /\ foundPuts' = [p \in Pages |-> foundPuts[p] + (IF p \in rawFound THEN 1 ELSE 0)]
     /\ staged' = (staged \cup E) \ (UNION {Kids(p) : p \in E} \cup H)
     /\ receiptSeen' = IF Shadow THEN (receiptSeen \/ submitted) ELSE FALSE
ReceiptInputs(E, K, refills, cached, H) ==
  SelectedInputs(E, E, K, refills, cached, H, E # {})

\* A previous interval's UNKNOWN receipt does not invalidate another root's
\* independently certified first receipt. Keep that first-root certificate
\* separate from the interval-wide basis; generation changes invalidate both.
FirstCertificate(E, K, refills, cached, certified, recovery) ==
  LET R == E \ K
      rawFound == ((R \cap stored) \ refills) \cup (R \cap cached)
      born == (R \ rawFound) \ putSet
  IN IF ~Shadow THEN firstCurrent
     ELSE IF recovery THEN {}
     ELSE firstCurrent \cup IF Ready /\ certified /\ refills = {} /\ cached = {}
                              THEN born ELSE {}
Receipts(E, K, refills, cached) ==
  /\ ReceiptInputs(E, K, refills, cached, {})
  /\ \E certified \in BOOLEAN :
     /\ basisCurrent' = (basisCurrent /\ (E = {} \/
         (Ready /\ certified /\ refills = {} /\ cached = {})))
     /\ firstCurrent' = FirstCertificate(E, K, refills, cached, certified,
                          counting /\ (doubt \/ cached # {}) /\ E # {})

\* ---------------------------------------------------------------------------
\* Commit moves the World in memory from cur to r. It replaces Rp, encodes C,
\* and puts O of C at once.

OverflowSets(C) ==
  CASE Overflow = "none" -> {{}}
    [] Overflow = "whole" -> {{}, C}
    [] OTHER -> {O \in SUBSET C : \A p \in O : Kids(p) \subseteq O \cup stored}

\* A physical commit invalidates the earlier read even for same-count ABA.
InvalidateRead == IF walk.on
                  THEN [on |-> TRUE, valid |-> FALSE, rec |-> walk.rec]
                  ELSE walk
StaleRead == IF walk.on
             THEN [on |-> TRUE, valid |-> FALSE, rec |-> StaleRecord(walk.rec)]
             ELSE walk

\* ReplacementSteps.Join is an observational summary of the private step
\* list. Original gains new Rp before new Cr; prior-created pages never become
\* original. Every Net/MarkOriginal query uses these unions and endpoints.
\* Receipt/discard/refill history and ordered replay-role history stay separate.
AppendJoinedStep(s, from, to, rp, cr) ==
  IF s = <<>>
  THEN <<[from |-> from, to |-> to, replaced |-> rp, created |-> cr, original |-> rp]>>
  ELSE <<[from |-> s[1].from, to |-> to,
           replaced |-> s[1].replaced \cup rp, created |-> s[1].created \cup cr,
           original |-> s[1].original \cup (rp \ s[1].created)]>>

Commit ==
  /\ commits < MaxCommits
  /\ \E r \in Roots \ {cur}, Rp \in Closed(cur) :
     \E W \in SUBSET (Rp \cap Reach(r)) :
     LET C == (Reach(r) \ Reach(cur)) \cup W \cup {r} IN
     \* A dirty page a written page still names is encoded again.
     /\ \A p \in C : Kids(p) \cap Rp \subseteq C
     /\ \E O \in OverflowSets(C) :
        LET E == O \cup IF Shadow /\ save.on THEN save.entries ELSE {}
            h == IF Shadow /\ save.on THEN save.roots ELSE wrapper
        IN \E countInput \in {IF counting /\ doubt /\ E # {} THEN Zero
               ELSE IF Shadow THEN TLCEval(ClassCounts(E, O, {}, h))
               ELSE TLCEval(PutCounts(O, {}, {}))} :
           \E refills \in IF Shadow THEN SUBSET (E \cap stored) ELSE {{}},
              cached \in IF Shadow THEN SUBSET (E \ stored) ELSE {{}} :
           /\ steps' = AppendJoinedStep(steps, cur, r, Rp, C)
           /\ kept' = kept \cup (C \ O)
           /\ extra' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN Zero ELSE countInput
           /\ stored' = stored \cup E
           /\ SelectedInputs(E, O, {}, refills, cached, HoldKids(h), O # {})
           /\ \E certified \in BOOLEAN :
              /\ basisCurrent' = IF counting /\ (doubt \/ cached # {}) /\ E # {}
                   THEN putSet \cup O = {}
                   ELSE basisCurrent /\ (O = {} \/ (Ready /\ certified /\ refills = {} /\ cached = {}))
              /\ firstCurrent' = FirstCertificate(O, {}, refills, cached, certified,
                     counting /\ (doubt \/ cached # {}) /\ E # {})
           /\ save' = Idle
           /\ wrapper' = h /\ wrappers' = IF Shadow /\ save.on THEN wrappers \cup {h} ELSE wrappers
           /\ wrapperReleases' = IF Shadow /\ save.on /\ h # wrapper
                                   THEN wrapperReleases \cup {wrapper} ELSE wrapperReleases
           /\ walk' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN StaleRead
                          ELSE IF E = {} THEN walk ELSE InvalidateRead
           /\ rel' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN Zero ELSE rel
           /\ queue' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN StaleQueue(queue) ELSE queue
           /\ counting' = (counting /\ ~((doubt \/ cached # {}) /\ E # {}))
           /\ doubt' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN FALSE ELSE doubt
           /\ lineage' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN FALSE ELSE lineage
           /\ lineageCurrent' = (lineageCurrent /\ ~(counting /\ (doubt \/ cached # {}) /\ E # {}))
           /\ lossy' = (lossy \/ (counting /\ (doubt \/ cached # {}) /\ E # {}))
           /\ cur' = r
           /\ unwritten' = IF Shadow THEN Append(unwritten, [epoch |-> commits + 1, root |-> r]) ELSE unwritten
  /\ commits' = commits + 1
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ curEpoch' = IF Shadow THEN commits + 1 ELSE curEpoch
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<base, written, writtenEpoch, pendingWorlds, checkpointEpoch>>
  /\ UNCHANGED <<storageMode, named, funded, pin, hold, underflow, partStore, retired>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Discard puts pages of a commit from cur at once, as Commit does, and then
\* drops the commit: the World in memory and the steps stay as they were.
Discard ==
  /\ Discards
  /\ commits < MaxCommits
  /\ \E r \in Roots \ {cur}, Rp \in Closed(cur) :
     \E W \in SUBSET (Rp \cap Reach(r)) :
     LET C == (Reach(r) \ Reach(cur)) \cup W \cup {r} IN
     /\ \A p \in C : Kids(p) \cap Rp \subseteq C
     /\ \E O \in OverflowSets(C) \ {{}} :
        LET E == O \cup IF Shadow /\ save.on THEN save.entries ELSE {}
            h == IF Shadow /\ save.on THEN save.roots ELSE wrapper
        IN \E countInput \in {IF counting /\ doubt /\ E # {} THEN Zero
               ELSE IF Shadow THEN TLCEval(ClassCounts(E, O, {}, h))
               ELSE TLCEval(PutCounts(O, {}, {}))} :
           \E refills \in IF Shadow THEN SUBSET (E \cap stored) ELSE {{}},
              cached \in IF Shadow THEN SUBSET (E \ stored) ELSE {{}} :
           /\ extra' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN Zero ELSE countInput
           /\ stored' = stored \cup E
           /\ kept' = kept
           /\ SelectedInputs(E, O, {}, refills, cached, HoldKids(h), O # {})
           /\ \E certified \in BOOLEAN :
              /\ basisCurrent' = IF counting /\ (doubt \/ cached # {}) /\ E # {}
                   THEN putSet \cup O = {}
                   ELSE basisCurrent /\ (O = {} \/ (Ready /\ certified /\ refills = {} /\ cached = {}))
              /\ firstCurrent' = FirstCertificate(O, {}, refills, cached, certified,
                     counting /\ (doubt \/ cached # {}) /\ E # {})
           /\ save' = Idle
           /\ wrapper' = h /\ wrappers' = IF Shadow /\ save.on THEN wrappers \cup {h} ELSE wrappers
           /\ wrapperReleases' = IF Shadow /\ save.on /\ h # wrapper
                                   THEN wrapperReleases \cup {wrapper} ELSE wrapperReleases
           /\ walk' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN StaleRead
                          ELSE IF E = {} THEN walk ELSE InvalidateRead
           /\ rel' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN Zero ELSE rel
           /\ queue' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN StaleQueue(queue) ELSE queue
           /\ counting' = (counting /\ ~((doubt \/ cached # {}) /\ E # {}))
           /\ doubt' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN FALSE ELSE doubt
           /\ lineage' = IF counting /\ (doubt \/ cached # {}) /\ E # {} THEN FALSE ELSE lineage
           /\ lineageCurrent' = (lineageCurrent /\ ~(counting /\ (doubt \/ cached # {}) /\ E # {}))
           /\ lossy' = (lossy \/ (counting /\ (doubt \/ cached # {}) /\ E # {}))
  /\ commits' = commits + 1
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ unwritten' = IF Shadow THEN Append(unwritten, [epoch |-> commits + 1, root |-> cur]) ELSE unwritten
  /\ curEpoch' = IF Shadow THEN commits + 1 ELSE curEpoch
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<base, written, writtenEpoch, pendingWorlds, checkpointEpoch>>
  /\ UNCHANGED <<storageMode, cur, named, steps, funded, pin, hold, underflow, partStore, retired>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* ---------------------------------------------------------------------------
\* Join and Net, as the code computes them for interval s with found pages F
\* and carried pages P.

Original(s) == IF s = <<>> THEN {} ELSE s[1].original

\* The joined origin map is exactly the source's earliest-Rp-before-prior-Cr
\* predicate, including under every mutant Net/counting rule.
IsOriginal(s, p) == p \in Original(s)

Net(s, F, P) ==
  LET from == s[1].from
      to == s[Len(s)].to
      replaced == TLCEval(UNION {s[i].replaced : i \in 1..Len(s)})
      stepCreated == TLCEval(UNION {s[i].created : i \in 1..Len(s)})
      \* A page a put stored afresh that no step wrote: a discarded commit's.
      stray == TLCEval(IF StrayRule = "created"
               THEN (P \ F) \ (stepCreated \cup replaced) ELSE {})
      created == TLCEval(stepCreated \cup stray)
      original == TLCEval(Original(s))
      isStored(p) == p \in original \/ p \in P
      \* A created page no put carried is the stored page: a live one is
      \* stored, since the tree under the root is, and a dead one is never
      \* listed.
      foundPages == TLCEval({p \in created : (p \in F \/ p \notin P) /\ p \notin original})
      createdNet == TLCEval(created \ foundPages)
      RECURSIVE NetReach(_)
      NetReach(p) == {p} \cup IF p \in created \cup replaced
                              THEN UNION {NetReach(k) : k \in Kids(p)} ELSE {}
      live == TLCEval(NetReach(to))
      \* An original page's put was counted, so under "stored" it stands with
      \* the found pages: its stored copy's edges were counted already.
      directNet == TLCEval(IF OriginalRule = "stored" THEN createdNet \ original
                   ELSE createdNet)
      directSrc == TLCEval(IF OriginalRule = "stored"
                   THEN live \cap (createdNet \ original)
                   ELSE live \cap (createdNet \cup replaced))
      direct(k) == (IF k = to THEN 1 ELSE 0)
                     + Cardinality({p \in directSrc : k \in Kids(p)})
      RECURSIVE DeadOriginal(_)
      DeadOriginal(p) == {p} \cup UNION {DeadOriginal(k) :
                           k \in {c \in Kids(p) : c \in replaced \cap original
                                                  /\ c \notin live}}
      deadOriginal == TLCEval(IF from \in live THEN {} ELSE DeadOriginal(from))
      untouched == TLCEval(IF ListRule = "all" THEN {}
                   ELSE ((replaced \cap original) \ live) \ deadOriginal)
      replacedStored == TLCEval({p \in replaced \ untouched : isStored(p) /\ (p \notin foundPages \/ p \in live)})
      replacedList == TLCEval((replacedStored \ live)
                        \cup {p \in created : p \notin live /\ p \notin replaced
                                              /\ isStored(p) /\ p \notin foundPages})
      \* A dead page stored afresh named its children by edges no put counted.
      dropped == TLCEval(IF DropRule = "own"
                 THEN replacedList \cap (createdNet \ original) ELSE {})
      namedKids == TLCEval(UNION {Kids(p) : p \in replacedStored \ dropped})
      readNamed == TLCEval(UNION {Kids(p) : p \in (replacedStored \ live) \cap original})
      removed == TLCEval({x \in namedKids \cup {from} :
                    x \notin live /\
                    IF ListRule = "all"
                    THEN x \notin replaced /\ x \notin created
                    ELSE x \notin replacedList /\ x \notin untouched})
      \* A found page's holders were counted at its put, and a page stored
      \* afresh takes its first holder without a count.
      heldBase(k) == k \in live /\ k # from /\ k # to
                     /\ IF OriginalRule = "stored"
                        THEN k \notin created \ original \/ k \in directNet
                             \/ (CountRule \in {"net", "hold"} /\ k \in foundPages)
                        ELSE k \notin replaced /\ k \notin created
      edges(k) == IF OriginalRule = "stored" /\ k \in directNet
                  THEN IF FreshHeld /\ direct(k) > 0 THEN direct(k) - 1 ELSE 0
                  ELSE direct(k)
      held == TLCEval([k \in Pages |->
                 IF ~heldBase(k) THEN 0
                 ELSE IF HeldRule = "parent" THEN edges(k)
                 ELSE IF edges(k) > 0 /\ k \notin readNamed THEN 1 ELSE 0])
  IN [from |-> from, replaced |-> replacedList \ dropped, dropped |-> dropped,
      removed |-> removed,
      kept |-> live \ readNamed, held |-> held,
      existed |-> {k \in live : k \in foundPages
                                \/ (direct(k) = 0 /\ k \notin directNet)}]

\* NetPuts: hold each child once per edge from a page stored afresh, less a
\* fresh child's first holder.
NetPuts(P, fresh) ==
  LET edges(k) == Cardinality({e \in P \cap fresh : k \in Kids(e)})
  IN [k \in Pages |-> IF edges(k) = 0 THEN 0
                      ELSE edges(k) - (IF k \in P /\ k \in fresh THEN 1 ELSE 0)]

\* ---------------------------------------------------------------------------
\* Name puts the pages in memory the root reaches, moves the replay hold to
\* the root, and names the interval's record. Forfeit does the same save with
\* the interval's puts dropped: no record, and a recount.

\* The walk passes through a page an overflow put stored above a kept page,
\* as the page set tracks it, so a kept page below one is put too.
RECURSIVE KeptReach(_)
KeptReach(p) == IF p \in kept THEN {p} \cup UNION {KeptReach(k) : k \in Kids(p)}
                ELSE IF p \in putSet THEN UNION {KeptReach(k) : k \in Kids(p)}
                ELSE {}

\* Eight list fields follow the actual codec's order. A leaf's identity is
\* its complete local payload, not its header sequence, From or generation.
ListData(rec, fallback) ==
  <<[p \in Pages |-> IF p \in rec.replaced THEN 1 ELSE 0],
    [p \in Pages |-> IF p \in rec.removed THEN 1 ELSE 0],
    [p \in Pages |-> IF p \in rec.kept THEN 1 ELSE 0],
    rec.held,
    [p \in Pages |-> IF p \in rec.existed THEN 1 ELSE 0],
    [p \in Pages |-> IF p \in rec.dropped THEN 1 ELSE 0],
    rec.found, fallback>>
PageOrder == <<"L", "M", "A", "B", "D", "R0", "R1", "R2", "R3", "R4">>
PageRanks == TLCEval([p \in Pages |-> CHOOSE i \in 1..Len(PageOrder) : PageOrder[i] = p])
PageRank(p) == PageRanks[p]
Atoms(data) == UNION { {<<k, p, n>> : n \in 1..data[k][p]} : k \in 1..8, p \in Pages }
Before(a, b) == a[1] < b[1] \/ (a[1] = b[1] /\
                 (PageRank(a[2]) < PageRank(b[2]) \/ (a[2] = b[2] /\ a[3] < b[3])))
\* Sparse atoms encode all eight complete count lists bijectively. Source
\* encoding sorts each copied full field before packing, preserving repeats.
\* A cut keeps earlier fields whole and takes a sorted prefix of one field.
\* Every possible relative ref rank is represented by the pages before its
\* pivot; the chosen occurrence preserves cuts through repeated equal refs.
\* Different records may choose inconsistent orders, a conservative extension
\* of a concrete store's fixed hash order. No possible sorted cut is omitted.
CutCandidates(atoms) == TLCEval(UNION {
  {[pivot |-> a, before |-> prior] :
     prior \in SUBSET {b[2] : b \in {v \in atoms : v[1] = a[1] /\ v[2] # a[2]}}}
  : a \in atoms})
PrefixAtoms(atoms, cut) == TLCEval({a \in atoms :
  a[1] < cut.pivot[1] \/ (a[1] = cut.pivot[1] /\
    (a[2] \in cut.before \/ (a[2] = cut.pivot[2] /\ a[3] <= cut.pivot[3])))})
\* Suffix occurrence numbers restart at one, so equal chunks reuse the same
\* physical leaf even when they occur at different positions in larger lists.
SuffixAtoms(atoms, cut) == TLCEval({
  <<a[1], a[2], IF a[1] = cut.pivot[1] /\ a[2] = cut.pivot[2]
                 THEN a[3] - cut.pivot[3] ELSE a[3]>>
  : a \in atoms \ PrefixAtoms(atoms, cut)})
\* The finite two-piece case represents multipart records. Physical leaves
\* deduplicate, while rec.found retains every logical occurrence's refund.
Layouts(input) ==
  LET atoms == TLCEval(Atoms(TLCEval(input)))
      cuts == TLCEval({c \in CutCandidates(atoms) : PrefixAtoms(atoms, c) # atoms})
      at(c) == [large |-> TRUE, parts |-> {PrefixAtoms(atoms, c), SuffixAtoms(atoms, c)}]
  IN TLCEval({[large |-> FALSE, parts |-> IF atoms = {} THEN {} ELSE {atoms}],
               [large |-> TRUE, parts |-> IF atoms = {} THEN {} ELSE {atoms}]}
              \cup {at(c) : c \in cuts})

\* The save: the naming batch's counts, with the replay hold block naming the
\* root. The batch always carries the root, reread from the store when it is
\* not in memory. A hold block naming the same root as the last one is the
\* same block, found with its child, so the root gains nothing; a new one
\* counts its edge to the root, fresh or found.
SaveCounts(E, K) ==
  LET put == PutCounts(E, K, {cur})
      holdAdd == IF ~Ready \/ hold = cur THEN 0
                 ELSE IF HoldRule = "always" \/ cur \in stored THEN 1 ELSE 0
  IN [p \in Pages |-> put[p] + (IF p = cur THEN holdAdd ELSE 0)]

MoveHold == /\ hold' = cur
            /\ rel' = IF ~Ready \/ hold = cur THEN rel ELSE [rel EXCEPT ![hold] = @ + 1]

\* Singleton bindings materialize Net, fallback and saved values once for a
\* raw put observation. A LET operator alone remains lazy in TLC and expands
\* the same graph walk at each split/output field; every original layout and
\* transition remains present with these finite value bindings.
Name ==
  /\ ~Shadow
  /\ (steps # <<>> \/ putSet # {})
  /\ LET chain == IF steps = <<>>
                   THEN <<[from |-> named, to |-> cur, replaced |-> {}, created |-> {}, original |-> {}]>>
                   ELSE steps
         E == KeptReach(cur) \cup {cur}
         K == {p \in E : IsOriginal(chain, p) /\ p # chain[1].from}
     IN \E refills \in IF Shadow THEN SUBSET (E \cap stored) ELSE {{}},
           cached \in IF Shadow THEN SUBSET (E \ stored) ELSE {{}} :
        LET rawFound == (((E \ K) \cap stored) \ refills) \cup ((E \ K) \cap cached)
            P == putSet \cup (E \ K)
         fresh == freshSet \cup ((E \ K) \ rawFound)
         foundAll == P \ fresh
         foundAny == foundSet \cup rawFound
         F == IF FoundRule = "every" THEN foundAll ELSE foundAny
         netted == (funded /\ lineageCurrent) \/ Unfunded = "steps"
     IN \E net \in {IF netted THEN TLCEval(Net(chain, F, P)) ELSE Idle},
           fallback \in {TLCEval(NetPuts(P, fresh))} :
        LET foundHolds(p) == IF CountRule = "hold"
                    THEN foundPuts[p] + (IF p \in rawFound THEN 1 ELSE 0)
                    ELSE 0
         raw == TLCEval([id |-> commits, superseded |-> FALSE, current |-> FALSE,
                 transfer |-> FALSE, parts |-> {}, large |-> FALSE,
                 from |-> IF netted THEN net.from ELSE None,
                 replaced |-> IF netted THEN net.replaced ELSE {},
                 dropped |-> IF netted THEN net.dropped ELSE {},
                 removed |-> IF netted THEN net.removed ELSE {},
                 kept |-> IF netted THEN net.kept ELSE {},
                 existed |-> IF netted THEN net.existed ELSE {},
                 held |-> IF netted THEN net.held ELSE fallback,
                 found |-> [p \in Pages |-> foundHolds(p)]])
     IN \E material \in {[record |-> raw, fallback |-> fallback,
                           saved |-> TLCEval(SaveCounts(E, K))]} :
        \E layout \in IF Shadow THEN Layouts(ListData(material.record, material.fallback))
                      ELSE {[large |-> FALSE, parts |-> {}]} :
        LET trusted == basisCurrent /\ Ready /\ ~layout.large /\ refills = {} /\ cached = {}
            recovery == Shadow /\ (layout.large \/ (counting /\ (doubt \/ cached # {})))
            rec == [material.record EXCEPT !.current = trusted,
                              !.transfer = (trusted /\ material.record.from # None),
                              !.parts = layout.parts, !.large = layout.large]
            held == IF trusted THEN material.record.held ELSE material.fallback
            saved == material.saved
            newParts == layout.parts \ partStore
            partAdd(p) == Cardinality({part \in newParts : p \in PartKids(part) /\ p \in stored \cup E})
            next(p) == saved[p] + (IF Ready THEN held[p] + partAdd(p)
                                      + (IF rec.transfer /\ rec.from = p THEN 1 ELSE 0) ELSE 0)
            pending == Append(queue, rec)
        IN /\ stored' = stored \cup E
           /\ staged' = (staged \cup E) \ (UNION {Kids(p) : p \in E} \cup {cur})
           /\ partStore' = partStore \cup layout.parts
           /\ underflow' = (underflow \/ \E p \in Pages : next(p) < 0)
           /\ extra' = IF recovery THEN Zero ELSE [p \in Pages |-> IF next(p) < 0 THEN 0 ELSE next(p)]
           /\ queue' = IF recovery THEN StaleQueue(pending) ELSE pending
           /\ walk' = IF recovery /\ walk.on
                        THEN [on |-> TRUE, valid |-> FALSE, rec |-> StaleRecord(walk.rec)]
                        ELSE InvalidateRead
           \* Source shadow adds the valid From transfer literally. A save
           \* that reuses its same ordinary hold block retires no such edge;
           \* unmatched transfer surplus remains until recount. The separate
           \* counted-free ideal preserves its old implicit-lineage accounting.
           /\ rel' = IF recovery THEN Zero
                      ELSE [p \in Pages |-> rel[p]
                        + (IF Ready /\ hold # cur /\ p = hold THEN 1 ELSE 0)
                        + (IF ~Shadow /\ Ready /\ lineage /\ p = named /\
                              (cur # named \/ (rec.transfer /\ rec.from \notin rec.existed))
                           THEN 1 ELSE 0)]
           /\ hold' = cur
           /\ counting' = (counting /\ ~recovery)
           /\ doubt' = IF recovery /\ counting THEN FALSE ELSE doubt
           /\ funded' = (trusted /\ (cur \in firstFresh \cup (((E \ K) \ rawFound) \ putSet)))
           /\ lineage' = IF recovery THEN FALSE ELSE (funded' \/ (cur = named /\ lineage))
           /\ lossy' = (lossy \/ ~funded \/ ~trusted \/ recovery)
           /\ lineageCurrent' = (basisCurrent /\ Ready /\ ~recovery)
  /\ named' = cur
  /\ steps' = <<>> /\ kept' = {}
  /\ putSet' = {} /\ freshSet' = {} /\ firstFresh' = {}
  /\ foundPuts' = Zero
  /\ basisCurrent' = TRUE
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, pin, commits, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Source shadow rebuilds only stored World/record child lists and real stage,
\* pin and replay-hold roots. Historical From, Dropped and Found refunds are
\* skipped, including unnamed old receipts. The idealized counted-free branch
\* retains its separate owned-claim reconstruction; it does not describe shadow.
RecountOf(st, h) ==
  LET holders(p) == Cardinality({q \in st : p \in Kids(q)})
                      + (IF p \in staged THEN 1 ELSE 0)
                      + (IF Shadow THEN WrapperHolders(p) ELSE IF p = h THEN 1 ELSE 0)
                      + (IF p = pin THEN 1 ELSE 0)
                      + (IF Shadow THEN PhysicalPartHolders(p)
                         ELSE Cardinality({i \in 1..Len(queue) : queue[i].transfer /\ queue[i].from = p})
                           + Cardinality({i \in 1..Len(queue) : p \in queue[i].dropped})
                           + Sum([i \in 1..Len(queue) |-> queue[i].found[p]], 1..Len(queue)))
  IN [p \in Pages |-> IF p \in st /\ holders(p) > 0 THEN holders(p) - 1 ELSE 0]

Forfeit ==
  /\ ~Shadow
  /\ Forfeits
  /\ steps # <<>>
  /\ LET E == KeptReach(cur) \cup {cur}
         K == {p \in E : IsOriginal(steps, p) /\ p # steps[1].from}
         pending == IF walk.on THEN <<walk.rec>> \o queue ELSE queue
     IN /\ stored' = stored \cup E
        /\ staged' = (staged \cup E) \ (UNION {Kids(p) : p \in E} \cup {cur})
        /\ extra' = IF Recount
                     THEN [p \in Pages |-> 0]
                     ELSE SaveCounts(E, K)
        /\ hold' = cur
        /\ rel' = IF Recount THEN [p \in Pages |-> 0]
                   ELSE IF ~Ready \/ hold = cur THEN rel
                   ELSE [rel EXCEPT ![hold] = @ + 1]
        /\ queue' = IF Recount THEN StaleQueue(pending)
                     ELSE queue
  /\ walk' = IF Recount THEN Idle ELSE InvalidateRead
  /\ counting' = IF Recount THEN FALSE ELSE counting
  /\ doubt' = IF Recount /\ counting THEN FALSE ELSE doubt
  /\ basisCurrent' = TRUE
  /\ lineageCurrent' = FALSE
  /\ funded' = FALSE
  /\ lineage' = ~Recount
  /\ lossy' = TRUE
  /\ named' = cur
  /\ steps' = <<>> /\ kept' = {}
  /\ putSet' = {} /\ freshSet' = {} /\ firstFresh' = {}
  /\ foundPuts' = [p \in Pages |-> 0]
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, pin, commits, underflow, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* ---------------------------------------------------------------------------
\* Apply runs ResolveDeaths on the oldest record, deciding each page of the
\* walk once all its parents in the walk are decided, in any such order, then
\* ChangeHolds and the deferred Found releases, retiring the record atomically.
\* Foreground puts, names and pins may commit between read and write. Any
\* physical commit invalidates the captured revision; only a valid plan may
\* publish changes, otherwise the still-owned record retries off the writer.

\* A dropped page's edges were never counted, so the death walk does not follow
\* them. If the page survives adoption, it funds each child before that child's
\* decision, including another Dropped page with its own root loss.
Children(n, dropped) == IF n \in stored /\ n \notin dropped THEN Kids(n) ELSE {}

\* The walk: the listed pages no listed page names, and every page below them
\* through children that are not live.
WalkFrom(roots, live, dropped) ==
  LET RECURSIVE Down(_)
      Down(n) == {n} \cup UNION {Down(c) : c \in Children(n, dropped) \ live}
  IN UNION {Down(r) : r \in roots}

StartApply ==
  /\ ~walk.on
  /\ (Shadow \/ Ready)
  /\ queue # <<>>
  /\ LET rec == Head(queue)
         precise == rec.current /\ Ready /\ ~rec.large
         listed == IF rec.superseded \/ ~precise THEN {} ELSE rec.replaced \cup rec.removed \cup rec.dropped
         namedL == {c \in listed : \E n \in listed : c \in Children(n, rec.dropped)}
         live == IF ~precise THEN {} ELSE rec.kept \cup {c \in UNION {Children(n, {}) : n \in rec.replaced} :
                                  c \notin listed}
     IN walk' = [on |-> TRUE, valid |-> TRUE, rec |-> rec, listed |-> listed, live |-> live,
                 precise |-> precise, dropped |-> IF precise THEN rec.dropped ELSE {},
                 counted |-> {k \in Pages : rec.held[k] > 0} \cup rec.existed,
                 roots |-> listed \ namedL,
                 nodes |-> WalkFrom(listed \ namedL, live, rec.dropped),
                 done |-> {}, frees |-> {},
                 changes |-> [p \in Pages |-> 0]]
  /\ queue' = Tail(queue)
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A page loses one count for each parent in the walk that frees, and a root
\* of the walk one more; it lives while its count covers them.
\* Independent ready decisions read only their own fixed snapshot count and
\* decided parents. Shared-child integer deltas commute; only EndApply writes
\* the aggregate. Physical changes discard the private plan. One stable ready
\* order therefore represents every topological plan, without removing external
\* interleavings. Mutated rules retain all orders so their witnesses survive.
CanonicalWalk == WalkRule = "parents" /\ HeldRule = "parent" /\ FoundRule = "every"
                 /\ Unfunded = "puts" /\ HoldRule = "always" /\ Recount
                 /\ OriginalRule = "stored" /\ ListRule = "reached"
                 /\ FreshHeld /\ MarkedParent /\ DropRule = "own"
                 /\ CountRule = "hold" /\ StrayRule = "created"

WalkStep ==
  /\ walk.on /\ walk.valid
  /\ LET ready == {n \in walk.nodes \ walk.done :
                      {q \in walk.nodes : n \in Children(q, {})} \subseteq walk.done}
         choices == IF CanonicalWalk
                      THEN IF ready = {} THEN {} ELSE
                           {CHOOSE n \in ready : \A m \in ready : PageRank(n) <= PageRank(m)}
                      ELSE walk.nodes \ walk.done
     IN \E n \in choices :
     LET parents == {q \in walk.nodes : n \in Children(q, {})}
         Decide(killed) ==
           LET counts == killed > 0 /\ extra[n] + walk.changes[n] >= killed
               lives == killed = 0 \/ counts
               kids == Children(n, {})
               delta(c) == IF n \in walk.dropped
                           THEN IF lives THEN 1 ELSE 0
                           ELSE IF c \in walk.live
                           THEN IF lives /\ c \notin walk.counted THEN 1
                                ELSE IF ~lives /\ c \in walk.counted THEN -1
                                ELSE 0
                           ELSE 0
           IN walk' = [walk EXCEPT
                         !.done = @ \cup {n},
                         !.frees = IF lives THEN @ ELSE @ \cup {n},
                         !.changes = [p \in Pages |-> @[p]
                                        - (IF p = n /\ counts THEN killed ELSE 0)
                                        + (IF p \in kids THEN delta(p) ELSE 0)]]
     IN IF WalkRule = "parents"
        THEN /\ parents \subseteq walk.done
             /\ Decide((IF n \in walk.roots THEN 1 ELSE 0)
                         + Cardinality((parents \ walk.dropped) \cap walk.frees))
        ELSE \E q \in (parents \cap walk.done)
                   \cup (IF n \in walk.roots THEN {"root"} ELSE {}) :
               Decide(IF q = "root" \/ (q \in walk.frees /\ q \notin walk.dropped) THEN 1 ELSE 0)
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Current shadow retires durable record/mark ownership before invalidating
\* counts on a would-free parent. The old deltas/returns are not executed:
\* the walk rebuilds from retained bytes and the remaining named claims.
\* A leaf-only decision keeps complete counts because it has no child funding
\* to misidentify when its retained incarnation is found by a later put.
EndApply ==
  /\ walk.on /\ walk.valid
  /\ walk.done = walk.nodes
  /\ LET rec == walk.rec
         recovery == Shadow /\ ((counting /\ doubt) \/
                        (walk.precise /\ \E p \in walk.frees : Kids(p) # {}))
         rootLoss == IF ~rec.superseded /\ walk.precise /\ rec.from \notin walk.listed /\ rec.from \in rec.existed
                     THEN 1 ELSE 0
         changes == [p \in Pages |-> walk.changes[p]
                                     - (IF p = rec.from THEN rootLoss ELSE 0)]
         next(p) == extra[p] + changes[p]
     IN /\ underflow' = (underflow \/ (~recovery /\ \E p \in Pages : next(p) < 0))
        /\ extra' = IF recovery THEN [p \in Pages |-> 0]
                     ELSE [p \in Pages |-> IF next(p) < 0 THEN 0 ELSE next(p)]
        /\ stored' = IF storageMode = "shadow" THEN stored ELSE stored \ walk.frees
        /\ staged' = IF storageMode = "shadow" THEN staged ELSE staged \ walk.frees
        /\ rel' = IF recovery THEN Zero
                   ELSE [p \in Pages |-> rel[p]
                          + (IF Ready /\ (walk.precise \/ (~Shadow /\ rec.superseded)) THEN rec.found[p] ELSE 0)
                          + (IF Ready /\ ~Shadow /\ rec.superseded /\ rec.transfer /\ rec.from = p THEN 1 ELSE 0)
                          + (IF Ready /\ ~Shadow /\ rec.superseded /\ p \in rec.dropped THEN 1 ELSE 0)]
        /\ retired' = IF Shadow
                       THEN retired \cup {[id |-> rec.id, parts |-> rec.parts, released |-> FALSE]} ELSE retired
        /\ doubt' = IF recovery THEN FALSE ELSE doubt
        /\ queue' = IF recovery
                     THEN StaleQueue(queue) ELSE queue
        /\ counting' = (counting /\ ~recovery)
        /\ basisCurrent' = IF recovery THEN putSet = {} ELSE basisCurrent
        /\ lineageCurrent' = IF recovery THEN FALSE ELSE lineageCurrent
        /\ lineage' = IF recovery THEN FALSE ELSE lineage
        /\ lossy' = (lossy \/ recovery)
  /\ walk' = Idle
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, pin, hold, commits, partStore>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* ---------------------------------------------------------------------------
\* Releases, the reader pin.

\* Resolve consumes a node's whole release count as the source does, exposing
\* excess returns even when shadow retains its leaf bytes. Underflow is sticky
\* even when the same transaction starts recovery.
\* The source stops its budgeted slice immediately after the first parent
\* free, with children queued, then its caller starts recount in that same
\* physical transaction. No later release can hide an underflow before counts
\* become incomplete. Other nonfree/leaf nodes in that slice compose these
\* actions; their count changes and sticky diagnostics are preserved.
Resolve(p) ==
  /\ ~walk.on /\ Ready
  /\ rel[p] > 0
  /\ LET frees == rel[p] > extra[p]
         recovery == storageMode = "shadow" /\ frees /\ Kids(p) # {}
     IN /\ extra' = IF recovery THEN [q \in Pages |-> 0]
                     ELSE [extra EXCEPT ![p] = IF frees THEN 0 ELSE @ - rel[p]]
        /\ rel' = IF recovery THEN [q \in Pages |-> 0]
                   ELSE [q \in Pages |-> (IF q = p THEN 0 ELSE rel[q])
                                    + (IF frees /\ q \in Children(p, {}) THEN 1 ELSE 0)]
        /\ stored' = IF frees /\ storageMode = "counted-free" THEN stored \ {p} ELSE stored
        /\ staged' = IF frees /\ storageMode = "counted-free" THEN staged \ {p} ELSE staged
        /\ queue' = IF recovery
                     THEN StaleQueue(queue) ELSE queue
        /\ counting' = ~recovery
        /\ basisCurrent' = IF recovery THEN putSet = {} ELSE basisCurrent
        /\ lineageCurrent' = IF recovery THEN FALSE ELSE lineageCurrent
        /\ lineage' = IF recovery THEN FALSE ELSE lineage
        /\ lossy' = (lossy \/ recovery)
        /\ underflow' = (underflow \/ rel[p] > extra[p] + 1)
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, walk, pin, hold, commits, partStore, retired, doubt>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A large subtree can exceed the work budget even with a small list payload.
\* Missing small-list bytes also take this same recovery path. This abstract
\* choice preserves all exact small paths instead of shrinking their bound.
AbortApply ==
  /\ Shadow /\ walk.on /\ walk.valid
  /\ retired' = IF Shadow
                 THEN retired \cup {[id |-> walk.rec.id, parts |-> walk.rec.parts, released |-> FALSE]} ELSE retired
  /\ queue' = StaleQueue(queue) /\ walk' = Idle
  /\ counting' = FALSE
  /\ doubt' = IF counting THEN FALSE ELSE doubt
  /\ extra' = Zero /\ rel' = Zero
  /\ basisCurrent' = (putSet = {})
  /\ lineageCurrent' = FALSE /\ lineage' = FALSE /\ lossy' = TRUE
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, hold, pin, commits, underflow, staged, partStore>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Returning the due name and erasing its physical header are different
\* events. An empty header is a leaf: its owner return starts no recount, but
\* later physical count-metadata erasure still advances generation/rebuilds.
ResolveRecord ==
  /\ Shadow /\ Ready /\ ~walk.on
  /\ \E header \in {h \in retired : ~h.released} :
     LET recovery == header.parts # {} /\ header.id \notin headerAliases
     IN /\ retired' = (retired \ {header}) \cup {[header EXCEPT !.released = TRUE]}
        /\ queue' = IF recovery THEN StaleQueue(queue) ELSE queue
        /\ extra' = IF recovery THEN Zero ELSE extra
        /\ rel' = IF recovery THEN Zero ELSE rel
        /\ counting' = ~recovery
        /\ doubt' = IF recovery THEN FALSE ELSE doubt
        /\ basisCurrent' = IF recovery THEN putSet = {} ELSE basisCurrent
        /\ lineageCurrent' = (lineageCurrent /\ ~recovery)
        /\ lineage' = (lineage /\ ~recovery)
        /\ lossy' = (lossy \/ recovery)
        /\ sighted' = IF recovery THEN FALSE ELSE sighted
        /\ tainted' = IF recovery THEN FALSE ELSE tainted
        /\ waiting' = IF recovery THEN StaleWaiting(waiting) ELSE waiting
        /\ firstCurrent' = IF recovery THEN {} ELSE firstCurrent
  /\ UNCHANGED <<save, receiptSeen, aliases, wrapper, wrappers, wrapperReleases,
       base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch,
       storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, walk, pin, hold, commits, underflow, staged, partStore>>
  /\ UNCHANGED headerAliases

Pin ==
  /\ Pins /\ pin = None /\ commits < MaxCommits
  /\ \E root \in Reach(named) :
     /\ pin' = root
     /\ extra' = IF counting /\ doubt THEN Zero
                  ELSE IF Ready THEN [extra EXCEPT ![root] = @ + 1] ELSE extra
  /\ rel' = IF counting /\ doubt THEN Zero ELSE rel
  /\ queue' = IF counting /\ doubt THEN StaleQueue(queue) ELSE queue
  /\ counting' = (counting /\ ~doubt)
  /\ doubt' = IF counting /\ doubt THEN FALSE ELSE doubt
  /\ basisCurrent' = IF counting /\ doubt THEN putSet = {} ELSE basisCurrent
  /\ lineageCurrent' = (lineageCurrent /\ ~(counting /\ doubt))
  /\ lineage' = IF counting /\ doubt THEN FALSE ELSE lineage
  /\ lossy' = (lossy \/ (counting /\ doubt))
  /\ commits' = commits + 1
  /\ walk' = IF counting /\ doubt THEN StaleRead ELSE InvalidateRead
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, hold, underflow, staged, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

Unpin ==
  /\ pin # None
  /\ pin' = None
  /\ extra' = IF counting /\ doubt THEN Zero ELSE extra
  /\ rel' = IF counting /\ doubt THEN Zero
             ELSE IF Ready THEN [rel EXCEPT ![pin] = @ + 1] ELSE rel
  /\ queue' = IF counting /\ doubt THEN StaleQueue(queue) ELSE queue
  /\ counting' = (counting /\ ~doubt)
  /\ doubt' = IF counting /\ doubt THEN FALSE ELSE doubt
  /\ basisCurrent' = IF counting /\ doubt THEN putSet = {} ELSE basisCurrent
  /\ lineageCurrent' = (lineageCurrent /\ ~(counting /\ doubt))
  /\ lineage' = IF counting /\ doubt THEN FALSE ELSE lineage
  /\ lossy' = (lossy \/ (counting /\ doubt))
  /\ walk' = IF counting /\ doubt THEN StaleRead ELSE InvalidateRead
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, hold, commits, underflow, staged, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A global rebuild invalidates receipts and a pending read's certificate.
\* Equality flags abstract the persisted generation: another rebuild repeats
\* this transition without an unbounded integer or a limit on recounts.
\* While incomplete, historical Apply decisions and Resolve are disabled.
\* Shadow may still retire an invalid record, without consuming old counts.
\* Finish replaces both counters from the preserved graph and real holders.
\* Zeroing incomplete counters quotients unobservable scratch values;
\* underflow, receipts, stored data, staging and record effects are retained.
BeginRecount ==
  /\ counting
  /\ LET pending == IF walk.on THEN <<walk.rec>> \o queue ELSE queue
     IN queue' = StaleQueue(pending)
  /\ extra' = [p \in Pages |-> 0]
  /\ rel' = [p \in Pages |-> 0]
  /\ counting' = FALSE
  /\ doubt' = FALSE
  /\ basisCurrent' = (putSet = {})
  /\ lineageCurrent' = FALSE
  /\ lineage' = FALSE /\ lossy' = TRUE
  /\ walk' = Idle
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ TRUE THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ TRUE THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, pin, hold, commits, underflow, staged, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Only real roots and ordinary record child edges protect bytes. Working
\* World's stored frontier is protected by its existing page-retainer roots.
StoredFrontier == Reach(cur) \ kept
MarkLive == UserLive \cup StoredFrontier
             \cup UNION {Reach(r) : r \in staged \cup PartRoots}
UnknownLive == MarkLive \ stored
PhysicalHoles == {p \in stored : Kids(p) \ stored # {}}
                  \cup UNION {PartKids(part) \ stored : part \in partStore}
                  \cup UNION {HoldKids(h) \ stored : h \in wrappers}

\* Completion sums the preserved physical graph. Source shadow has no record
\* owner callback; its stale records retain physical part identity alone. The
\* idealized counted-free branch reconstructs its explicitly separate claims.
\* Writes during the walk remain allowed and enter this completion sum. Sticky
\* Doubt prevents a lost sighting prefix from becoming trusted; a later known
\* mark requests a fresh walk. SharingWalk checks the individual slices.
FinishRecount ==
  /\ ~counting /\ sighted
  /\ extra' = RecountOf(stored, hold)
  /\ rel' = [p \in Pages |-> 0]
  /\ queue' = IF Shadow THEN StaleQueue(queue)
               ELSE [i \in 1..Len(queue) |-> [queue[i] EXCEPT !.superseded = TRUE]]
  /\ counting' = TRUE
  /\ doubt' = (doubt \/ (Shadow /\ PhysicalHoles # {}))
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, walk, pin, hold, commits, lossy, underflow, basisCurrent, staged, lineageCurrent, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A rejected plan changes no data, deltas, release queue or record owner.
\* The source retries once off the writer, then retains it for the next pass;
\* repeated passes are finite equality states here, not a retry-count bound.
RetryApply ==
  /\ walk.on /\ ~walk.valid
  /\ queue' = <<walk.rec>> \o queue
  /\ walk' = Idle
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Unrelated metadata or incarnation writes also advance the physical revision.
OtherWrite ==
  /\ walk.on /\ walk.valid
  /\ walk' = InvalidateRead
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Releasing a construction/candidate stage is an ordinary owner operation.
\* Other real roots must retain the working World's stored dependencies.
ReleaseStage ==
  /\ Shadow
  /\ \E root \in staged :
     LET other == UserLive \cup UNION {Reach(r) : r \in (staged \ {root}) \cup PartRoots}
     IN /\ StoredFrontier \subseteq other
        /\ staged' = staged \ {root}
        /\ extra' = IF counting /\ doubt THEN Zero ELSE extra
        /\ rel' = IF counting /\ doubt THEN Zero
                   ELSE IF Ready THEN [rel EXCEPT ![root] = @ + 1] ELSE rel
  /\ queue' = IF counting /\ doubt THEN StaleQueue(queue) ELSE queue
  /\ walk' = IF counting /\ doubt THEN StaleRead ELSE InvalidateRead
  /\ counting' = (counting /\ ~doubt)
  /\ doubt' = IF counting /\ doubt THEN FALSE ELSE doubt
  /\ basisCurrent' = IF counting /\ doubt THEN putSet = {} ELSE basisCurrent
  /\ lineageCurrent' = (lineageCurrent /\ ~(counting /\ doubt))
  /\ lineage' = IF counting /\ doubt THEN FALSE ELSE lineage
  /\ lossy' = (lossy \/ (counting /\ doubt))
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, hold, pin, commits, underflow, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A missing reached list is Doubt even if the block's bytes are absent.
\* This transition exposes readiness loss; it never invents a child list.
ReadHole ==
  /\ Shadow /\ ~doubt
  /\ (UnknownLive # {} \/ (~counting /\ PhysicalHoles # {}))
  /\ doubt' = TRUE
  /\ walk' = InvalidateRead
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, sighted, tainted>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Byte-only cache eviction is not metadata erasure and has no transition.
\* An ordinary sweep may erase just a child while its unmarked parent remains.
\* Every actual c/h/l erasure advances provenance and starts the existing walk
\* when completed. An already-running walk retains phase/cursor and counters.
Erase ==
  /\ Shadow /\ UnknownLive = {}
  /\ \E gone \in SUBSET (stored \ MarkLive),
        leaves \in SUBSET (partStore \ PendingParts), headers \in SUBSET {h \in retired : h.id \notin headerAliases}, oldHolds \in SUBSET (wrappers \ ({wrapper} \cup aliases)) :
     /\ (gone # {} \/ leaves # {} \/ headers # {} \/ oldHolds # {})
     /\ wrappers' = wrappers \ oldHolds
     /\ wrapperReleases' = wrapperReleases \ oldHolds
     /\ stored' = stored \ gone
     /\ partStore' = partStore \ leaves
     /\ retired' = retired \ headers
     /\ staged' = staged \ gone
     /\ extra' = IF counting THEN Zero
                  ELSE [p \in Pages |-> IF p \in gone THEN 0 ELSE extra[p]]
     /\ rel' = IF counting THEN Zero
                ELSE [p \in Pages |-> IF p \in gone THEN 0 ELSE rel[p]]
  /\ queue' = StaleQueue(queue)
  /\ walk' = StaleRead
  /\ sighted' = IF counting THEN FALSE ELSE sighted
  /\ tainted' = IF counting THEN FALSE ELSE (tainted \/ sighted)
  /\ counting' = FALSE
  /\ doubt' = IF counting THEN FALSE ELSE (doubt \/ sighted)
  /\ basisCurrent' = (putSet = {})
  /\ lineageCurrent' = FALSE /\ lineage' = FALSE /\ lossy' = TRUE
  /\ waiting' = IF Shadow /\ TRUE THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ TRUE THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, hold, pin, commits, underflow>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A complete ordinary mark with no unknown roots can refresh completed
\* Doubt. It does not restart a walk already in progress.
RefreshKnown ==
  /\ Shadow /\ counting /\ doubt /\ UnknownLive = {}
  /\ counting' = FALSE /\ doubt' = FALSE
  /\ extra' = Zero /\ rel' = Zero
  /\ queue' = StaleQueue(queue) /\ walk' = StaleRead
  /\ basisCurrent' = (putSet = {})
  /\ lineageCurrent' = FALSE /\ lineage' = FALSE /\ lossy' = TRUE
  /\ sighted' = IF counting /\ ~counting' THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ ~counting' THEN FALSE ELSE tainted
  /\ waiting' = IF Shadow /\ (counting /\ ~counting') THEN StaleWaiting(waiting) ELSE waiting
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, curEpoch, checkpointEpoch>>
  /\ firstCurrent' = IF Shadow /\ (counting /\ ~counting') THEN {} ELSE firstCurrent
  /\ UNCHANGED <<storageMode, cur, named, stored, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, hold, pin, commits, underflow, staged, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Clearing precedes sighting. The finite prefix flag preserves the active
\* cursor distinction needed when c/h/l metadata is erased during a walk.
\* Crossing that clearing prefix supersedes old wrapper owner returns. New
\* returns written after it remain through completion, as SharingWalk checks.
Sight ==
  /\ ~counting /\ ~sighted
  /\ sighted' = TRUE
  /\ wrapperReleases' = {}
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch>>
  /\ UNCHANGED <<firstCurrent, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, walk, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, tainted>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Cache eviction preserves the accounted incarnation and its generation.
\* A later raw-new body refill uses existing positive accounting but returns
\* an uncertified receipt, never a fresh counted-lineage certificate.

\* ---------------------------------------------------------------------------
\* Literal source-shadow replay-base wrappers and checkpoint/record boundary.
\* Each foreground position supplies one checkpoint generation. Head-only hold
\* saves do not create a new checkpoint or a free-running record sequence.
\* Ordered wrapper identities survive root retirement until actual metadata
\* erasure; a fresh wrapper funds each stored World child once.
WrapperEntries(h) == UNION {KeptReach(r) : r \in HoldKids(h)}
Gather(h) == [on |-> TRUE, roots |-> h,
              entries |-> WrapperEntries(h) \cup (HoldKids(h) \cap stored),
              kept |-> WrapperEntries(h)]
WrapperAvailable(h, E) == \A r \in HoldKids(h) : Reach(r) \subseteq stored \cup E
WrapperCounts(h, E, N, K) == ClassCounts(E, N, K, h)
QueueWrapper(h) == IF h = wrapper THEN wrapperReleases
                   ELSE wrapperReleases \cup {wrapper}

\* These are publication effects shared by the source-shadow events. Counts
\* become untrusted in the same transaction as a recovery request. Existing
\* walks retain their prefix and sticky Doubt; only a newly started walk clears.
ShadowPublication(recovery, counts, returns, ws, q, lin, current, loss, certs) ==
  /\ firstCurrent' = IF recovery THEN {} ELSE certs
  /\ extra' = IF recovery THEN Zero ELSE counts
  /\ rel' = IF recovery THEN Zero ELSE returns
  /\ queue' = IF recovery THEN StaleQueue(q) ELSE q
  /\ waiting' = IF recovery THEN StaleWaiting(ws) ELSE ws
  /\ walk' = IF recovery THEN StaleRead ELSE InvalidateRead
  /\ counting' = (counting /\ ~recovery)
  /\ doubt' = IF recovery /\ counting THEN FALSE ELSE doubt
  /\ lineage' = IF recovery THEN FALSE ELSE lin
  /\ lineageCurrent' = IF recovery THEN FALSE ELSE current
  /\ lossy' = (lossy \/ recovery \/ loss)
  /\ sighted' = IF counting /\ recovery THEN FALSE ELSE sighted
  /\ tainted' = IF counting /\ recovery THEN FALSE ELSE tainted

\* A replay position becomes written once; advancing to a later position also
\* consumes all earlier ones. This changes the next hold's logical input, not
\* the physical graph until its save lands.
WrittenAdvance ==
  /\ Shadow /\ unwritten # <<>>
  /\ LET latest == unwritten[Len(unwritten)]
         h == RootOrder(base, latest.root, pendingWorlds, waiting)
     IN /\ written' = latest.root /\ writtenEpoch' = latest.epoch
        /\ unwritten' = <<>> /\ save' = Gather(h)
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, base, pendingWorlds, waiting,
       curEpoch, checkpointEpoch, storageMode, cur, named, stored, extra, rel,
       kept, steps, putSet, freshSet, firstFresh, foundPuts, funded,
       lineage, queue, walk, pin, hold, commits, lossy, underflow, counting,
       basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED <<receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A saved checkpoint may advance beyond the old written frontier. Adoption
\* consumes the covered position/checkpoint prefixes once; written zero in
\* the source is represented by written=base, which deduplicates in RootOrder.
\* Later already-written positions remain the separate written root.
BaseAdopt ==
  /\ Shadow /\ pendingWorlds # <<>>
  /\ \E i \in 1..Len(pendingWorlds) :
     /\ base' = pendingWorlds[i].root
     /\ written' = IF writtenEpoch <= pendingWorlds[i].epoch
                       THEN pendingWorlds[i].root ELSE written
     /\ writtenEpoch' = IF writtenEpoch <= pendingWorlds[i].epoch
                            THEN pendingWorlds[i].epoch ELSE writtenEpoch
     /\ unwritten' = SubSeq(unwritten,
           Cardinality({j \in 1..Len(unwritten) : unwritten[j].epoch <= pendingWorlds[i].epoch}) + 1,
           Len(unwritten))
     /\ pendingWorlds' = SubSeq(pendingWorlds, i + 1, Len(pendingWorlds))
     /\ save' = Gather(RootOrder(base', written', pendingWorlds', waiting))
  /\ UNCHANGED <<firstCurrent, wrapper, wrappers, wrapperReleases, waiting, curEpoch, checkpointEpoch, storageMode, cur, named,
       stored, extra, rel, kept, steps, putSet, freshSet, firstFresh,
       foundPuts, funded, lineage, queue, walk, pin, hold, commits, lossy,
       underflow, counting, basisCurrent, staged, lineageCurrent, partStore,
       retired, doubt, sighted, tainted>>
  /\ UNCHANGED <<receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* A hold-only save submits its immutable gathered entries. The store may
\* have completed another receipt since gathering, changing the exact kept
\* pointers selected by pages.split; each possible matching subset remains.
\* Ordinary root copies are physical entries but never synthetic World puts.
\* A zero-selected-page engine save adds no interval receipt; checkpoint's
\* own naming publication below deliberately has a different empty case.
HoldLand ==
  /\ Shadow /\ save.on
  /\ LET h == save.roots
         E == save.entries
     IN /\ WrapperAvailable(h, E)
        /\ \E N \in SUBSET (save.kept \cap kept) :
           \E refills \in SUBSET (E \cap stored), cached \in SUBSET (E \ stored),
                 certified \in BOOLEAN :
           LET recovery == counting /\ (doubt \/ cached # {})
           IN /\ stored' = stored \cup E /\ kept' = kept \ N
              /\ SelectedInputs(E, N, {}, refills, cached, HoldKids(h), N # {})
              /\ basisCurrent' = IF recovery THEN putSet \cup N = {}
                   ELSE basisCurrent /\ (N = {} \/ (Ready /\ certified /\ refills = {} /\ cached = {}))
              /\ ShadowPublication(recovery, ClassCounts(E, N, {}, h), rel,
                     waiting, queue, lineage, lineageCurrent, FALSE,
                     FirstCertificate(N, {}, refills, cached, certified, recovery))
        /\ wrapper' = h /\ wrappers' = wrappers \cup {h}
        /\ wrapperReleases' = QueueWrapper(h) /\ save' = Idle
  /\ UNCHANGED <<base, written, writtenEpoch, unwritten, pendingWorlds,
       curEpoch, checkpointEpoch, storageMode, cur, named, steps,
       funded, pin, hold, commits, underflow, partStore, retired>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* An encoder can retain historical Net data from uniform old receipts even
\* after a foreign generation change: it does not read the current Volume
\* epoch. UNKNOWN/mixed inputs instead select NetPuts. The current-equality
\* flag cannot distinguish those old cases, so both payload modes remain when
\* it is false; naming still skips all old history. No current-only encoding
\* guard discards legitimate old physical leaf identities. A new naming
\* publication is different: its receipt is current or UNKNOWN. Mixing it with
\* a noncurrent interval basis, or observing an UNKNOWN naming input, makes
\* countingBasis incomplete and encode drops every historical step. This does
\* not alter the separate first-root birth certificate used by its successor.
\* This model starts with nonempty From R0. The source bootstrap's empty
\* From bypasses counted; this rule does not describe that bootstrap case.
\* certified also represents missing receipt metadata or ownership edits in
\* the naming transaction that invalidate its final basis; raw entry existence
\* alone cannot infer this field. An early name has no new receipt to observe.
SealCheckpoint ==
  /\ Shadow /\ checkpointEpoch < curEpoch
  /\ (steps # <<>> \/ putSet # {})
  /\ \E forfeit \in IF Forfeits THEN BOOLEAN ELSE {FALSE} :
     LET chain == IF steps = <<>>
                   THEN <<[from |-> named, to |-> cur, replaced |-> {}, created |-> {}, original |-> {}]>>
                   ELSE steps
         early == ~save.on /\ cur \in HoldKids(wrapper)
         checkpoints == IF early THEN pendingWorlds
                        ELSE Append(pendingWorlds, [epoch |-> curEpoch, root |-> cur])
         h == IF early THEN wrapper ELSE RootOrder(base, written, checkpoints,
                IF forfeit THEN waiting ELSE Append(waiting, [from |-> named]))
     IN \E checkpointRoots \in {TLCEval(h)} :
        \E checkpointInput \in {TLCEval(IF early
              THEN [entries |-> {}, kept |-> {}] ELSE Gather(checkpointRoots))} :
        LET E == checkpointInput.entries
            K == {p \in E : IsOriginal(chain, p) /\ p # chain[1].from}
        IN /\ WrapperAvailable(checkpointRoots, E)
           /\ \E N \in IF early THEN {{}} ELSE SUBSET (checkpointInput.kept \cap kept) :
                 \* Receipt and layout choices leave this physical count map unchanged.
                 \E checkpointCounts \in {TLCEval(ClassCounts(E, N, K, checkpointRoots))} :
                 \E certified \in IF early THEN {TRUE} ELSE BOOLEAN :
                 \E refills \in IF Shadow THEN SUBSET (E \cap stored) ELSE {{}},
                    cached \in IF Shadow THEN SUBSET (E \ stored) ELSE {{}} :
                 LET rawFound == (((N \ K) \cap stored) \ refills) \cup ((N \ K) \cap cached)
                     P == putSet \cup (N \ K)
                  fresh == freshSet \cup ((N \ K) \ rawFound)
                  foundAll == P \ fresh
                  foundAny == foundSet \cup rawFound
                  F == IF FoundRule = "every" THEN foundAll ELSE foundAny
                  modes == IF Unfunded = "steps" THEN {TRUE}
                           ELSE IF ~early /\ (~basisCurrent \/ ~Ready \/ ~certified
                                     \/ refills # {} \/ cached # {}) THEN {FALSE}
                           ELSE IF early /\ ~receiptSeen THEN {FALSE}
                           ELSE IF ~basisCurrent THEN BOOLEAN
                           ELSE {funded /\ lineageCurrent}
              IN \E netted \in modes :
                 \E net \in {IF netted THEN TLCEval(Net(chain, F, P)) ELSE Idle},
                    fallback \in {TLCEval(NetPuts(P, fresh))} :
                 LET foundHolds(p) == IF CountRule = "hold"
                             THEN foundPuts[p] + (IF p \in rawFound THEN 1 ELSE 0)
                             ELSE 0
                  raw == TLCEval([id |-> curEpoch, superseded |-> FALSE, current |-> FALSE,
                          transfer |-> FALSE, parts |-> {}, large |-> FALSE,
                          from |-> IF netted THEN net.from ELSE None,
                          replaced |-> IF netted THEN net.replaced ELSE {},
                          dropped |-> IF netted THEN net.dropped ELSE {},
                          removed |-> IF netted THEN net.removed ELSE {},
                          kept |-> IF netted THEN net.kept ELSE {},
                          existed |-> IF netted THEN net.existed ELSE {},
                          held |-> IF netted THEN net.held ELSE fallback,
                          found |-> [p \in Pages |-> foundHolds(p)]])
              IN \E material \in {[record |-> raw, fallback |-> fallback,
                                    saved |-> checkpointCounts]} :
                 \E layout \in IF Shadow THEN Layouts(ListData(material.record, material.fallback))
                               ELSE {[large |-> FALSE, parts |-> {}]} :
                 LET current == (receiptSeen \/ ~early) /\ basisCurrent /\ Ready
                                  /\ (early \/ certified) /\ refills = {} /\ cached = {}
                     recovery == (forfeit /\ Recount) \/ (counting /\ (doubt \/ cached # {}))
                     rec == [material.record EXCEPT !.current = current,
                              !.parts = layout.parts, !.large = layout.large]
                     entry == [id |-> curEpoch, from |-> named, to |-> cur,
                               record |-> rec, fallback |-> material.fallback, prepared |-> {}]
                     freshRoot == ~forfeit /\ (cur \in firstCurrent \/
                       (Ready /\ certified /\ refills = {} /\ cached = {} /\ cur \in (((N \ K) \ rawFound) \ putSet)))
                 IN /\ stored' = stored \cup E /\ kept' = kept \ N
                    /\ staged' = (staged \cup E) \ (UNION {Kids(p) : p \in E} \cup HoldKids(checkpointRoots))
                    /\ wrapper' = checkpointRoots /\ wrappers' = wrappers \cup {checkpointRoots}
                    /\ wrapperReleases' = QueueWrapper(checkpointRoots)
                    /\ pendingWorlds' = checkpoints
                    /\ ShadowPublication(recovery, material.saved, rel,
                          IF forfeit THEN waiting ELSE Append(waiting, entry), queue,
                          ~forfeit /\ (freshRoot \/ (cur = named /\ lineage)),
                          freshRoot, forfeit \/ ~funded \/ ~current, {})
                    /\ funded' = freshRoot
  /\ save' = Idle /\ receiptSeen' = FALSE
  /\ named' = cur /\ hold' = cur /\ checkpointEpoch' = curEpoch
  /\ steps' = <<>>
  /\ putSet' = {} /\ freshSet' = {} /\ firstFresh' = {}
  /\ foundPuts' = Zero /\ basisCurrent' = TRUE
  /\ UNCHANGED <<base, written, writtenEpoch, unwritten, curEpoch,
       storageMode, cur, pin, commits, underflow, partStore, retired>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* One prepared physical leaf is an ordinary stage-owned parent of its
\* distinct Found children. Other fates, publications and recounts may run
\* between leaves and between preparation and carrying. Reuse never funds
\* those children again. Index/header preparation has no World child edges;
\* the independent preparation model checks its ownership/lease sequence.
PrepareRecord ==
  /\ Shadow
  /\ \E i \in 1..Len(waiting) :
     \E part \in waiting[i].record.parts \ waiting[i].prepared :
     \E cached \in IF part \notin partStore THEN BOOLEAN ELSE {FALSE} :
     LET recovery == counting /\ (doubt \/ cached)
         counts == [p \in Pages |-> extra[p]
           + (IF Ready /\ part \notin partStore /\ p \in PartKids(part) /\ p \in stored THEN 1 ELSE 0)]
         ws == [waiting EXCEPT ![i].prepared = @ \cup {part}]
     IN /\ partStore' = partStore \cup {part}
        /\ basisCurrent' = IF recovery THEN putSet = {} ELSE basisCurrent
        /\ ShadowPublication(recovery, counts, rel, ws, queue,
                             lineage, lineageCurrent, FALSE, firstCurrent)
  /\ UNCHANGED <<wrapper, wrappers, wrapperReleases, base, written, writtenEpoch,
       unwritten, pendingWorlds, curEpoch, checkpointEpoch, storageMode, cur,
       named, stored, kept, steps, putSet, freshSet, firstFresh,
       foundPuts, funded, pin, hold, commits, underflow, staged, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Durable due names are ordered by their stored sequence, even when encoded
\* records become ready out of order. Physical leaf multiplicity is separate
\* from the record's per-successful-put Found refund counts.
InsertRecord(q, rec) ==
  LET before == Cardinality({i \in 1..Len(q) : q[i].id < rec.id})
  IN SubSeq(q, 1, before) \o <<rec>> \o SubSeq(q, before + 1, Len(q))

CarryRecord ==
  /\ Shadow
  /\ \E i \in 1..Len(waiting) :
     /\ waiting[i].prepared = waiting[i].record.parts
     /\ LET entry == waiting[i]
            ws == RemoveAt(waiting, i)
            h == IF save.on THEN save.roots ELSE wrapper
            E == IF save.on THEN save.entries ELSE {}
            selected == IF save.on THEN save.kept \cap kept ELSE {}
        IN /\ WrapperAvailable(h, E)
           /\ \E N \in SUBSET selected :
              \E refills \in SUBSET (E \cap stored), cached \in SUBSET (E \ stored),
                    certified \in BOOLEAN :
              LET recovery == entry.record.large \/ (counting /\ (doubt \/ cached # {}))
                  trusted == entry.record.current /\ Ready /\ ~recovery
                  rec == [entry.record EXCEPT !.current = trusted,
                           !.transfer = (trusted /\ entry.record.from # None)]
                  held == IF trusted THEN rec.held ELSE entry.fallback
              IN \E putCounts \in {TLCEval(ClassCounts(E, N, {}, h))} :
                 LET counts == [p \in Pages |-> putCounts[p]
                       + (IF Ready /\ ~entry.record.large THEN held[p]
                          + (IF rec.transfer /\ rec.from = p THEN 1 ELSE 0) ELSE 0)]
                 IN /\ stored' = stored \cup E /\ kept' = kept \ N
                    /\ SelectedInputs(E, N, {}, refills, cached, HoldKids(h), N # {})
                    /\ wrapper' = h /\ wrappers' = wrappers \cup {h}
                    /\ wrapperReleases' = QueueWrapper(h) /\ save' = Idle
                    /\ basisCurrent' = IF recovery THEN putSet \cup N = {}
                        ELSE basisCurrent /\ (N = {} \/
                            (Ready /\ certified /\ refills = {} /\ cached = {}))
                    /\ ShadowPublication(recovery, counts, rel, ws,
                         InsertRecord(queue, rec), lineage, lineageCurrent, FALSE,
                         FirstCertificate(N, {}, refills, cached, certified, recovery))
  /\ UNCHANGED <<base, written, writtenEpoch, unwritten, pendingWorlds,
       curEpoch, checkpointEpoch, storageMode, cur, named, steps, funded,
       pin, hold, commits, underflow, partStore, retired>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Preparation failure forfeits the record through the existing recovery
\* path. Abandoned stage-owned leaves keep their physical child edges until
\* normal cleanup/erase; no Found, From or historical death is refunded here.
FailPreparation ==
  /\ Shadow /\ Forfeits /\ waiting # <<>>
  /\ \E i \in 1..Len(waiting) :
     /\ basisCurrent' = IF Recount THEN putSet = {} ELSE basisCurrent
     /\ ShadowPublication(Recount, extra, rel, RemoveAt(waiting, i),
                           queue, FALSE, FALSE, TRUE, firstCurrent)
  /\ funded' = FALSE
  /\ UNCHANGED <<save, receiptSeen, wrapper, wrappers, wrapperReleases,
       base, written, writtenEpoch, unwritten, pendingWorlds,
       curEpoch, checkpointEpoch, storageMode, cur, named, steps,
       pin, hold, commits, underflow, partStore, retired, stored, kept,
       staged, putSet, freshSet, firstFresh, foundPuts>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* Each original wrapper owner return is consumed once. Reuse by the active
\* name or another ordinary alias keeps the same physical parent alive. Its final free starts shadow
\* recovery before returning any World child, leaving the physical edge lists
\* for the ordinary mark/erase path rather than inventing direct From refunds.
ResolveWrapper ==
  /\ Shadow /\ Ready /\ wrapperReleases # {}
  /\ \E h \in wrapperReleases :
     LET recovery == h # wrapper /\ h \notin aliases
     IN /\ wrapperReleases' = wrapperReleases \ {h}
        /\ basisCurrent' = IF recovery THEN putSet = {} ELSE basisCurrent
        /\ ShadowPublication(recovery, extra, rel, waiting, queue,
                             lineage, lineageCurrent, FALSE, firstCurrent)
  /\ UNCHANGED <<wrapper, wrappers, base, written, writtenEpoch, unwritten,
       pendingWorlds, curEpoch, checkpointEpoch, storageMode, cur, named,
       stored, kept, steps, putSet, freshSet, firstFresh, foundPuts,
       funded, pin, hold, commits, underflow, staged, partStore, retired>>
  /\ UNCHANGED <<save, receiptSeen>>
  /\ UNCHANGED aliases
  /\ UNCHANGED headerAliases

\* An independent ordinary owner can alias the same content-addressed
\* wrapper. It adds an owner of that block, not another copy of its child list.
\* Monotone acquisition is finite because the modeled physical wrappers are
\* created only by bounded role events; no unrelated publication budget exists.
AliasWrapper ==
  /\ Shadow
  /\ \E h \in wrappers \ aliases : aliases' = aliases \cup {h}
  /\ walk' = InvalidateRead
  /\ UNCHANGED <<save, receiptSeen, firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED headerAliases

\* Receipt completion can drop already-stored kept entries after gathering.
\* A captured immutable entry remains in save; publication observes which of
\* its kept pointers still match. Each drop removes at least one finite page.
DropStoredPages ==
  /\ Shadow
  /\ \E dropped \in SUBSET (kept \cap stored) \ {{}} : kept' = kept \ dropped
  /\ UNCHANGED <<aliases, save, receiptSeen, firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch, storageMode, cur, named, stored, extra, rel, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, walk, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>
  /\ UNCHANGED headerAliases

\* Other buckets can retain an identical physical record header. Such a
\* root makes normal metadata cleanup inapplicable, not perpetually pending:
\* Quiet still requires coverage with the live alias. No new World child edge
\* is funded by taking another owner of the already-stored header itself.
AliasHeader ==
  /\ Shadow
  /\ \E id \in ({queue[i].id : i \in 1..Len(queue)}
                 \cup (IF walk.on THEN {walk.rec.id} ELSE {})
                 \cup {h.id : h \in retired}) \ headerAliases :
       headerAliases' = headerAliases \cup {id}
  /\ walk' = InvalidateRead
  /\ UNCHANGED <<aliases, save, receiptSeen, firstCurrent, wrapper, wrappers, wrapperReleases, base, written, writtenEpoch, unwritten, pendingWorlds, waiting, curEpoch, checkpointEpoch, storageMode, cur, named, stored, extra, rel, kept, steps, putSet, freshSet, firstFresh, foundPuts, funded, lineage, queue, pin, hold, commits, lossy, underflow, counting, basisCurrent, staged, lineageCurrent, partStore, retired, doubt, sighted, tainted>>

ProgressNext ==
  \/ Commit \/ Discard \/ Name \/ Forfeit \/ FinishRecount
  \/ StartApply \/ WalkStep \/ EndApply \/ AbortApply \/ RetryApply
  \/ \E p \in Pages : Resolve(p)
  \/ Pin \/ Unpin \/ ResolveRecord \/ ReleaseStage \/ ReadHole \/ Erase \/ RefreshKnown
  \/ Sight
  \/ SealCheckpoint \/ PrepareRecord \/ CarryRecord \/ FailPreparation
  \/ WrittenAdvance \/ BaseAdopt \/ HoldLand \/ ResolveWrapper \/ AliasWrapper \/ AliasHeader \/ DropStoredPages
  \/ UNCHANGED vars

Next == ProgressNext \/ BeginRecount \/ OtherWrite \/ UNCHANGED vars

Spec == Init /\ [][Next]_vars

\* Under bounded foreground work and no repeated external resets, a due record
\* must retire even when its historical references or counts are unavailable.
\* This is a separate liveness obligation; full safety retains external churn.
PendingIds == {waiting[i].id : i \in 1..Len(waiting)}
                \cup {queue[i].id : i \in 1..Len(queue)}
                \cup IF walk.on THEN {walk.rec.id} ELSE {}
RetirementFair ==
  /\ WF_vars(StartApply) /\ WF_vars(WalkStep) /\ WF_vars(EndApply)
  /\ WF_vars(AbortApply) /\ WF_vars(RetryApply) /\ WF_vars(FinishRecount)
  /\ WF_vars(ResolveRecord) /\ WF_vars(ReleaseStage) /\ WF_vars(ReadHole)
  /\ WF_vars(Erase) /\ WF_vars(RefreshKnown) /\ WF_vars(Sight)
  /\ WF_vars(PrepareRecord) /\ WF_vars(CarryRecord)
  /\ WF_vars(DropStoredPages)
  /\ WF_vars(WrittenAdvance) /\ WF_vars(BaseAdopt) /\ WF_vars(HoldLand) /\ WF_vars(ResolveWrapper)
  /\ \A p \in Pages : WF_vars(Resolve(p))
ProgressSpec == Init /\ [][ProgressNext]_vars /\ RetirementFair
RecordsRetire == \A id \in 1..MaxCommits : (id \in PendingIds) ~> (id \notin PendingIds)

Quiet == Ready /\ {h \in retired : h.id \notin headerAliases} = {} /\ steps = <<>> /\ putSet = {} /\ queue = <<>> /\ ~walk.on
         /\ waiting = <<>> /\ wrapperReleases = {} /\ ~save.on
         /\ \A p \in Pages : rel[p] = 0

\* The tree in memory reads: each page is in memory or stored.
MemoryReads == \A p \in Reach(cur) : p \in kept \/ p \in stored

\* No named, held or pinned root reaches a freed page.
NoFreedRead == Live \subseteq stored

\* A release names a stored page, so no page frees twice.
ReleasesStored == \A p \in Pages : rel[p] > 0 => p \in stored

\* The engine counts on the lineage only where the counts hold it.
FundedHeld == (funded /\ lineageCurrent /\ counting /\ basisCurrent) => lineage

\* No count change went below zero, including conservative recount paths.
NoUnderflow == ~underflow

\* Once everything applied, no count is below the page's holders minus one.
CountsCover == (Ready => ~tainted) /\
               (Quiet => LET live == TLCEval(Live)
                         IN \A p \in live : extra[p] >= Holders(p, live) - 1)

\* Exact physical reclamation belongs only to the idealized counted-free
\* primitive. Actual shadow retains incarnations; parent frees instead start
\* a full budgeted recount and can leave surplus edges until the mark sweeps.
\* Coverage, readable live bytes and no underflow apply to both contracts.
CountsExact == (Quiet /\ ~lossy /\ storageMode = "counted-free") => LET live == TLCEval(Live)
                                                                       IN \A p \in live : extra[p] = Holders(p, live) - 1
NoLeak == (Quiet /\ ~lossy /\ storageMode = "counted-free") => stored = Live
====
