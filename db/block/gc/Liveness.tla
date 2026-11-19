---- MODULE Liveness ----
\* Format 10's block liveness: deferred counts under hold until free, driven
\* by an increment log (U) netted promptly and a release log (D) netted after
\* Grace, with a ripe position on each queued block.
\*
\* Land is the one writer of holders. It stores a batch and moves a root in
\* one transaction: each edge from a born parent to a born child adds to the
\* child's birth count, each other edge from a born parent and a stored root
\* target appends an entry to U, and every stored block it names leaves the
\* queue at once (dequeue on name). Moving a root off a block appends a
\* release to D marked with the U position of its commit. NetUps nets U in
\* order. NetDowns nets a release once it is Grace old and U has netted past
\* its mark (the release mark), and queues a listed block it brings to zero at
\* the current U position (its ripe position). FreeStart deletes a queued block
\* at zero once U has netted past its ripe position, and FreeStep releases its
\* children one transaction at a time, which covers the inline free as the
\* case where the steps follow at once. A listed block holds its children
\* until it is freed itself.
\*
\* A reader observes main, may pin what it observed within Window, and drops
\* its pin. F1's held check lets a batch put again a block that is stored,
\* even one queued at zero, and treats a born child that such a block names
\* as held; the hole shape seeds that case.
\*
\* Callers found from source: Land (db/block/gc/land.go), the collector's
\* netting and free slices (collector.go, netting.go), the staged writer
\* (db/volume/common/kvtx/stage.go) and the reader pins (leases.go). Adopt.tla
\* models how a Volume reaches these counts; AuditWalk.tla models the audit.

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
  Shape,    \* "share": two versions share a two-deep subtree; "hole": a
            \* block parked at zero names a hole, and a new block may name it
  Writers,  \* the roots Land may move: a subset of {"main", "stg"}
  Reader,   \* whether a reader observes and pins main
  MaxTxns,  \* bounds the Land transactions, so the model is finite
  Grace,    \* the age a release reaches before it nets
  Window    \* the age within which a reader may still pin what it observed

None == "none"
Blocks == {"a", "b", "c", "d"}
RootNames == {"main", "stg", "pin"}
Idle == [b |-> None, todo |-> {}]

Kids(b) ==
  CASE Shape = "share" -> [a |-> {"c"}, b |-> {"c"}, c |-> {"d"}, d |-> {}][b]
    [] Shape = "hole" -> [a |-> {"d"}, b |-> {}, c |-> {"a"}, d |-> {}][b]

VARIABLES
  listed,    \* blocks stored with their child list
  H,         \* the netted holders of each block
  queued,    \* listed blocks at zero that hold a 'Q' entry
  ripe,      \* each queued block's ripe position; 0 otherwise
  roots,     \* each root's target, or None
  upLog,     \* U past upCursor: the blocks of unnetted increments
  upCursor,  \* the U position netted so far
  downLog,   \* D: unnetted releases [b, age, mark]
  rel,       \* the free in progress: its block and the children it still
             \* releases, todo empty when none is
  seen,      \* the reader's observation: [b, age], b None when idle
  freed,     \* blocks freed and not stored again since
  under,     \* whether a decrement ever met a zero count
  txns       \* Land transactions so far
vars == <<listed, H, queued, ripe, roots, upLog, upCursor, downLog, rel, seen, freed, under, txns>>

upSeq == upCursor + Len(upLog)

RECURSIVE Reach(_)
Reach(S) ==
  LET next == S \cup UNION {Kids(b) : b \in S}
  IN IF next = S THEN S ELSE Reach(next)

Targets == {roots[n] : n \in RootNames} \ {None}

RECURSIVE SeqOf(_)
SeqOf(S) ==
  IF S = {} THEN <<>>
  ELSE LET x == CHOOSE x \in S : TRUE IN <<x>> \o SeqOf(S \ {x})

Count(s, b) == Cardinality({i \in 1..Len(s) : s[i] = b})
Ups(b) == Count(upLog, b)
Downs(b) == Cardinality({i \in 1..Len(downLog) : downLog[i].b = b})
Pending(b) == IF b \in rel.todo THEN 1 ELSE 0
Holders(b) ==
  Cardinality({n \in RootNames : roots[n] = b})
    + Cardinality({p \in listed : b \in Kids(p)})

