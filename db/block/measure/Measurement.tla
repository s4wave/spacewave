--------------------------- MODULE Measurement ---------------------------
EXTENDS Naturals, FiniteSets

(***************************************************************************
 Each successful range can be lost before the local transaction commits.
 pending models nodes.state=0; done models committed measured nodes.
 Land commits the count and children atomically. A 503 stops all reads; an
 explicit resume keeps the attempt ledger. B2 has no write transition.
 Ranges are abstracted to one block. LocalRead models the immutable Volume
 and cached blocks without remote reservations. SourceHole covers unavailable,
 corrupt and invalid-extent judgments; unknown refs and typed decode failures
 take the same terminal transition after reading. Repaired graph lists are
 abstracted by Edges. Repeated seeds cannot put a hole back into pending.
 Limit and ByteLimit are immutable work-file bindings across Resume.
 Edges is the union of useful children decoded under all reaching Worlds;
 descriptor replay from verified cache is hidden by this abstraction. A failed
 descriptor can remain a reported hole while another supplies useful edges.
 ConfigurationStop abstracts the third consecutive 404 and is durable.
 Byte sizes are unit costs here, covering atomic reservation across crashes.
 Schema decoding, input identity and filesystem aliases are separate conformance
 checks; this graph model does not establish their correctness.
***************************************************************************)
CONSTANTS Blocks, Roots, Edges, Limit, ByteLimit, None
VARIABLES pending, done, current, count, attempts, spent, holes, phase, remoteWrites
vars == <<pending, done, current, count, attempts, spent, holes, phase, remoteWrites>>

Kids(S) == UNION {Edges[b] : b \in S}
RECURSIVE ReachAt(_, _)
ReachAt(S, n) == IF n = 0 THEN S ELSE ReachAt(S \cup Kids(S), n - 1)
Reach == ReachAt(Roots, Cardinality(Blocks))

Init == /\ pending = Roots
        /\ done = {}
        /\ current = None
        /\ count = 0
        /\ attempts = 0
        /\ spent = 0
        /\ holes = {}
        /\ phase = "running"
        /\ remoteWrites = 0

Read(b) == /\ phase = "running" /\ current = None
           /\ b \in pending /\ attempts < Limit /\ spent < ByteLimit
           /\ current' = b /\ attempts' = attempts + 1 /\ spent' = spent + 1
           /\ UNCHANGED <<pending, done, count, holes, phase, remoteWrites>>

LocalRead(b) == /\ phase = "running" /\ current = None /\ b \in pending
                /\ current' = b
                /\ UNCHANGED <<pending, done, count, attempts, spent, holes, phase, remoteWrites>>

SlowDown == /\ phase = "running" /\ current = None
            /\ pending # {} /\ attempts < Limit /\ spent < ByteLimit
            /\ phase' = "stopped" /\ attempts' = attempts + 1 /\ spent' = spent + 1
            /\ UNCHANGED <<pending, done, current, count, holes, remoteWrites>>

Land == /\ phase = "running" /\ current \in pending
        /\ done' = done \cup {current}
        /\ pending' = (pending \cup Edges[current]) \ (done' \cup holes)
        /\ count' = count + 1 /\ current' = None
        /\ UNCHANGED <<attempts, spent, holes, phase, remoteWrites>>

Crash == /\ current # None /\ current' = None /\ phase' = "stopped"
         /\ UNCHANGED <<pending, done, count, attempts, spent, holes, remoteWrites>>

Resume == /\ phase = "stopped" /\ phase' = "running"
          /\ UNCHANGED <<pending, done, current, count, attempts, spent, holes, remoteWrites>>

Finish == /\ pending = {} /\ phase = "running" /\ phase' = "done"
          /\ UNCHANGED <<pending, done, current, count, attempts, spent, holes, remoteWrites>>

SourceHole == /\ phase = "running" /\ current \in pending
              /\ holes' = holes \cup {current}
              /\ pending' = pending \ {current} /\ current' = None
              /\ UNCHANGED <<done, count, attempts, spent, phase, remoteWrites>>

ConfigurationStop == /\ phase = "running" /\ current \in pending
                     /\ holes' = holes \cup {current}
                     /\ pending' = pending \ {current} /\ current' = None
                     /\ phase' = "configuration"
                     /\ UNCHANGED <<done, count, attempts, spent, remoteWrites>>

BudgetStop == /\ phase = "running" /\ current = None /\ pending # {}
              /\ (attempts = Limit \/ spent = ByteLimit)
              /\ phase' = "budget"
              /\ UNCHANGED <<pending, done, current, count, attempts, spent, holes, remoteWrites>>

Terminal == /\ phase \in {"done", "budget", "configuration"} /\ UNCHANGED vars

Next == (\E b \in Blocks : Read(b) \/ LocalRead(b)) \/ SlowDown \/ Land \/ Crash \/ Resume \/ Finish \/ SourceHole \/ ConfigurationStop \/ BudgetStop \/ Terminal
Spec == Init /\ [][Next]_vars

TypeOK == /\ pending \subseteq Blocks /\ done \subseteq Blocks
          /\ current \in Blocks \cup {None}
          /\ phase \in {"running", "stopped", "done", "budget", "configuration"}
          /\ attempts \in 0..Limit /\ spent \in 0..ByteLimit
          /\ holes \subseteq Blocks
NoDoubleCount == count = Cardinality(done)
NoLostFrontier == /\ Roots \subseteq pending \cup done \cup holes
                 /\ Kids(done) \subseteq pending \cup done \cup holes
                 /\ pending \cap done = {}
Complete == (phase = "done" /\ holes = {}) => done = Reach
ReadOnly == remoteWrites = 0

FixtureEdges == [b \in Blocks |-> IF b \in {"a", "b"} THEN {"c"}
                                  ELSE IF b = "c" THEN {"d"} ELSE {}]
=============================================================================
