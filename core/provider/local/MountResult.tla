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
Spec == Init /\ [][Next]_vars
        /\ WF_vars(Retire) /\ WF_vars(CancelFirst) /\ WF_vars(SkipMiddle)
        /\ WF_vars(EnterThird) /\ WF_vars(Claim(3)) /\ WF_vars(Publish(3))
NewestOwns == scheduled = 3 => owner \in {0, 3}
MountBecomesReady == <>[](ready = 3)
==============================================================================
