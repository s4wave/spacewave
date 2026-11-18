----------------------------- MODULE ColdPackReads -----------------------------
EXTENDS Naturals, FiniteSets, TLC

(***************************************************************************
 The mounted manifest names one pack, whose exact index contains Live.
 Absent shares Live's persisted suffix and passes its bloom. A range read
 can fail or return not-found; the latter fences cached evidence for the
 mount. A crash discards all in-memory proof. Bytes and CRC checks are
 abstracted to verified reads and a durable record set, with a torn append
 represented by the undurable queue lost on crash. Existence never joins
 any foreground/background read or waits for the log. The model records its
 evidence at the linearization point; later deletion cannot change that
 recorded observation. The operator must not remove a named pack that is
 still relied on: unobserved deletion is deliberately not modeled as gone.
***************************************************************************)
CONSTANTS Live, Absent, PromoteCap, QueueCap, TrustSuffix, IgnoreGone
VARIABLES ready, enabled, present, gone, tail, durable, verified, queue,
          answer
vars == <<ready, enabled, present, gone, tail, durable, verified, queue, answer>>

Keys == {Live, Absent}
Actual == {Live}

Init == /\ ready = FALSE /\ enabled = TRUE /\ present = TRUE /\ gone = FALSE
        /\ tail = {} /\ durable = {} /\ verified = {} /\ queue = {}
        /\ answer = [key |-> Absent, yes |-> FALSE, wasGone |-> FALSE]

Preload == /\ ~ready /\ ready' = TRUE
           /\ UNCHANGED <<enabled, present, gone, tail, durable, verified, queue, answer>>

Toggle == /\ enabled' = ~enabled
          /\ UNCHANGED <<ready, present, gone, tail, durable, verified, queue, answer>>

Delete == /\ present /\ present' = FALSE
          /\ UNCHANGED <<ready, enabled, gone, tail, durable, verified, queue, answer>>

Observed404 == /\ ready /\ enabled /\ ~present /\ ~gone /\ gone' = TRUE
               /\ UNCHANGED <<ready, enabled, present, tail, durable, verified, queue, answer>>

Fill == /\ ready /\ enabled /\ present /\ ~gone /\ tail' = Actual
        /\ UNCHANGED <<ready, enabled, present, gone, durable, verified, queue, answer>>

Evict == /\ tail # {} /\ tail' = {}
         /\ UNCHANGED <<ready, enabled, present, gone, durable, verified, queue, answer>>

Read(k) == /\ ready /\ enabled /\ present /\ ~gone /\ k \in Actual
           /\ verified' = IF k \in durable THEN verified \cup {k} ELSE verified
           /\ queue' = IF k \notin durable /\ Cardinality(queue) < QueueCap
                       THEN queue \cup {k} ELSE queue
           /\ UNCHANGED <<ready, enabled, present, gone, tail, durable, answer>>

SyncLog(k) == /\ k \in queue /\ Cardinality(durable) < PromoteCap
              /\ durable' = durable \cup {k} /\ verified' = verified \cup {k}
              /\ queue' = queue \ {k}
              /\ UNCHANGED <<ready, enabled, present, gone, tail, answer>>

Crash == /\ ready' = FALSE /\ gone' = FALSE /\ tail' = {}
         /\ verified' = {} /\ queue' = {}
         /\ UNCHANGED <<enabled, present, durable, answer>>

Exists(k) == /\ answer' = [key |-> k, wasGone |-> gone,
                            yes |-> ready /\ enabled /\ (~gone \/ IgnoreGone) /\
                                    (k \in (tail \cup verified) \/ (TrustSuffix /\ durable # {}))]
             /\ UNCHANGED <<ready, enabled, present, gone, tail, durable, verified, queue>>

Next == Preload \/ Toggle \/ Delete \/ Observed404 \/ Fill \/ Evict \/ Crash
        \/ \E k \in Keys: Read(k) \/ SyncLog(k) \/ Exists(k)

Spec == Init /\ [][Next]_vars

NoFalseYes == answer.yes => answer.key \in Actual
NoGoneYes == answer.yes => ~answer.wasGone
VerifiedIsDurable == verified \subseteq durable /\ durable \subseteq Actual
LogAndQueueBounded == Cardinality(durable) <= PromoteCap /\ Cardinality(queue) <= QueueCap
=============================================================================
