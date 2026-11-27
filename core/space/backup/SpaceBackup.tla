--------------------------- MODULE SpaceBackup ---------------------------
EXTENDS FiniteSets, Naturals, TLC
\* A two-block DAG, where root 2 refers to leaf 1. The local proofs survive
\* cancellation; volatile traversal state does not. Drive has no delete action.
VARIABLES durable, proven, published, running
vars == <<durable, proven, published, running>>
Blocks == {1, 2}
Children(b) == IF b = 2 THEN {1} ELSE {}
Init == /\ durable = {} /\ proven = {} /\ published = FALSE /\ running = FALSE
Start == /\ ~running /\ running' = TRUE /\ UNCHANGED <<durable, proven, published>>
Upload(b) == /\ running /\ b \in Blocks /\ durable' = durable \cup {b}
             /\ UNCHANGED <<proven, published, running>>
Prove(b) == /\ running /\ b \in durable /\ Children(b) \subseteq proven \* proof guard
            /\ proven' = proven \cup {b} /\ UNCHANGED <<durable, published, running>>
Publish == /\ running /\ 2 \in proven /\ published' = TRUE /\ running' = FALSE
           /\ UNCHANGED <<durable, proven>>
Cancel == /\ running /\ running' = FALSE /\ UNCHANGED <<durable, proven, published>>
Next == Start \/ (\E b \in Blocks: Upload(b) \/ Prove(b)) \/ Publish \/ Cancel
TypeOK == /\ durable \subseteq Blocks /\ proven \subseteq Blocks
          /\ published \in BOOLEAN /\ running \in BOOLEAN
ProofsDurable == proven \subseteq durable
ProofsComplete == \A b \in proven: Children(b) \subseteq proven
VisibleComplete == published => Blocks \subseteq durable
Spec == Init /\ [][Next]_vars
=============================================================================
