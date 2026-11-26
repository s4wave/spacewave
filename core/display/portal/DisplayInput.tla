---- MODULE DisplayInput ----
EXTENDS FiniteSets

\* Each viewer can hold one representative code. Motion has the same
\* admission guard. An admitted event finishes before that viewer releases
\* its votes. The daemon releases held input before ending a session it
\* still owns. A portal that already closed cannot receive releases.
CONSTANT Viewers
VARIABLES state, admitted, held, down
vars == <<state, admitted, held, down>>

Init ==
  /\ state = "live"
  /\ admitted = {}
  /\ held = {}
  /\ down = FALSE

Join(v) ==
  /\ state = "live"
  /\ v \notin admitted
  /\ admitted' = admitted \cup {v}
  /\ UNCHANGED <<state, held, down>>

Press(v) ==
  /\ state = "live"
  /\ v \in admitted \ held
  /\ held' = held \cup {v}
  /\ down' = TRUE
  /\ UNCHANGED <<state, admitted>>

Release(v) ==
  /\ state = "live"
  /\ v \in held
  /\ held' = held \ {v}
  /\ down' = (held' # {})
  /\ UNCHANGED <<state, admitted>>

Leave(v) ==
  /\ state = "live"
  /\ v \in admitted
  /\ admitted' = admitted \ {v}
  /\ held' = held \ {v}
  /\ down' = (held' # {})
  /\ UNCHANGED state

ReleaseForEnd ==
  /\ state = "live"
  /\ state' = "released"
  /\ admitted' = {}
  /\ held' = {}
  /\ down' = FALSE

End ==
  /\ state = "released"
  /\ state' = "ended"
  /\ UNCHANGED <<admitted, held, down>>

PortalClosed ==
  /\ state \in {"live", "released"}
  /\ state' = "portalClosed"
  /\ admitted' = {}
  /\ held' = {}
  /\ UNCHANGED down

Done ==
  /\ state \in {"ended", "portalClosed"}
  /\ UNCHANGED vars

Next == (\E v \in Viewers : Join(v) \/ Press(v) \/ Release(v) \/ Leave(v))
        \/ ReleaseForEnd \/ End \/ PortalClosed \/ Done
OnlyAdmittedHold == held \subseteq admitted
DownHasHolder == state = "live" => down = (held # {})
EndedSessionHasNoInput == state # "live" => admitted = {} /\ held = {}
DaemonEndReleasedInput == state = "ended" => ~down
====
