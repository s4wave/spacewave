---- MODULE AuditWalk ----
\* The streaming audit: a walk that checks the collector's counts and finds
\* holes on a Volume that writers and collection passes keep changing. It
\* reads the Volume only in short views and writes only its own scratch
\* store, so in this model no audit action changes a Volume variable.
\*
\* The Volume. Kids fixes each block's child list, since blocks are content
\* addressed. A stored block keeps its bytes and its list together. A block's
\* namers are the roots that name it and the stored blocks that list it; Tally
\* counts them. Holders splits each block's count into the netted holders and
\* delta, the net of the journal past the cursor. Without faults holders plus
\* delta equals Tally for every block (CountsExact). A put may store a parent
\* before its children (Ahead), whose bytes then sit in a writer's memory:
\* inflight. Two faults break the counts: Lose drops a stored block's bytes
\* and list and leaves every count alone, and Drop takes one holder from a
\* block. RecordHole stands for an earlier snapshot audit, which records a
\* hole and raises its holders to its namers.
\*
\* The walk runs in three phases, each a sequence of short views:
\*   edges    visits each namer in Namers order, the roots first and then the
\*            stored blocks, and writes an edge <<child, namer>> to scratch for
\*            each block the namer names in that view.
\*   check    visits each child in Order, in one view per child. It keeps only
\*            the child's edges still valid in that view: the root still names
\*            the child, or the parent is still stored. Seen counts them, a
\*            lower bound on the child's namers, so holders plus delta below
\*            Seen is a true undercount. A named child with no bytes, no hole
\*            record and no list becomes a hole candidate.
\*   confirm  rereads each candidate once the confirm delay has passed. The
\*            model assumes every put in flight finishes within the delay, so
\*            the confirmation waits until the candidate is not in flight. A
\*            candidate still named and absent then is a new hole.
\*
\* NoFalsePositive: every flag the walk raises is a true defect in the view
\* that raised it. Detects: the check flags every child whose defect stands
\* in its view, unless a namer passed by the edges phase took up the child
\* after the walk visited it (newNamer). Such a namer can hide one unit of
\* deficit until the next walk, which the walk admits.
EXTENDS Integers, FiniteSets, Sequences

CONSTANTS
  \* MaxFaults bounds Lose and Drop together.
  MaxFaults,
  \* MaxCount bounds holders and delta, to keep the model finite.
  MaxCount,
  \* Ahead lets a put store a parent before its children.
  Ahead,
  \* Roots is the number of root names, one or two.
  Roots

Blocks == {"a", "b", "c"}
Kids == [x \in Blocks |-> CASE x = "a" -> {"b", "c"}
                            [] x = "b" -> {"c"}
                            [] OTHER -> {}]
RootSeq == IF Roots = 1 THEN <<"r1">> ELSE <<"r1", "r2">>
RootNames == {RootSeq[i] : i \in 1..Len(RootSeq)}
None == "none"
Namers == RootSeq \o <<"a", "b", "c">>
Order == <<"a", "b", "c">>

VARIABLES
  \* The Volume.
  stored, inflight, holders, delta, root, hole, faults,
  \* The walk and its scratch store.
  phase, pos, edges, cand, newNamer,
  \* The checker's flags: a false positive, a missed defect.
  fp, missed

vol == <<stored, inflight, holders, delta, root, hole, faults>>
walk == <<phase, pos, edges, cand, newNamer>>
vars == <<vol, walk, fp, missed>>

RootNamers(x) == {n \in RootNames : root[n] = x}
ParentNamers(x) == {p \in stored : x \in Kids[p]}
Tally(x) == Cardinality(RootNamers(x)) + Cardinality(ParentNamers(x))

\* Under is a true undercount; Hole is a true new hole.
Under(x) == holders[x] + delta[x] < Tally(x)
Hole(x) == Tally(x) > 0 /\ x \notin stored /\ x \notin inflight /\ x \notin hole

\* Shift moves delta up by one for each block of up, down for each of down.
Shift(up, down) == [x \in Blocks |-> delta[x] + (IF x \in up THEN 1 ELSE 0)
                                              - (IF x \in down THEN 1 ELSE 0)]

\* Passed reports whether the edges phase already visited namer n.
Idx(n) == CHOOSE i \in 1..Len(Namers) : Namers[i] = n
Passed(n) == \/ phase \in {"check", "confirm", "done"}
             \/ phase = "edges" /\ Idx(n) < pos

