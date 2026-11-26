---- MODULE PeerRetry ----
EXTENDS Naturals
(* sync.go peerStreams: one SOSync owns a standing solicitation and,
   for each transport peer, the stream running with it and the renewal that
   reopens it.
   A stream that ends recoverably backs off and offers the protocol again to
   its own peer only. An end that a held-state event can lift (this side's
   refusal, a revocation, a history catch-up cannot bridge, a state too large
   to send) waits for the event, then renews at once. A denial by the remote
   side parks the peer: the remote's own watch renews from that side. A
   renewal lasts only while a link to its peer exists; a new link pairs both
   sides' standing offers, so it opens a stream with no renewal. A stream
   arriving from any offer cancels the peer's renewal. Stop fences arrivals
   and renewals, cancels every stream and renewal, and returns only once each
   has exited.
   The remote side and the transport are the environment: ArriveRemote is a
   remote renewal and has no fairness, nor do a link's loss and return, nor
   the held-state event. ArriveOwn is the stream our renewal pairs with and
   ArrivePair the stream a new link pairs, both opened by the solicit
   controller. The lower/higher peer-ID grace that keeps both sides from
   renewing together is a timing choice and is not modelled. Authentication
   and exchange stay in StreamLifecycle. *)
CONSTANTS Peers, MaxEnds, MaxDrops
VARIABLES owner, phase, lastEnd, lifted, ends, lostTo, linked, fresh, drops
vars == <<owner, phase, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* Phases in which a peer has a goroutine the owner must join.
Active == {"streaming", "backoff", "watch", "offered"}

\* Phases in which a peer holds a renewal.
Renewing == {"backoff", "watch", "offered"}

TypeOK ==
  /\ owner \in {"running", "stopping", "stopped"}
  /\ phase \in [Peers -> {"none", "parked", "stopped"} \cup Active]
  /\ lastEnd \in [Peers -> {"none", "recoverable", "parked", "watch"}]
  /\ lifted \in [Peers -> BOOLEAN]
  /\ ends \in [Peers -> 0..MaxEnds]
  /\ lostTo \in [Peers -> BOOLEAN]
  /\ linked \in [Peers -> BOOLEAN]
  /\ fresh \in [Peers -> BOOLEAN]
  /\ drops \in [Peers -> 0..MaxDrops]

\* Every peer starts with a new link, which the standing offers pair.
Init ==
  /\ owner = "running"
  /\ phase = [p \in Peers |-> "none"]
  /\ lastEnd = [p \in Peers |-> "none"]
  /\ lifted = [p \in Peers |-> FALSE]
  /\ ends = [p \in Peers |-> 0]
  /\ lostTo = [p \in Peers |-> FALSE]
  /\ linked = [p \in Peers |-> TRUE]
  /\ fresh = [p \in Peers |-> TRUE]
  /\ drops = [p \in Peers |-> 0]

\* A new link pairs both sides' standing offers and opens a stream.
ArrivePair(p) ==
  /\ owner = "running" /\ linked[p] /\ fresh[p] /\ phase[p] # "stopped"
  /\ phase' = [phase EXCEPT ![p] = "streaming"]
  /\ fresh' = [fresh EXCEPT ![p] = FALSE]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, drops>>

\* A stream from our own renewal pairs with the remote's standing offer.
ArriveOwn(p) ==
  /\ owner = "running" /\ phase[p] = "offered" /\ linked[p]
  /\ phase' = [phase EXCEPT ![p] = "streaming"]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* The remote side renews, so a stream arrives on our standing offer.
ArriveRemote(p) ==
  /\ owner = "running" /\ linked[p]
  /\ phase[p] \in {"none", "parked", "backoff", "watch"}
  /\ phase' = [phase EXCEPT ![p] = "streaming"]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* The stream ends with an error a fresh stream may clear.
EndRecoverable(p) ==
  /\ owner = "running" /\ phase[p] = "streaming" /\ ends[p] < MaxEnds
  /\ phase' = [phase EXCEPT ![p] = "backoff"] /\ lostTo' = lostTo
  /\ lastEnd' = [lastEnd EXCEPT ![p] = "recoverable"]
  /\ ends' = [ends EXCEPT ![p] = ends[p] + 1]
  /\ UNCHANGED <<owner, lifted, linked, fresh, drops>>

\* The remote side denies this side: only the remote's config can lift it.
EndParked(p) ==
  /\ owner = "running" /\ phase[p] = "streaming" /\ ends[p] < MaxEnds
  /\ phase' = [phase EXCEPT ![p] = "parked"]
  /\ lastEnd' = [lastEnd EXCEPT ![p] = "parked"]
  /\ ends' = [ends EXCEPT ![p] = ends[p] + 1]
  /\ UNCHANGED <<owner, lifted, lostTo, linked, fresh, drops>>

\* The stream ends in a state a held-state event can lift.
EndWatch(p) ==
  /\ owner = "running" /\ phase[p] = "streaming" /\ ends[p] < MaxEnds
  /\ phase' = [phase EXCEPT ![p] = "watch"]
  /\ lastEnd' = [lastEnd EXCEPT ![p] = "watch"]
  /\ lifted' = [lifted EXCEPT ![p] = FALSE]
  /\ ends' = [ends EXCEPT ![p] = ends[p] + 1]
  /\ UNCHANGED <<owner, lostTo, linked, fresh, drops>>

