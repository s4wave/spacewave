---- MODULE CatchupBinding ----
EXTENDS Naturals
(* catchup.go syncReceive.appendPage and acceptResponse, called by
   syncExchange.receive after a pinned Head/HistoryRequest. Two entries and
   revisions represent contiguous history and a stale/misbound response.
   The page loop may retain a valid prefix before a later entry fails: such
   buffers stay untrusted. A malformed page ends the stream; a budget failure
   queues recovery control, which rejects subsequent snapshots. Further failed
   page attempts are collapsed into the same unpublishable state.
   Entry lengths one and two stand for serialized lengths with budget two.
   A failed append may exceed the byte budget in its discarded buffer.
   Digests, config validation, access checks and atomic host publication are
   observed primitive results, not cryptographic proofs. Superseded responses
   are ignored. Branch merging shares the same pre-import binding checks;
   RejectFailedPrefix represents both caller paths that fence snapshot import.
   Merge algebra, sender pagination and simultaneous exchanges are excluded. *)
CONSTANTS CheckBinding, RejectFailedPrefix
VARIABLES cursor, size, failed, published, correct, phase
vars == <<cursor, size, failed, published, correct, phase>>
Init == /\ cursor = 0 /\ size = 0 /\ failed = FALSE
        /\ published = FALSE /\ correct = TRUE /\ phase = "pages"
AppendEntry(previous, revision, bytes) ==
  /\ phase = "pages" /\ ~failed /\ cursor < 3
  /\ IF previous = cursor /\ revision = 1
     THEN /\ size' = size + bytes
          /\ IF size + bytes <= 2
             THEN /\ cursor' = cursor + 1 /\ failed' = FALSE
             ELSE /\ failed' = TRUE /\ UNCHANGED cursor
     ELSE /\ failed' = TRUE /\ UNCHANGED <<cursor, size>>
  /\ UNCHANGED <<published, correct, phase>>
AcceptResponse(revision, baseOK, digestOK, configOK, hostOK) ==
  /\ phase = "pages" /\ (RejectFailedPrefix => ~failed)
  /\ (CheckBinding => cursor = 2 /\ revision = 1 /\ baseOK /\ digestOK /\ configOK)
  /\ hostOK
  /\ published' = TRUE /\ phase' = "done"
  /\ correct' = (cursor = 2 /\ revision = 1 /\ baseOK /\ digestOK /\ configOK)
  /\ UNCHANGED <<cursor, size, failed>>
Abort == /\ phase = "pages" /\ phase' = "done"
         /\ UNCHANGED <<cursor, size, failed, published, correct>>
Next == (\E p \in 0..2, rev, bytes \in {1, 2}: AppendEntry(p, rev, bytes))
        \/ (\E rev \in {1, 2}, b, d, c, h \in BOOLEAN: AcceptResponse(rev, b, d, c, h))
        \/ Abort \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars
OnlyPinnedResponse == published => correct
UntrustedPrefixHidden == failed => ~published
BoundedBuffer == (~failed => size <= 2) /\ cursor <= size
====
