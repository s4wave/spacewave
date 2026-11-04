--------------------------- MODULE StyleParity ---------------------------
EXTENDS Integers, FiniteSets

\* Unstyled highlights can retain a pre-transition color; authored highlight
\* properties are observed separately from the authoritative ordinary style.
\* One asynchronous resource arrival changes DOM content or attributes.
\* Its revision represents either change; repeated arrivals compose the same retry.
CONSTANTS ObserveImplicitHighlight, ResampleAfterPaint, RequireStableTree
VARIABLES phase, revision, highlightRevision, observed, pixels,
          treeRevision, beforeTree, imageTree, observedTree
vars == <<phase, revision, highlightRevision, observed, pixels,
          treeRevision, beforeTree, imageTree, observedTree>>
treeVars == <<treeRevision, beforeTree, imageTree, observedTree>>
Init == /\ phase = "loading" /\ revision = 0
        /\ highlightRevision = 0 /\ observed = {} /\ pixels = 0
        /\ treeRevision = 0 /\ beforeTree = 0 /\ imageTree = 0 /\ observedTree = 0
AssetsReady == /\ phase = "loading" /\ phase' = "settling"
               /\ UNCHANGED <<revision, highlightRevision, observed, pixels, treeVars>>
Freeze == /\ phase = "settling" /\ phase' = "frozen" /\ revision' = 1
          /\ beforeTree' = treeRevision
          /\ UNCHANGED <<highlightRevision, observed, pixels, treeRevision, imageTree, observedTree>>
\* Screenshot cleanup restores animation declarations after painting.
Paint == /\ phase = "frozen" /\ phase' = "painted"
         /\ pixels' = revision /\ revision' = 0 /\ imageTree' = treeRevision
         /\ UNCHANGED <<highlightRevision, observed, treeRevision, beforeTree, observedTree>>
Resample == /\ phase = "painted" /\ phase' = "ready"
            /\ revision' = IF ResampleAfterPaint THEN 1 ELSE revision
            /\ UNCHANGED <<pixels, highlightRevision, observed, treeVars>>
Resolve == /\ phase = "ready" /\ phase' = "resolved"
           /\ observed' = {revision} \cup
                 (IF ObserveImplicitHighlight THEN {highlightRevision} ELSE {})
           /\ observedTree' = treeRevision
           /\ UNCHANGED <<revision, highlightRevision, pixels, treeRevision, beforeTree, imageTree>>
ResourceArrives == /\ phase \in {"frozen", "painted", "ready"}
                   /\ treeRevision = 0 /\ treeRevision' = 1
                   /\ UNCHANGED <<phase, revision, highlightRevision, observed, pixels,
                                  beforeTree, imageTree, observedTree>>
\* Diagnostics and publication consume only an image whose tree stayed stable.
Observe == /\ phase = "resolved"
           /\ (~RequireStableTree \/ beforeTree = observedTree)
           /\ phase' = "observed"
           /\ UNCHANGED <<revision, highlightRevision, observed, pixels, treeVars>>
Recapture == /\ phase = "resolved" /\ RequireStableTree
             /\ beforeTree # observedTree /\ phase' = "settling"
             /\ UNCHANGED <<revision, highlightRevision, observed, pixels, treeVars>>
Capture == /\ phase = "observed" /\ phase' = "captured"
           /\ UNCHANGED <<revision, highlightRevision, observed, pixels, treeVars>>
\* The state deadline and observer failures retain diagnostics but publish no index row.
Fail == /\ phase \notin {"captured", "failed"} /\ phase' = "failed"
        /\ UNCHANGED <<revision, highlightRevision, observed, pixels, treeVars>>
Next == AssetsReady \/ Freeze \/ Paint \/ Resample \/ Resolve \/ ResourceArrives
        \/ Observe \/ Recapture \/ Capture \/ Fail \/ UNCHANGED vars
TypeOK == /\ phase \in {"loading", "settling", "frozen", "painted", "ready", "resolved", "observed", "captured", "failed"}
          /\ revision \in 0..1 /\ pixels \in 0..1 /\ highlightRevision \in 0..1
          /\ observed \subseteq 0..1
          /\ treeRevision \in 0..1 /\ beforeTree \in 0..1
          /\ imageTree \in 0..1 /\ observedTree \in 0..1
SameSample == phase \in {"observed", "captured"} => observed = {pixels}
SameTree == phase \in {"observed", "captured"} => imageTree = observedTree
Spec == Init /\ [][Next]_vars
=============================================================================