\* The held state changes so that it lifts the end: a readmission, a
\* recovered or advanced config, or a checkpoint.
Lift(p) ==
  /\ phase[p] = "watch" /\ ~lifted[p]
  /\ lifted' = [lifted EXCEPT ![p] = TRUE]
  /\ UNCHANGED <<owner, phase, lastEnd, ends, lostTo, linked, fresh, drops>>

\* The held-state watch sees the event and renews the peer's offer at once.
Lifted(p) ==
  /\ owner = "running" /\ phase[p] = "watch" /\ lifted[p]
  /\ phase' = [phase EXCEPT ![p] = "offered"]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* The peer's backoff elapses and it offers the protocol to that peer again.
Renew(p) ==
  /\ owner = "running" /\ phase[p] = "backoff"
  /\ phase' = [phase EXCEPT ![p] = "offered"]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* The peer's last link goes; a stream on it ends recoverably.
LinkDown(p) ==
  /\ linked[p] /\ drops[p] < MaxDrops
  /\ linked' = [linked EXCEPT ![p] = FALSE]
  /\ fresh' = [fresh EXCEPT ![p] = FALSE]
  /\ drops' = [drops EXCEPT ![p] = drops[p] + 1]
  /\ IF owner = "running" /\ phase[p] = "streaming"
       THEN /\ phase' = [phase EXCEPT ![p] = "backoff"]
            /\ lastEnd' = [lastEnd EXCEPT ![p] = "recoverable"]
       ELSE UNCHANGED <<phase, lastEnd>>
  /\ UNCHANGED <<owner, lifted, ends, lostTo>>

\* A link to the peer returns, carrying no pair yet.
LinkUp(p) ==
  /\ ~linked[p]
  /\ linked' = [linked EXCEPT ![p] = TRUE]
  /\ fresh' = [fresh EXCEPT ![p] = TRUE]
  /\ UNCHANGED <<owner, phase, lastEnd, lifted, ends, lostTo, drops>>

\* The link watch sees no link left and retires the renewal and its offer.
Unlinked(p) ==
  /\ owner = "running" /\ phase[p] \in Renewing /\ ~linked[p]
  /\ phase' = [phase EXCEPT ![p] = "parked"]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* The owner's context ends: arrivals and renewals are fenced, all cancelled.
Stop ==
  /\ owner = "running" /\ owner' = "stopping"
  /\ UNCHANGED <<phase, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* A cancelled stream or renewal returns and releases its offer.
Exit(p) ==
  /\ owner = "stopping" /\ phase[p] \in Active
  /\ phase' = [phase EXCEPT ![p] = "stopped"]
  /\ UNCHANGED <<owner, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

\* The owner returns once every peer goroutine has exited.
Join ==
  /\ owner = "stopping" /\ \A p \in Peers: phase[p] \notin Active
  /\ owner' = "stopped"
  /\ UNCHANGED <<phase, lastEnd, lifted, ends, lostTo, linked, fresh, drops>>

Next ==
  \/ \E p \in Peers:
       \/ ArrivePair(p) \/ ArriveOwn(p) \/ ArriveRemote(p)
       \/ EndRecoverable(p) \/ EndParked(p) \/ EndWatch(p)
       \/ Lift(p) \/ Lifted(p) \/ Renew(p)
       \/ LinkDown(p) \/ LinkUp(p) \/ Unlinked(p) \/ Exit(p)
  \/ Stop \/ Join

Spec ==
  /\ Init /\ [][Next]_vars /\ WF_vars(Join)
  /\ \A p \in Peers:
       WF_vars(ArrivePair(p)) /\ WF_vars(ArriveOwn(p)) /\ WF_vars(Renew(p))
       /\ WF_vars(Lifted(p)) /\ WF_vars(Unlinked(p)) /\ WF_vars(Exit(p))

\* No stream ends because another peer's stream failed.
NoCrossPeerRestart == \A p \in Peers: ~lostTo[p]

\* The owner returns only after every stream and renewal has exited.
Joined == owner = "stopped" => \A p \in Peers: phase[p] \notin Active

\* A linked peer whose stream ended recoverably streams again unless the owner
\* stops or the peer's last link goes.
RecoverableReopens ==
  \A p \in Peers:
    (lastEnd[p] = "recoverable" /\ phase[p] \notin {"streaming", "parked"})
      ~> (phase[p] = "streaming" \/ owner # "running" \/ ~linked[p])

\* A peer whose end the held state lifted streams again unless the owner stops
\* or the peer's last link goes.
LiftedReopens ==
  \A p \in Peers:
    (phase[p] = "watch" /\ lifted[p])
      ~> (phase[p] = "streaming" \/ owner # "running" \/ ~linked[p])

\* A renewal does not outlive its peer's last link: it ends unless the link
\* returns first or the owner stops.
RenewalNeedsLink ==
  \A p \in Peers:
    (phase[p] \in Renewing /\ ~linked[p])
      ~> (phase[p] \notin Renewing \/ linked[p] \/ owner # "running")

\* A new link reopens the peer's stream unless the owner stops or it goes.
RelinkReopens ==
  \A p \in Peers:
    (linked[p] /\ fresh[p])
      ~> (phase[p] = "streaming" \/ owner # "running" \/ ~linked[p])

\* Shutdown, once begun, completes.
ShutdownCompletes == owner = "stopping" ~> owner = "stopped"
====
