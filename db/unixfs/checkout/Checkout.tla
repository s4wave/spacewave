----------------------------- MODULE Checkout -----------------------------
EXTENDS Naturals, FiniteSets
CONSTANT Design
\* Two Core processes sharing one destination; the same lease covers checking,
\* prefetch, assembly, publication and cleanup. Eviction probes that lease.
\* A complete checkout survives a process crash; private staging does not
\* count as publication. Power-loss durability and live-session eviction policy
\* are outside this recoverable-download model.
VARIABLES pc, owner, cache, published, refetched
vars == <<pc, owner, cache, published, refetched>>
Processes == {"oldCore", "newCore"}
Init == /\ pc = [p \in Processes |-> "idle"] /\ owner = "none"
        /\ cache = FALSE /\ published = FALSE /\ refetched = FALSE
Acquire(p) == /\ pc[p] = "idle"
              /\ (owner = "none" \/ Design = "unlocked")
              /\ owner' = p /\ pc' = [pc EXCEPT ![p] = "checking"]
              /\ UNCHANGED <<cache, published, refetched>>
Check(p) == /\ pc[p] = "checking"
            /\ pc' = [pc EXCEPT ![p] = IF published THEN "cleanup" ELSE "fetching"]
            /\ UNCHANGED <<owner, cache, published, refetched>>
Fetch(p) == /\ pc[p] = "fetching" /\ cache' = TRUE
            /\ pc' = [pc EXCEPT ![p] = "assembling"]
            /\ UNCHANGED <<owner, published, refetched>>
Copy(p) == /\ pc[p] = "assembling"
           /\ refetched' = (refetched \/ ~cache)
           /\ pc' = [pc EXCEPT ![p] = "verifying"]
           /\ UNCHANGED <<owner, cache, published>>
Verify(p) == /\ pc[p] = "verifying" /\ pc' = [pc EXCEPT ![p] = "publishing"]
             /\ UNCHANGED <<owner, cache, published, refetched>>
Publish(p) == /\ pc[p] = "publishing" /\ published' = TRUE
              /\ pc' = [pc EXCEPT ![p] = "cleanup"]
              /\ UNCHANGED <<owner, cache, refetched>>
Cleanup(p) == /\ pc[p] = "cleanup" /\ cache' = FALSE
              /\ pc' = [pc EXCEPT ![p] = "release"]
              /\ UNCHANGED <<owner, published, refetched>>
Release(p) == /\ pc[p] = "release" /\ owner' = "none"
              /\ pc' = [pc EXCEPT ![p] = "done"]
              /\ UNCHANGED <<cache, published, refetched>>
Crash(p) == /\ pc[p] \notin {"idle", "done"}
            /\ pc' = [pc EXCEPT ![p] = "done"]
            /\ owner' = IF owner = p THEN "none" ELSE owner
            /\ UNCHANGED <<cache, published, refetched>>
Evict == /\ owner = "none" /\ cache' = FALSE
         /\ UNCHANGED <<pc, owner, published, refetched>>
Next == \/ \E p \in Processes : Acquire(p) \/ Check(p) \/ Fetch(p) \/ Copy(p)
             \/ Verify(p) \/ Publish(p) \/ Cleanup(p) \/ Release(p) \/ Crash(p)
        \/ Evict \/ UNCHANGED vars
Writers == {p \in Processes : pc[p] \notin {"idle", "done"}}
OneWriter == Cardinality(Writers) <= 1
FetchedBlocksSurvive == ~refetched
LeaseCoversWork == \A p \in Writers : owner = p
Spec == Init /\ [][Next]_vars
=============================================================================
