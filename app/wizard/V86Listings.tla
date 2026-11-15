---- MODULE V86Listings ----
\* The V86 wizard's image step reads the Space's images and VMs before it
\* seeds a default source. A first read yields a partial listing after each
\* page and ends in the complete listing; it can fail at any point. The step
\* derives what it shows from what was read and whether the read failed:
\* loading shows while the listing is incomplete and the read runs, beside
\* any rows already read; a failure before the listing is complete shows the
\* reason with Retry, which reads again, and keeps the rows already read; a
\* failure after it is complete stays quiet while the watch reopens by
\* itself. The default source is seeded once, only from a complete listing.

VARIABLES
  got,    \* what the read has yielded: "none", "partial" or "complete"
  failed, \* whether the read has failed and not reopened
  seeded  \* whether the default source was seeded
vars == <<got, failed, seeded>>

\* View is what the step shows.
View == IF failed /\ got # "complete" THEN "failed"
        ELSE IF got = "complete" THEN "ready"
        ELSE "loading"

Shown == IF View = "failed" THEN {"retry"} ELSE {}

TypeOK ==
  /\ got \in {"none", "partial", "complete"}
  /\ failed \in BOOLEAN
  /\ seeded \in BOOLEAN

Init == got = "none" /\ failed = FALSE /\ seeded = FALSE

\* A page of the first read arrives before the listing is complete.
ReadPage ==
  /\ ~failed
  /\ got \in {"none", "partial"}
  /\ got' = "partial"
  /\ UNCHANGED <<failed, seeded>>

\* The first read completes the listing.
ReadEnds ==
  /\ ~failed
  /\ got \in {"none", "partial"}
  /\ got' = "complete"
  /\ UNCHANGED <<failed, seeded>>

\* The read fails, before or after any page, or after the listing is
\* complete; the rows read so far stay.
Fail ==
  /\ ~failed
  /\ failed' = TRUE
  /\ UNCHANGED <<got, seeded>>

Retry ==
  /\ "retry" \in Shown
  /\ failed' = FALSE
  /\ UNCHANGED <<got, seeded>>

\* A watch that failed after a complete listing reopens by itself.
Reopen ==
  /\ failed
  /\ got = "complete"
  /\ failed' = FALSE
  /\ UNCHANGED <<got, seeded>>

Seed ==
  /\ ~seeded
  /\ View = "ready"
  /\ seeded' = TRUE
  /\ UNCHANGED <<got, failed>>

Next == ReadPage \/ ReadEnds \/ Fail \/ Retry \/ Reopen \/ Seed

\* A failure after a partial listing shows, rather than loading forever.
PartialFailureIsShown == failed /\ got = "partial" => View = "failed"

\* Loading shows only while the read runs, so a failure never stalls on it.
LoadingIsTrue == View = "loading" => ~failed /\ got # "complete"

\* A failure shows its next step.
FailureHasNextStep == View = "failed" => "retry" \in Shown

\* The default comes only from listings read to completion.
SeedOnlyFromRead == seeded => got = "complete"
====