\* Took records the blocks a passed namer n takes up as new namers.
Took(n, xs) == newNamer' = IF Passed(n) THEN newNamer \cup xs ELSE newNamer

TypeOK ==
  /\ stored \subseteq Blocks
  /\ inflight \subseteq Blocks
  /\ stored \cap inflight = {}
  /\ hole \subseteq Blocks
  /\ holders \in [Blocks -> Int]
  /\ delta \in [Blocks -> Int]
  /\ root \in [RootNames -> Blocks \cup {None}]
  /\ faults \in 0..MaxFaults
  /\ phase \in {"idle", "edges", "check", "confirm", "done"}
  /\ edges \subseteq (Blocks \X (RootNames \cup Blocks))
  /\ cand \subseteq Blocks
  /\ newNamer \subseteq Blocks

Init ==
  /\ stored = {}
  /\ inflight = {}
  /\ holders = [x \in Blocks |-> 0]
  /\ delta = [x \in Blocks |-> 0]
  /\ root = [n \in RootNames |-> None]
  /\ hole = {}
  /\ faults = 0
  /\ phase = "idle"
  /\ pos = 1
  /\ edges = {}
  /\ cand = {}
  /\ newNamer = {}
  /\ fp = FALSE
  /\ missed = FALSE

----
\* Writers and the collector.

\* Put stores x and its list in one commit, journals an up pair for each
\* child, and deletes x's hole record. Under Ahead a child not yet stored
\* stays in the writer's memory until its own put.
Put(x) ==
  /\ x \notin stored
  /\ Ahead \/ Kids[x] \subseteq stored
  /\ stored' = stored \cup {x}
  /\ inflight' = (inflight \ {x}) \cup (Kids[x] \ stored)
  /\ delta' = Shift(Kids[x], {})
  /\ hole' = hole \ {x}
  /\ Took(x, Kids[x])
  /\ UNCHANGED <<holders, root, faults, phase, pos, edges, cand, fp, missed>>

\* SetRoot moves root n onto a stored block or off every block.
SetRoot(n, x) ==
  /\ x \in stored \cup {None}
  /\ x # root[n]
  /\ delta' = Shift(IF x = None THEN {} ELSE {x},
                    IF root[n] = None THEN {} ELSE {root[n]})
  /\ root' = [root EXCEPT ![n] = x]
  /\ Took(n, IF x = None THEN {} ELSE {x})
  /\ UNCHANGED <<stored, inflight, holders, hole, faults, phase, pos, edges, cand, fp, missed>>

\* Net moves x's journal delta into its netted holders.
Net(x) ==
  /\ delta[x] # 0
  /\ holders' = [holders EXCEPT ![x] = @ + delta[x]]
  /\ delta' = [delta EXCEPT ![x] = 0]
  /\ UNCHANGED <<stored, inflight, root, hole, faults, walk, fp, missed>>

\* Free deletes a stored block whose netted holders are zero with nothing
\* past the cursor, and journals a down pair for each child.
Free(x) ==
  /\ x \in stored
  /\ holders[x] = 0
  /\ delta[x] = 0
  /\ stored' = stored \ {x}
  /\ delta' = Shift({}, Kids[x])
  /\ UNCHANGED <<inflight, holders, root, hole, faults, walk, fp, missed>>

\* Lose is a storage fault: x's bytes and list go and no count moves.
Lose(x) ==
  /\ faults < MaxFaults
  /\ x \in stored
  /\ stored' = stored \ {x}
  /\ faults' = faults + 1
  /\ UNCHANGED <<inflight, holders, delta, root, hole, walk, fp, missed>>

\* Drop is a counting fault: x loses one netted holder.
Drop(x) ==
  /\ faults < MaxFaults
  /\ holders[x] > 0
  /\ holders' = [holders EXCEPT ![x] = @ - 1]
  /\ faults' = faults + 1
  /\ UNCHANGED <<stored, inflight, delta, root, hole, walk, fp, missed>>

\* RecordHole is an earlier snapshot audit recording hole x and raising its
\* holders to its namers.
RecordHole(x) ==
  /\ Hole(x)
  /\ hole' = hole \cup {x}
  /\ holders' = [holders EXCEPT ![x] = IF Under(x) THEN Tally(x) - delta[x] ELSE @]
  /\ UNCHANGED <<stored, inflight, delta, root, faults, walk, fp, missed>>

