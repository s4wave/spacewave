---- MODULE DeadlistPreparation ----
\* Bounded ordinary-block preparation of a deadlist before its name publishes.
\* The worker is owned by the engine lifetime. It captures an immutable job,
\* writes parts outside the engine writer, and rechecks entry identity before
\* attaching completion. A stage owns leaves, indexes and the header until
\* naming is durable or the job is abandoned; process reaping drops old stages.
\* Original interval receipt/generation accounting is in IntervalCounts.
\* credits is a ghost observation of fresh HoldsRoots child funding, not a
\* persisted per-node lifecycle. Existing physical parts never fund twice.
EXTENDS Integers, FiniteSets
CONSTANTS FundOutside, FundReuse, AttachStale
None == "none"
Payload == "payload"
Leaves == {"part-a", "part-b"}
Nodes == Leaves \cup {"index", "header", Payload}
Kids(n) == CASE n \in Leaves -> {Payload}
             [] n = "index" -> Leaves
             [] n = "header" -> {"index"}
             [] OTHER -> {}
RECURSIVE Reach(_)
Reach(n) == {n} \cup UNION {Reach(c) : c \in Kids(n)}
ReachRoots(rs) == UNION {Reach(r) : r \in rs}
VARIABLES phase, current, stored, stage, named, old, credits, work, badAttach
vars == <<phase, current, stored, stage, named, old, credits, work, badAttach>>
Init ==
  /\ phase = "waiting" /\ current = TRUE /\ stage = {} /\ named = FALSE
  /\ old \in {None, "part-a"}
  /\ stored = {Payload} \cup (IF old = None THEN {} ELSE {old})
  /\ credits = [n \in Leaves |-> IF n = old THEN 1 ELSE 0]
  /\ work = 0 /\ badAttach = FALSE
Encode ==
  /\ phase = "waiting" /\ current
  /\ phase' = "preparing"
  /\ UNCHANGED <<current, stored, stage, named, old, credits, work, badAttach>>
Prepare(n) ==
  /\ phase = "preparing" /\ current
  /\ n \in Nodes \ {Payload}
  /\ Kids(n) \subseteq stored
  /\ LET fresh == n \notin stored
     IN /\ stored' = stored \cup {n}
        /\ credits' = [p \in Leaves |-> credits[p]
              + (IF p = n /\ ((fresh /\ FundOutside) \/ (~fresh /\ FundReuse)) THEN 1 ELSE 0)]
  /\ stage' = (stage \ Kids(n)) \cup {n}
  /\ work' = Cardinality(Kids(n))
  /\ UNCHANGED <<phase, current, named, old, badAttach>>
Ready ==
  /\ phase = "preparing" /\ current
  /\ "header" \in stage
  /\ phase' = "ready"
  /\ UNCHANGED <<current, stored, stage, named, old, credits, work, badAttach>>
Name ==
  /\ phase \in {"ready", "orphaned"}
  /\ current \/ AttachStale
  /\ named' = TRUE /\ phase' = "named"
  /\ badAttach' = (badAttach \/ ~current)
  /\ UNCHANGED <<current, stored, stage, old, credits, work>>
ReleaseStage ==
  /\ phase = "named"
  /\ stage' = {}
  /\ UNCHANGED <<phase, current, stored, named, old, credits, work, badAttach>>
Cancel ==
  /\ phase \in {"waiting", "preparing", "ready"}
  /\ current' = FALSE /\ phase' = "orphaned" /\ stage' = {}
  /\ UNCHANGED <<stored, named, old, credits, work, badAttach>>
Restart ==
  /\ current
  /\ current' = FALSE /\ stage' = {}
  /\ phase' = IF named THEN "named" ELSE "orphaned"
  /\ UNCHANGED <<stored, named, old, credits, work, badAttach>>
DropOld ==
  /\ old # None
  /\ old' = None
  /\ UNCHANGED <<phase, current, stored, stage, named, credits, work, badAttach>>
Collect ==
  /\ LET roots == stage \cup (IF named THEN {"header"} ELSE {})
                       \cup {Payload} \cup (IF old = None THEN {} ELSE {old})
         live == ReachRoots(roots)
     IN /\ stored' = stored \cap live
        /\ credits' = [p \in Leaves |-> IF p \in stored' THEN credits[p] ELSE 0]
  /\ UNCHANGED <<phase, current, stage, named, old, work, badAttach>>
Next == Encode \/ (\E n \in Nodes : Prepare(n)) \/ Ready \/ Name
        \/ ReleaseStage \/ Cancel \/ Restart \/ DropOld \/ Collect
Spec == Init /\ [][Next]_vars
NamedReads == named => Reach("header") \subseteq stored
StageReads == ReachRoots(stage) \subseteq stored
FundedNamed == named => \A p \in Leaves : credits[p] = 1
NoDoubleFunding == \A p \in Leaves : credits[p] <= 1
BoundedWriter == work <= 2
NoStaleAttach == ~badAttach
====
