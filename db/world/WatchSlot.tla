----------------------------- MODULE WatchSlot -----------------------------
\* WatchSlot.Take/Put/Close, called by Engine.subscribeChanges and a view's
\* deferred Close. Other callers that fail Take own independent subscriptions.
\* Two holder identities bound the model; a returning call eventually puts
\* its holder under fairness. Holder ReleaseWatch does no slot re-entry.
EXTENDS Naturals, FiniteSets
VARIABLES closed, busy, held, active, released, allocated
vars == <<closed, busy, held, active, released, allocated>>
Init == /\ closed = FALSE /\ busy = FALSE /\ held = 0 /\ active = 0
        /\ released = {} /\ allocated = 0
Take == /\ ~closed /\ ~busy
        /\ (held # 0 \/ allocated < 2)
        /\ busy' = TRUE /\ held' = 0
        /\ active' = IF held # 0 THEN held ELSE allocated + 1
        /\ allocated' = IF held # 0 THEN allocated ELSE allocated + 1
        /\ UNCHANGED <<closed, released>>
Put == /\ busy /\ busy' = FALSE /\ active' = 0
       /\ IF closed
          THEN /\ held' = 0 /\ released' = released \cup {active}
          ELSE /\ held' = active /\ UNCHANGED released
       /\ UNCHANGED <<closed, allocated>>
Close == /\ ~closed /\ closed' = TRUE /\ held' = 0
         /\ released' = IF held # 0 THEN released \cup {held} ELSE released
         /\ UNCHANGED <<busy, active, allocated>>
Next == Take \/ Put \/ Close \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
FairSpec == Spec /\ WF_vars(Put)
ClosedEmpty == closed => held = 0
LiveHolder == held # 0 => held \notin released
ClosedReleases == closed ~> (~busy /\ held = 0)
=============================================================================
