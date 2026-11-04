---- MODULE PayloadPages ----
\* A payload can reuse metadata computed by replay and kept only in memory.
\* Staged writes materialize that dependency closure before their temporary
\* tracking entry can be swept. Direct/candidate payloads carry it in the
\* publication, including the fallback write-before-state path. Existing
\* publication ownership keeps captured bytes across retirement/restart.
EXTENDS Naturals, FiniteSets
CONSTANTS StageCarries, PublicationCarries, FallbackCarries
Nodes == {"blob", "a", "b", "root"}
Edges == {<<"root", "b">>, <<"b", "a">>, <<"a", "blob">>}
Children(from) == {n \in Nodes : \E p \in from : <<p, n>> \in Edges}
RECURSIVE ReachN(_, _)
ReachN(from, n) == IF n = 0 THEN from ELSE ReachN(from \cup Children(from), n - 1)
Reach(from) == ReachN(from, Cardinality(Nodes))
VARIABLES producer, writer, phase, physical, kept, tracked, captured, head, restarted
vars == <<producer, writer, phase, physical, kept, tracked, captured, head, restarted>>
Payload == IF producer = "direct" THEN "b" ELSE "root"
Init == /\ producer \in {"stage", "candidate", "direct"}
        /\ writer \in {"atomic", "fallback"}
        /\ phase = "build" /\ physical = {"blob"} /\ kept = {"a", "b"}
        /\ tracked = FALSE /\ captured = {} /\ head = "" /\ restarted = FALSE
Build == /\ phase = "build"
         /\ physical' = physical \cup (IF producer = "stage" THEN {"root"} \cup (IF StageCarries THEN kept ELSE {}) ELSE {})
         /\ tracked' = (producer = "stage") /\ phase' = "ready"
         /\ UNCHANGED <<producer, writer, kept, captured, head, restarted>>
Sweep == /\ phase = "ready" /\ tracked
         /\ tracked' = FALSE
         /\ UNCHANGED <<producer, writer, phase, physical, kept, captured, head, restarted>>
Prepare == /\ phase = "ready"
           /\ captured' = IF (IF writer = "atomic" THEN PublicationCarries ELSE FallbackCarries)
                           /\ (producer # "stage" \/ tracked)
                          THEN Reach({Payload}) \cap kept ELSE {}
           /\ phase' = "prepared"
           /\ UNCHANGED <<producer, writer, physical, kept, tracked, head, restarted>>
Restart == /\ phase = "prepared" /\ ~restarted
           /\ kept' = {} /\ restarted' = TRUE
           /\ UNCHANGED <<producer, writer, phase, physical, tracked, captured, head>>
Publish == /\ phase = "prepared"
           /\ physical' = physical \cup captured \cup (IF producer = "candidate" THEN {"root"} ELSE {})
           /\ head' = Payload /\ phase' = "done"
           /\ UNCHANGED <<producer, writer, kept, tracked, captured, restarted>>
Fail == /\ phase = "prepared" /\ phase' = "failed"
        /\ UNCHANGED <<producer, writer, physical, kept, tracked, captured, head, restarted>>
Next == Build \/ Sweep \/ Prepare \/ Restart \/ Publish \/ Fail \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
TypeOK == /\ physical \subseteq Nodes /\ kept \subseteq Nodes /\ captured \subseteq Nodes
          /\ head \in Nodes \cup {""} /\ phase \in {"build", "ready", "prepared", "done", "failed"}
PublishedReadable == head # "" => Reach({head}) \subseteq physical
====
