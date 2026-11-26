---- MODULE SpaceBacklog ----
EXTENDS Naturals, Sequences, FiniteSets
(* cloud-sohost-backlog.go soStateBacklog: the inline state one mounted Space
   has received from the account's Session websocket and not yet applied.
   The socket reader only records each frame in the backlog; the Space's own
   worker takes the backlog whole and applies it, so a slow Space delays only
   itself.
   A snapshot replaces the held messages. A delta contiguous with the held end
   extends the held run. A gap, an overflow of the run, or a new connection
   enters resync, which drops every held message: the worker's resync pull
   reads the cloud after it takes the backlog, so the pull covers each of them.
   The worker applies a held run only from the held state's end; a run that
   starts past it triggers the pull routine, as handleStateDelta's gap does,
   and so does a held snapshot older than the accepted state.
   The cloud and the network are the environment: commits, snapshots the cloud
   sends, a frame lost mid-stream, and disconnects. A frame not read before a
   disconnect is lost. Pulls
   do not fail here; a failed resync pull waits for the next frame or
   connection. Account outcomes: dormant and ready replace each other, while a
   terminal outcome (unauthenticated, deleted) ends the tracker, so nothing
   after it can replace it. *)
CONSTANTS MaxHead, MaxRun, MaxSnaps, MaxDrops, MaxLosses
None == MaxHead + 1
\* head: the cloud change log's last seqno.
\* applied: the seqnos the Space's accepted state includes.
\* wire: frames sent on the live connection and not yet read.
\* mode, snap, run: the backlog. run holds delta entries by seqno.
\* pullPending: the pull routine was triggered.
VARIABLES head, applied, conn, wire, mode, snap, run, pullPending, status, terminal, snaps, drops, losses
vars == <<head, applied, conn, wire, mode, snap, run, pullPending, status, terminal, snaps, drops, losses>>

Max(S) == CHOOSE x \in S : \A y \in S : y <= x
Top == IF applied = {} THEN 0 ELSE Max(applied)
\* HeldEnd is the seqno the held messages reach, or None when none are held.
HeldEnd == IF run # {} THEN Max(run) ELSE snap

Frame == [kind : {"delta", "snap"}, since : 0..MaxHead, to : 0..MaxHead]

TypeOK ==
  /\ head \in 0..MaxHead
  /\ applied \subseteq 1..MaxHead
  /\ conn \in {"up", "down", "ended"}
  /\ mode \in {"empty", "holding", "resync"}
  /\ snap \in 0..None
  /\ run \subseteq 1..MaxHead
  /\ pullPending \in BOOLEAN
  /\ status \in {"ready", "dormant", "terminal"}
  /\ terminal \in BOOLEAN

Init ==
  /\ head = 0 /\ applied = {} /\ conn = "up" /\ wire = <<>>
  /\ mode = "empty" /\ snap = None /\ run = {} /\ pullPending = FALSE
  /\ status = "ready" /\ terminal = FALSE /\ snaps = 0 /\ drops = 0 /\ losses = 0

\* Resync drops the held messages; the coming pull covers them.
EnterResync == mode' = "resync" /\ snap' = None /\ run' = {}

\* The cloud commits one entry and sends its delta while the socket is up.
Commit ==
  /\ head < MaxHead
  /\ head' = head + 1
  /\ wire' = IF conn = "up" THEN Append(wire, [kind |-> "delta", since |-> head, to |-> head + 1]) ELSE wire
  /\ UNCHANGED <<applied, conn, mode, snap, run, pullPending, status, terminal, snaps, drops, losses>>

\* The cloud sends a full snapshot at its head.
SendSnapshot ==
  /\ conn = "up" /\ snaps < MaxSnaps
  /\ wire' = Append(wire, [kind |-> "snap", since |-> head, to |-> head])
  /\ snaps' = snaps + 1
  /\ UNCHANGED <<head, applied, conn, mode, snap, run, pullPending, status, terminal, drops, losses>>

