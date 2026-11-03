---- MODULE Replacement ----
\* Filesystem publication validates staging before displacing the old tree.
\* The destination lease excludes another writer throughout these renames.
\* A process crash can retain a displaced copy; cache eviction can reclaim it.
EXTENDS Naturals
VARIABLES current, previous, staging, phase
vars == <<current,previous,staging,phase>>
Init == /\ current = "old" /\ previous = FALSE /\ staging = TRUE /\ phase = "ready"
Displace == /\ phase = "ready" /\ previous' = TRUE /\ current' = "none"
            /\ phase' = "displaced" /\ UNCHANGED staging
Install == /\ phase = "displaced" /\ staging /\ current' = "new"
           /\ staging' = FALSE /\ phase' = "installed" /\ UNCHANGED previous
Refuse == /\ phase = "displaced" /\ current' = "old" /\ previous' = FALSE
          /\ phase' = "failed" /\ UNCHANGED staging
Cleanup == /\ phase = "installed" /\ previous' = FALSE /\ phase' = "done"
           /\ UNCHANGED <<current,staging>>
Crash == /\ phase \in {"ready","displaced","installed"} /\ phase' = "crashed"
         /\ UNCHANGED <<current,previous,staging>>
Next == Displace \/ Install \/ Refuse \/ Cleanup \/ Crash
Spec == Init /\ [][Next]_vars
DataRetained == current # "none" \/ previous
PublishedIsComplete == current = "new" => ~staging
=============================================================================
