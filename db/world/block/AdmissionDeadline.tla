---- MODULE AdmissionDeadline ----
\* EngineTx.CommitBlockTransaction calls waitDurable after Admit and after
\* releasing the writer turn. Init starts there: spaceTurn.Admit pushed the
\* operation, and the adapter owns its payloads. opQueue.execute, replay, and
\* installHead advance it independently; a head for another operation may
\* arrive without proving this one. Only the Space Authority is modeled:
\* its WaitDecided and WaitDurable require the same published evidence.
\* Local Authority decides and makes durable in Admit. Lost reports from a
\* two-stage authority and its Head read are covered by the Go run-through.
\* Acknowledge and adapter-only writeOp do not call waitDurable.
\*
\* The timer abstracts 30 seconds to one enabled step. Fairness applies to
\* the timer and wakeup, not to driver progress or engine close. Either a
\* definitive answer or cancellation can win when both are ready.

VARIABLES requestCanceled, cause, work, result, laterHead
vars == <<requestCanceled, cause, work, result, laterHead>>

Init == /\ requestCanceled = FALSE
        /\ cause = "live"
        /\ work = "queued"
        /\ result = "waiting"
        /\ laterHead = FALSE

CancelRequest == /\ ~requestCanceled /\ result = "waiting"
                 /\ requestCanceled' = TRUE
                 /\ UNCHANGED <<cause, work, result, laterHead>>

TakeForQueueOperation == /\ work = "queued"
                         /\ work' = "taken"
                         /\ UNCHANGED <<requestCanceled, cause, result, laterHead>>
PlaceByReplay == /\ work = "taken"
                 /\ work' = "placed"
                 /\ UNCHANGED <<requestCanceled, cause, result, laterHead>>
PublishDecision == /\ work = "placed"
                   /\ work' \in {"applied", "refused", "superseded"}
                   /\ UNCHANGED <<requestCanceled, cause, result, laterHead>>
ObserveOtherHead == /\ ~laterHead
                    /\ laterHead' = TRUE
                    /\ UNCHANGED <<requestCanceled, cause, work, result>>

BudgetExpiry == /\ TRUE
                /\ cause = "live" /\ result = "waiting"
                /\ cause' = "deadline"
                /\ UNCHANGED <<requestCanceled, work, result, laterHead>>
EngineClose == /\ cause = "live" /\ result = "waiting"
               /\ cause' = "close"
               /\ UNCHANGED <<requestCanceled, work, result, laterHead>>

WaitProven == /\ result = "waiting"
              /\ work \in {"applied", "refused", "superseded"}
              /\ result' = work
              /\ UNCHANGED <<requestCanceled, cause, work, laterHead>>
WaitCanceled == /\ result = "waiting" /\ cause # "live"
                /\ result' = "unknown"
                /\ work' = IF cause = "close" /\ work = "queued"
                           THEN "removed" ELSE work
                /\ UNCHANGED <<requestCanceled, cause, laterHead>>

Done == result # "waiting" /\ UNCHANGED vars
Next == CancelRequest \/ TakeForQueueOperation \/ PlaceByReplay \/ PublishDecision
        \/ ObserveOtherHead \/ BudgetExpiry \/ EngineClose
        \/ WaitProven \/ WaitCanceled \/ Done
Spec == Init /\ [][Next]_vars
        /\ WF_vars(CancelRequest) /\ WF_vars(BudgetExpiry)
        /\ WF_vars(WaitProven \/ WaitCanceled)

ExactOutcome == result \in {"applied", "refused", "superseded"} => result = work
UnknownIsNotRequestCancel == result = "unknown" => cause # "live"
TimeoutKeepsWork == cause = "deadline" => work # "removed"
WaitEnds == <>(result # "waiting")
====
