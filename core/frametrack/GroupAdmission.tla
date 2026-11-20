---- MODULE GroupAdmission ----
\* One incoming group can interleave with registration and release on either
\* peer. Bifrost provides the peer identity. Only the header provides the
\* claimed subscription ID. Cancellation is abstracted as closing the stream.

CONSTANTS Device, Other, First, Sibling, Second

Peers == {Device, Other}
Ids == {First, Sibling, Second}
Owner(id) == IF id = Second THEN Other ELSE Device

VARIABLES issued, live, group
vars == <<issued, live, group>>

HasSubscription(p, subscriptions) ==
  \E id \in subscriptions : Owner(id) = p

Init ==
  /\ issued = {}
  /\ live = {}
  /\ group = [phase |-> "closed", peer |-> Device, id |-> First]

Register(id) ==
  /\ id \notin issued
  /\ issued' = issued \cup {id}
  /\ live' = live \cup {id}
  /\ UNCHANGED group

Open(p, id) ==
  /\ group.phase = "closed"
  /\ group' = [phase |-> IF HasSubscription(p, live) THEN "header" ELSE "closed",
               peer |-> p, id |-> id]
  /\ UNCHANGED <<issued, live>>

ReadHeader ==
  /\ group.phase = "header"
  /\ group' = [group EXCEPT !.phase =
       IF group.id \in live /\ Owner(group.id) = group.peer
       THEN "reading" ELSE "closed"]
  /\ UNCHANGED <<issued, live>>

Release(id) ==
  /\ id \in live
  /\ live' = live \ {id}
  /\ group' =
       IF (group.phase = "reading" /\ group.id = id) \/
          (group.phase = "header" /\ ~HasSubscription(group.peer, live'))
       THEN [group EXCEPT !.phase = "closed"] ELSE group
  /\ UNCHANGED issued

Close ==
  /\ group.phase # "closed"
  /\ group' = [group EXCEPT !.phase = "closed"]
  /\ UNCHANGED <<issued, live>>

Next ==
  \/ \E id \in Ids : Register(id) \/ Release(id)
  \/ \E p \in Peers, id \in Ids : Open(p, id)
  \/ ReadHeader
  \/ Close

TypeOK ==
  /\ issued \subseteq Ids
  /\ live \subseteq issued
  /\ group \in [phase : {"closed", "header", "reading"}, peer : Peers, id : Ids]

OnlyLiveDeviceReads ==
  group.phase = "reading" => group.id \in live /\ Owner(group.id) = group.peer

OnlySubscribedPeerSendsHeader ==
  group.phase = "header" => HasSubscription(group.peer, live)
====
