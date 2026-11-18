----------------------------- MODULE MountResult -----------------------------
EXTENDS Naturals
VARIABLES phase, live, owner, ready, scheduled
vars == <<phase, live, owner, ready, scheduled>>
Runs == 1..3

(* One retained tracker and a delete followed by a recreate, replace, or
   reload. All call restartSharedObject; only the order of callbacks matters.
   The middle callback may exit while waiting for the first, allowing the
   third to enter. Entry (keyed) and claiming the result (sobjectResult.start)
   can interleave. Storage and host setup are abstracted by Publish, since
   this model asks whether a viable mount may publish, not whether I/O works.
   Retirement and cancellation are separate; no new run can enter until both.
   The mount caller is an observer: ready = 3 is the new result it can await.
   Caller timeout is omitted to check that cancellation cannot strand it. *)
Init == /\ phase = [r \in Runs |-> IF r = 1 THEN "entered" ELSE "waiting"]
        /\ live = {1}
        /\ owner = 0
        /\ ready = 0
        /\ scheduled = 1

Retire == /\ scheduled = 1
          /\ owner' = 0
          /\ ready' = 0
          /\ scheduled' = 2
          /\ UNCHANGED <<phase, live>>

CancelFirst == /\ scheduled = 2
               /\ live = {1}
               /\ live' = {2}
               /\ UNCHANGED <<phase, owner, ready, scheduled>>

(* The next restart retires and cancels the middle callback while it only
   waits for its predecessor. That callback can exit without joining it. *)
SkipMiddle == /\ scheduled = 2
              /\ live = {2}
              /\ live' = {3}
              /\ scheduled' = 3
              /\ phase' = [phase EXCEPT ![2] = "done"]
              /\ owner' = 0
              /\ ready' = 0

EnterThird == /\ phase[2] = "done"
              /\ phase[3] = "waiting"
              /\ phase' = [phase EXCEPT ![3] = "entered"]
              /\ UNCHANGED <<live, owner, ready, scheduled>>

Claim(r) == /\ phase[r] = "entered"
            /\ r \in live
            /\ phase' = [phase EXCEPT ![r] = "claimed"]
            /\ owner' = r
            /\ ready' = 0
            /\ UNCHANGED <<live, scheduled>>

Publish(r) == /\ phase[r] = "claimed"
              /\ r = owner
              /\ r \in live
              /\ phase' = [phase EXCEPT ![r] = "published"]
              /\ ready' = r
              /\ UNCHANGED <<live, owner, scheduled>>

ExitCanceled(r) == /\ phase[r] \in {"entered", "claimed", "published"}
                   /\ r \notin live
                   /\ phase' = [phase EXCEPT ![r] = "done"]
                   /\ UNCHANGED <<live, owner, ready, scheduled>>

Done == /\ phase[1] = "done"
        /\ phase[3] = "published"
        /\ UNCHANGED vars

Next == Retire \/ CancelFirst \/ SkipMiddle \/ EnterThird
        \/ (\E r \in Runs : Claim(r) \/ Publish(r) \/ ExitCanceled(r)) \/ Done
(* The two other races have their own bounded schedules. Their actions
   interleave with the delayed-claim schedule without sharing its tracker.
   Publication models the promise's replacement token, including a resolved
   error; read and validation are separate steps of one await. *)
(* Account stop is terminal: a result can no longer publish, and a pending
   wait releases its reference independently of the caller's lifetime. Here
   returned = 3 represents cancellation rather than a publication. *)
VARIABLES publication, captured, returned, staleReturn, stopped
resultVars == <<publication, captured, returned, staleReturn, stopped>>
InitResult == /\ publication = 1
              /\ captured = 0
              /\ returned = 0
              /\ staleReturn = FALSE
              /\ stopped = FALSE

CaptureResult == /\ ~stopped /\ publication # 0 /\ captured = 0 /\ returned = 0
                 /\ captured' = publication
                 /\ UNCHANGED <<publication, returned, staleReturn, stopped>>
RetireResult == /\ publication = 1
                /\ publication' = 0
                /\ UNCHANGED <<captured, returned, staleReturn, stopped>>
PublishSuccessor == /\ ~stopped /\ publication = 0
                    /\ publication' = 2
                    /\ UNCHANGED <<captured, returned, staleReturn, stopped>>
ValidateResult == /\ ~stopped /\ captured # 0
                  /\ returned' = IF captured = publication THEN captured ELSE 0
                  /\ staleReturn' = (returned' # 0 /\ captured # publication)
                  /\ captured' = 0
                  /\ UNCHANGED <<publication, stopped>>
StopAccount == /\ ~stopped
               /\ stopped' = TRUE
               /\ UNCHANGED <<publication, captured, returned, staleReturn>>
CancelResult == /\ stopped /\ returned = 0
                /\ returned' = 3
                /\ captured' = 0
                /\ UNCHANGED <<publication, staleReturn, stopped>>
NextResult == \/ CaptureResult \/ RetireResult \/ PublishSuccessor \/ ValidateResult
              \/ StopAccount \/ CancelResult
