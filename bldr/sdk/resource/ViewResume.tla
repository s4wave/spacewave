---------------------------- MODULE ViewResume ----------------------------
\* HeldViews.resume: a full snapshot, an unchanged response or a delta.
\* Two versions and one authorization-scope replacement; persistence decoding
\* and record-specific merge functions are checked by the client run-through.
EXTENDS Naturals
VARIABLES held, scope, baseScope, delivered, invalid, failed
vars == <<held, scope, baseScope, delivered, invalid, failed>>
Init == /\ held = 0 /\ scope = 0 /\ baseScope = 0
        /\ delivered = FALSE /\ invalid = FALSE /\ failed = FALSE
Full(v) == /\ v \in 1..2 /\ held' = v /\ baseScope' = scope
           /\ delivered' = TRUE /\ failed' = FALSE
           /\ UNCHANGED <<scope, invalid>>
Unchanged(v) == /\ v \in 1..2
                /\ IF held = v /\ baseScope = scope
                   THEN /\ delivered' = TRUE /\ failed' = FALSE
                        /\ UNCHANGED held
                   ELSE /\ held' = 0 /\ delivered' = FALSE /\ failed' = TRUE
                /\ UNCHANGED <<scope, baseScope, invalid>>
Delta == /\ IF held # 0 /\ baseScope = scope
            THEN /\ held' = 2 /\ delivered' = TRUE /\ failed' = FALSE
            ELSE /\ held' = 0 /\ delivered' = FALSE /\ failed' = TRUE
         /\ UNCHANGED <<scope, baseScope, invalid>>
ChangeScope == /\ scope = 0 /\ scope' = 1 /\ held' = 0
               /\ delivered' = FALSE /\ failed' = FALSE
               /\ UNCHANGED <<baseScope, invalid>>
Next == (\E v \in 1..2 : Full(v) \/ Unchanged(v)) \/ Delta \/ ChangeScope
Spec == Init /\ [][Next]_vars
UsableBase == delivered => held # 0 /\ baseScope = scope
RefusalYieldsNothing == failed => ~delivered
=============================================================================
