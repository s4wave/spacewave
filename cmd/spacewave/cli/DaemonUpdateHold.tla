------------------------- MODULE DaemonUpdateHold -------------------------
EXTENDS Naturals
\* Modeled callers: serve's watcher (accept/reselect, prepare, report failure,
\* reacquire launcher, cancel, claim); clients/services (release); idle timer
\* (expire); desktop control (quit, excluding only its requester and desktop).
\* owners counts service holds allocated by the watcher, so replacing its
\* remembered identity without releasing the previous hold is observable.
\* Restart-now deliberately interrupts other work; executable publication and
\* desktop restart are unchanged and outside this lifetime model.
VARIABLES phase, holds, owners, stopping
vars == <<phase, holds, owners, stopping>>
Live == {"preparing", "prepared", "unreported", "reacquire"}
Init == /\ phase = "watching"
        /\ holds = {"client", "desktop", "other", "requester"}
        /\ owners = 0 /\ stopping = FALSE
\* A new selection reuses any hold retained across failure or owner change.
Accept == /\ phase \in {"watching", "prepared", "unreported", "reacquire"}
          /\ ~stopping /\ phase' = "preparing"
          /\ owners' = IF owners = 0 THEN 1 ELSE owners
          /\ UNCHANGED <<holds, stopping>>
Prepare == /\ phase = "preparing" /\ ~stopping
           /\ phase' = "prepared"
           /\ UNCHANGED <<holds, owners, stopping>>
\* A stale selection's rejected failure report retains accepted intent.
UnreportedFailure == /\ phase = "preparing" /\ phase' = "unreported"
                     /\ UNCHANGED <<holds, owners, stopping>>
OwnerChanged == /\ phase \in {"prepared", "unreported"}
                /\ phase' = "reacquire"
                /\ UNCHANGED <<holds, owners, stopping>>
ReportedFailure == /\ phase \in Live /\ phase' = "failed"
                   /\ UNCHANGED <<holds, owners, stopping>>
Cancel == /\ phase \in Live /\ phase' = "cancelled"
          /\ UNCHANGED <<holds, owners, stopping>>
ReleaseOwner == /\ phase \in {"failed", "handoff", "cancelled"}
                /\ owners > 0 /\ owners' = owners - 1
                /\ UNCHANGED <<phase, holds, stopping>>
ReleaseOther == \E h \in holds:
                  /\ holds' = holds \ {h}
                  /\ UNCHANGED <<phase, owners, stopping>>
Claim == /\ phase = "prepared" /\ ~stopping
         /\ holds \ {"desktop"} = {} /\ owners = 1
         /\ phase' = "handoff" /\ stopping' = TRUE
         /\ UNCHANGED <<holds, owners>>
DesktopQuit == /\ "desktop" \in holds /\ ~stopping
               /\ holds \ {"desktop", "requester"} = {}
               /\ owners = 0
               /\ stopping' = TRUE
               /\ UNCHANGED <<phase, holds, owners>>
Expire == /\ holds = {} /\ owners = 0 /\ ~stopping
          /\ stopping' = TRUE
          /\ UNCHANGED <<phase, holds, owners>>
Done == /\ stopping /\ phase \notin Live /\ UNCHANGED vars
Next == Accept \/ Prepare \/ UnreportedFailure \/ OwnerChanged \/ ReportedFailure
        \/ Cancel \/ ReleaseOwner \/ ReleaseOther \/ Claim \/ DesktopQuit \/ Expire \/ Done
Spec == Init /\ [][Next]_vars
AcceptedHasOwner == phase \in Live => owners = 1
AcceptedKeepsDaemon == phase \in Live => ~stopping
NoDuplicateOwners == owners <= 1
ClaimWaitsForOthers == phase = "handoff" => holds \ {"desktop"} = {}
=============================================================================
