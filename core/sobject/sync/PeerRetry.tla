---- MODULE PeerRetry ----
EXTENDS Naturals
(* sync.go peerStreams: one SOSync owns a standing solicitation and,
   for each transport peer, the stream running with it and the renewal that
   reopens it.
   A stream that ends recoverably backs off and offers the protocol again to
   its own peer only; a stream this side refused waits until held authority
   admits the participant, then renews at once; a terminal end holds the peer
   until the remote side renews. A stream arriving from any offer cancels the
   peer's renewal. Stop fences arrivals and renewals, cancels every stream and
   renewal, and returns only once each has exited.
   The remote side and the transport are the environment: ArriveRemote is a
   remote renewal and has no fairness; ArriveOwn is the stream our renewal
   pairs with, which the solicit controller opens. The lower/higher peer-ID
   grace that keeps both sides from renewing together is a timing choice and
   is not modelled. Authentication and exchange stay in StreamLifecycle. *)
CONSTANTS Peers, MaxEnds
VARIABLES owner, phase, lastEnd, admitted, ends, lostTo
vars == <<owner, phase, lastEnd, admitted, ends, lostTo>>

\* Phases in which a peer has a goroutine the owner must join.
Active == {"streaming", "backoff", "readmit", "offered"}

TypeOK ==
  /\ owner \in {"running", "stopping", "stopped"}
  /\ phase \in [Peers -> {"none", "held", "stopped"} \cup Active]
  /\ lastEnd \in [Peers -> {"none", "recoverable", "terminal", "refused"}]
  /\ admitted \in [Peers -> BOOLEAN]
  /\ ends \in [Peers -> 0..MaxEnds]
  /\ lostTo \in [Peers -> BOOLEAN]

Init ==
  /\ owner = "running"
  /\ phase = [p \in Peers |-> "none"]
  /\ lastEnd = [p \in Peers |-> "none"]
  /\ admitted = [p \in Peers |-> FALSE]
  /\ ends = [p \in Peers |-> 0]
  /\ lostTo = [p \in Peers |-> FALSE]

\* A stream from our own renewal pairs with the remote's standing offer.
ArriveOwn(p) ==
  /\ owner = "running" /\ phase[p] = "offered"
  /\ phase' = [phase EXCEPT ![p] = "streaming"]
  /\ UNCHANGED <<owner, lastEnd, admitted, ends, lostTo>>

\* The remote side renews, so a stream arrives on our standing offer.
ArriveRemote(p) ==
  /\ owner = "running" /\ phase[p] \in {"none", "held", "backoff", "readmit"}
  /\ phase' = [phase EXCEPT ![p] = "streaming"]
  /\ UNCHANGED <<owner, lastEnd, admitted, ends, lostTo>>

\* The stream ends with an error a fresh stream may clear.
EndRecoverable(p) ==
  /\ owner = "running" /\ phase[p] = "streaming" /\ ends[p] < MaxEnds
  /\ phase' = [phase EXCEPT ![p] = "backoff"] /\ lostTo' = lostTo
  /\ lastEnd' = [lastEnd EXCEPT ![p] = "recoverable"]
  /\ ends' = [ends EXCEPT ![p] = ends[p] + 1]
  /\ UNCHANGED <<owner, admitted>>

\* The stream ends in a state only authority or recovery can change.
EndTerminal(p) ==
  /\ owner = "running" /\ phase[p] = "streaming" /\ ends[p] < MaxEnds
  /\ phase' = [phase EXCEPT ![p] = "held"]
  /\ lastEnd' = [lastEnd EXCEPT ![p] = "terminal"]
  /\ ends' = [ends EXCEPT ![p] = ends[p] + 1]
  /\ UNCHANGED <<owner, admitted, lostTo>>

\* This side's held authority refuses the participant.
EndRefused(p) ==
  /\ owner = "running" /\ phase[p] = "streaming" /\ ends[p] < MaxEnds
  /\ phase' = [phase EXCEPT ![p] = "readmit"]
  /\ lastEnd' = [lastEnd EXCEPT ![p] = "refused"]
  /\ admitted' = [admitted EXCEPT ![p] = FALSE]
  /\ ends' = [ends EXCEPT ![p] = ends[p] + 1]
  /\ UNCHANGED <<owner, lostTo>>

\* A config change grants the refused participant readable access.
Admit(p) ==
  /\ phase[p] = "readmit" /\ ~admitted[p]
  /\ admitted' = [admitted EXCEPT ![p] = TRUE]
  /\ UNCHANGED <<owner, phase, lastEnd, ends, lostTo>>

\* The readmission watch sees the grant and renews the peer's offer at once.
Readmitted(p) ==
  /\ owner = "running" /\ phase[p] = "readmit" /\ admitted[p]
  /\ phase' = [phase EXCEPT ![p] = "offered"]
  /\ UNCHANGED <<owner, lastEnd, admitted, ends, lostTo>>

\* The peer's backoff elapses and it offers the protocol to that peer again.
Renew(p) ==
  /\ owner = "running" /\ phase[p] = "backoff"
  /\ phase' = [phase EXCEPT ![p] = "offered"]
  /\ UNCHANGED <<owner, lastEnd, admitted, ends, lostTo>>

\* The owner's context ends: arrivals and renewals are fenced, all cancelled.
Stop ==
  /\ owner = "running" /\ owner' = "stopping"
  /\ UNCHANGED <<phase, lastEnd, admitted, ends, lostTo>>

\* A cancelled stream or renewal returns and releases its offer.
Exit(p) ==
  /\ owner = "stopping" /\ phase[p] \in Active
  /\ phase' = [phase EXCEPT ![p] = "stopped"]
  /\ UNCHANGED <<owner, lastEnd, admitted, ends, lostTo>>

\* The owner returns once every peer goroutine has exited.
Join ==
  /\ owner = "stopping" /\ \A p \in Peers: phase[p] \notin Active
  /\ owner' = "stopped"
  /\ UNCHANGED <<phase, lastEnd, admitted, ends, lostTo>>

Next ==
  \/ \E p \in Peers:
       \/ ArriveOwn(p) \/ ArriveRemote(p) \/ EndRecoverable(p)
       \/ EndTerminal(p) \/ EndRefused(p) \/ Admit(p) \/ Readmitted(p)
       \/ Renew(p) \/ Exit(p)
  \/ Stop \/ Join

Spec ==
  /\ Init /\ [][Next]_vars /\ WF_vars(Join)
  /\ \A p \in Peers:
       WF_vars(ArriveOwn(p)) /\ WF_vars(Renew(p)) /\ WF_vars(Readmitted(p))
       /\ WF_vars(Exit(p))

\* No stream ends because another peer's stream failed.
NoCrossPeerRestart == \A p \in Peers: ~lostTo[p]

\* The owner returns only after every stream and renewal has exited.
Joined == owner = "stopped" => \A p \in Peers: phase[p] \notin Active

\* A peer whose stream ended recoverably streams again unless the owner stops.
RecoverableReopens ==
  \A p \in Peers:
    (lastEnd[p] = "recoverable" /\ phase[p] # "streaming")
      ~> (phase[p] = "streaming" \/ owner # "running")

\* A refused peer that authority admits streams again unless the owner stops.
ReadmittedReopens ==
  \A p \in Peers:
    (phase[p] = "readmit" /\ admitted[p])
      ~> (phase[p] = "streaming" \/ owner # "running")

\* Shutdown, once begun, completes.
ShutdownCompletes == owner = "stopping" ~> owner = "stopped"
====
