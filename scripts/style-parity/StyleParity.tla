--------------------------- MODULE StyleParity ---------------------------
EXTENDS Integers, FiniteSets

\* Chromium may retain a pre-transition color in an unstyled highlight. The
\* ordinary style is its authoritative default; authored highlight properties
\* are observed separately. Unused pseudo defaults are not painted properties.
CONSTANTS ObserveImplicitHighlight, ResampleAfterPaint
VARIABLES phase, revision, highlightRevision, observed, pixels
vars == <<phase, revision, highlightRevision, observed, pixels>>
Init == /\ phase = "loading" /\ revision = 0
        /\ highlightRevision = 0 /\ observed = {} /\ pixels = 0
AssetsReady == /\ phase = "loading" /\ phase' = "settling"
               /\ UNCHANGED <<revision, highlightRevision, observed, pixels>>
Freeze == /\ phase = "settling" /\ phase' = "frozen" /\ revision' = 1
          /\ UNCHANGED <<highlightRevision, observed, pixels>>
\* Playwright captures the static frame, then its cleanup restores animation
\* declarations. The computed-style read must return to that same sample.
Paint == /\ phase = "frozen" /\ phase' = "painted"
         /\ pixels' = revision /\ revision' = 0
         /\ UNCHANGED <<highlightRevision, observed>>
Resample == /\ phase = "painted" /\ phase' = "ready"
            /\ revision' = IF ResampleAfterPaint THEN 1 ELSE revision
            /\ UNCHANGED <<pixels, highlightRevision, observed>>
Resolve == /\ phase = "ready" /\ phase' = "resolved"
           /\ observed' = {revision} \cup
                 (IF ObserveImplicitHighlight THEN {highlightRevision} ELSE {})
           /\ UNCHANGED <<revision, highlightRevision, pixels>>
Capture == /\ phase = "resolved" /\ phase' = "captured"
           /\ UNCHANGED <<revision, highlightRevision, observed, pixels>>
Next == AssetsReady \/ Freeze \/ Paint \/ Resample \/ Resolve \/ Capture \/ UNCHANGED vars
TypeOK == /\ phase \in {"loading", "settling", "frozen", "painted", "ready", "resolved", "captured"}
          /\ revision \in 0..1 /\ pixels \in 0..1 /\ highlightRevision \in 0..1
          /\ observed \subseteq 0..1
SameSample == phase = "captured" => observed = {pixels}
Spec == Init /\ [][Next]_vars
=============================================================================