\* The reader records one frame in the backlog.
Read ==
  /\ conn = "up" /\ wire # <<>>
  /\ LET m == Head(wire) IN
       \/ /\ mode = "resync"
          /\ UNCHANGED <<mode, snap, run>>
       \/ /\ mode # "resync" /\ m.kind = "snap"
          /\ mode' = "holding" /\ snap' = m.to /\ run' = {}
       \/ /\ mode = "empty" /\ m.kind = "delta"
          /\ mode' = "holding" /\ snap' = None /\ run' = {m.to}
       \/ /\ mode = "holding" /\ m.kind = "delta" /\ m.since <= HeldEnd
          /\ LET next == IF m.to > HeldEnd THEN run \cup {m.to} ELSE run IN
               IF Cardinality(next) > MaxRun
               THEN EnterResync
               ELSE mode' = "holding" /\ snap' = snap /\ run' = next
       \/ /\ mode = "holding" /\ m.kind = "delta" /\ m.since > HeldEnd
          /\ EnterResync
  /\ wire' = Tail(wire)
  /\ UNCHANGED <<head, applied, conn, pullPending, status, terminal, snaps, drops, losses>>

\* The cloud loses one frame mid-stream; the frame after it shows the gap.
LoseFrame ==
  /\ conn = "up" /\ Len(wire) >= 2 /\ losses < MaxLosses
  /\ wire' = Tail(wire) /\ losses' = losses + 1
  /\ UNCHANGED <<head, applied, conn, mode, snap, run, pullPending, status, terminal, snaps, drops>>

\* The socket drops. Unread frames are lost; the account turns dormant.
Disconnect ==
  /\ conn = "up" /\ drops < MaxDrops
  /\ conn' = "down" /\ wire' = <<>> /\ drops' = drops + 1
  /\ status' = "dormant"
  /\ UNCHANGED <<head, applied, mode, snap, run, pullPending, terminal, snaps, losses>>

\* A new connection subscribes, then marks every Space's boundary before it
\* reads its first frame.
Connect ==
  /\ conn = "down"
  /\ conn' = "up" /\ status' = "ready"
  /\ EnterResync
  /\ UNCHANGED <<head, applied, wire, pullPending, terminal, snaps, drops, losses>>

\* The tracker ends on a terminal outcome and applies it after the socket.
End ==
  /\ conn \in {"up", "down"}
  /\ conn' = "ended" /\ wire' = <<>> /\ status' = "terminal" /\ terminal' = TRUE
  /\ UNCHANGED <<head, applied, mode, snap, run, pullPending, snaps, drops, losses>>

\* The worker takes the backlog whole. Resync pulls a fresh read. Otherwise
\* it applies the held snapshot, whose rollback check sends an older one to
\* the pull routine, then the held run from the state's end.
Work ==
  /\ mode # "empty"
  /\ mode' = "empty" /\ snap' = None /\ run' = {}
  /\ IF mode = "resync"
     THEN applied' = 1..head /\ pullPending' = pullPending
     ELSE IF snap # None /\ snap < Top
     THEN applied' = applied /\ pullPending' = TRUE
     ELSE LET base == IF snap # None THEN 1..snap ELSE applied
              baseTop == IF base = {} THEN 0 ELSE Max(base)
              from == IF snap # None THEN snap ELSE Max(run) - Cardinality(run)
          IN IF run # {} /\ from > baseTop
             THEN applied' = base /\ pullPending' = TRUE
             ELSE applied' = base \cup run /\ pullPending' = pullPending
  /\ UNCHANGED <<head, conn, wire, status, terminal, snaps, drops, losses>>

\* The pull routine reads the cloud and applies its state.
Pull ==
  /\ pullPending
  /\ applied' = 1..head /\ pullPending' = FALSE
  /\ UNCHANGED <<head, conn, wire, mode, snap, run, status, terminal, snaps, drops, losses>>

Next == Commit \/ SendSnapshot \/ Read \/ LoseFrame \/ Disconnect \/ Connect \/ End \/ Work \/ Pull
Spec == Init /\ [][Next]_vars

\* The accepted state is always the change log up to some seqno.
Prefix == applied = 1..Top

\* The held run is contiguous.
RunContiguous == run = {} \/ run = (Max(run) - Cardinality(run) + 1)..Max(run)

\* With the socket up, every frame read and no work left, the Space holds the
\* cloud's head: no change can be stranded behind a quiet connection.
Converged ==
  (conn = "up" /\ wire = <<>> /\ mode = "empty" /\ ~pullPending) => applied = 1..head

\* Nothing replaces a terminal account outcome.
TerminalKept == terminal => status = "terminal"
====
