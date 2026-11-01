---------------------------- MODULE StylexCss ----------------------------
EXTENDS Integers, FiniteSets

\* Vite starts a transform on its current source snapshot. Theme changes count
\* as source revisions. Resolution may finish after a later transform starts.
CONSTANT CommitLatest
VARIABLES revision, slot, pending, compiled, delivered, ready
vars == <<revision, slot, pending, compiled, delivered, ready>>
Init == /\ revision = 0 /\ slot = -1 /\ pending = {}
        /\ compiled = -1 /\ delivered = -1 /\ ready = FALSE
Edit == /\ revision < 2 /\ revision' = revision + 1
        /\ ready' = FALSE /\ compiled' = -1
        /\ UNCHANGED <<slot, pending, delivered>>
StartCompile == /\ slot # revision /\ slot' = revision
                /\ pending' = pending \cup {revision} /\ compiled' = -1
                /\ UNCHANGED <<revision, delivered, ready>>
FinishCompile(r) ==
    /\ r \in pending /\ pending' = pending \ {r}
    /\ compiled' = IF ~CommitLatest \/ r = slot THEN r ELSE compiled
    /\ UNCHANGED <<revision, slot, delivered, ready>>
CompileError(r) ==
    /\ r \in pending /\ pending' = pending \ {r}
    /\ UNCHANGED <<revision, slot, compiled, delivered, ready>>
LoadCss == /\ slot = revision /\ compiled # -1
           /\ delivered' = compiled /\ ready' = TRUE
           /\ UNCHANGED <<revision, slot, pending, compiled>>
Next == Edit \/ StartCompile \/ (\E r \in pending : FinishCompile(r) \/ CompileError(r))
        \/ LoadCss \/ UNCHANGED vars
TypeOK == /\ revision \in 0..2 /\ slot \in -1..2
          /\ pending \subseteq 0..2 /\ compiled \in -1..2
          /\ delivered \in -1..2 /\ ready \in BOOLEAN
CacheOwner == slot = revision /\ compiled # -1 => compiled = slot
CurrentCss == ready => delivered = revision
Spec == Init /\ [][Next]_vars
=============================================================================
