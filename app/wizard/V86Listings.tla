---- MODULE V86Listings ----
\* The V86 wizard's image step reads the Space's images and VMs before it
\* seeds a default source. The image read is a watch: its first read yields a
\* partial listing after each page and ends in the complete listing. The
\* watch can drop at any point; it then reconnects by itself, reading again
\* from the first page, until its reconnect cap stops it. The VM listing is a
\* single read that lands or fails.
\*
\* The step derives what it shows from what was read and the state of each
\* read. While the watch reconnects the read still runs: before the listing
\* is complete loading stays, and after it the step stays quiet. A stopped
\* image read before the listing is complete shows the reason with Retry and
\* keeps the rows already read; after it, a notice with Retry sits above the
\* rows, which may be out of date. The loading card names
\* the listing still being read. The default source is seeded once, only from
\* both listings read to completion.

VARIABLES
  got,   \* what the image read has yielded: "none", "partial" or "complete"
  read,  \* the image watch: "running", "reconnecting" or "stopped"
  vms,   \* the VM listing: "loading", "read" or "failed"
  seed   \* the default source: "no", "ok", or "bad" if seeded from less
vars == <<got, read, vms, seed>>

\* ImageView is what the step shows for the image listing.
ImageView == IF read = "stopped" /\ got # "complete" THEN "failed"
             ELSE IF got = "complete" THEN "ready"
             ELSE "loading"

\* Notice is set while a complete image listing's watch is stopped.
Notice == got = "complete" /\ read = "stopped"

\* Card is the loading card: the listing it names, or "none".
Card == IF ImageView = "failed" \/ vms = "failed" THEN "none"
        ELSE IF ImageView = "loading" THEN "images"
        ELSE IF vms = "loading" THEN "vms"
        ELSE "none"

Shown ==
  (IF ImageView = "failed" \/ Notice THEN {"retryImages"} ELSE {})
    \cup (IF vms = "failed" THEN {"retryVms"} ELSE {})

TypeOK ==
  /\ got \in {"none", "partial", "complete"}
  /\ read \in {"running", "reconnecting", "stopped"}
  /\ vms \in {"loading", "read", "failed"}
  /\ seed \in {"no", "ok", "bad"}

Init == got = "none" /\ read = "running" /\ vms = "loading" /\ seed = "no"

\* A page of the first read arrives before the listing is complete.
ReadPage ==
  /\ read = "running"
  /\ got \in {"none", "partial"}
  /\ got' = "partial"
  /\ UNCHANGED <<read, vms, seed>>

\* The first read completes the listing.
ReadEnds ==
  /\ read = "running"
  /\ got \in {"none", "partial"}
  /\ got' = "complete"
  /\ UNCHANGED <<read, vms, seed>>

\* The watch drops, before or after any page, or after the listing is
\* complete; the rows read so far stay.
Drop ==
  /\ read = "running"
  /\ read' = "reconnecting"
  /\ UNCHANGED <<got, vms, seed>>

\* The watch reconnects by itself, before or after the listing is complete.
\* Its new first read may yield a page before it completes again, so a
\* complete listing can drop back to partial.
Reconnect ==
  /\ read = "reconnecting"
  /\ read' = "running"
  /\ got' \in IF got = "complete" THEN {"complete", "partial"} ELSE {got}
  /\ UNCHANGED <<vms, seed>>

\* The watch reaches its reconnect cap and stops.
Stop ==
  /\ read = "reconnecting"
  /\ read' = "stopped"
  /\ UNCHANGED <<got, vms, seed>>

RetryImages ==
  /\ "retryImages" \in Shown
  /\ read' = "running"
  /\ UNCHANGED <<got, vms, seed>>

VmsRead ==
  /\ vms = "loading"
  /\ vms' \in {"read", "failed"}
  /\ UNCHANGED <<got, read, seed>>

RetryVms ==
  /\ "retryVms" \in Shown
  /\ vms' = "loading"
  /\ UNCHANGED <<got, read, seed>>

Seed ==
  /\ seed = "no"
  /\ ImageView = "ready"
  /\ vms = "read"
  /\ seed' = IF got = "complete" /\ vms = "read" THEN "ok" ELSE "bad"
  /\ UNCHANGED <<got, read, vms>>

Next ==
  \/ ReadPage \/ ReadEnds \/ Drop \/ Reconnect \/ Stop \/ RetryImages
  \/ VmsRead \/ RetryVms \/ Seed

\* Loading shows only while its listing is read, so a failure never stalls
\* on it, and it names that listing.
LoadingIsTrue ==
  /\ Card = "images" => read # "stopped" /\ got # "complete"
  /\ Card = "vms" => vms = "loading" /\ got = "complete"

\* A failure shows its next step.
FailureHasNextStep ==
  /\ ImageView = "failed" => "retryImages" \in Shown
  /\ vms = "failed" => "retryVms" \in Shown

\* A stopped image watch is never quiet: before the listing is complete it is
\* a failure, after it a notice over rows that may be out of date.
StopIsShown == read = "stopped" => ImageView = "failed" \/ Notice

\* The default comes only from listings read to completion.
SeedOnlyFromRead == seed # "bad"
====