Init ==
  /\ listed = IF Shape = "share" THEN {"a", "c", "d"} ELSE {"a", "b"}
  /\ H = IF Shape = "share" THEN [b \in Blocks |-> IF b = "b" THEN 0 ELSE 1]
         ELSE [b \in Blocks |-> IF b \in {"b", "d"} THEN 1 ELSE 0]
  /\ queued = IF Shape = "share" THEN {} ELSE {"a"}
  /\ ripe = [b \in Blocks |-> 0]
  /\ roots = [n \in RootNames |->
               IF n = "main" THEN (IF Shape = "share" THEN "a" ELSE "b")
               ELSE None]
  /\ upLog = <<>>
  /\ upCursor = 0
  /\ downLog = <<>>
  /\ rel = Idle
  /\ seen = [b |-> None, age |-> 0]
  /\ freed = {}
  /\ under = FALSE
  /\ txns = 0

\* Decrement lowers b's count by one, queuing a listed block it brings to
\* zero at the current U position.
Decrement(b) ==
  /\ under' = (under \/ H[b] = 0)
  /\ H' = [H EXCEPT ![b] = IF @ = 0 THEN 0 ELSE @ - 1]
  /\ IF b \in listed /\ H[b] = 1
       THEN /\ queued' = queued \cup {b}
            /\ ripe' = [ripe EXCEPT ![b] = upSeq]
       ELSE UNCHANGED <<queued, ripe>>

\* LandTx stores batch and moves root n to t in one transaction. Each block
\* the batch gives birth to is held: by the root, by a parent the batch
\* stores, born or put again, or by a hole count it already has.
LandTx(n, t, batch) ==
  LET born == batch \ listed
      move == t # roots[n]
      edges == UNION {{<<p, k>> : k \in Kids(p) \ born} : p \in born}
      tUp == move /\ t # None /\ t \notin born
      ups == [i \in 1..Cardinality(edges) |-> SeqOf(edges)[i][2]]
               \o (IF tUp THEN <<t>> ELSE <<>>)
      named == {e[2] : e \in edges} \cup (IF tUp THEN {t} ELSE {})
      birth(b) == Cardinality({p \in born : b \in Kids(p)})
                    + (IF move /\ b = t THEN 1 ELSE 0)
  IN
  /\ txns < MaxTxns
  /\ move \/ batch # {}
  /\ t \in listed \cup batch \cup {None}
  /\ \A p \in born : Kids(p) \subseteq listed \cup batch
  /\ \A b \in born : (move /\ b = t) \/ (\E p \in batch : b \in Kids(p)) \/ H[b] > 0
  /\ listed' = listed \cup batch
  /\ freed' = freed \ batch
  /\ H' = [b \in Blocks |-> IF b \in born THEN H[b] + birth(b) ELSE H[b]]
  /\ upLog' = upLog \o ups
  /\ queued' = queued \ named
  /\ ripe' = [b \in Blocks |-> IF b \in named THEN 0 ELSE ripe[b]]
  /\ downLog' = IF move /\ roots[n] # None
                  THEN Append(downLog, [b |-> roots[n], age |-> 0, mark |-> upSeq + Len(ups)])
                  ELSE downLog
  /\ roots' = [roots EXCEPT ![n] = t]
  /\ txns' = txns + 1
  /\ UNCHANGED <<upCursor, rel, under>>

Land ==
  \E n \in Writers, t \in Blocks \cup {None}, batch \in SUBSET Blocks :
    /\ n # "main" \/ t # None
    /\ LandTx(n, t, batch)
    /\ UNCHANGED seen

ReaderObserve ==
  /\ Reader
  /\ seen.b = None
  /\ roots["pin"] = None
  /\ seen' = [b |-> roots["main"], age |-> 0]
  /\ UNCHANGED <<listed, H, queued, ripe, roots, upLog, upCursor, downLog, rel, freed, under, txns>>

ReaderPin ==
  /\ seen.b # None
  /\ seen.age <= Window
  /\ LandTx("pin", seen.b, {})
  /\ seen' = [b |-> None, age |-> 0]

PinRelease ==
  /\ roots["pin"] # None
  /\ LandTx("pin", None, {})
  /\ UNCHANGED seen

