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
\*
\* With the deferred root move, Admit queues the admission's root move in
\* memory instead of storing it. Two blocks stand for the admission's blocks:
\* a payload, which the writer's stage holds and the queued move requires
\* held, and a World page, which the overflow stage holds. Landing the queue
\* stores the root that holds both and releases the overflow stage. The
\* writer's stage lives until waitDurable returns, which a deadline can make
\* happen before the move lands. Collect frees a block nothing holds. A crash
\* drops the queue and the opQueue and expires every stage lease.

VARIABLES requestCanceled, cause, work, result, laterHead,
          writerStage, overflowStage, move, root, payloadFreed, pageFreed,
          crashed
holdVars == <<writerStage, overflowStage, move, root, payloadFreed, pageFreed,
              crashed>>
vars == <<requestCanceled, cause, work, result, laterHead, holdVars>>

Init == /\ requestCanceled = FALSE
        /\ cause = "live"
        /\ work = "queued"
        /\ result = "waiting"
        /\ laterHead = FALSE
        /\ writerStage = TRUE
        /\ overflowStage = TRUE
        /\ move = "queued"
        /\ root = FALSE
        /\ payloadFreed = FALSE
        /\ pageFreed = FALSE
        /\ crashed = FALSE

CancelRequest == /\ ~requestCanceled /\ result = "waiting"
                 /\ requestCanceled' = TRUE
                 /\ UNCHANGED <<cause, work, result, laterHead, holdVars>>

\* opQueue.execute writes the SOState that queues the operation only after
\* the admission's move landed.
TakeForQueueOperation == /\ work = "queued" /\ ~crashed
                         /\ move = "landed"
                         /\ work' = "taken"
                         /\ UNCHANGED <<requestCanceled, cause, result, laterHead, holdVars>>
PlaceByReplay == /\ work = "taken"
                 /\ work' = "placed"
                 /\ UNCHANGED <<requestCanceled, cause, result, laterHead, holdVars>>
PublishDecision == /\ work = "placed"
                   /\ work' \in {"applied", "refused", "superseded"}
                   /\ UNCHANGED <<requestCanceled, cause, result, laterHead, holdVars>>
ObserveOtherHead == /\ ~laterHead
                    /\ laterHead' = TRUE
                    /\ UNCHANGED <<requestCanceled, cause, work, result, holdVars>>

BudgetExpiry == /\ TRUE
                /\ cause = "live" /\ result = "waiting"
                /\ cause' = "deadline"
                /\ UNCHANGED <<requestCanceled, work, result, laterHead, holdVars>>
EngineClose == /\ cause = "live" /\ result = "waiting"
               /\ cause' = "close"
               /\ UNCHANGED <<requestCanceled, work, result, laterHead, holdVars>>

WaitProven == /\ result = "waiting"
              /\ work \in {"applied", "refused", "superseded"}
              /\ result' = work
              /\ UNCHANGED <<requestCanceled, cause, work, laterHead, holdVars>>
WaitCanceled == /\ result = "waiting" /\ cause # "live"
                /\ result' = "unknown"
                /\ work' = IF cause = "close" /\ work = "queued"
                           THEN "removed" ELSE work
                /\ UNCHANGED <<requestCanceled, cause, laterHead, holdVars>>

\* The queued move lands with RequireHeld on the payload, so a freed payload
\* refuses it. The page carries no RequireHeld. Either way the overflow stage
\* is released.
LandQueue == /\ move = "queued" /\ ~crashed
             /\ move' = IF payloadFreed THEN "refused" ELSE "landed"
             /\ root' = ~payloadFreed
             /\ overflowStage' = FALSE
             /\ UNCHANGED <<requestCanceled, cause, work, result, laterHead,
                            writerStage, payloadFreed, pageFreed, crashed>>
\* CommitBlockTransaction releases the writer's stage once waitDurable
\* returns, whatever its answer.
ReleaseStages == /\ writerStage /\ result # "waiting"
                 /\ writerStage' = FALSE
                 /\ UNCHANGED <<requestCanceled, cause, work, result, laterHead,
                                overflowStage, move, root, payloadFreed,
                                pageFreed, crashed>>
\* A refused landing fails the admission in the durable step.
Fail == /\ move = "refused" /\ work = "queued" /\ ~crashed
        /\ work' = "failed"
        /\ result' = IF result = "waiting" THEN "failed" ELSE result
        /\ UNCHANGED <<requestCanceled, cause, laterHead, holdVars>>
\* The durable state, the landed root and the freed flags survive a crash.
Crash == /\ ~crashed
         /\ crashed' = TRUE
         /\ writerStage' = FALSE /\ overflowStage' = FALSE
         /\ move' = IF move = "queued" THEN "lost" ELSE move
         /\ result' = IF result = "waiting" THEN "crashed" ELSE result
         /\ UNCHANGED <<requestCanceled, cause, work, laterHead, root,
                        payloadFreed, pageFreed>>
CollectPayload == /\ ~payloadFreed /\ ~writerStage /\ ~root
                  /\ payloadFreed' = TRUE
                  /\ UNCHANGED <<requestCanceled, cause, work, result, laterHead,
                                 writerStage, overflowStage, move, root,
                                 pageFreed, crashed>>
CollectPage == /\ ~pageFreed /\ ~overflowStage /\ ~root
               /\ pageFreed' = TRUE
               /\ UNCHANGED <<requestCanceled, cause, work, result, laterHead,
                              writerStage, overflowStage, move, root,
                              payloadFreed, crashed>>
Collect == CollectPayload \/ CollectPage

Done == result # "waiting" /\ UNCHANGED vars
Next == CancelRequest \/ TakeForQueueOperation \/ PlaceByReplay \/ PublishDecision
        \/ ObserveOtherHead \/ BudgetExpiry \/ EngineClose
        \/ WaitProven \/ WaitCanceled \/ LandQueue \/ ReleaseStages
        \/ Fail \/ Crash \/ Collect \/ Done
Spec == Init /\ [][Next]_vars
        /\ WF_vars(CancelRequest) /\ WF_vars(BudgetExpiry)
        /\ WF_vars(WaitProven \/ WaitCanceled)

ExactOutcome == result \in {"applied", "refused", "superseded"} => result = work
UnknownIsNotRequestCancel == result = "unknown" => cause # "live"
TimeoutKeepsWork == cause = "deadline" => work # "removed"
WaitEnds == <>(result # "waiting")
\* A durable state that names the operation keeps its blocks.
DurableStateHolds == work \in {"taken", "placed", "applied", "refused",
                                "superseded"}
                     => ~payloadFreed /\ ~pageFreed
====
