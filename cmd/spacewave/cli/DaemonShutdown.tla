---- MODULE DaemonShutdown ----
EXTENDS FiniteSets
(* daemon-accept.go and daemon-control.go: the daemon's accepted connections
   and its shutdown. Each trackedConn is accepted, serves, closes, and ends
   when its serving goroutine exits. A shutdown is requested by a granted
   Shutdown RPC or a desktop Quit, each from one requesting connection; the
   request closes the listener, so admission stops. The serve loop then waits
   for the requester to read its reply and close (for Shutdown, after the
   control handler wrote its acknowledgement), drains every remaining
   connection, and completes. Cancelling the serving context skips the wait,
   but the drain still closes and awaits every connection. *)
CONSTANTS Conns
None == "none"
\* conn: each connection's state; "idle" has not connected yet.
\* phase: the serve loop's shutdown state.
\* requester, kind: who requested shutdown, and how.
\* acked: the requester's reply was written.
\* late: a connection was admitted after the request.
VARIABLES conn, listening, phase, requester, kind, acked, canceled, late
vars == <<conn, listening, phase, requester, kind, acked, canceled, late>>

TypeOK ==
  /\ conn \in [Conns -> {"idle", "serving", "closing", "closed"}]
  /\ listening \in BOOLEAN
  /\ phase \in {"serving", "awaitReply", "draining", "complete"}
  /\ requester \in Conns \cup {None}
  /\ kind \in {"none", "rpc", "quit"}
  /\ acked \in BOOLEAN /\ canceled \in BOOLEAN /\ late \in BOOLEAN

Init ==
  /\ conn = [c \in Conns |-> "idle"] /\ listening = TRUE /\ phase = "serving"
  /\ requester = None /\ kind = "none" /\ acked = FALSE
  /\ canceled = FALSE /\ late = FALSE

\* The listener admits a connection.
Accept(c) ==
  /\ listening /\ conn[c] = "idle"
  /\ conn' = [conn EXCEPT ![c] = "serving"]
  /\ late' = (requester # None)
  /\ UNCHANGED <<listening, phase, requester, kind, acked, canceled>>

\* A serving connection requests shutdown; the request closes the listener.
Request(c, k) ==
  /\ phase = "serving" /\ requester = None /\ conn[c] = "serving"
  /\ requester' = c /\ kind' = k /\ listening' = FALSE
  /\ UNCHANGED <<conn, phase, acked, canceled, late>>

\* The requester's reply is written.
Reply ==
  /\ requester # None /\ ~acked /\ conn[requester] = "serving"
  /\ acked' = TRUE
  /\ UNCHANGED <<conn, listening, phase, requester, kind, canceled, late>>

\* A peer closes its connection; the requester closes only after its reply.
PeerClose(c) ==
  /\ conn[c] = "serving" /\ (c = requester => acked)
  /\ conn' = [conn EXCEPT ![c] = "closing"]
  /\ UNCHANGED <<listening, phase, requester, kind, acked, canceled, late>>

\* The serving goroutine of a closed connection exits.
Exit(c) ==
  /\ conn[c] = "closing"
  /\ conn' = [conn EXCEPT ![c] = "closed"]
  /\ UNCHANGED <<listening, phase, requester, kind, acked, canceled, late>>

\* The serving context ends, which closes the listener.
Cancel ==
  /\ ~canceled /\ canceled' = TRUE /\ listening' = FALSE
  /\ UNCHANGED <<conn, phase, requester, kind, acked, late>>

\* Accepting ends once the listener closes.
AcceptEnds ==
  /\ phase = "serving" /\ ~listening
  /\ phase' = IF requester # None THEN "awaitReply" ELSE "draining"
  /\ UNCHANGED <<conn, listening, requester, kind, acked, canceled, late>>

\* The serve loop waits until the requester read its reply and closed.
ReplyRead ==
  /\ phase = "awaitReply"
  /\ canceled \/ (acked /\ conn[requester] = "closed")
  /\ phase' = "draining"
  /\ UNCHANGED <<conn, listening, requester, kind, acked, canceled, late>>

\* The drain closes every remaining connection.
CloseAll ==
  /\ phase = "draining"
  /\ conn' = [c \in Conns |-> IF conn[c] = "serving" THEN "closing" ELSE conn[c]]
  /\ UNCHANGED <<listening, phase, requester, kind, acked, canceled, late>>

\* The drain completes once every connection's goroutine exited.
Complete ==
  /\ phase = "draining"
  /\ \A c \in Conns : conn[c] \in {"idle", "closed"}
  /\ phase' = "complete"
  /\ UNCHANGED <<conn, listening, requester, kind, acked, canceled, late>>

Next ==
  \/ \E c \in Conns : Accept(c) \/ PeerClose(c) \/ Exit(c)
  \/ \E c \in Conns, k \in {"rpc", "quit"} : Request(c, k)
  \/ Reply \/ Cancel \/ AcceptEnds \/ ReplyRead \/ CloseAll \/ Complete
Spec == Init /\ [][Next]_vars

\* Shutdown completes only after every tracked connection closed.
DrainedBeforeComplete ==
  phase = "complete" => \A c \in Conns : conn[c] \in {"idle", "closed"}

\* Unless the serving context ended, the drain starts only after the requester
\* read its reply and closed.
ReplyBeforeDrain ==
  (phase \in {"draining", "complete"} /\ requester # None /\ ~canceled)
    => (acked /\ conn[requester] = "closed")

\* No connection is admitted after shutdown was requested.
NoLateAdmission == ~late
====
