------------------------- MODULE DownloadPlatform -------------------------
EXTENDS Naturals

CONSTANT Platform
VARIABLES hydrated, rendered
vars == <<hydrated, rendered>>
Init == /\ hydrated = FALSE /\ rendered = "neutral"
Hydrate == /\ ~hydrated /\ hydrated' = TRUE /\ rendered' = Platform
Next == Hydrate \/ UNCHANGED vars
ConsistentHydration == ~hydrated => rendered = "neutral"
DetectedBuild == hydrated => rendered = Platform
Spec == Init /\ [][Next]_vars
=============================================================================