NoRetiredResult == ~staleReturn
AccountWaitEnds == (stopped /\ returned = 0) ~> (returned = 3)
ResultWaitEnds == <>(returned # 0 \/ stopped)

(* A has read state; replace and delete bypass the intermediate callback as
   above. C starts reading and then publishes a refusal. The enrollment commits
   its placeholder without dropping the health reference. A can complete
   before or after any of these steps, but only the current run owns the flag.
   Storage I/O is abstracted: the placeholder is assumed to commit. *)
VARIABLES loadOwner, awaiting, oldCompleted, enrollment
loadVars == <<loadOwner, awaiting, oldCompleted, enrollment>>
InitLoad == /\ loadOwner = 1
            /\ awaiting = TRUE
            /\ oldCompleted = FALSE
            /\ enrollment = "before delete"

RefuseAfterDelete == /\ enrollment = "before delete"
                     /\ loadOwner' = 3
                     /\ awaiting' = TRUE
                     /\ enrollment' = "refused"
                     /\ UNCHANGED oldCompleted
CompleteOldLoad == /\ ~oldCompleted
                   /\ awaiting' = IF loadOwner = 1 THEN FALSE ELSE awaiting
                   /\ oldCompleted' = TRUE
                   /\ UNCHANGED <<loadOwner, enrollment>>
CommitPlaceholder == /\ enrollment = "refused"
                     /\ enrollment' = "placeholder committed"
                     /\ UNCHANGED <<loadOwner, awaiting, oldCompleted>>
Revive == /\ enrollment = "placeholder committed" /\ awaiting
          /\ enrollment' = "mounted"
          /\ awaiting' = FALSE
          /\ loadOwner' = 4
          /\ UNCHANGED oldCompleted
NextLoad == RefuseAfterDelete \/ CompleteOldLoad \/ CommitPlaceholder \/ Revive
RefusalKeepsRevive == enrollment = "placeholder committed" => awaiting
PlaceholderMounts == <>(enrollment = "mounted")

(* The same-id create is serialized after retirement by the account lock.
   The store serializes delete and create commits against a held writer.
   A preflight can pass before storage changes; the check inside OpenWrite
   must distinguish a canceled mount from a host key that exists again. *)
VARIABLES lifetime, writePhase, wrongWrite
writeVars == <<lifetime, writePhase, wrongWrite>>
InitWrite == /\ lifetime = "old"
             /\ writePhase = "idle"
             /\ wrongWrite = FALSE
HostPresent == lifetime \in {"old", "fresh"}
MountLive == lifetime \in {"old", "deleted"}
Preflight == /\ writePhase = "idle" /\ MountLive
             /\ writePhase' = "waiting"
             /\ UNCHANGED <<lifetime, wrongWrite>>
OpenWrite == /\ writePhase = "waiting"
             /\ writePhase' = IF (HostPresent /\ MountLive) THEN "held" ELSE "refused"
             /\ UNCHANGED <<lifetime, wrongWrite>>
CommitWrite == /\ writePhase = "held"
               /\ writePhase' = "committed"
               /\ wrongWrite' = (lifetime = "fresh")
               /\ UNCHANGED lifetime
DeleteCommit == /\ lifetime = "old" /\ writePhase # "held"
                /\ lifetime' = "deleted"
                /\ UNCHANGED <<writePhase, wrongWrite>>
RetireMount == /\ lifetime = "deleted"
               /\ lifetime' = "retired"
               /\ UNCHANGED <<writePhase, wrongWrite>>
CreateCommit == /\ lifetime = "retired" /\ writePhase # "held"
                /\ lifetime' = "fresh"
                /\ UNCHANGED <<writePhase, wrongWrite>>
NextWrite == Preflight \/ OpenWrite \/ CommitWrite \/ DeleteCommit \/ RetireMount \/ CreateCommit
NoCrossLifetimeWrite == ~wrongWrite

allVars == <<vars, resultVars, loadVars, writeVars>>
InitAll == Init /\ InitResult /\ InitLoad /\ InitWrite
NextAll == \/ Next /\ UNCHANGED <<resultVars, loadVars, writeVars>>
           \/ NextResult /\ UNCHANGED <<vars, loadVars, writeVars>>
           \/ NextLoad /\ UNCHANGED <<vars, resultVars, writeVars>>
           \/ NextWrite /\ UNCHANGED <<vars, resultVars, loadVars>>
Spec == InitAll /\ [][NextAll]_allVars
        /\ WF_vars(Retire) /\ WF_vars(CancelFirst) /\ WF_vars(SkipMiddle)
        /\ WF_vars(EnterThird) /\ WF_vars(Claim(3)) /\ WF_vars(Publish(3))
        /\ WF_resultVars(CaptureResult) /\ WF_resultVars(RetireResult)
        /\ WF_resultVars(PublishSuccessor) /\ WF_resultVars(ValidateResult)
        /\ WF_resultVars(CancelResult)
        /\ WF_loadVars(RefuseAfterDelete) /\ WF_loadVars(CommitPlaceholder)
        /\ WF_loadVars(Revive)
NewestOwns == scheduled = 3 => owner \in {0, 3}
MountBecomesReady == <>[](ready = 3)
==============================================================================
