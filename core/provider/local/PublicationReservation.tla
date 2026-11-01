---- MODULE PublicationReservation ----
\* sqlPublisher reserve/submit/release: reserve exact block bytes without a
\* World or provider writer lock; consume the claim once at admission. A World
\* candidate remains held while its writer lock is released. If replay's save
\* grew meanwhile, release and reserve again outside that lock. Cancellation
\* releases an unconsumed claim. A failed original base rejects the candidate.
\* Bounds use two publications and two bytes; each prepared write is one byte.
\* stale captures failure already visible at writer reacquisition. Later
\* failure may still admit a dependent publication; SQLPrimary.tla checks its
\* failed receipt, head CAS and restart durability.
EXTENDS Naturals
CONSTANTS HoldWhileWaiting, CheckBase, BoundPreparation
VARIABLES pending, reserved, actor, lock, canceled, failed, admitted, badBase, candidates, stale
vars == <<pending, reserved, actor, lock, canceled, failed, admitted, badBase, candidates, stale>>
Init == /\ pending \in 0..2 /\ reserved = 0 /\ actor = "prepare"
        /\ lock = TRUE /\ canceled = FALSE /\ failed = FALSE /\ admitted = FALSE /\ badBase = FALSE /\ candidates = 1 /\ stale = FALSE
Prepare == /\ actor = "prepare" /\ actor' = "reserve"
           /\ lock' = HoldWhileWaiting
           /\ UNCHANGED <<pending, reserved, canceled, failed, admitted, badBase, candidates, stale>>
Reserve == /\ actor = "reserve" /\ pending + reserved < 2 /\ ~canceled
           /\ reserved' = reserved + 1 /\ actor' = "locked" /\ lock' = TRUE /\ stale' = failed
           /\ UNCHANGED <<pending, canceled, failed, admitted, badBase, candidates>>
Cancel == /\ actor \in {"reserve", "locked"} /\ ~canceled
          /\ canceled' = TRUE
          /\ UNCHANGED <<pending, reserved, actor, lock, failed, admitted, badBase, candidates, stale>>
FailBase == /\ ~failed /\ failed' = TRUE
            /\ UNCHANGED <<pending, reserved, actor, lock, canceled, admitted, badBase, candidates, stale>>
Submit == /\ actor = "locked" /\ ~canceled /\ (CheckBase => ~stale)
          /\ reserved' = reserved - 1 /\ pending' = pending + 1
          /\ actor' = "done" /\ lock' = FALSE /\ admitted' = TRUE /\ badBase' = stale /\ candidates' = 0
          /\ UNCHANGED <<canceled, failed, stale>>
Drop == /\ ((actor = "locked" /\ (canceled \/ failed))
             \/ (actor = "reserve" /\ canceled))
        /\ reserved' = 0 /\ candidates' = 0 /\ actor' = "done" /\ lock' = FALSE
        /\ UNCHANGED <<pending, canceled, failed, admitted, badBase, stale>>
Settle == /\ pending > 0 /\ pending' = pending - 1
          /\ UNCHANGED <<reserved, actor, lock, canceled, failed, admitted, badBase, candidates, stale>>
AnotherCandidate == /\ ~BoundPreparation /\ candidates = 1 /\ actor = "reserve" /\ ~lock
                    /\ candidates' = 2
                    /\ UNCHANGED <<pending, reserved, actor, lock, canceled, failed, admitted, badBase, stale>>
Next == AnotherCandidate \/ Prepare \/ Reserve \/ Cancel \/ FailBase \/ Submit \/ Drop \/ Settle \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
Bounded == pending + reserved <= 2
NoWaitUnderLock == actor = "reserve" => ~lock
NoFailedBase == ~badBase
NoCandidateHerd == candidates <= 1
Released == actor = "done" => reserved = 0
====