NetUps ==
  /\ upLog # <<>>
  /\ LET b == Head(upLog) IN
       /\ H' = [H EXCEPT ![b] = @ + 1]
       /\ queued' = queued \ {b}
       /\ ripe' = [ripe EXCEPT ![b] = 0]
  /\ upLog' = Tail(upLog)
  /\ upCursor' = upCursor + 1
  /\ UNCHANGED <<listed, roots, downLog, rel, seen, freed, under, txns>>

NetDowns ==
  /\ downLog # <<>>
  /\ LET e == Head(downLog) IN
       /\ e.age >= Grace
       /\ upCursor >= e.mark
       /\ Decrement(e.b)
  /\ downLog' = Tail(downLog)
  /\ UNCHANGED <<listed, roots, upLog, upCursor, rel, seen, freed, txns>>

FreeStart(b) ==
  /\ rel.todo = {}
  /\ b \in queued
  /\ H[b] = 0
  /\ ripe[b] <= upCursor
  /\ listed' = listed \ {b}
  /\ freed' = freed \cup {b}
  /\ queued' = queued \ {b}
  /\ ripe' = [ripe EXCEPT ![b] = 0]
  /\ rel' = [b |-> b, todo |-> Kids(b)]
  /\ UNCHANGED <<H, roots, upLog, upCursor, downLog, seen, under, txns>>

FreeStep(k) ==
  /\ k \in rel.todo
  /\ Decrement(k)
  /\ rel' = [rel EXCEPT !.todo = @ \ {k}]
  /\ UNCHANGED <<listed, roots, upLog, upCursor, downLog, seen, freed, txns>>

Tick ==
  /\ \/ \E i \in 1..Len(downLog) : downLog[i].age < Grace
     \/ seen.b # None /\ seen.age <= Window
  /\ downLog' = [i \in 1..Len(downLog) |->
                  [downLog[i] EXCEPT !.age = IF @ < Grace THEN @ + 1 ELSE @]]
  /\ seen' = IF seen.b # None /\ seen.age <= Window
               THEN [seen EXCEPT !.age = @ + 1] ELSE seen
  /\ UNCHANGED <<listed, H, queued, ripe, roots, upLog, upCursor, rel, freed, under, txns>>

Free == \E b \in Blocks : FreeStart(b)
Step == \E k \in Blocks : FreeStep(k)

Next ==
  \/ Land \/ ReaderObserve \/ ReaderPin \/ PinRelease
  \/ NetUps \/ NetDowns \/ Free \/ Step \/ Tick

Spec ==
  /\ Init /\ [][Next]_vars
  /\ WF_vars(NetUps) /\ WF_vars(NetDowns) /\ WF_vars(Free) /\ WF_vars(Step)
  /\ WF_vars(Tick)

\* No held root reaches a freed block.
NoFreedRead == Reach(Targets) \cap freed = {}

\* A reader may pin what it observed within Window: it is all still stored.
ReaderWindow ==
  seen.b # None /\ seen.age <= Window =>
    seen.b \in listed /\ Reach({seen.b}) \cap freed = {}

\* A listed block's children stay stored, or counted as holes, until it is
\* freed itself.
HoldUntilFree ==
  \A p \in listed : \A k \in Kids(p) : k \notin freed /\ (k \in listed \/ H[k] >= 1)

\* The netted count with the unnetted logs and the free in progress equals
\* the roots and listed parents that name each block.
CountsExact ==
  \A b \in Blocks : H[b] + Ups(b) = Holders(b) + Downs(b) + Pending(b)

NoUnderflow == ~under

\* No increment appended after a queued block's ripe position names it, so a
\* free can trust H without U being empty.
NoLateNamer ==
  \A b \in queued : \A i \in 1..Len(upLog) : upCursor + i > ripe[b] => upLog[i] # b

\* A block queued at zero is freed or named again.
FreesProgress == \A b \in Blocks : (b \in queued) ~> (b \notin queued)

\* Once writes stop, the logs drain and only what the roots reach is stored.
EventuallyCollected ==
  <>[](/\ upLog = <<>> /\ downLog = <<>> /\ rel.todo = {}
       /\ listed \subseteq Reach(Targets))
====
