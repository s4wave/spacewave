---------------------------- MODULE EventSource ----------------------------
EXTENDS Naturals, Sequences, FiniteSets
CONSTANTS Devices, MaxSeq
VARIABLES bound, wstate, sstate, wcursor, batch, batchEnd, ackPending,
          stored, log, head, gapThrough, acked
vars == <<bound, wstate, sstate, wcursor, batch, batchEnd, ackPending,
          stored, log, head, gapThrough, acked>>

(* Models one event-source node of a standing run, read by the watcher in
   event-source-watcher.go on every Device whose process binding is approved.
   Each watcher opens the source's EventSourceService, reads a batch from its
   cursor, and commits the batch through DeliverEvents, which stores the
   arrivals and moves the run's stored cursor in one World operation only when
   the request names the stored cursor; otherwise it answers STALE_CURSOR and
   the watcher resumes from the stored cursor. After a delivery the watcher
   calls AckEvents with the delivered cursor so a retaining source may drop
   what the run holds.

   The source is abstracted as a seqno log: events (gapThrough, head] are
   readable, and an expiry moves gapThrough past events nobody acked, which
   the source reports as one gap event whose cursor is gapThrough. Arrivals
   are records <<kind, seq>> appended to log in commit order. sstate is the
   source client's signer and read state on each Device; the bound is MaxSeq
   events and therefore at most MaxSeq gaps. *)
NoBatch == {}
Max(a, b) == IF a > b THEN a ELSE b
Range(s) == {s[i] : i \in DOMAIN s}

Init == /\ bound = FALSE
        /\ wstate = [d \in Devices |-> "WaitingBinding"]
        /\ sstate \in [Devices -> {"SignerLocked", "SignerMissing"}]
        /\ wcursor = [d \in Devices |-> 0]
        /\ batch = [d \in Devices |-> NoBatch]
        /\ batchEnd = [d \in Devices |-> 0]
        /\ ackPending = [d \in Devices |-> 0]
        /\ stored = 0
        /\ log = <<>>
        /\ head = 0
        /\ gapThrough = 0
        /\ acked = 0

\* BindingChanged: the run's process binding becomes usable or unusable.
\* Losing it stops every watcher and drops its unsent batch.
BindingChanged ==
  /\ bound' = ~bound
  /\ wstate' = IF bound THEN [d \in Devices |-> "WaitingBinding"] ELSE wstate
  /\ batch' = IF bound THEN [d \in Devices |-> NoBatch] ELSE batch
  /\ UNCHANGED <<sstate, wcursor, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

\* Open: the watcher builds the source client from its object type.
Open(d) ==
  /\ bound
  /\ wstate[d] \in {"WaitingBinding", "Backoff"}
  /\ wstate' = [wstate EXCEPT ![d] = "Opening"]
  /\ UNCHANGED <<bound, sstate, wcursor, batch, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

\* SignerReady: the session that signs inbox requests unlocks or appears.
SignerReady(d) ==
  /\ sstate[d] \in {"SignerLocked", "SignerMissing"}
  /\ sstate' = [sstate EXCEPT ![d] = "Reading"]
  /\ UNCHANGED <<bound, wstate, wcursor, batch, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

\* Stream: the opened source streams from the run's stored cursor.
Stream(d) ==
  /\ wstate[d] = "Opening"
  /\ sstate[d] \in {"Reading", "Gap"}
  /\ wstate' = [wstate EXCEPT ![d] = "Streaming"]
  /\ wcursor' = [wcursor EXCEPT ![d] = stored]
  /\ UNCHANGED <<bound, sstate, batch, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

\* Receive: the cloud inbox receives one event.
Receive ==
  /\ head < MaxSeq
  /\ head' = head + 1
  /\ UNCHANGED <<bound, wstate, sstate, wcursor, batch, batchEnd, ackPending, stored, log, gapThrough, acked>>

\* Expire: retention drops every unacked event through head.
Expire ==
  /\ head > Max(acked, gapThrough)
  /\ gapThrough' = head
  /\ UNCHANGED <<bound, wstate, sstate, wcursor, batch, batchEnd, ackPending, stored, log, head, acked>>

\* BatchRead: the source answers a read from the watcher's cursor, as one gap
\* event when events after the cursor expired, else the retained events; the
\* source can return nothing at or below gapThrough.
BatchRead(d) ==
  /\ wstate[d] = "Streaming"
  /\ sstate[d] \in {"Reading", "Gap"}
  /\ \/ /\ gapThrough > wcursor[d]
        /\ batch' = [batch EXCEPT ![d] = {<<"gap", gapThrough>>}]
        /\ batchEnd' = [batchEnd EXCEPT ![d] = gapThrough]
        /\ sstate' = [sstate EXCEPT ![d] = "Gap"]
     \/ /\ gapThrough <= wcursor[d]
        /\ head > wcursor[d]
        /\ batch' = [batch EXCEPT ![d] = {<<"ev", s>> : s \in (Max(wcursor[d], gapThrough) + 1)..head}]
        /\ batchEnd' = [batchEnd EXCEPT ![d] = head]
        /\ sstate' = [sstate EXCEPT ![d] = "Reading"]
  /\ wstate' = [wstate EXCEPT ![d] = "Delivering"]
  /\ UNCHANGED <<bound, wcursor, ackPending, stored, log, head, gapThrough, acked>>

