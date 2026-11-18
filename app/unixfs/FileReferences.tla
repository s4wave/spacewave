---- MODULE FileReferences ----
\* The references strip in Spacewave's UnixFS file viewer: the declared
\* (world.ref) edges that point into the open file, such as the Observation
\* from the LlmSession that wrote it. Each entry opens its own viewer. The strip
\* follows the quad change feed, so a reference written later appears.

People == {"reader"}
Buttons == {"open", "more", "retry"}

VARIABLES load, refs, hasMore
vars == <<load, refs, hasMore>>

\* refs: what the newest page holds: "none", or referrers ("found"). Read
\* authority is per Space, so every referrer in a readable World is readable;
\* an untyped one, such as a placeholder, still opens.
Shown(p) ==
  IF load = "failed" THEN {"retry"}
  ELSE IF load = "loading" THEN {}
  ELSE (IF refs = "found" THEN {"open"} ELSE {})
       \cup (IF hasMore THEN {"more"} ELSE {})

Allowed(p, b) ==
  CASE b = "retry" -> load = "failed"
    [] b = "open"  -> load = "ready" /\ refs = "found"
    [] b = "more"  -> load = "ready" /\ hasMore

\* The strip hides itself when nothing points at the file; the hint is what
\* a reader is told instead. A failed read says why beside its Retry.
Hint(p) ==
  IF load = "loading" THEN "Reading references"
  ELSE IF load = "failed" THEN ""
  ELSE IF refs = "none" THEN "Nothing references this file"
  ELSE ""

Init == load = "loading" /\ refs = "none" /\ hasMore = FALSE
Loaded ==
  /\ load = "loading" /\ load' = "ready"
  /\ refs' \in {"none", "found"} /\ hasMore' \in BOOLEAN
Fail == load = "loading" /\ load' = "failed" /\ UNCHANGED <<refs, hasMore>>
Retry == "retry" \in Shown("reader") /\ load' = "loading" /\ UNCHANGED <<refs, hasMore>>
More ==
  /\ "more" \in Shown("reader")
  /\ refs' = "found" /\ hasMore' \in BOOLEAN
  /\ UNCHANGED load
\* A watched quad into the file commits, such as a new Observation.
Arrive ==
  /\ load = "ready" /\ refs = "none"
  /\ refs' = "found" /\ UNCHANGED <<load, hasMore>>
Open == "open" \in Shown("reader") /\ UNCHANGED vars
Next == Loaded \/ Fail \/ Retry \/ More \/ Arrive \/ Open

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck
====
