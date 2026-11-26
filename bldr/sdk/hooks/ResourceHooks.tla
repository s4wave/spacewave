--------------------------- MODULE ResourceHooks ---------------------------
\* This pin models the first-value deadline of useStreamingResource.
\* An attempt may give a value, end empty, fail, or miss its deadline.
\* A deadline aborts the current attempt and schedules a reopen without
\* changing what the hook shows. Delays are abstracted to events. The
\* deadline multiplier is 2^stalls, capped at 8, and resets on a value.

EXTENDS Naturals

VARIABLES phase, value, yielded, stalls

vars == <<phase, value, yielded, stalls>>

TypeOK ==
  /\ phase \in {"open", "backoff", "settled"}
  /\ value \in 0..2
  /\ yielded \in BOOLEAN
  /\ stalls \in 0..3

Init ==
  /\ phase = "open"
  /\ value = 0
  /\ yielded = FALSE
  /\ stalls = 0

Emit ==
  /\ phase = "open"
  /\ value' \in 1..2
  /\ yielded' = TRUE
  /\ stalls' = 0
  /\ UNCHANGED phase

\* A retry over a value opens an attempt that has yet to yield.
Retry ==
  /\ phase # "backoff"
  /\ phase' = "open"
  /\ yielded' = FALSE
  /\ UNCHANGED <<value, stalls>>

End ==
  /\ phase = "open"
  /\ phase' = "settled"
  /\ value' = IF yielded THEN value ELSE 0
  /\ UNCHANGED <<yielded, stalls>>

Fail ==
  /\ phase = "open"
  /\ phase' = "settled"
  /\ value' = 0
  /\ UNCHANGED <<yielded, stalls>>

Stall ==
  /\ phase = "open" /\ ~yielded
  /\ phase' = "backoff"
  /\ stalls' = IF stalls < 3 THEN stalls + 1 ELSE 3
  /\ UNCHANGED <<value, yielded>>

Reopen ==
  /\ phase = "backoff"
  /\ phase' = "open"
  /\ UNCHANGED <<value, yielded, stalls>>

Next == Emit \/ Retry \/ End \/ Fail \/ Stall \/ Reopen

OwnFailureKeepsContent ==
  [][((Stall \/ Reopen) /\ value # 0) => value' = value]_vars

NoDeadlineAfterYield == yielded => (phase # "backoff" /\ stalls = 0)
=============================================================================