\* Ordered appends the batch to the arrival log in seqno order.
RECURSIVE Ordered(_)
Ordered(S) == IF S = {} THEN <<>>
              ELSE LET e == CHOOSE x \in S : \A y \in S : x[2] <= y[2]
                   IN <<e>> \o Ordered(S \ {e})

\* Delivered: DeliverEvents names the stored cursor, so it stores the batch
\* and moves the cursor in one World operation.
Delivered(d) ==
  /\ wstate[d] = "Delivering"
  /\ stored = wcursor[d]
  /\ log' = log \o Ordered(batch[d])
  /\ stored' = batchEnd[d]
  /\ wcursor' = [wcursor EXCEPT ![d] = batchEnd[d]]
  /\ ackPending' = [ackPending EXCEPT ![d] = batchEnd[d]]
  /\ batch' = [batch EXCEPT ![d] = NoBatch]
  /\ wstate' = [wstate EXCEPT ![d] = "Streaming"]
  /\ UNCHANGED <<bound, sstate, batchEnd, head, gapThrough, acked>>

\* StaleCursor: another Device moved the stored cursor first.
StaleCursor(d) ==
  /\ wstate[d] = "Delivering"
  /\ stored # wcursor[d]
  /\ batch' = [batch EXCEPT ![d] = NoBatch]
  /\ wstate' = [wstate EXCEPT ![d] = "Resuming"]
  /\ UNCHANGED <<bound, sstate, wcursor, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

\* Resume: the watcher reads the stored cursor and streams from it.
Resume(d) ==
  /\ wstate[d] = "Resuming"
  /\ wcursor' = [wcursor EXCEPT ![d] = stored]
  /\ wstate' = [wstate EXCEPT ![d] = "Streaming"]
  /\ UNCHANGED <<bound, sstate, batch, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

\* Acked: the source drops what the delivered cursor covers. The source keeps
\* the highest ack, so a late ack from a slower Device moves nothing back.
Acked(d) ==
  /\ ackPending[d] > 0
  /\ acked' = Max(acked, ackPending[d])
  /\ ackPending' = [ackPending EXCEPT ![d] = 0]
  /\ UNCHANGED <<bound, wstate, sstate, wcursor, batch, batchEnd, stored, log, head, gapThrough>>

\* StreamFailed: the read fails, perhaps because the signer locked again.
StreamFailed(d) ==
  /\ wstate[d] \in {"Streaming", "Delivering"}
  /\ wstate' = [wstate EXCEPT ![d] = "Backoff"]
  /\ batch' = [batch EXCEPT ![d] = NoBatch]
  /\ sstate' \in {[sstate EXCEPT ![d] = s] : s \in {sstate[d], "SignerLocked"}}
  /\ UNCHANGED <<bound, wcursor, batchEnd, ackPending, stored, log, head, gapThrough, acked>>

Next == \/ BindingChanged
        \/ Receive
        \/ Expire
        \/ \E d \in Devices :
             \/ Open(d) \/ SignerReady(d) \/ Stream(d) \/ BatchRead(d)
             \/ Delivered(d) \/ StaleCursor(d) \/ Resume(d) \/ Acked(d)
             \/ StreamFailed(d)
Spec == Init /\ [][Next]_vars

TypeOK ==
  /\ wstate \in [Devices -> {"WaitingBinding", "Opening", "Streaming", "Delivering", "Resuming", "Backoff"}]
  /\ sstate \in [Devices -> {"SignerLocked", "SignerMissing", "Reading", "Gap"}]
  /\ stored \in 0..MaxSeq /\ acked \in 0..MaxSeq

\* ArrivalBehindCursor: no arrival is stored without the cursor past it.
ArrivalBehindCursor == \A e \in Range(log) : e[2] <= stored

\* AckBehindCursor: the source never drops what the run has not stored.
AckBehindCursor == acked <= stored

\* ExactlyOnce: each event and each gap_through value arrives once.
ExactlyOnce == \A i, j \in DOMAIN log : i # j => log[i] # log[j]

\* NoSilentLoss: every seqno the cursor passed arrived or lies under a gap.
NoSilentLoss ==
  \A s \in 1..stored :
    \/ <<"ev", s>> \in Range(log)
    \/ \E g \in s..MaxSeq : <<"gap", g>> \in Range(log)
=============================================================================
