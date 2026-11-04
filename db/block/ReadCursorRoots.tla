----------------------- MODULE ReadCursorRoots -----------------------
EXTENDS Naturals
\* World opens immutable revisions; HEAD publication never retargets an open
\* reader. FollowRef and Cursor copy references before exposing mutable owners.
CONSTANT OwnRefs
VARIABLES head, caller, reader, expected, opened, materialized, changed, bad
vars == <<head, caller, reader, expected, opened, materialized, changed, bad>>
Init == /\ head = 0 /\ caller = 0 /\ reader = 0 /\ expected = 0
        /\ opened = FALSE /\ materialized = 0 /\ changed = FALSE /\ bad = FALSE
Open == /\ ~opened /\ opened' = TRUE
        /\ caller' = head /\ reader' = head /\ expected' = head
        /\ UNCHANGED <<head, materialized, changed, bad>>
Publish == /\ head = 0 /\ head' = 1
           /\ UNCHANGED <<caller, reader, expected, opened, materialized, changed, bad>>
MutateCaller == /\ opened /\ ~changed
                /\ changed' = TRUE /\ caller' = 1 - caller
                /\ reader' = IF OwnRefs THEN reader ELSE 1 - caller
                /\ UNCHANGED <<head, expected, opened, materialized, bad>>
Materialize == /\ opened /\ materialized' = reader
               /\ UNCHANGED <<head, caller, reader, expected, opened, changed, bad>>
MutateCursor == /\ opened /\ materialized' = 1 - materialized
                /\ reader' = IF OwnRefs THEN reader ELSE 1 - materialized
                /\ UNCHANGED <<head, caller, expected, opened, changed, bad>>
Read == /\ opened /\ bad' = (bad \/ reader # expected)
        /\ UNCHANGED <<head, caller, reader, expected, opened, materialized, changed>>
Next == Open \/ Publish \/ MutateCaller \/ Materialize \/ MutateCursor \/ Read
StableSnapshot == ~bad
Spec == Init /\ [][Next]_vars
======================================================================
