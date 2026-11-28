---------------------------- MODULE ChangeAdmission ----------------------------
EXTENDS Naturals
(* EngineTx registers candidate evidence before LocalAuthority.Admit. SetRoots
   may make the stored root readable before it returns; Head keeps the cached
   head while the local proposal is unsettled. After success the Authority
   confirms before publishing or waking its head watch. Committer return is a
   separate actor, as is feed planning. Space installHead confirms placed
   candidates under the lock that makes the head reportable. Rebased heads and
   imports carry no exact evidence, modeled by a different actualRoot.
   Storage locking, ring eviction and subscriber coalescing are left to their
   component specs and ChangeFeed; this model holds one retained transition.
   Failed durability abandons evidence even when a later external publication
   has the same root. No root comparison alone confirms an admission. *)
VARIABLES phase, stored, succeeded, confirmed, reported, returned, planned,
          exact, actualRoot
vars == <<phase, stored, succeeded, confirmed, reported, returned, planned,
          exact, actualRoot>>
Init == /\ phase = "proposed" /\ stored = FALSE /\ succeeded = FALSE
        /\ confirmed = FALSE /\ reported = FALSE /\ returned = FALSE
        /\ planned = FALSE /\ exact = FALSE /\ actualRoot \in {"candidate", "rebased"}
Store == /\ phase = "proposed" /\ stored' = TRUE /\ phase' = "swapped"
         /\ UNCHANGED <<succeeded, confirmed, reported, returned, planned, exact, actualRoot>>
Decide == /\ phase = "swapped" /\ phase' = "durable" /\ succeeded' = TRUE
          /\ confirmed' = (actualRoot = "candidate")
          /\ UNCHANGED <<stored, reported, returned, planned, exact, actualRoot>>
Fail == /\ phase \in {"proposed", "swapped"} /\ phase' = "failed"
        /\ UNCHANGED <<stored, succeeded, confirmed, reported, returned, planned, exact, actualRoot>>
(* Root reads cannot bypass the unsettled local proposal. A failed proposal
   may later share its roots with an external write, which still lacks evidence. *)
HeadRead == /\ stored /\ phase \in {"durable", "failed"} /\ ~reported
            /\ reported' = TRUE
            /\ UNCHANGED <<phase, stored, succeeded, confirmed, returned, planned, exact, actualRoot>>
Return == /\ phase \in {"durable", "failed"} /\ ~returned /\ returned' = TRUE
          /\ UNCHANGED <<phase, stored, succeeded, confirmed, reported, planned, exact, actualRoot>>
Feed == /\ reported /\ ~planned /\ planned' = TRUE /\ exact' = confirmed
        /\ UNCHANGED <<phase, stored, succeeded, confirmed, reported, returned, actualRoot>>
Done == /\ returned /\ (planned \/ ~stored) /\ UNCHANGED vars
Next == Store \/ Decide \/ Fail \/ HeadRead \/ Return \/ Feed \/ Done
Spec == Init /\ [][Next]_vars
ExactDurable == planned /\ succeeded /\ actualRoot = "candidate" => exact
EvidenceFromSuccess == confirmed => succeeded /\ actualRoot = "candidate"
NoFailedEvidence == planned /\ phase = "failed" => ~exact
NoPrematureReport == reported => phase # "swapped" /\ (succeeded => confirmed \/ actualRoot = "rebased")
=============================================================================