----
\* The walk. Each action is one view of the Volume and one scratch commit.

Start ==
  /\ phase = "idle"
  /\ phase' = "edges"
  /\ UNCHANGED <<vol, pos, edges, cand, newNamer, fp, missed>>

\* NamedBy returns the edges namer n contributes in the current view.
NamedBy(n) ==
  IF n \in RootNames
  THEN IF root[n] = None THEN {} ELSE {<<root[n], n>>}
  ELSE IF n \in stored THEN {<<k, n>> : k \in Kids[n]} ELSE {}

ScanEdges ==
  /\ phase = "edges"
  /\ pos <= Len(Namers)
  /\ edges' = edges \cup NamedBy(Namers[pos])
  /\ pos' = pos + 1
  /\ UNCHANGED <<vol, phase, cand, newNamer, fp, missed>>

EndEdges ==
  /\ phase = "edges"
  /\ pos > Len(Namers)
  /\ phase' = "check"
  /\ pos' = 1
  /\ UNCHANGED <<vol, edges, cand, newNamer, fp, missed>>

\* Valid reports whether edge e still holds in the current view.
Valid(e) == IF e[2] \in RootNames THEN root[e[2]] = e[1] ELSE e[2] \in stored
Seen(c) == Cardinality({e \in edges : e[1] = c /\ Valid(e)})

\* Absent reports a block with no bytes, no list and no hole record.
Absent(c) == c \notin stored /\ c \notin hole

Check ==
  /\ phase = "check"
  /\ pos <= Len(Order)
  /\ LET c == Order[pos]
         flagU == holders[c] + delta[c] < Seen(c)
         flagH == Seen(c) > 0 /\ Absent(c)
     IN /\ cand' = IF flagH THEN cand \cup {c} ELSE cand
        /\ fp' = (fp \/ (flagU /\ ~Under(c)))
        /\ missed' = (missed \/ (c \notin newNamer /\ ((Under(c) /\ ~flagU) \/ (Hole(c) /\ c \notin cand'))))
  /\ pos' = pos + 1
  /\ UNCHANGED <<vol, phase, edges, newNamer>>

EndCheck ==
  /\ phase = "check"
  /\ pos > Len(Order)
  /\ phase' = "confirm"
  /\ UNCHANGED <<vol, pos, edges, cand, newNamer, fp, missed>>

\* Confirm rereads candidate c after the confirm delay and reports it as a
\* new hole if it is still named and absent.
Confirm(c) ==
  /\ phase = "confirm"
  /\ c \in cand
  /\ c \notin inflight   \* the delay outlasts every put in flight
  /\ fp' = (fp \/ (Seen(c) > 0 /\ Absent(c) /\ ~Hole(c)))
  /\ cand' = cand \ {c}
  /\ UNCHANGED <<vol, phase, pos, edges, newNamer, missed>>

Done ==
  /\ phase = "confirm"
  /\ cand = {}
  /\ phase' = "done"
  /\ UNCHANGED <<vol, pos, edges, cand, newNamer, fp, missed>>

\* The Volume changes only until the walk is done, since no later state can
\* raise a flag.
Next ==
  \/ /\ phase # "done"
     /\ \/ \E x \in Blocks : Put(x) \/ Net(x) \/ Free(x) \/ Lose(x) \/ Drop(x) \/ RecordHole(x)
        \/ \E n \in RootNames, x \in Blocks \cup {None} : SetRoot(n, x)
  \/ Start \/ ScanEdges \/ EndEdges \/ Check \/ EndCheck \/ Done
  \/ \E c \in Blocks : Confirm(c)

Spec == Init /\ [][Next]_vars

----
\* Invariants.

NoFalsePositive == ~fp
Detects == ~missed

\* CountsExact checks the model's writers: without faults holders plus delta
\* equals each block's namers.
CountsExact == faults = 0 => \A x \in Blocks : holders[x] + delta[x] = Tally(x)

\* Bounded keeps the counts finite.
Bounded == \A x \in Blocks : holders[x] \in -MaxCount..MaxCount /\ delta[x] \in -MaxCount..MaxCount
====
