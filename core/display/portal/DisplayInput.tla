---- MODULE DisplayInput ----
EXTENDS FiniteSets

\* Each viewer can hold one representative code. Motion has the same
\* admission guard. An admitted event finishes before that viewer releases
\* its votes. The portal ends all input when the session closes.
CONSTANT Viewers
VARIABLES live, admitted, held, down
vars == <<live, admitted, held, down>>

Init ==
  /\ live = TRUE
  /\ admitted = {}
  /\ held = {}
  /\ down = FALSE

Join(v) ==
  /\ live
  /\ v \notin admitted
  /\ admitted' = admitted \cup {v}
  /\ UNCHANGED <<live, held, down>>

Press(v) ==
  /\ live
  /\ v \in admitted \ held
  /\ held' = held \cup {v}
  /\ down' = TRUE
  /\ UNCHANGED <<live, admitted>>

Release(v) ==
  /\ live
  /\ v \in held
  /\ held' = held \ {v}
  /\ down' = (held' # {})
  /\ UNCHANGED <<live, admitted>>

Leave(v) ==
  /\ live
  /\ v \in admitted
  /\ admitted' = admitted \ {v}
  /\ held' = held \ {v}
  /\ down' = (held' # {})
  /\ UNCHANGED live

End ==
  /\ live
  /\ live' = FALSE
  /\ admitted' = {}
  /\ held' = {}
  /\ down' = FALSE

Done ==
  /\ ~live
  /\ UNCHANGED vars

Next == (\E v \in Viewers : Join(v) \/ Press(v) \/ Release(v) \/ Leave(v)) \/ End \/ Done
OnlyAdmittedHold == held \subseteq admitted
DownHasHolder == down = (held # {})
EndedSessionHasNoInput == ~live => admitted = {} /\ held = {} /\ ~down
====
