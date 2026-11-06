---- MODULE StableCheckpoint ----
\* The stable checkpoint of an owner-controlled SharedObject: the stable point
\* (stable-point.go StablePoint and builtOn), the checkpointer's step
\* (body-driver.go checkpoint) and its adoption (body-checkpointer.go adopt).
\*
\* Writers on the roster write operations that build on every operation they
\* know, each after its author's previous one, and learn each other's in any
\* order an operation's ancestors allow. The checkpointer places an operation
\* once it holds the ancestors, anywhere after them: that over-approximates
\* replay's deterministic order, which inserts a new operation without
\* reordering the placed ones. The stable prefix is the longest prefix of the
\* order above the checkpoint that every roster member has built on through
\* its latest operation there. The driver reads the order, takes the body's
\* state after the stable prefix (Read), and adopts a checkpoint over it under
\* the host's state lock (Adopt); with CheckStale, adopt first checks that the
\* prefix is still a prefix of the stable point, and skips it otherwise.
\*
\* A Joiner may join the roster once, from any snapshot of the operations that
\* holds the checkpoint, such as an older one a peer served. Its operations
\* then build on less than the checkpointer had placed. Without CheckStale, a
\* checkpoint read before the join covers operations the joiner never built
\* on, and its next operation belongs inside the checkpoint
\* (testdata/StableCheckpoint-nostale.cfg).
\*
\* Misordered records a placement that the stable prefix or the checkpoint
\* does not precede: an operation whose ancestors lack a stable or covered
\* one, so replay could order it inside them. Outside the model: the open
\* sequence, whose stable point is the sequenced prefix; group control, whose
\* checkpoints are voted on (ProposeCheckpoint, JudgeCheckpoint); dropping
\* lagging members from the roster, whose operations below the checkpoint are
\* refused; the acknowledgment cadence; key epochs and the body's state data.
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Writers, Joiner, MaxOps, MinStable, CheckStale
VARIABLES nonce, deps, known, order, covered, roster, drv, snap, misordered
vars == <<nonce, deps, known, order, covered, roster, drv, snap, misordered>>

Members == Writers \cup {Joiner}
Ids == Members \X (1..MaxOps)
Created == {<<m, n>> \in Ids : n <= nonce[m]}
Range(s) == {s[i] : i \in DOMAIN s}
Placed == Range(order)
Held == covered \cup Placed

\* Closed reports that a set of operations holds the ancestors of each one.
Closed(S) == \A o \in S : deps[o] \subseteq S

\* BuiltOn is what every later operation of m descends from: its latest
\* operation above the checkpoint and that one's ancestors, or nothing when m
\* has none there.
Own(m) == {o \in Placed : o[1] = m}
BuiltOn(m) == IF Own(m) = {} THEN {}
              ELSE LET latest == CHOOSE o \in Own(m) : \A p \in Own(m) : p[2] <= o[2]
                   IN {latest} \cup deps[latest]
Built == IF roster = {} THEN Placed
         ELSE {o \in Placed : \A m \in roster : o \in BuiltOn(m)}
StableLen == IF Len(order) = 0 THEN 0
             ELSE CHOOSE k \in 0..Len(order) :
                    /\ \A i \in 1..k : order[i] \in Built
                    /\ (k = Len(order) \/ order[k + 1] \notin Built)
Stable == SubSeq(order, 1, StableLen)
IsPrefix(p, s) == Len(p) <= Len(s) /\ SubSeq(s, 1, Len(p)) = p

Init == /\ nonce = [m \in Members |-> 0]
        /\ deps = [o \in Ids |-> {}]
        /\ known = [m \in Members |-> {}]
        /\ order = <<>> /\ covered = {} /\ roster = Writers
        /\ drv = "idle" /\ snap = <<>> /\ misordered = FALSE

\* Write appends an operation of m that builds on everything m knows.
Write(m) == /\ m \in roster /\ nonce[m] < MaxOps
            /\ LET o == <<m, nonce[m] + 1>>
               IN /\ nonce' = [nonce EXCEPT ![m] = @ + 1]
                  /\ deps' = [deps EXCEPT ![o] = known[m]]
                  /\ known' = [known EXCEPT ![m] = @ \cup {o}]
            /\ UNCHANGED <<order, covered, roster, drv, snap, misordered>>

\* Learn delivers an operation to a roster member once it holds the
\* operation's ancestors.
Learn(m, o) == /\ m \in roster
               /\ o \in Created \ known[m] /\ deps[o] \subseteq known[m]
               /\ known' = [known EXCEPT ![m] = @ \cup {o}]
               /\ UNCHANGED <<nonce, deps, order, covered, roster, drv, snap, misordered>>

\* Place inserts an operation the checkpointer receives after its placed
\* ancestors, at any later position.
Place(o) == /\ o \in Created \ Held /\ deps[o] \subseteq Held
            /\ LET after == {i \in DOMAIN order : order[i] \in deps[o]}
                   first == IF after = {} THEN 1
                            ELSE 1 + CHOOSE i \in after : \A j \in after : j <= i
               IN \E p \in first..(Len(order) + 1) :
                    order' = SubSeq(order, 1, p - 1) \o <<o>> \o SubSeq(order, p, Len(order))
            /\ misordered' = (misordered \/ ~((Range(Stable) \cup covered) \subseteq deps[o]))
            /\ UNCHANGED <<nonce, deps, known, covered, roster, drv, snap>>

\* Join adds the joiner to the roster from a snapshot holding the checkpoint.
Join == /\ Joiner \notin roster
        /\ \E K \in SUBSET Created :
             /\ covered \subseteq K /\ Closed(K)
             /\ known' = [known EXCEPT ![Joiner] = K]
        /\ roster' = roster \cup {Joiner}
        /\ UNCHANGED <<nonce, deps, order, covered, drv, snap, misordered>>

\* Read takes the body's state after the stable prefix.
Read == /\ drv = "idle" /\ StableLen >= MinStable
        /\ drv' = "built" /\ snap' = Stable
        /\ UNCHANGED <<nonce, deps, known, order, covered, roster, misordered>>

\* Adopt signs the checkpoint over the read prefix on the held state, or skips
\* it when the prefix is no longer stable.
Adopt == /\ drv = "built"
         /\ IF ~CheckStale \/ IsPrefix(snap, Stable)
              THEN /\ covered' = covered \cup Range(snap)
                   /\ order' = SelectSeq(order, LAMBDA o : o \notin Range(snap))
              ELSE UNCHANGED <<covered, order>>
         /\ drv' = "idle" /\ snap' = <<>>
         /\ UNCHANGED <<nonce, deps, known, roster, misordered>>

Next == \/ \E m \in Members : Write(m)
        \/ \E m \in Members, o \in Ids : Learn(m, o)
        \/ \E o \in Ids : Place(o)
        \/ Join \/ Read \/ Adopt
        \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Read) /\ WF_vars(Adopt)

TypeOK == /\ nonce \in [Members -> 0..MaxOps]
          /\ covered \subseteq Ids /\ Placed \subseteq Ids
          /\ drv \in {"idle", "built"}
\* No operation is placed inside the stable prefix or the checkpoint.
Ordered == ~misordered
\* The checkpoint covers each operation once: none stays placed above it.
CoveredOnce == covered \cap Placed = {}
\* Every operation the checkpointer holds has its ancestors held.
HeldClosed == Closed(Held)
\* A stable prefix is checkpointed, or stops being stable, while the driver
\* runs: Adopt covers it, or a join resets the stable point.
Checkpoints == (StableLen >= MinStable) ~> (StableLen < MinStable)
====
