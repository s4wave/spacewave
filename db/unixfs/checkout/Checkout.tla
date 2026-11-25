----------------------------- MODULE Checkout -----------------------------
EXTENDS Naturals, FiniteSets
CONSTANT Design
\* Two Core processes sharing one destination; the same lease covers checking,
\* prefetch, assembly, publication and cleanup. Eviction probes that lease.
\* A complete checkout survives a process crash; private staging does not
\* count as publication. Another program, or an older engine, may change a
\* published checkout's files; changed means they no longer match the
\* fingerprint recorded at publication, and a reuse writes such a checkout
\* again instead of returning it. Power-loss durability and live-session
\* eviction policy are outside this recoverable-download model.
VARIABLES pc, owner, cache, published, refetched, changed, served
vars == <<pc, owner, cache, published, refetched, changed, served>>
Processes == {"oldCore", "newCore"}
Init == /\ pc = [p \in Processes |-> "idle"] /\ owner = "none"
        /\ cache = FALSE /\ published = FALSE /\ refetched = FALSE
        /\ changed = FALSE /\ served = FALSE
Acquire(p) == /\ pc[p] = "idle"
              /\ (owner = "none" \/ Design = "unlocked")
              /\ owner' = p /\ pc' = [pc EXCEPT ![p] = "checking"]
              /\ UNCHANGED <<cache, published, refetched, changed, served>>
Reusable == published /\ ~changed
Check(p) == /\ pc[p] = "checking"
            /\ pc' = [pc EXCEPT ![p] = IF Reusable THEN "cleanup" ELSE "fetching"]
            /\ served' = (served \/ (Reusable /\ changed))
            /\ UNCHANGED <<owner, cache, published, refetched, changed>>
Change == /\ published /\ changed' = TRUE
          /\ UNCHANGED <<pc, owner, cache, published, refetched, served>>
Fetch(p) == /\ pc[p] = "fetching" /\ cache' = TRUE
            /\ pc' = [pc EXCEPT ![p] = "assembling"]
            /\ UNCHANGED <<owner, published, refetched, changed, served>>
Copy(p) == /\ pc[p] = "assembling"
           /\ refetched' = (refetched \/ ~cache)
           /\ pc' = [pc EXCEPT ![p] = "verifying"]
           /\ UNCHANGED <<owner, cache, published, changed, served>>
Verify(p) == /\ pc[p] = "verifying" /\ pc' = [pc EXCEPT ![p] = "publishing"]
             /\ UNCHANGED <<owner, cache, published, refetched, changed, served>>
Publish(p) == /\ pc[p] = "publishing" /\ published' = TRUE /\ changed' = FALSE
              /\ pc' = [pc EXCEPT ![p] = "cleanup"]
              /\ UNCHANGED <<owner, cache, refetched, served>>
Cleanup(p) == /\ pc[p] = "cleanup" /\ cache' = FALSE
              /\ pc' = [pc EXCEPT ![p] = "release"]
              /\ UNCHANGED <<owner, published, refetched, changed, served>>
Release(p) == /\ pc[p] = "release" /\ owner' = "none"
              /\ pc' = [pc EXCEPT ![p] = "done"]
              /\ UNCHANGED <<cache, published, refetched, changed, served>>
Crash(p) == /\ pc[p] \notin {"idle", "done"}
            /\ pc' = [pc EXCEPT ![p] = "done"]
            /\ owner' = IF owner = p THEN "none" ELSE owner
            /\ UNCHANGED <<cache, published, refetched, changed, served>>
Evict == /\ owner = "none" /\ cache' = FALSE
         /\ UNCHANGED <<pc, owner, published, refetched, changed, served>>
Next == \/ \E p \in Processes : Acquire(p) \/ Check(p) \/ Fetch(p) \/ Copy(p)
             \/ Verify(p) \/ Publish(p) \/ Cleanup(p) \/ Release(p) \/ Crash(p)
        \/ Evict \/ Change \/ UNCHANGED vars
Writers == {p \in Processes : pc[p] \notin {"idle", "done"}}
OneWriter == Cardinality(Writers) <= 1
FetchedBlocksSurvive == ~refetched
ReuseIsWritten == ~served
LeaseCoversWork == \A p \in Writers : owner = p
Spec == Init /\ [][Next]_vars
=============================================================================
